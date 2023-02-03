package dhtwrapper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	mathRand "math/rand"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/getlantern/libp2p/common"
	"github.com/pkg/errors"
)

// FreePeerBatch is a compressed batch of freePeers
type FreePeerBatch []byte

// Maximum number of FreePeers per FreePeerBatch. See the README ("on
// FreePeerBatch" section) why and how this number was calculated.
const MaxFreePeerSizePerBatch = 25

const emptyBatchPrefix = "bunnyfoofoo"

// MakeFreePeerBatch encodes freePeers into a compressed batch blob
// that can be pushed to the DHT.
//
// If freePeers is nil, a random byte blob will be returned
func MakeFreePeerBatch(
	freePeers []*common.GenericFreePeer,
) (FreePeerBatch, error) {
	if len(freePeers) == 0 {
		// if we received a nil or empty list of freePeers, we don't want to
		// send an empty batch to the DHT. Just make a dummy batch filled with
		// a random value
		return FreePeerBatch(
			[]byte(fmt.Sprintf("%s %d", emptyBatchPrefix, mathRand.Int63())),
		), nil
	}

	// JSON-marshal and concatenate the freePeers into a single string
	var sb strings.Builder
	for _, p := range freePeers {
		b, err := json.Marshal(p)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"failed to marshal freePeer.Info: %v",
				p.String(),
			)
		}
		sb.Write(b)
		sb.WriteString("\n")
	}

	// Brotli Compress
	// brotli compression was chosen after evaluating bzip2, gzip,
	// smaz and lzma. brotli is the most efficient and most compact for this use
	// case. See the README for more details.
	// ---------------
	var b bytes.Buffer
	w := brotli.NewWriter(&b)
	_, err := w.Write([]byte(sb.String()))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to write compressed payload")
	}
	w.Close()
	return b.Bytes(), nil
}

func (b FreePeerBatch) Decode() ([]*common.GenericFreePeer, error) {
	if len(b) == 0 {
		return nil, errors.New("invalid FreePeerBatch")
	}

	if strings.HasPrefix(string(b), emptyBatchPrefix) {
		// if this is an empty batch, return an empty list of freePeers
		return nil, nil
	}

	// Decompress
	// -------------
	decompressedPayload, err := ioutil.ReadAll(
		brotli.NewReader(bytes.NewReader(b)),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read decompressed payload")
	}

	// Parse
	// ------
	var freePeers []*common.GenericFreePeer
	for _, line := range strings.Split(string(decompressedPayload), "\n") {
		if line == "" {
			continue
		}
		p := &common.GenericFreePeer{}
		if err := json.Unmarshal([]byte(line), p); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal freePeer.Info")
		}
		freePeers = append(freePeers, p)
	}
	return freePeers, nil
}
