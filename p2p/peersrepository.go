package p2p

import (
	"sync"
	"time"
)

const defaultMaxDurationToKeepUnusedPeers = 9 * time.Minute
const defaultFlushUnusedPeersDelay = 10 * time.Minute

// PeersRepository data structure does the following:
// - receives peers from a channel and stores only the unique ones
// - flushes old peers periodically
//
// Use this structure like this:
// - Make a new instance with pr := NewPeersRepository()
// - Call pr.StartCollectionAndFlushRoutines() to start the collection and
//   flushing routines
// - Access unique peers through pr.Loop()
// - When you start using a specific peer, mark it with pr.UsedPeer = whatever
// - Unused old peers will be automatically dropped after 'maxDurationToKeepUnusedPeers'
type PeersRepository struct {
	C chan Peer
	// Reference to a peer that's actively being used.
	// This peer is excluded from the "Flush routine"
	UsedPeer *Peer

	// XXX <18-01-22, soltzen> We're using a map here to mimic a "Set"
	// structure that will add only unique values. Using a map also gives us
	// some nice properties:
	// - Traversal is always shuffled
	//   - this is necessary so that different "Censored" peers don't focus on
	//     the same "Free" peers at the same time
	// - Can add/delete peers with O(1) complexity
	//
	// The key in this map is the IP of the peer, while the value is the peer itself.
	// The key is always *just* the IP, not the port, since one IP can update
	// its port, but the DHT bootstrap nodes would report all the ports: we are
	// only interested in the *last* announced port in this system
	mu  sync.RWMutex
	set map[string]Peer

	maxPeers                     int
	maxDurationToKeepUnusedPeers time.Duration
	flushUnusedPeersDelay        time.Duration
	collectOnce                  sync.Once
	// Closes when we wanna shutdown this structure
	doneChan chan struct{}
}

func NewPeersRepository(
	maxPeers int,
	maxDurationToKeepUnusedPeers time.Duration,
	flushUnusedPeersDelay time.Duration) *PeersRepository {
	return &PeersRepository{
		C:                            make(chan Peer),
		set:                          make(map[string]Peer),
		maxPeers:                     maxPeers,
		maxDurationToKeepUnusedPeers: maxDurationToKeepUnusedPeers,
		flushUnusedPeersDelay:        flushUnusedPeersDelay,
		doneChan:                     make(chan struct{}),
	}
}

func (pr *PeersRepository) Len() int {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	return len(pr.set)
}

// TODO <18-01-22, soltzen> Currently announce.GetPeers closes the channel,
// prevent it from doing that, else all our shit won't work
func (pr *PeersRepository) StartCollectionAndFlushRoutines() {
	// Run this collection once only per repo: running it multiple times is not
	// necessary, since the channel doesn't need to close, and the flush
	// routine needs to be scheduled once only
	pr.collectOnce.Do(func() {
		// Collection routine: save unique peers to 'pr.set', without exceeding
		// the limit
		go func() {
			for p := range pr.C {
				select {
				case <-pr.doneChan:
					// Done channel is closed: return
					return
				default:
				}

				log.Debugf("Received peer: %+v", p)
				if pr.Len() >= pr.maxPeers {
					// XXX <18-01-22, soltzen> If this message occurs in logs
					// many times, increase the maxPeers limit
					log.Debugf("Max p2p peers limit reached [%v]: ignoring peer: %+v", pr.maxPeers, p)
					continue
				}
				pr.mu.Lock()
				pr.set[p.String()] = p
				pr.mu.Unlock()
			}
		}()

		// Flush routine: periodically collect unused peers older than
		// 'pr.maxDurationToKeepUnusedPeers'
		go func() {
			for range time.Tick(pr.flushUnusedPeersDelay) {
				select {
				case <-pr.doneChan:
					// Done channel is closed: return
					return
				default:
				}

				pr.mu.Lock()
				for _, p := range pr.set {
					if pr.UsedPeer != nil && p.String() == pr.UsedPeer.String() {
						// Ignore currently used peers
						continue
					}
					if time.Now().Sub(p.CollectedAt) > pr.maxDurationToKeepUnusedPeers {
						delete(pr.set, p.String())
					}
				}
				pr.mu.Unlock()
			}
		}()
	})
}

// Returning false in f() means we failed to get what we need, keep looping through peers.
// Returning true means we got what we need; break the loop.
func (pr *PeersRepository) Loop(f func(*Peer) bool) {
	pr.mu.RLock()
	for _, p := range pr.set {
		if f(&p) {
			break
		}
	}
	pr.mu.RUnlock()
}

func (pr *PeersRepository) Remove(p *Peer) {
	pr.mu.Lock()
	delete(pr.set, p.String())
	if pr.UsedPeer != nil && p.String() == pr.UsedPeer.String() {
		pr.UsedPeer = nil
	}
	pr.mu.Unlock()
}

func (pr *PeersRepository) Close() {
	close(pr.doneChan)
}
