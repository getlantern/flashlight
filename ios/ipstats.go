package ios

import (
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"time"

	humanize "github.com/dustin/go-humanize"
)

var (
	networkByteOrder = binary.BigEndian
)

type ipstat [2]uint64

func (s *ipstat) incrSent(n int) *ipstat {
	if s == nil {
		s = &ipstat{0, 0}
	}
	atomic.AddUint64(&s[0], uint64(n))
	return s
}

func (s *ipstat) sent() uint64 {
	return atomic.LoadUint64(&s[0])
}

func (s *ipstat) incrRecv(n int) *ipstat {
	if s == nil {
		s = &ipstat{0, 0}
	}
	atomic.AddUint64(&s[1], uint64(n))
	return s
}

func (s *ipstat) recv() uint64 {
	return atomic.LoadUint64(&s[1])
}

func newIPStats() *ipstats {
	stats := &ipstats{
		ips: make(map[string]*ipstat, 0),
	}
	go stats.printStats()
	return stats
}

type ipstats struct {
	ips map[string]*ipstat
	mx  sync.Mutex
}

func (stats *ipstats) incrSent(pkt []byte) {
	ip := dstIP(pkt)
	if ip == "" {
		return
	}
	n := len(pkt)
	stats.mx.Lock()
	stats.ips[ip] = stats.ips[ip].incrSent(n)
	stats.mx.Unlock()
}

func (stats *ipstats) incrRecv(pkt []byte) {
	ip := srcIP(pkt)
	if ip == "" {
		return
	}
	n := len(pkt)
	stats.mx.Lock()
	stats.ips[ip] = stats.ips[ip].incrRecv(n)
	stats.mx.Unlock()
}

func (stats *ipstats) printStats() {
	for {
		time.Sleep(5 * time.Second)
		stats.mx.Lock()
		ips := make(map[string]*ipstat, len(stats.ips))
		for i, s := range stats.ips {
			ips[i] = s
		}
		stats.mx.Unlock()
		for i, s := range ips {
			log.Debugf("%v     Sent: %v   Recv: %v", i, humanize.Bytes(s.sent()), humanize.Bytes(s.recv()))
		}
	}
}

func srcIP(pkt []byte) string {
	if len(pkt) < 16 {
		return ""
	}
	ip := net.IP(pkt[12:16])
	return ip.To4().String()
}

func dstIP(pkt []byte) string {
	if len(pkt) < 20 {
		return ""
	}
	ip := net.IP(pkt[16:20])
	return ip.To4().String()
}
