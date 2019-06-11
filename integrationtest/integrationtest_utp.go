// +build android, linux

package integrationtest

import (
	"io/ioutil"
	"testing"

	"github.com/getlantern/flashlight/client"
)

// newHelper prepares a new integration test helper including a web server for
// content, a proxy server and a config server that ties it all together. It
// also enables ForceProxying on the client package to make sure even localhost
// origins are served through the proxy. Make sure to close the Helper with
// Close() when finished with the test.
func NewHelper(t *testing.T, httpsAddr string, obfs4Addr string, lampshadeAddr string, quicAddr string, wssAddr string, httpsUTPAddr string, obfs4UTPAddr string, lampshadeUTPAddr string) (*Helper, error) {
	ConfigDir, err := ioutil.TempDir("", "integrationtest_helper")
	log.Debugf("ConfigDir is %v", ConfigDir)
	if err != nil {
		return nil, err
	}

	helper := &Helper{
		t:                           t,
		ConfigDir:                   ConfigDir,
		HTTPSProxyServerAddr:        httpsAddr,
		HTTPSUTPAddr:                httpsUTPAddr,
		OBFS4ProxyServerAddr:        obfs4Addr,
		OBFS4UTPProxyServerAddr:     obfs4UTPAddr,
		LampshadeProxyServerAddr:    lampshadeAddr,
		LampshadeUTPProxyServerAddr: lampshadeUTPAddr,
		QUICProxyServerAddr:         quicAddr,
		WSSProxyServerAddr:          wssAddr,
	}
	helper.SetProtocol("https")
	client.ForceProxying()

	// Web server serves known content for testing
	err = helper.startWebServer()
	if err != nil {
		helper.Close()
		return nil, err
	}

	// This is the remote proxy server
	err = helper.startProxyServer()
	if err != nil {
		helper.Close()
		return nil, err
	}

	// This is a fake config server that serves up a config that points at our
	// testing proxy server.
	err = helper.startConfigServer()
	if err != nil {
		helper.Close()
		return nil, err
	}

	// We have to write out a config file so that Lantern doesn't try to use the
	// default config, which would go to some remote proxies that can't talk to
	// our fake config server.
	err = helper.writeConfig()
	if err != nil {
		helper.Close()
		return nil, err
	}

	return helper, nil
}

