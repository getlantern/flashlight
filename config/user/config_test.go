package userconfig

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getlantern/eventual/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/getlantern/flashlight/v7/apipb"
)

func TestInitWithSavedConfig(t *testing.T) {
	conf := newTestConfig()
	defer resetConfig()

	filename, err := newTestConfigFile(conf)
	require.NoError(t, err, "unable to create test config file")
	defer os.Remove(filename)

	initialize(DefaultConfigSaveDir, filename, true)
	existing, _ := GetConfig(eventual.DontWait)

	want := fmt.Sprintf("%+v", conf)
	got := fmt.Sprintf("%+v", existing)
	assert.Equal(t, want, got, "failed to read existing config file")
}

func TestNotifyOnConfig(t *testing.T) {
	conf := newTestConfig()
	defer resetConfig()

	filename, err := newTestConfigFile(conf)
	require.NoError(t, err, "unable to create test config file")
	defer os.Remove(filename)

	called := make(chan struct{}, 1)
	OnConfigChange(func(old, new *UserConfig) {
		called <- struct{}{}
	})

	initialize(DefaultConfigSaveDir, filename, true)
	_config.SetConfig(newTestConfig())

	select {
	case <-called:
		t.Log("recieved config change notification")
	case <-time.After(time.Second):
		assert.Fail(t, "timeout waiting for config change notification")
	}
}

func TestInvalidFile(t *testing.T) {
	tmpfile, err := os.CreateTemp("", DefaultConfigFilename)
	require.NoError(t, err, "couldn't create temp file")
	tmpfile.WriteString("real-list-of-lantern-ips: https://youtu.be/dQw4w9WgXcQ?t=85")
	tmpfile.Close()

	_, err = readExistingConfig(tmpfile.Name(), true)
	assert.Error(t, err, "should get error if config file is invalid")

	os.Remove(tmpfile.Name())
}

const (
	token = "AF325DF3432FDS"

	shadowsocksSecret   = "foobarbaz"
	shadowsocksUpstream = "local"
	shadowsocksCipher   = "AEAD_CHACHA20_POLY1305"

	tlsmasqOriginAddr   = "orgin.com"
	tlsmasqSNI          = "test.com"
	tlsmasqSuites       = "0xcca9,0x1301" // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_AES_128_GCM_SHA256
	tlsmasqMinVersion   = "0x0303"        // TLS 1.2
	tlsmasqServerSecret = "d0cd0e2e50eb2ac7cb1dc2c94d1bc8871e48369970052ff866d1e7e876e77a13246980057f70d64a2bdffb545330279f69bce5fd"
)

func newTestConfig() *UserConfig {
	p0 := buildProxy("shadowsocks")
	p1 := buildProxy("tlsmasq")
	pCfg := []*apipb.ProxyConnectConfig{p0, p1}
	return &UserConfig{
		ProToken: token,
		Country:  "Mars",
		Ip:       "109.117.115.107",
		Proxy:    &apipb.ConfigResponse_Proxy{Proxies: pCfg},
	}
}

func newTestConfigFile(conf *UserConfig) (string, error) {
	tmpfile, err := os.CreateTemp("", DefaultConfigFilename)
	if err != nil {
		return "", nil
	}
	buf, err := protojson.Marshal(conf)
	if err != nil {
		return "", err
	}
	_, err = tmpfile.Write(buf)
	if err != nil {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
		return "", err
	}
	tmpfile.Sync()
	tmpfile.Close()
	return tmpfile.Name(), nil
}

func buildProxy(proto string) *apipb.ProxyConnectConfig {
	conf := &apipb.ProxyConnectConfig{
		Name:      "AshKetchumAll",
		AuthToken: token,
		CertPem:   []byte("cert"),
		Addr:      "localhost",
		Port:      8080,
	}

	switch proto {
	case "tlsmasq":
		conf.ProtocolConfig = &apipb.ProxyConnectConfig_ConnectCfgTlsmasq{
			ConnectCfgTlsmasq: &apipb.ProxyConnectConfig_TLSMasqConfig{
				OriginAddr:               tlsmasqOriginAddr,
				Secret:                   []byte(tlsmasqServerSecret),
				TlsMinVersion:            tlsmasqMinVersion,
				TlsSupportedCipherSuites: strings.Split(tlsmasqSuites, ","),
			},
		}
	case "shadowsocks":
		conf.ProtocolConfig = &apipb.ProxyConnectConfig_ConnectCfgShadowsocks{
			ConnectCfgShadowsocks: &apipb.ProxyConnectConfig_ShadowsocksConfig{
				Secret: shadowsocksSecret,
				Cipher: shadowsocksCipher,
			},
		}
	default:
	}

	return conf
}

func resetConfig() {
	_config.mu.Lock()
	_config.config.Reset()
	_config.filePath = ""
	_config.readable = true
	_config.listeners = nil
	_config.mu.Unlock()
}
