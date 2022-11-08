// +build linux

package bbr

import (
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/getlantern/bbrconn"
	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/http-proxy-lantern/v2/common"
	"github.com/getlantern/netx"
	"github.com/getlantern/ops"
	"github.com/getlantern/proxy/v2/filters"
)

type middleware struct {
	statsByClient map[string]*stats
	upstreamABE   uint64

	mx sync.Mutex
}

func New() Middleware {
	log.Debug("Tracking bbr metrics on Linux")
	return &middleware{
		statsByClient: make(map[string]*stats),
	}
}

// Apply implements the interface filters.Filter.
func (bm *middleware) Apply(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	resp, nextCS, err := next(cs, req)
	if resp != nil {
		bm.AddMetrics(nextCS, req, resp)
	}
	return resp, nextCS, err
}

func (bm *middleware) AddMetrics(cs *filters.ConnectionState, req *http.Request, resp *http.Response) {
	conn := cs.Downstream()
	s := bm.statsFor(conn)

	bbrRequested := req.Header.Get(common.BBRRequested)
	clear := bbrRequested == "clear"
	if clear {
		log.Tracef("Clearing stats for %v", conn.RemoteAddr())
		s.clear()
	}

	netx.WalkWrapped(conn, func(conn net.Conn) bool {
		switch t := conn.(type) {
		case bbrconn.Conn:
			// Found bbr conn, get info
			bytesSent := t.BytesWritten()
			bbrInfo, infoErr := t.BBRInfo()
			bm.track(false, s, conn.RemoteAddr(), bytesSent, nil, bbrInfo, infoErr)
			return false
		}

		// Keep looking
		return true
	})

	if bbrRequested == "" {
		// BBR info not requested, ignore
		return
	}
	if resp.Header == nil {
		resp.Header = make(http.Header, 1)
	}
	resp.Header.Set(common.BBRAvailableBandwidthEstimateHeader, fmt.Sprint(s.estABE(bm.getUpstreamABE())))
}

func (bm *middleware) statsFor(conn net.Conn) *stats {
	addr := conn.RemoteAddr().String()
	host, _, _ := net.SplitHostPort(addr)
	bm.mx.Lock()
	s := bm.statsByClient[host]
	if s == nil {
		s = newStats()
		bm.statsByClient[host] = s
	}
	bm.mx.Unlock()
	return s
}

func (bm *middleware) track(reportToBorda bool, s *stats, remoteAddr net.Addr, bytesSent int, info *bbrconn.TCPInfo, bbrInfo *bbrconn.BBRInfo, err error) {
	if err != nil {
		log.Tracef("Unable to get BBR info (this happens when connections are closed unexpectedly): %v", err)
		return
	}
	s.update(float64(bytesSent), float64(bbrInfo.MaxBW)*8/1000/1000)
	if reportToBorda {
		go func() {
			// We do this inside a goroutine because we explicitly don't want to inherit
			// the existing context (to reduce data volumes to borda)
			op := ops.Begin("tcpinfo")
			clientIP, _, err := net.SplitHostPort(remoteAddr.String())
			if err == nil {
				op.Set("client_ip", clientIP)
			}
			op.Set("tcp_bytes_sent", borda.Sum(bytesSent))
			op.Set("tcp_sender_mss", borda.Avg(float64(info.SenderMSS)))
			op.Set("tcp_rtt", borda.Avg(float64(info.RTT/nanosPerMilli)))
			op.Set("tcp_segments_sent", borda.Sum(float64(info.SysSegsOut)))
			op.Set("tcp_segments_sent_retransmitted", borda.Sum(float64(info.SysTotalRetransSegs)))
			estMbps := s.estABE(bm.getUpstreamABE())
			if estMbps > 0 {
				// Report ABE if available
				op.Set("est_mbps", borda.Avg(estMbps))
				op.Set("est_mbps_min", borda.Min(estMbps))
				op.Set("est_mbps_max", borda.Max(estMbps))
			}
			log.Tracef("Reporting tcp info for %v", remoteAddr)
			op.End()
		}()
	}
}

func (bm *middleware) Wrap(l net.Listener) net.Listener {
	log.Debugf("Enabling bbr metrics on %v", l.Addr())
	return &bbrlistener{l, bm}
}

func (bm *middleware) ABE(cs *filters.ConnectionState) float64 {
	conn := cs.Downstream()
	if conn == nil {
		return 0
	}
	return bm.statsFor(conn).estABE(bm.getUpstreamABE())
}

type bbrlistener struct {
	net.Listener
	bm *middleware
}

func (l *bbrlistener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return bbrconn.Wrap(conn, func(bytesSent int, info *bbrconn.TCPInfo, bbrInfo *bbrconn.BBRInfo, err error) {
		l.bm.track(true, l.bm.statsFor(conn), conn.RemoteAddr(), bytesSent, info, bbrInfo, err)
	})
}
