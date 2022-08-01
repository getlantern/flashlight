package bypass

import (
	"testing"

	"github.com/getlantern/common/apipb"
	"github.com/getlantern/flashlight/common"

	"github.com/stretchr/testify/assert"
)

func TestHTTPRequest(t *testing.T) {
	p := &proxy{
		ProxyConfig: &apipb.ProxyConfig{
			Addr: "http://cookies.com",
		},
	}
	uc := &common.NullUserConfig{}

	r, err := p.newRequest(uc, "https://bypass.iantem.io/v1/")
	assert.NoError(t, err)

	assert.Equal(t, "https://bypass.iantem.io/v1/", r.URL.String())

}
