package bypass

import (
	"testing"

	"github.com/getlantern/flashlight/v7/common"

	"github.com/getlantern/common/config"

	"github.com/stretchr/testify/assert"
)

func TestHTTPRequest(t *testing.T) {
	p := &proxy{
		ProxyConfig: &config.ProxyConfig{
			Addr: "http://cookies.com",
		},
	}
	uc := &common.NullUserConfig{}

	r, err := p.newRequest(uc, "https://iantem.io/v1/bypass")
	assert.NoError(t, err)

	assert.Equal(t, "https://iantem.io/v1/bypass", r.URL.String())

}
