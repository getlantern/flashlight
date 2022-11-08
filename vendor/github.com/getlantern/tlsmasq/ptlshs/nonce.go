package ptlshs

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

// Nonce format:
//
// +-------------------------------------------------------------------------+
// | 8-byte timestamp: nanoseconds since UTC epoch | 24 bytes of random data |
// +-------------------------------------------------------------------------+

// A nonce used in proxied TLS handshakes. This is used to ensure that the completion signal (sent
// by the client after a completed handshake) is not replayable.
type nonce [32]byte

func newNonce(ttl time.Duration) (*nonce, error) {
	n := nonce{}
	binary.LittleEndian.PutUint64(n[:], uint64(time.Now().Add(ttl).UnixNano()))
	if _, err := rand.Read(n[8:]); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return &n, nil
}

func (n nonce) expiration() time.Time {
	return time.Unix(0, int64(binary.LittleEndian.Uint64(n[:])))
}

type nonceCache struct {
	// We sort nonces we've seen into buckets using their expiration timestamps. Each bucket has a
	// a beginning, relative to startTime, and a span, equal to bucketSpan. All nonces in a bucket
	// with beginning b will have an expiration >= b and < b + bucketSpan. When all nonces in a
	// bucket have expired, the bucket will be removed from the buckets map.

	startTime  time.Time
	bucketSpan time.Duration

	buckets     map[time.Duration]map[nonce]bool
	bucketsLock sync.Mutex

	done      chan struct{}
	closeOnce sync.Once
}

func newNonceCache(sweepEvery time.Duration) *nonceCache {
	nc := nonceCache{
		time.Now(), sweepEvery,
		map[time.Duration]map[nonce]bool{}, sync.Mutex{},
		make(chan struct{}), sync.Once{},
	}
	return &nc
}

func (nc *nonceCache) isValid(n nonce) bool {
	expiration := n.expiration()
	if time.Now().After(expiration) {
		return false
	}
	bucket := nc.getBucket(expiration)

	nc.bucketsLock.Lock()
	defer nc.bucketsLock.Unlock()
	if bucket[n] {
		return false
	}
	bucket[n] = true
	return true
}

func (nc *nonceCache) getBucket(exp time.Time) map[nonce]bool {
	diff := exp.Sub(nc.startTime)
	bucketStart := diff - (diff % nc.bucketSpan)

	nc.bucketsLock.Lock()
	bucket, ok := nc.buckets[bucketStart]
	if !ok {
		bucket = map[nonce]bool{}
		nc.buckets[bucketStart] = bucket
	}
	nc.bucketsLock.Unlock()
	if !ok {
		cutoff := nc.startTime.Add(bucketStart + nc.bucketSpan)
		time.AfterFunc(time.Until(cutoff), func() {
			nc.bucketsLock.Lock()
			delete(nc.buckets, bucketStart)
			nc.bucketsLock.Unlock()
		})
	}
	return bucket
}

func (nc *nonceCache) close() {
	nc.closeOnce.Do(func() { close(nc.done) })
}
