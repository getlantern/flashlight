package bypass

import (
	"testing"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"

	"github.com/stretchr/testify/assert"
)

func TestHTTPRequest(t *testing.T) {
	p := &proxy{
		ChainedServerInfo: &chained.ChainedServerInfo{
			Addr: "http://cookies.com",
		},
	}
	uc := &common.NullUserConfig{}

	r, err := p.newRequest(uc, "https://bypass.iantem.io/v1/")
	assert.NoError(t, err)

	assert.Equal(t, "https://bypass.iantem.io/v1/", r.URL.String())

}
