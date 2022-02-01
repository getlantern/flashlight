package p2p

import (
	"context"
	"time"

	"github.com/anacrolix/dht/v2"
)

type Peer struct {
	Infohash    [20]byte
	CollectedAt time.Time
	dht.Peer
}

type ReplicaP2pFunctions interface {
	GetPeers(ctx context.Context, ss []*dht.Server, ihs [][20]byte, peers chan<- Peer) error
	Announce(ss []*dht.Server, ihs [][20]byte, port int) error
}
