package dhtup

import (
	"net"
	"path/filepath"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/squirrel"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	sqliteStorage "github.com/anacrolix/torrent/storage/sqlite"
	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("dhtup")

type Context struct {
	DhtServer      *dht.Server
	TorrentClient  *torrent.Client
	TorrentStorage storage.ClientImplCloser
}

func NewContext(publicIp net.IP, cacheDir string) (_ Context, err error) {
	dhtCfg := dht.NewDefaultServerConfig()
	// This is used to secure the local node ID. If our IP changes it is bad luck, and not a huge
	// deal.
	dhtCfg.PublicIP = publicIp
	ds, err := dht.NewServer(nil)
	if err != nil {
		return
	}
	cfg := torrent.NewDefaultClientConfig()
	// We could set the torrent public IPs, but it mainly uses them to configure its implicit DHT
	// instances, of which we have none. The IPs the client use should track any changes.

	// Because we add our own, and maintain it manually.
	cfg.NoDHT = true
	// Avoid predictable port assignment, and avoid colliding with the Replica UI server.
	cfg.ListenPort = 0
	cfg.Debug = false
	cfg.Seed = true
	cfg.DropMutuallyCompletePeers = true
	ts := makeStorage(cacheDir)
	cfg.DefaultStorage = ts
	tc, err := torrent.NewClient(cfg)
	if err != nil {
		ds.Close()
		ts.Close()
		return
	}
	tc.AddDhtServer(torrent.AnacrolixDhtServerWrapper{Server: ds})
	return Context{
		DhtServer:      ds,
		TorrentClient:  tc,
		TorrentStorage: ts,
	}, nil
}

func makeStorage(cacheDir string) (s storage.ClientImplCloser) {
	// No path means a temporary file that is removed from the disk after opening. Perfect.
	s, err := sqliteStorage.NewDirectStorage(squirrel.NewCacheOpts{})
	if err == nil {
		return
	}
	log.Errorf("creating dht config sqlite storage: %v", err)
	return storage.NewFileOpts(storage.NewFileClientOpts{
		ClientBaseDir: filepath.Join(cacheDir, "data"),
		// Since many of our torrents will be iterations of the same resources, we divide them
		// up based on infohash to avoid info name collisions.
		TorrentDirMaker: func(baseDir string, info *metainfo.Info, infoHash metainfo.Hash) string {
			return filepath.Join(baseDir, infoHash.HexString())
		},
	})
}
