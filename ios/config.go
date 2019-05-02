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
	"sync/atomic"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/fronted"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/config/generated"
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
	configFolderPath  string
	uc                common.UserConfig
	rt                http.RoundTripper
	hasFetchedProxies int64
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

	for _, masqueradeSet := range global.Client.MasqueradeSets {
		for _, masquerade := range masqueradeSet {
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
	err := yaml.Unmarshal(hardcodedProxies, updated)
	if err != nil {
		log.Error(err)
		return cfg, false
	} else {
		needsSaving := atomic.CompareAndSwapInt64(&cf.hasFetchedProxies, 0, 1)
		if needsSaving {
			cf.saveConfig(proxiesYaml, hardcodedProxies)
		}
		return updated, needsSaving
	}
	// didFetch, err := cf.updateFromWeb(proxiesYaml, etag, updated, "http://config.getiantem.org/proxies.yaml.gz")
	// if err != nil {
	// 	log.Error(err)
	// }
	// if didFetch {
	// 	cfg = updated
	// }
	// return cfg, didFetch
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
	common.AddCommonHeaders(cf.uc, req)

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

	log.Debugf("Updated %v from cloud", name)

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

var hardcodedProxies = []byte(`server-0:
  addr: 168.63.217.81:443
  authtoken: gTg60ZF0uDCMB00Z1JWBpt7SP9D6VxcsSbq9tRjI71d6fQUqdgyQg2WNJ3i2BWC5
  cert: '-----BEGIN CERTIFICATE-----

    MIIDYzCCAkugAwIBAgIJAMvUEkDs2cKSMA0GCSqGSIb3DQEBCwUAMFcxHzAdBgNV

    BAMMFkh1bWJsZXIgUHN5Y2hvYW5hbHlzaXMxEjAQBgNVBAcMCUZpZXJpbmVzczET

    MBEGA1UECAwKQ2FsaWZvcm5pYTELMAkGA1UEBhMCVVMwHhcNMTkwNDAxMDUyMjM0

    WhcNMjAwMzMxMDUyMjM0WjBXMR8wHQYDVQQDDBZIdW1ibGVyIFBzeWNob2FuYWx5

    c2lzMRIwEAYDVQQHDAlGaWVyaW5lc3MxEzARBgNVBAgMCkNhbGlmb3JuaWExCzAJ

    BgNVBAYTAlVTMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArvY/EBsL

    Ve1G0lhUDQH8VVZE++ZmbSWk3bGoXi59i66u9YczZqsN7Up2l+HWU1OfhqAyCK4p

    NGy6fYI92hXXaLCMa0d/H5/rg2mSCInl2IfPEwrdfAqoGQ+Sf7uD1cOi6yUWoTfq

    ZJ4rpcWtcZdoU8q2SSHOpTCYCBwxFFag5wsPrHDSQdoZRHgiFMMtCE4rYTV3Ojfx

    PLpnJLIN0mczfSzC1Q/3E3oWkfF8sk6vgB/sLZY9grsKs4k7RcvBaOh26Clf/bxd

    kqODk6+zhb/WvaDe0SCugG3OT/vAtZnZrBURfkPd2E0WUSNCh6oEKS+vF009pyVs

    B6epcabjPFAopwIDAQABozIwMDAdBgNVHQ4EFgQUlh8xsYK/XZW+joFxFam9Husu

    tawwDwYDVR0RBAgwBocEqD/ZUTANBgkqhkiG9w0BAQsFAAOCAQEAW6R5zuAKrKbt

    9CJQ4xlZUk7scAFcf1jLYoyCt0h4oGNvCwPEyRGysYyt1sYjYdcdtZkGufB6qXQ1

    fC3HJ4tfkMUYYagT4xglxGcjOIUW25gxPyocJf+RJOXgj0gyPJbJohSFD43l0rOg

    bQshFzXOnvOFKG2+qHZCT/niCUZBsgkEFnZftGzZA+TkbpIYth5+rGMFNO2BCd19

    r/M8LIN+YMXSwG6PIhZPvHo8cdOboA2/gqlmLF5YnVn96TPAWGHxe8pkbmiHPhEL

    SFxl8H47L4NK8EjM28fwYm0gNk7ClnevggOg7hJTJqjc22AMQZMLygI+QaXBAAb/

    jQeltkev4A==

    -----END CERTIFICATE-----

    '
  location:
    city: Hong Kong
    country: China
    countrycode: HK
    latitude: 22.28
    longitude: 114.15
  pipeline: true
  pluggabletransport: lampshade
  pluggabletransportsettings:
    maxpadding: '100'
  qos: 10
  trusted: true
  weight: 1000000
`)
