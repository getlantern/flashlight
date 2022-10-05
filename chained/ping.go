package chained

import (
	"net"

	"github.com/getlantern/flashlight/ops"
	"github.com/go-ping/ping"
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

	pinger, err := ping.NewPinger(host)
	if err != nil {
		op.FailIf(log.Errorf("while creating pinger %v: %v", host, err))
		return
	}
	pinger.Count = 100
	pinger.Size = 1200
	if err := pinger.Run(); err != nil {
		op.FailIf(log.Errorf("while pinging %v: %v", host, err))
		return
	}
	stats := pinger.Statistics()
	op.SetMetricPercentile("ping_rtt_min", stats.MinRtt.Seconds())
	op.SetMetricPercentile("ping_rtt_avg", stats.AvgRtt.Seconds())
	op.SetMetricPercentile("ping_rtt_max", stats.MaxRtt.Seconds())
	op.SetMetricPercentile("ping_plr", stats.PacketLoss)
	log.Debugf("Packet Loss Rate: %v", stats.PacketLoss)
	log.Debugf("Successfully pinged %v", p.Name())
}
