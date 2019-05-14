package ios

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testDeviceID1 = "test1"
	testDeviceID2 = "test2"
)

func TestConfigure(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "config_test")
	if !assert.NoError(t, err) {
		return
	}
	defer os.RemoveAll(tmpDir)
	ioutil.WriteFile(filepath.Join(tmpDir, "global.yaml"), []byte{}, 0644)
	ioutil.WriteFile(filepath.Join(tmpDir, "global.yaml.etag"), []byte{}, 0644)
	ioutil.WriteFile(filepath.Join(tmpDir, "proxies.yaml"), []byte{}, 0644)
	ioutil.WriteFile(filepath.Join(tmpDir, "proxies.yaml.etag"), []byte{}, 0644)
	ioutil.WriteFile(filepath.Join(tmpDir, "masquerade_cache"), []byte{}, 0644)
	ioutil.WriteFile(filepath.Join(tmpDir, "userconfig.yaml"), []byte{}, 0644)

	result1, err := Configure(tmpDir, testDeviceID1)
	if !assert.NoError(t, err) {
		return
	}

	assert.True(t, result1.VPNNeedsReconfiguring)
	assert.NotEmpty(t, result1.IPSToExcludeFromVPN)

	c := &configurer{configFolderPath: tmpDir}
	uc, err := c.readUserConfig()
	if assert.NoError(t, err) {
		assert.Equal(t, testDeviceID1, uc.GetDeviceID())
	}

	result2, err := Configure(tmpDir, testDeviceID2)
	if !assert.NoError(t, err) {
		return
	}
	assert.False(t, result2.VPNNeedsReconfiguring)
	ips1 := strings.Split(result1.IPSToExcludeFromVPN, ",")
	ips2 := strings.Split(result2.IPSToExcludeFromVPN, ",")
	sort.Strings(ips1)
	sort.Strings(ips2)
	assert.Equal(t, ips1, ips2)

	uc, err = c.readUserConfig()
	if assert.NoError(t, err) {
		assert.Equal(t, testDeviceID2, uc.GetDeviceID())
	}
}
