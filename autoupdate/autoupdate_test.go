package autoupdate

import (
	"strings"
	"testing"

	"github.com/getlantern/flashlight/common"
	"github.com/stretchr/testify/assert"
)

func TestSetUpdateURL(t *testing.T) {
	expected := "https://update.abc.com/update/" + strings.ToLower(common.AppName)
	setUpdateURL("https://update.abc.com")
	assert.Equal(t, expected, getUpdateURL(), "should append correct path to the url")
	setUpdateURL("https://update.abc.com/")
	assert.Equal(t, expected, getUpdateURL(), "should handle trailing slash")
	setUpdateURL("")
	assert.Equal(t, expected, getUpdateURL(), "should ignore empty URL")
}
