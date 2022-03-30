package p2p

import (
	"context"
	"net"
	"sync"
	"time"
)

const defaultMaxDurationToKeepUnusedPeers = 9 * time.Minute
const defaultFlushUnusedPeersDelay = 10 * time.Minute

// The PeersRepository data structure does the following:
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
	setMu         sync.RWMutex
	set           map[string]Peer
	peersToDropMu sync.Mutex
	peersToDrop   []*Peer

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
	pr.setMu.RLock()
	defer pr.setMu.RUnlock()
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

				// If AreAllPeersOnLocalhost, then this peer is on the same LAN
				// network as us: set the IP to be localhost. This flag will
				// **only** be set in testing
				if AreAllPeersOnLocalhost {
					p.IP = net.IPv4(127, 0, 0, 1)
				}

				pr.setMu.Lock()
				// Check if we can accommodate for more peers
				if len(pr.set) >= pr.maxPeers {
					// XXX <18-01-22, soltzen> If this message occurs in logs
					// many times, increase the maxPeers limit
					log.Debugf("Max p2p peers limit reached [%v]: ignoring peer: %+v",
						pr.maxPeers, p)
					pr.setMu.Unlock()
					continue
				}
				// Only print a log statement if this is a new peer. This
				// reduces the noise tremendously
				if _, ok := pr.set[p.String()]; !ok {
					log.Debugf("Received new peer: %+v", p)
					pr.set[p.String()] = p
				}
				pr.setMu.Unlock()
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

				pr.setMu.Lock()
				// Drop old peers
				for _, p := range pr.set {
					if pr.UsedPeer != nil && p.String() == pr.UsedPeer.String() {
						// Ignore currently used peers
						continue
					}
					if time.Now().Sub(p.CollectedAt) > pr.maxDurationToKeepUnusedPeers {
						delete(pr.set, p.String())
					}
				}

				// Drop peers that are in the "to drop" list
				pr.peersToDropMu.Lock()
				for _, p := range pr.peersToDrop {
					if pr.UsedPeer != nil && p.String() == pr.UsedPeer.String() {
						pr.UsedPeer = nil
					}
					delete(pr.set, p.String())
				}
				pr.peersToDrop = nil
				pr.peersToDropMu.Unlock()

				pr.setMu.Unlock()
			}
		}()
	})
}

// HasAnyValue returns true if the set (i.e., the peers repository) contains
// any value.
//
// This function loops forever until A) the set gets a value, or B) the
// supplied context is done
func (pr *PeersRepository) HasAnyValue(ctx context.Context) bool {
	for range time.Tick(100 * time.Millisecond) {
		if pr.Len() > 0 {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		default:
		}
	}
	panic("unreachable")
}

// Loop waits until there's at least one value in the repository, and then
// attempts to loop over each peer and execute f() with it.
//
// If f() returns false, the loop will continue.
// Else, the loop will break.
func (pr *PeersRepository) Loop(ctx context.Context, f func(*Peer) bool) {
	// If we have no values, wait until we do
	if !pr.HasAnyValue(ctx) {
		return
	}
	pr.setMu.RLock()
	defer pr.setMu.RUnlock()

	// Always start with the UserPeer
	if pr.UsedPeer != nil && f(pr.UsedPeer) {
		return
	}
	for _, p := range pr.set {
		if f(&p) {
			break
		}
	}
}

// Drop marks the peer as to be dropped.
func (pr *PeersRepository) Drop(p *Peer) {
	pr.peersToDropMu.Lock()
	defer pr.peersToDropMu.Unlock()
	pr.peersToDrop = append(pr.peersToDrop, p)
}

func (pr *PeersRepository) Close() {
	close(pr.doneChan)
}
