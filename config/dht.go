package config

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/missinggo/v2"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
)

type dhtFetcher struct {
	configDhtTarget krpc.ID
	dhtResources    dhtStuff
	filePath        string
}

type localhostPeerAddr struct{}

func (localhostPeerAddr) String() string {
	return "localhost:42069"
}

func (d dhtFetcher) fetch() (retB []byte, sleep time.Duration, err error) {
	// There's some noise around default noSleep and default sleep times that I don't quite follow.
	// We can override this value for specific cases below should they warrant better handling.
	sleep = 1 * time.Hour
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Minute)
	defer cancel()
	res, _, err := getput.Get(ctx, d.configDhtTarget, d.dhtResources.dhtServer, nil, []byte("globalconfig"))
	if err != nil {
		err = fmt.Errorf("getting latest infohash: %w", err)
	}
	var bep46Payload krpc.Bep46Payload
	err = bencode.Unmarshal(res.V, &bep46Payload)
	if err != nil {
		err = fmt.Errorf("unmarshalling bep46 payload: %w", err)
		return
	}
	// We could do a dance here to determine if the torrent has changed, and return nil bytes per
	// the fetcher interface, but if we already have the torrent it costs us nothing to read it
	// again as it's cached. We also might want to drop old torrents that we're not using anymore.
	// Other config file names or resources may hold references to shared torrents. For now, we can
	// let the old torrents accumulate because there shouldn't be much churn, and we can continue to
	// seed them for other peers.
	t, _ := d.dhtResources.torrentClient.AddTorrentOpt(torrent.AddTorrentOpts{
		InfoHash: bep46Payload.Ih,
	})
	// Add a local seed, assuming that trackers will fail due to same IP.
	t.AddPeers([]torrent.PeerInfo{{
		Addr:    localhostPeerAddr{},
		Trusted: true,
	}})
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
		if f.DisplayPath() == d.filePath {
			break
		}
	}
	if f == nil {
		// Well this is awkward.
		err = fmt.Errorf("file not found in torrent")
		return
	}
	r := f.NewReader()
	defer r.Close()
	retB, err = io.ReadAll(
		// I don't know why a standard interface doesn't exist for this.
		missinggo.ContextedReader{
			R:   r,
			Ctx: ctx,
		},
	)
	if err != nil {
		err = fmt.Errorf("reading all torrent file: %w", err)
		return
	}
	return
}

type dhtStuff struct {
	dhtServer     *dht.Server
	torrentClient *torrent.Client
}

func newDhtStuff() (_ dhtStuff, err error) {
	ds, err := dht.NewServer(nil)
	if err != nil {
		return
	}
	cfg := torrent.NewDefaultClientConfig()
	// Because we add our own, and maintain it manually.
	cfg.NoDHT = true
	// Avoid predictable port assignment, and avoid colliding with the Replica UI server.
	cfg.ListenPort = 0
	cfg.Debug = true
	tc, err := torrent.NewClient(cfg)
	if err != nil {
		ds.Close()
		return
	}
	tc.AddDhtServer(torrent.AnacrolixDhtServerWrapper{Server: ds})
	return dhtStuff{ds, tc}, nil
}
