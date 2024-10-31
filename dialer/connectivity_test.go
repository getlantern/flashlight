// Description: Tests for the connectivity check dialer.
package dialer

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConnectTimeProxyDialer(t *testing.T) {
	//dialer := newMockProxyDialer("dialer1", false)
	dialer := newTcpConnDialer()
	ctd1 := connectTimeProxyDialer{
		ProxyDialer: dialer, connectTime: 1 * time.Second,
	}
	ctd2 := connectTimeProxyDialer{
		ProxyDialer: dialer, connectTime: 100 * time.Second,
	}
	ctd3 := connectTimeProxyDialer{
		ProxyDialer: dialer, connectTime: 10 * time.Second,
	}

	dialers := dialersByConnectTime{ctd1, ctd2, ctd3}
	sort.Sort(dialers)

	// Make sure the lowest connect time is first
	require.True(t, dialers[0].connectTime < dialers[1].connectTime, "Expected dialer1 to have the lowest connect time")
	require.True(t, dialers[1].connectTime < dialers[2].connectTime, "Expected dialer1 to have the lowest connect time")
}
