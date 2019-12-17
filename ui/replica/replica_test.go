package replicaUi

import (
	"net/url"
	"testing"

	"github.com/getlantern/testify/assert"
)

func TestParseEmptyUrl(t *testing.T) {
	u, err := url.Parse("")
	assert.NoError(t, err)
	assert.Equal(t, "", u.Path)
}
