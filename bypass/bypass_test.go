package bypass

import (
	"testing"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/v7/common"

	"github.com/stretchr/testify/assert"
)

func TestHTTPRequest(t *testing.T) {
	p := &proxy{
		ProxyConfig: &config.ProxyConfig{
			Addr: "http://cookies.com",
		},
	}
	uc := &common.NullUserConfig{}

	_, err := p.newRequest(uc)
	assert.NoError(t, err)
}
