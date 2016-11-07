package borda

import (
	"net"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/measured"

	"github.com/getlantern/flashlight/chained"
)

func newTrafficReporter(bc *borda.Client, submitInterval time.Duration, enabled func() bool) (*trafficReporter, proxyWrapper) {
	submitter := bc.ReducingSubmitter("client_results", 1000, func(existingValues map[string]float64, newValues map[string]float64) {
		for key, value := range newValues {
			existingValues[key] += value
		}
	})
	collectInterval := submitInterval / 10
	r := &trafficReporter{enabled, measured.New(100), submitter, collectInterval}
	r.m.Start(submitInterval, r)
	return r, r.WrapProxy
}

type trafficReporter struct {
	enabled         func() bool
	m               *measured.Measured
	submitter       borda.Submitter
	collectInterval time.Duration
}

func (r trafficReporter) ReportTraffic(tt map[string]*measured.TrafficTracker) error {
	if !r.enabled() {
		return nil
	}
	for _, ti := range tt {
		err := r.submitter(map[string]float64{
			"client_bytes_recv": float64(ti.TotalIn),
			"client_bytes_sent": float64(ti.TotalOut),
		},
			map[string]interface{}{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r trafficReporter) Stop() {
	r.m.Stop()
}

func (r trafficReporter) WrapProxy(p chained.Proxy) chained.Proxy {
	mp := measuredProxy{p, nil}
	mp.dialFn = r.m.Dialer(func(net, addr string) (net.Conn, error) {
		return mp.Proxy.DialServer()
	}, r.collectInterval)
	return mp
}

type measuredProxy struct {
	chained.Proxy
	dialFn measured.DialFunc
}

func (p measuredProxy) DialServer() (net.Conn, error) {
	return p.dialFn("placeholder", "placeholder")
}
