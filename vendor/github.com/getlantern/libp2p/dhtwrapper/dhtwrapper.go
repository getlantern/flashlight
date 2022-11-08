package dhtwrapper

import (
	"context"
	"crypto/ed25519"
	"hash/crc32"
	"net"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/traversal"
	"github.com/anacrolix/torrent/bencode"
	"github.com/getlantern/libp2p/common"
	"github.com/pkg/errors"
)

// DHTWrapper is a wrapper around our usage of the DHT for the p2pproxing project: https://drive.google.com/drive/u/1/folders/18V3Y9z4puWOS4nSR6NtS4vpP2p-blLTb
//
// It allows an easy way to put/get FreePeers on the DHT without the caller
// worrying about the details
type DHTWrapper interface {
	PutFreePeers(
		ctx context.Context,
		freePeers []*common.GenericFreePeer,
		privKeySeed []byte,
		salt string,
		seq int64,
	) (krpc.ID, *traversal.Stats, error)

	GetFreePeers(
		ctx context.Context,
		targetAndSalt common.Bep44TargetAndSalt,
		lastChecksum *uint32,
	) (freePeers []*common.GenericFreePeer,
		checksum uint32,
		hasNewPeers bool, err error)

	Close()
}

type dhtWrapperImpl struct {
	dhtServer *dht.Server
}

// NewDHTWrapper creates a new DHTWrapper
func NewDHTWrapper(pubIpv4 net.IP) (DHTWrapper, error) {
	dhtCfg := dht.NewDefaultServerConfig()
	// We're using the public IP here mainly to secure the local node ID. This
	// is the ID that other remote nodes will use to validate our peer. If the
	// IP changes (e.g., due to a network switch on mobile devices), it's not
	// great, but it's not gonna be a problem.
	dhtCfg.PublicIP = pubIpv4
	ds, err := dht.NewServer(nil)
	if err != nil {
		return nil, errors.Wrapf(err, "while creating DHT server")
	}

	return &dhtWrapperImpl{dhtServer: ds}, nil
}

// PutFreePeers takes a bunch of "freePeers" as input, creates a new
// FreePeerBatch out of it, and then push a new BEP44Target to the DHT using
// "privKeySeed", "salt" and "seq"
//
// Note: this function **accepts** an empty/nil "freePeers" slice. This just
// means that it'll push an empty message to the DHT. This is still useful when
// you want to reserve a Bep44Target (and reference it in global-config, for
// example) and expect that it would be filled with FreePeers later.
func (d *dhtWrapperImpl) PutFreePeers(
	ctx context.Context,
	freePeers []*common.GenericFreePeer,
	privKeySeed []byte,
	salt string,
	seq int64,
) (krpc.ID, *traversal.Stats, error) {
	batch, err := MakeFreePeerBatch(freePeers)
	if err != nil {
		return krpc.ID{}, nil, errors.Wrapf(err, "while creating FreePeerBatch")
	}

	// Init target
	put := bep44.Put{
		V:    batch,
		Salt: []byte(salt),
		Seq:  seq,
	}
	privKey := ed25519.NewKeyFromSeed(privKeySeed)
	put.K = (*[32]byte)(privKey.Public().(ed25519.PublicKey))
	put.Sign(privKey)

	// Push
	target := put.Target()
	stats, err := getput.Put(
		ctx,
		target,
		d.dhtServer,
		put.Salt,
		func(seq int64) bep44.Put { return put },
	)
	if err != nil {
		return krpc.ID{}, nil, errors.Wrapf(err, "while putting to the DHT")
	}
	// zap.S().
	// 	Debugf("PublishingRoutine #%d: Push successful. Stats: %v",
	// 		routineID, stats)
	return target, stats, nil
}

// GetFreePeers uses "targetAndSalt" to fetch FreePeers from the DHT
// (previously pushed with "DHTWrapper.PutFreePeers()" as a BEP44Target to the
// DHT).
//
// "lastChecksum" is optional. If it's not nil, fetch a payload (if any) from
// the DHT using "targetAndSalt" and compare it's CRC32 checksum with
// "lastChecksum". If they are equal, return early and don't process the
// payload. This saves us some time to avoid unnecessary processing of
// FreePeers if the DHT content didn't change.
//
// Return the fetched FreePeers, the CRC32 checksum of the Bep44 payload,
// whether we fetched new peers, and any errors we encountered.
func (d *dhtWrapperImpl) GetFreePeers(
	ctx context.Context,
	targetAndSalt common.Bep44TargetAndSalt,
	lastChecksum *uint32,
) (freePeers []*common.GenericFreePeer,
	checksum uint32, hasNewPeers bool, err error) {
	v, _, err := getput.Get(
		ctx,
		targetAndSalt.Target,
		d.dhtServer,
		nil,
		[]byte(targetAndSalt.Salt),
	)
	if err != nil {
		return nil, 0, false, errors.Wrapf(err, "while getting from the DHT")
	}
	var payload []byte
	err = bencode.Unmarshal(v.V, &payload)
	if err != nil {
		return nil, 0, false, errors.Wrapf(err, "while unmarshalling payload bytes")
	}
	checksum = crc32.ChecksumIEEE(payload)
	if lastChecksum != nil && checksum == *lastChecksum {
		return nil, checksum, false, nil
	}
	freePeers, err = FreePeerBatch(payload).Decode()
	if err != nil {
		return nil, 0, false, errors.Wrapf(err, "while unmarshalling payload bytes")
	}
	return freePeers, checksum, true, nil
}

// Close closes the *dht.Server instance. It's not really necessary to call it
// but it's good hygiene
func (d *dhtWrapperImpl) Close() {
	d.dhtServer.Close()
}
