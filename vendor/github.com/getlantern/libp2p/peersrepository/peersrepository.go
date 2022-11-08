package peersrepository

import (
	"context"
	"sync"
	"time"

	"github.com/getlantern/libp2p/common"
	"github.com/getlantern/libp2p/logger"
)

// PeersRepository provides a repository of FreePeers, fetched from the DHT (by
// p2p/pullfreepeersroutine.go), and are available for proxying.
type PeersRepository struct {
	sync.Mutex

	// A fixed-size stack of batches of peers (taken in
	// p2p/pullfreepeersroutine.go. This is the result of running
	// FreePeerBatch.Decode()), where the first element is the most recent
	// batch.
	batchStack [][]*common.GenericFreePeer
}

func NewPeersRepository2(totalBatches int) *PeersRepository {
	return &PeersRepository{
		batchStack: make([][]*common.GenericFreePeer, totalBatches),
	}
}

func (p *PeersRepository) PushBatch(batch []*common.GenericFreePeer) {
	p.Lock()
	defer p.Unlock()

	// push batch to the first position, removing the oldest one
	for i := len(p.batchStack) - 1; i != 0; i-- {
		p.batchStack[i] = p.batchStack[i-1]
	}
	p.batchStack[0] = batch
}

// GetUniqueFreePeers returns a set intersection between p.batchStack,
// yielding only unique peers.
//
// NOTE GetUniqueFreePeers will BLOCK until context is done, or until
// p.batchStack is populated.
func (p *PeersRepository) GetUniqueFreePeers(
	ctx context.Context,
) ([]*common.GenericFreePeer, error) {
	tryToLockAndSetIntersect := func() []*common.GenericFreePeer {
		defer time.Sleep(1 * time.Second)

		// Try to lock
		if ok := p.TryLock(); !ok {
			// If we failed, return
			return nil
		}
		defer p.Unlock()
		// If there're no peers yet, return
		totalPeers := 0
		for _, batch := range p.batchStack {
			totalPeers += len(batch)
		}
		if totalPeers == 0 {
			return nil
		}

		logger.Log.Infof(
			"PeersRepository: set intersection of %d batches with total %d peers",
			len(p.batchStack),
			totalPeers,
		)
		return setIntersection(p.batchStack)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			if peers := tryToLockAndSetIntersect(); peers != nil {
				return peers, nil
			}
		}
	}
}

func (pr *PeersRepository) DropPeer(targetPeer *common.GenericFreePeer) {
	pr.Lock()
	defer pr.Unlock()

	// Loop through all batches and remove the peer from them
	for batchIdx := range pr.batchStack {
		for i, p := range pr.batchStack[batchIdx] {
			if p.String() == targetPeer.String() {
				pr.batchStack[batchIdx] = append(
					pr.batchStack[batchIdx][:i],
					pr.batchStack[batchIdx][i+1:]...)
				break
			}
		}
	}
}

func setIntersection(
	batchStack [][]*common.GenericFreePeer,
) []*common.GenericFreePeer {
	uniquePeers := make(map[string]*common.GenericFreePeer)
	for _, batch := range batchStack {
		for _, peer := range batch {
			uniquePeers[peer.String()] = peer
		}
	}
	uniquePeersSlice := make([]*common.GenericFreePeer, 0, len(uniquePeers))
	for _, peer := range uniquePeers {
		uniquePeersSlice = append(uniquePeersSlice, peer)
	}
	if len(uniquePeersSlice) == 0 {
		return nil
	}
	return uniquePeersSlice
}
