package ptlshs

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"hash"
	"math/big"
	"time"
)

// Both the client and the server send a completion signal.
//
// Client signal format:
//
// +-------------------------------------------------------------------+
// | signalPrefix | 32-byte nonce  | padding up to signalLen: all zeros |
// +-------------------------------------------------------------------+
//
// Server signal format:
//
// +--------------------------------------------------------------------+
// | signalPrefix | transcript MAC | padding up to signalLen: all zeros |
// +--------------------------------------------------------------------+
//
// where 'transcript MAC' is a MAC of everything sent from the server to the client. This MAC is
// performed using SHA-256 and the pre-shared secret. For an explanation of this MAC's purpose, see
// clientConn.watchForCompletion.

const (
	// We target this range to make the client completion signal look like an HTTP GET request.
	minSignalLenClient, maxSignalLenClient = 50, 300

	// The server signal is made to look like the response. The maximum payload size for supported
	// cipher suites is in the range of 1151 to 1187 bytes. We cap the server signal below this
	// range to avoid complications from split records.
	absMinSignalLenServer, absMaxSignalLenServer = 250, 1150

	serverSignalLenSpread = 50
)

// Initialized in init. We narrow the range so that the server responses are somewhat consistent.
var minSignalLenServer, maxSignalLenServer int

func init() {
	// Choose a random number in range to serve as the minimum for this runtime.
	var (
		err    error
		absMin = absMinSignalLenServer
		absMax = absMaxSignalLenServer - serverSignalLenSpread
	)
	minSignalLenServer, err = randInt(absMin, absMax)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize random server signal length: %v", err))
	}
	maxSignalLenServer = minSignalLenServer + serverSignalLenSpread
}

var signalPrefix = []byte("handshake complete")

type clientSignal []byte

func newClientSignal(ttl time.Duration) (*clientSignal, error) {
	nonce, err := newNonce(ttl)
	if err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	sLen, err := randInt(minSignalLenClient, maxSignalLenClient)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random signal length: %w", err)
	}

	cs := make(clientSignal, sLen)
	n := copy(cs[:], signalPrefix)
	copy(cs[n:], nonce[:])
	return &cs, nil
}

func parseClientSignal(b []byte) (*clientSignal, error) {
	if len(b) < minSignalLenClient {
		return nil, fmt.Errorf("expected %d bytes, received %d", minSignalLenClient, len(b))
	}
	if !bytes.HasPrefix(b, signalPrefix) {
		return nil, fmt.Errorf("missing signal prefix")
	}
	cs := make(clientSignal, len(b))
	copy(cs[:], b[:])
	return &cs, nil
}

func (cs clientSignal) getNonce() nonce {
	n := nonce{}
	copy(n[:], cs[len(signalPrefix):])
	return n
}

type serverSignal []byte

func newServerSignal(transcriptHMACSHA256 []byte) (*serverSignal, error) {
	sLen, err := randInt(minSignalLenServer, maxSignalLenServer)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random signal length: %w", err)
	}
	ss := make(serverSignal, sLen)
	n := copy(ss[:], signalPrefix)
	copy(ss[n:], transcriptHMACSHA256)
	return &ss, nil
}

func parseServerSignal(b []byte) (*serverSignal, error) {
	if len(b) < absMinSignalLenServer {
		return nil, fmt.Errorf("expected %d bytes, received %d", absMinSignalLenServer, len(b))
	}
	if !bytes.HasPrefix(b, signalPrefix) {
		return nil, fmt.Errorf("missing signal prefix")
	}
	ss := make(serverSignal, len(b))
	copy(ss[:], b[:])
	return &ss, nil
}

func (ss serverSignal) validMAC(mac []byte) bool {
	embedded := ss[len(signalPrefix) : len(signalPrefix)+sha256.Size]
	return hmac.Equal(mac, embedded)
}

func signalHMAC(s Secret) hash.Hash {
	return hmac.New(sha256.New, s[:])
}

// Generates a random integer in [min, max) using crypto/rand. Panics if max-min <= 0.
func randInt(min, max int) (int, error) {
	delta, err := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	if err != nil {
		return 0, err
	}
	return int(delta.Int64()) + min, nil
}
