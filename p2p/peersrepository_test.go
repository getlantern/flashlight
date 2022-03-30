package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/stretchr/testify/require"
)

func TestPeersRepository(t *testing.T) {
	t.Run("Assert that PeersRepository can receive new peers, won't block receiving, and will flush old peers periodically", func(t *testing.T) {
		maxPeers := 10
		flushDelay := 1 * time.Second
		keepUnusedPeersDuration := 2 * time.Second
		pr := NewPeersRepository(maxPeers, keepUnusedPeersDuration, flushDelay)
		go func() {
			for i := 0; i < maxPeers; i++ {
				// Peer here doesn't matter: just its metadata does
				pr.C <- Peer{[20]byte{}, time.Now(), dht.Peer{IP: nil, Port: i}}
			}
		}()
		// Start collecting and sleep just a tiny bit, to make sure we have
		// anything at all to work with
		pr.StartCollectionAndFlushRoutines()
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, maxPeers, pr.Len())

		// Assert that, after we wait for the first flush, all old peers are dropped
		time.Sleep(keepUnusedPeersDuration + 1*time.Second)
		require.Equal(t, 0, pr.Len())
	})

	t.Run("Assert that a used peer will not be flushed", func(t *testing.T) {
		maxPeers := 10
		flushDelay := 1 * time.Second
		keepUnusedPeersDuration := 2 * time.Second
		pr := NewPeersRepository(maxPeers, keepUnusedPeersDuration, flushDelay)
		go func() {
			for i := 0; i < maxPeers; i++ {
				// Peer here doesn't matter: just it's metadata does
				pr.C <- Peer{[20]byte{}, time.Now(), dht.Peer{IP: nil, Port: i}}
			}
		}()
		// Start collecting and sleep just a tiny bit, to make sure we have
		// anything at all to work with
		pr.StartCollectionAndFlushRoutines()
		time.Sleep(100 * time.Millisecond)

		// Mark the fifth peer (random choice) as the used peer
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		j := 0
		var chosenPeer *Peer
		pr.Loop(ctx, func(p *Peer) bool {
			if j == 5 {
				pr.UsedPeer = p
				chosenPeer = p
				return true
			}
			j++
			return false
		})

		// Sleep until the first flush of old peers occur
		time.Sleep(keepUnusedPeersDuration + 1*time.Second)
		// Assert our used peer didn't get flushed away
		require.Equal(t, 1, pr.Len())
		// Assert our used peer's identity
		require.Equal(t, chosenPeer, pr.UsedPeer)
	})

	t.Run("Assert that PeersRepository will timeout properly", func(t *testing.T) {
		maxPeers := 10
		flushDelay := 1 * time.Second
		keepUnusedPeersDuration := 2 * time.Second
		pr := NewPeersRepository(maxPeers, keepUnusedPeersDuration, flushDelay)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pr.StartCollectionAndFlushRoutines()
		pr.Loop(ctx, func(p *Peer) bool {
			require.FailNow(t, "Shouldn't have received any peers")
			return true
		})
		require.Error(t, context.DeadlineExceeded, ctx.Err())
		require.Equal(t, 0, pr.Len())
	})
}
