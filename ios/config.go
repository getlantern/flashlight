package ios

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/fronted"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/config/generated"
	"github.com/getlantern/flashlight/email"
)

const (
	userConfigYaml = "userconfig.yaml"
	globalYaml     = "global.yaml"
	proxiesYaml    = "proxies.yaml"
)

// ConfigResult captures the result of calling Configure()
type ConfigResult struct {
	// VPNNeedsReconfiguring indicates that something in the config changed and
	// that the VPN needs to be reconfigured.
	VPNNeedsReconfiguring bool

	// IPSToExcludeFromVPN lists all IPS that should be excluded from the VPNS's
	// routes in a comma-delimited string
	IPSToExcludeFromVPN string
}

// Configure fetches updated configuration from the cloud and stores it in
// configFolderPath. There are 5 files that must be initialized in
// configFolderPath - global.yaml, global.yaml.etag, proxies.yaml,
// proxies.yaml.etag and masquerade_cache. deviceID should be a string that
// uniquely identifies the current device.
func Configure(configFolderPath string, deviceID string) (*ConfigResult, error) {
	log.Debugf("Configuring client for device '%v' at config path '%v'", deviceID, configFolderPath)
	cf := &configurer{
		configFolderPath: configFolderPath,
		uc:               userConfigFor(deviceID),
	}
	return cf.configure()
}

type configurer struct {
	configFolderPath string
	uc               common.UserConfig
	rt               http.RoundTripper
}

func (cf *configurer) configure() (*ConfigResult, error) {
	result := &ConfigResult{}

	if err := cf.writeUserConfig(); err != nil {
		return nil, err
	}

	global, globalEtag, globalInitialized, err := cf.openGlobal()
	if err != nil {
		return nil, err
	}

	email.SetDefaultRecipient(global.ReportIssueEmail)

	proxies, proxiesEtag, proxiesInitialized, err := cf.openProxies()
	if err != nil {
		return nil, err
	}

	result.VPNNeedsReconfiguring = globalInitialized || proxiesInitialized

	if frontingErr := cf.configureFronting(global); frontingErr != nil {
		log.Errorf("Unable to configure fronting, sticking with embedded configuration: %v", err)
	} else {
		var globalUpdated, proxiesUpdated bool
		global, globalUpdated = cf.updateGlobal(global, globalEtag)
		proxies, proxiesUpdated = cf.updateProxies(proxies, proxiesEtag)

		result.VPNNeedsReconfiguring = result.VPNNeedsReconfiguring || globalUpdated || proxiesUpdated
	}

	for _, provider := range global.Client.Fronted.Providers {
		for _, masquerade := range provider.Masquerades {
			if len(result.IPSToExcludeFromVPN) == 0 {
				result.IPSToExcludeFromVPN = masquerade.IpAddress
			} else {
				result.IPSToExcludeFromVPN = fmt.Sprintf("%v,%v", result.IPSToExcludeFromVPN, masquerade.IpAddress)
			}
		}
	}

	for _, proxy := range proxies {
		if proxy.Addr != "" {
			host, _, _ := net.SplitHostPort(proxy.Addr)
			result.IPSToExcludeFromVPN = fmt.Sprintf("%v,%v", host, result.IPSToExcludeFromVPN)
			log.Debugf("Added %v", host)
		}
		if proxy.MultiplexedAddr != "" {
			host, _, _ := net.SplitHostPort(proxy.MultiplexedAddr)
			result.IPSToExcludeFromVPN = fmt.Sprintf("%v,%v", host, result.IPSToExcludeFromVPN)
			log.Debugf("Added %v", host)
		}
	}

	return result, nil
}

func (cf *configurer) writeUserConfig() error {
	bytes, err := yaml.Marshal(cf.uc)
	if err != nil {
		return errors.New("Unable to marshal user config: %v", err)
	}
	if writeErr := ioutil.WriteFile(cf.fullPathTo(userConfigYaml), bytes, 0644); writeErr != nil {
		return errors.New("Unable to save userconfig.yaml: %v", err)
	}
	return nil
}

func (cf *configurer) readUserConfig() (*common.UserConfigData, error) {
	bytes, err := ioutil.ReadFile(cf.fullPathTo(userConfigYaml))
	if err != nil {
		return nil, errors.New("Unable to read userconfig.yaml: %v", err)
	}
	if len(bytes) == 0 {
		return nil, errors.New("Empty userconfig.yaml")
	}
	uc := &common.UserConfigData{}
	if parseErr := yaml.Unmarshal(bytes, uc); parseErr != nil {
		return nil, errors.New("Unable to parse userconfig.yaml: %v", err)
	}
	return uc, nil
}

func (cf *configurer) openGlobal() (*config.Global, string, bool, error) {
	cfg := &config.Global{}
	etag, updated, err := cf.openConfig(globalYaml, cfg, generated.GlobalConfig)
	return cfg, etag, updated, err
}

func (cf *configurer) openProxies() (map[string]*chained.ChainedServerInfo, string, bool, error) {
	cfg := make(map[string]*chained.ChainedServerInfo)
	etag, updated, err := cf.openConfig(proxiesYaml, cfg, generated.EmbeddedProxies)
	return cfg, etag, updated, err
}

func (cf *configurer) openConfig(name string, cfg interface{}, embedded []byte) (string, bool, error) {
	var initialized bool
	bytes, err := ioutil.ReadFile(cf.fullPathTo(name))
	if err == nil && len(bytes) > 0 {
		log.Debugf("Loaded %v from file", name)
	} else {
		log.Debugf("Initializing %v from embedded", name)
		bytes = embedded
		initialized = true
		if writeErr := ioutil.WriteFile(cf.fullPathTo(name), bytes, 0644); writeErr != nil {
			return "", false, errors.New("Unable to write embedded %v to disk: %v", name, writeErr)
		}
	}
	if parseErr := yaml.Unmarshal(bytes, cfg); parseErr != nil {
		return "", false, errors.New("Unable to parse %v: %v", name, parseErr)
	}
	etagBytes, err := ioutil.ReadFile(cf.fullPathTo(name + ".etag"))
	if err != nil {
		log.Debugf("No known etag for %v", name)
		etagBytes = []byte{}
	}
	return string(etagBytes), initialized, nil
}

func (cf *configurer) configureFronting(global *config.Global) error {
	certs, err := global.TrustedCACerts()
	if err != nil {
		return errors.New("Unable to read trusted CAs from global config, can't configure domain fronting: %v", err)
	}

	fronted.Configure(certs, global.Client.FrontedProviders(), "cloudfront", cf.fullPathTo("masquerade_cache"))
	chained.ConfigureFronting(certs, global.Client.FrontedProviders(), cf.configFolderPath)
	rt, ok := fronted.NewDirect(1 * time.Minute)
	if !ok {
		return errors.New("Timed out waiting for fronting to finish configuring")
	}

	cf.rt = rt
	return nil
}

func (cf *configurer) updateGlobal(cfg *config.Global, etag string) (*config.Global, bool) {
	updated := &config.Global{}
	didFetch, err := cf.updateFromWeb(globalYaml, etag, updated, "https://globalconfig.flashlightproxy.com/global.yaml.gz")
	if err != nil {
		log.Error(err)
	}
	if didFetch {
		cfg = updated
	}
	return cfg, didFetch
}

func (cf *configurer) updateProxies(cfg map[string]*chained.ChainedServerInfo, etag string) (map[string]*chained.ChainedServerInfo, bool) {
	updated := make(map[string]*chained.ChainedServerInfo)
	didFetch, err := cf.updateFromWeb(proxiesYaml, etag, updated, "http://config.getiantem.org/proxies.yaml.gz")
	if err != nil {
		log.Error(err)
	}
	if didFetch {
		cfg = updated
	}
	return cfg, didFetch
}

// TODO: DRY violation with ../config/fetcher.go
func (cf *configurer) updateFromWeb(name string, etag string, cfg interface{}, url string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, errors.New("Unable to construct request to fetch %v from %v: %v", name, url, err)
	}

	if etag != "" {
		req.Header.Set(common.IfNoneMatchHeader, etag)
	}
	req.Header.Set("Accept", "application/x-gzip")
	// Prevents intermediate nodes (domain-fronters) from caching the content
	req.Header.Set("Cache-Control", "no-cache")
	common.AddHeadersForInternalServices(req, cf.uc, true)

	// make sure to close the connection after reading the Body
	// this prevents the occasional EOFs errors we're seeing with
	// successive requests
	req.Close = true

	resp, err := cf.rt.RoundTrip(req)
	if err != nil {
		return false, errors.New("Unable to fetch cloud config at %s: %s", url, err)
	}
	dump, dumperr := httputil.DumpResponse(resp, false)
	if dumperr != nil {
		log.Errorf("Could not dump response: %v", dumperr)
	} else {
		log.Debugf("Response headers from %v:\n%v", url, string(dump))
	}
	defer func() {
		if closeerr := resp.Body.Close(); closeerr != nil {
			log.Errorf("Error closing response body: %v", closeerr)
		}
	}()

	if resp.StatusCode == 304 {
		log.Debugf("%v unchanged in cloud", name)
		return false, nil
	} else if resp.StatusCode != 200 {
		if dumperr != nil {
			return false, errors.New("Bad config response code for %v: %v", name, resp.StatusCode)
		}
		return false, errors.New("Bad config resp for %v:\n%v", name, string(dump))
	}

	newEtag := resp.Header.Get(common.EtagHeader)
	buf := &bytes.Buffer{}
	body := io.TeeReader(resp.Body, buf)
	gzReader, err := gzip.NewReader(body)
	if err != nil {
		return false, errors.New("Unable to open gzip reader: %s", err)
	}

	defer func() {
		if err := gzReader.Close(); err != nil {
			log.Errorf("Unable to close gzip reader: %v", err)
		}
	}()

	bytes, err := ioutil.ReadAll(gzReader)
	if err != nil {
		return false, errors.New("Unable to read response for %v: %v", name, err)
	}

	if parseErr := yaml.Unmarshal(bytes, cfg); parseErr != nil {
		return false, errors.New("Unable to parse update for %v: %v", name, parseErr)
	}

	if newEtag == "" {
		sum := md5.Sum(buf.Bytes())
		newEtag = hex.EncodeToString(sum[:])
	}
	cf.saveConfig(name, bytes)
	cf.saveEtag(name, newEtag)

	if name == "proxies.yaml" {
		log.Debugf("Updated proxies.yaml from cloud:\n%v", string(bytes))
	} else {
		log.Debugf("Updated %v from cloud", name)
	}

	return newEtag != etag, nil
}

func (cf *configurer) openFile(filename string) (*os.File, error) {
	file, err := os.Open(cf.fullPathTo(filename))
	if err != nil {
		err = errors.New("Unable to open %v: %v", filename, err)
	}
	return file, err
}

func (cf *configurer) saveConfig(name string, bytes []byte) {
	err := ioutil.WriteFile(cf.fullPathTo(name), bytes, 0644)
	if err != nil {
		log.Errorf("Unable to save config for %v: %v", name, err)
	}
}

func (cf *configurer) saveEtag(name string, etag string) {
	err := ioutil.WriteFile(cf.fullPathTo(name+".etag"), []byte(etag), 0644)
	if err != nil {
		log.Errorf("Unable to save etag for %v: %v", name, err)
	}
}

func (cf *configurer) fullPathTo(filename string) string {
	return filepath.Join(cf.configFolderPath, filename)
}
