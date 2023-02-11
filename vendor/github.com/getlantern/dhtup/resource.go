package dhtup

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"

	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/pkg/errors"
)

type OpenedResource interface {
	missinggo.ReadContexter
	io.Closer
}

// NameAndInfohashBep46Payload is a Bep46Payload that also includes the name of
// the resource along with the infohash.
//
// The concrete case we use it is when we're publishing a resource (e.g., a
// list of FreePeers) where the webseed and source files are located on, for
// example, an AWS S3 bucket but its name is variable. See [p2pregistrar's
// GetReadyFreePeersFileNameForS3()](https://github.com/getlantern/p2pregistrar/blob/67631ce3a7ec0cae3f2a7b0fd98f97064497fc69/server/seedpeersroutine.go#L40)
// function for a full reference, but here's a quick one for you:
//
//      For example, the webseed file for the free-peers file with RunNonce
//      "aaa", GitCommitHash "bbb" and seedRoutineId 0 on Bucket
//      p2pproxying-staging would be:
//
//       aaa-bbb-0__free-peers.torrent
//
// Since it's essential for us to know the source and webseed URLs to get
// metainfo about the torrent we're fetching (i.e., the one containing the list
// of FreePeers, published by the p2pregistrar), the easiest way to retrieve
// this name is to just push it alongside the infohash to the DHT.
type NameAndInfohashBep46Payload struct {
	Ih   metainfo.Hash `bencode:"ih"`
	Name string        `bencode:"name"`
}

type Resource interface {
	// Fetches the krpc.Bep46Payload for this resource
	FetchBep46Payload(context.Context) (metainfo.Hash, error)
	// Fetches the NameAndInfohashBep46Payload for this resource
	FetchBep46PayloadWithName(context.Context) (NameAndInfohashBep46Payload, error)
	// Pushes a NameAndInfohashBep46Payload to the DHT
	PutBep46PayloadWithName(context.Context, string, torrent.InfoHash, int64, bool) (krpc.ID, error)
	// Makes a torrent out of the info in the Bep46Payload and returns the torrent's io.ReadCloser
	FetchTorrentFileReader(context.Context, metainfo.Hash) (OpenedResource, bool, error)
	// Fetches the bep46 payload for this resource, and returns the torrent's io.ReadCloser.
	// This is basically, running FetchBep46Payload() and then FetchTorrentFileReader()
	Open(ctx context.Context) (_ OpenedResource, temporary bool, _ error)
	Target() krpc.ID
	Salt() string
	// Helper functions that allow us to inject the webseed and source URLs
	// after the initialization of a Resource
	SetWebseeds([]string)
	SetSources([]string)
}

// ResourceImpl implements Resource
type ResourceImpl struct {
	ResourceInput
}

// ResourceInput is a typed constructor for Resource
type ResourceInput struct {
	DhtTarget    krpc.ID
	DhtContext   *Context
	FilePath     string
	WebSeedUrls  []string
	Salt         string
	MetainfoUrls []string

	// Only used for putting objects
	PrivKeySeed [32]byte
}

func NewResource(input ResourceInput) Resource {
	return &ResourceImpl{input}
}

func (me *ResourceImpl) FetchBep46Payload(ctx context.Context) (metainfo.Hash, error) {
	// TODO <22-03-22, soltzen> Have an option in this system to store the
	// current `seq` parameter and only download new ones
	res, _, err := getput.Get(ctx, me.ResourceInput.DhtTarget, me.ResourceInput.DhtContext.DhtServer, nil, []byte(me.ResourceInput.Salt))
	if err != nil {
		return metainfo.Hash{}, fmt.Errorf("getting latest infohash: %w", err)
	}
	bep46Payload := &krpc.Bep46Payload{}
	err = bencode.Unmarshal(res.V, bep46Payload)
	if err != nil {
		return metainfo.Hash{}, fmt.Errorf("unmarshalling bep46 payload: %w", err)
	}
	return bep46Payload.Ih, nil
}

func (me *ResourceImpl) FetchTorrentFileReader(ctx context.Context, bep46PayloadInfohash metainfo.Hash) (
	ret OpenedResource,
	// The error is temporary, try again in a bit.
	temporary bool,
	err error,
) {
	temporary = true
	// We might want to drop old torrents that we're not using anymore. Other config file names or
	// resources may hold references to shared torrents. For now, we can let the old torrents
	// accumulate because there shouldn't be much churn, and we can continue to seed them for other
	// peers.
	t, _ := me.ResourceInput.DhtContext.TorrentClient.AddTorrentOpt(torrent.AddTorrentOpts{
		InfoHash: bep46PayloadInfohash,
	})
	// Add a backup method to obtain the torrent info.
	t.UseSources(me.ResourceInput.MetainfoUrls)
	// If we can't get the metainfo, we'll never be communicated these trackers, some of which may
	// provide the only way to actively connect to the publishing nodes. See
	// https://github.com/getlantern/lantern-internal/issues/5469.
	t.AddTrackers([][]string{DefaultTrackers})
	// Add a local seed for testing, assuming that announcing will fail to return our own IP.
	t.AddPeers([]torrent.PeerInfo{{
		Addr:    localhostPeerAddr{},
		Trusted: true,
	}})
	// An alternate source for the torrent data, since the first peer has no other peers to
	// bootstrap from.
	t.AddWebSeeds(me.ResourceInput.WebSeedUrls)
	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		err = fmt.Errorf("waiting for torrent info: %w", ctx.Err())
		return
	}
	var f *torrent.File
	for _, f = range t.Files() {
		// I think the opts fileName is just a base name, our torrent should be structured so that
		// the files sit in the root folder to match.
		if f.DisplayPath() == me.ResourceInput.FilePath {
			break
		}
	}
	if f == nil {
		// Well this is awkward.
		err = fmt.Errorf("file not found in torrent")
		// Fixing this would require a republish, which would be on the typical publishing schedule.
		temporary = false
		return
	}
	ret = f.NewReader()
	// Everything good, use the default!
	temporary = false
	return
}

func (me *ResourceImpl) FetchBep46PayloadWithName(ctx context.Context) (NameAndInfohashBep46Payload, error) {
	res, _, err := getput.Get(ctx, me.DhtTarget, me.DhtContext.DhtServer, nil, []byte(me.ResourceInput.Salt))
	if err != nil {
		return NameAndInfohashBep46Payload{}, fmt.Errorf("getting latest infohash: %w", err)
	}
	bep46Payload := NameAndInfohashBep46Payload{}
	err = bencode.Unmarshal(res.V, &bep46Payload)
	if err != nil {
		return NameAndInfohashBep46Payload{}, fmt.Errorf(
			"unmarshalling bep46 payload: %w",
			err,
		)
	}
	return bep46Payload, nil
}

// XXX <24-06-2022, soltzen> There's something weird about "salt" I should clarify here:
// - All the callers of this function and of GetBep46Payload() initialize salt
//   as a []byte, then cast it to a string with hex.EncodeToString()
// - This function takes "salt" as a string
// - And converts it later when used in getput to a []byte
//
// It's confusing, but there's a rationale: the `dht` CLI program
// (https://github.com/anacrolix/dht/blob/114cb15/cmd/dht/main.go), which is
// mentioned in the README, takes a string "salt" CLI argument and then
// converts it to a []byte with `[]byte(cli.Salt)`
// [here](https://github.com/afjoseph/dht/blob/bf548049b40e3da87370850006c8d9087b712cfd/cmd/dht/get.go#L34).
//
// So, if you generate your salts with cryptoRand.Read(), the salt value that's
// printed in logs (with stdout, or with /debug/ endpoint, or anywhere you get
// your logs) should match the CLI input to the `dht` binary, else your checks
// could fail and the README will be false.
//
// In other words:
//
//     b := []make([]byte, 16)
//     cryptoRand.Read(b)
//     var rightSalt []byte := []byte(hex.EncodeToString(b))
//     var badSalt := b
//
//     // And rightSalt != badSalt
//
func (me *ResourceImpl) PutBep46PayloadWithName(ctx context.Context,
	name string,
	infohash torrent.InfoHash,
	seq int64, autoseq bool) (krpc.ID, error) {
	// Construct BEP46 Put Payload
	put := bep44.Put{
		V:    NameAndInfohashBep46Payload{Ih: infohash, Name: name},
		Salt: []byte(me.ResourceInput.Salt),
		Seq:  seq,
	}
	privKey := ed25519.NewKeyFromSeed(me.PrivKeySeed[:])
	put.K = (*[32]byte)(privKey.Public().(ed25519.PublicKey))
	target := put.Target()

	// Push it to the DHT
	_, err := getput.Put(ctx, target,
		me.DhtContext.DhtServer,
		put.Salt, func(seq int64) bep44.Put {
			// Increment best seen seq by one.
			if autoseq {
				put.Seq = seq + 1
			}
			put.Sign(privKey)
			return put
		})
	if err != nil {
		return krpc.ID{}, errors.Wrapf(
			err,
			"while running PutBep46Payload() during a DHT traversal",
		)
	}
	return target, nil
}

func (me *ResourceImpl) Open(ctx context.Context) (
	ret OpenedResource,
	// The error is temporary, try again in a bit.
	temporary bool,
	err error,
) {
	temporary = true
	bep46Payload, err := me.FetchBep46Payload(ctx)
	if err != nil {
		err = fmt.Errorf("unmarshalling bep46 payload: %w", err)
		return
	}
	return me.FetchTorrentFileReader(ctx, bep46Payload)
}

func (me *ResourceImpl) Target() krpc.ID {
	return me.ResourceInput.DhtTarget
}

func (me *ResourceImpl) Salt() string {
	return me.ResourceInput.Salt
}

func (me *ResourceImpl) SetWebseeds(urls []string) {
	me.ResourceInput.WebSeedUrls = urls
}

func (me *ResourceImpl) SetSources(urls []string) {
	me.ResourceInput.MetainfoUrls = urls
}

type localhostPeerAddr struct{}

func (localhostPeerAddr) String() string {
	return "localhost:42069"
}
