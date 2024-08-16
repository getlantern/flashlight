package proxyconfig

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/getlantern/rot13"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/getlantern/flashlight/v7/apipb"
)

func TestInitWithSavedConfig(t *testing.T) {
	conf := newTestConfig()
	defer resetConfig()

	withTempConfigFile(t, conf, false, func(tmpfile *os.File) {
		Init("", false)
		existing, _ := GetConfig(context.Background())

		want := fmt.Sprintf("%+v", conf)
		got := fmt.Sprintf("%+v", existing)
		assert.Equal(t, want, got, "failed to read existing config file")
	})
}

func TestNotifyOnConfig(t *testing.T) {
	conf := newTestConfig()
	defer resetConfig()

	withTempConfigFile(t, conf, false, func(tmpfile *os.File) {
		called := make(chan struct{}, 1)
		OnConfigChange(func(old, new *ProxyConfig) {
			called <- struct{}{}
		})

		Init("", false)
		select {
		case <-called:
			t.Log("received existing config notification")
		case <-time.After(time.Second):
			assert.Fail(t, "timeout waiting for existing config notification")
		}

		_config.SetConfig(newTestConfig())

		select {
		case <-called:
			t.Log("recieved config change notification")
		case <-time.After(time.Second):
			assert.Fail(t, "timeout waiting for config change notification")
		}
	})
}

func TestInvalidFile(t *testing.T) {
	withTempConfigFile(t, nil, false, func(tmpfile *os.File) {
		tmpfile.WriteString("real-list-of-lantern-ips: https://youtu.be/dQw4w9WgXcQ?t=85")
		tmpfile.Sync()

		_, err := readExistingConfig(tmpfile.Name(), false)
		assert.Error(t, err, "should get error if config file is invalid")
	})
}

func TestReadObfuscatedConfig(t *testing.T) {
	conf := newTestConfig()
	withTempConfigFile(t, conf, true, func(tmpfile *os.File) {
		fileConf, err := readExistingConfig(tmpfile.Name(), true)
		assert.NoError(t, err, "unable to read obfuscated config file")

		want := fmt.Sprintf("%+v", conf)
		got := fmt.Sprintf("%+v", fileConf)
		assert.Equal(t, want, got, "obfuscated config file doesn't match")
	})
}

func TestSaveObfuscatedConfig(t *testing.T) {
	withTempConfigFile(t, nil, false, func(tmpfile *os.File) {
		tmpfile.Close()

		conf := newTestConfig()
		err := saveConfig(tmpfile.Name(), conf, true)
		require.NoError(t, err, "unable to save obfuscated config file")

		file, err := os.Open(tmpfile.Name())
		require.NoError(t, err, "unable to open obfuscated config file")
		defer file.Close()

		reader := rot13.NewReader(file)
		buf, err := io.ReadAll(reader)
		require.NoError(t, err, "unable to read obfuscated config file")

		fileConf := &ProxyConfig{}
		assert.NoError(t, proto.Unmarshal(buf, fileConf), "unable to unmarshal obfuscated config file")

		want := fmt.Sprintf("%+v", conf)
		got := fmt.Sprintf("%+v", fileConf)
		assert.Equal(t, want, got, "obfuscated config file doesn't match")
	})
}

func newTestConfig() *ProxyConfig {
	return &ProxyConfig{
		Country: "Mars",
		Ip:      "109.117.115.107",
		Proxy:   &apipb.ConfigResponse_Proxy{},
	}
}

func withTempConfigFile(t *testing.T, conf *ProxyConfig, obfuscate bool, f func(*os.File)) {
	tmpfile, err := os.OpenFile(defaultConfigFilename, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	require.NoError(t, err, "couldn't create temp file")
	defer func() { // clean up
		tmpfile.Close()
		os.Remove(tmpfile.Name())
	}()

	if conf != nil {
		buf, _ := proto.Marshal(conf)

		var writer io.Writer = tmpfile
		if obfuscate {
			writer = rot13.NewWriter(tmpfile)
		}

		_, err := writer.Write(buf)
		require.NoError(t, err, "unable to write to test config file")

		tmpfile.Sync()
	}

	f(tmpfile)
}

func resetConfig() {
	_config.mu.Lock()
	_config.config.Reset()
	_config.filePath = ""
	_config.obfuscate = false
	_config.listeners = nil
	_config.mu.Unlock()
}
