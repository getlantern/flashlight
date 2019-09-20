package chained

import (
	"net"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/go-ping"
)

func (p *proxy) Ping() {
	host, _, err := net.SplitHostPort(p.addr)
	if err != nil {
		log.Errorf("Unable to split address %v, not pinging")
		return
	}
	op := ops.Begin("icmp_ping").ChainedProxy(p.Name(), p.addr, p.protocol, p.network, p.multiplexed)
	defer op.End()
	log.Debugf("Pinging %v at %v", p.Name(), p.addr)
	stats, err := ping.Run(host, &ping.Opts{
		Count:       100,
		PayloadSize: 1200, // use a very large size similar to what we'd see in TCP packets
	})
	if err != nil {
		op.FailIf(log.Errorf("Error pinging %v: %v", host, err))
		return
	}
	op.SetMetricPercentile("ping_rtt_min", stats.RTTMin)
	op.SetMetricPercentile("ping_rtt_avg", stats.RTTAvg)
	op.SetMetricPercentile("ping_rtt_max", stats.RTTMax)
	op.SetMetricPercentile("ping_plr", stats.PLR)
	log.Debugf("Successfully pinged %v", p.Name())
}
