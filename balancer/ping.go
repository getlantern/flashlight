package balancer

import (
	"math/rand"

	"github.com/getlantern/flashlight/common"
)

// PingProxies maybe pings the client's proxies depending on the
// specified sample percentage.
func (bal *Balancer) PingProxies(pingSamplePercentage float64) {
	dialers := bal.copyOfDialers()
	for _, dialer := range dialers {
		if rand.Float64() < pingSamplePercentage/100 || common.InDevelopment() {
			go dialer.Ping()
		}
	}
}
