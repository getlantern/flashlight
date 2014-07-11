package statserver

import (
	"sync/atomic"
	"time"
)

var (
	publishInterval = 30 * time.Second
)

// Peer represents information about a peer
type Peer struct {
	ID              string    `json:"peerid"`
	LastConnected   time.Time `json:"lastConnected"`
	BytesDn         int64     `json:"bytesDn"`
	BytesUp         int64     `json:"bytesUp"`
	BytesUpDn       int64     `json:"bytesUpDn"`
	BPSDn           int64     `json:"bpsDn"`
	BPSUp           int64     `json:"bpsUp"`
	BPSUpDn         int64     `json:"bpsUpDn"`
	Country         string    `json:"country"`
	Latitude        float32   `json:"lat"`
	Longitude       float32   `json:"lon"`
	atLastReporting *Peer
}

type publish func(peer *Peer)

func newPeer(id string, pub publish) *Peer {
	peer := &Peer{
		ID: id,
		atLastReporting: &Peer{
			ID:            id,
			LastConnected: time.Now(),
		},
	}
	go peer.publishPeriodically(pub)
	return peer
}

func (peer *Peer) publishPeriodically(pub publish) {
	for {
		time.Sleep(publishInterval)
		pub(peer)
		*peer.atLastReporting = *peer
	}
}

func (peer *Peer) onBytesReceived(bytes int64) {
	atomic.AddInt64(&peer.BytesUp, bytes)
}

func (peer *Peer) onBytesSent(bytes int64) {
	atomic.AddInt64(&peer.BytesDn, bytes)
}
