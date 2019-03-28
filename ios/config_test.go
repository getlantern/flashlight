package ios

import (
	"io/ioutil"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigure(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "config_test")
	if !assert.NoError(t, err) {
		return
	}
	defer os.RemoveAll(tmpDir)

	result1, err := Configure(tmpDir)
	if !assert.NoError(t, err) {
		return
	}

	assert.True(t, result1.VPNNeedsReconfiguring)
	assert.NotEmpty(t, result1.IPSToExcludeFromVPN)

	result2, err := Configure(tmpDir)
	if !assert.NoError(t, err) {
		return
	}
	assert.False(t, result2.VPNNeedsReconfiguring)
	sort.Strings(result1.IPSToExcludeFromVPN)
	sort.Strings(result2.IPSToExcludeFromVPN)
	assert.Equal(t, result1.IPSToExcludeFromVPN, result2.IPSToExcludeFromVPN)
}
