package replica

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/getlantern/appdir"
	"golang.org/x/xerrors"

	"github.com/anacrolix/confluence/confluence"
	analog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	replicaUi "github.com/getlantern/flashlight/ui/replica"
	"github.com/getlantern/replica"
)

func NewHttpHandler() (_ *replicaUi.HttpHandler, exitFunc func(), err error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		panic(err)
	}
	replicaLogger := analog.Default
	const replicaDirElem = "replica"
	replicaConfigDir := appdir.General(replicaDirElem)
	uploadsDir := filepath.Join(replicaConfigDir, "uploads")
	replicaDataDir := filepath.Join(userCacheDir, replicaDirElem, "data")
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = replicaDataDir
	cfg.Seed = true
	//cfg.Debug = true
	cfg.Logger = replicaLogger.WithFilter(func(m analog.Msg) bool {
		return !m.HasValue("upnp-discover")
	})
	// DHT disabled pending fixes to transaction query concurrency.	 Also using S3 as a
	// fallback means we have a great tracker to rely on.
	//cfg.NoDHT = true
	// Household users may be behind a NAT/bad router, or on a limited device like a mobile. We
	// don't want to overload their networks, so ensure the default connection tracking
	// behaviour.
	//cfg.ConnTracker = conntrack.NewInstance()
	// Helps debug connection tracking, for best configuring DHT and other limits.
	http.HandleFunc("/debug/conntrack", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		cfg.ConnTracker.PrintStatus(w)
	})
	torrentClient, err := torrent.NewClient(cfg)
	if err != nil {
		//err = xerrors.Errorf("starting torrent client: %w", err)
		//return
		fmt.Printf("Error creating client: %v", err)
		if torrentClient != nil {
			torrentClient.Close()
		}
		// Try an ephemeral port in case there was an error binding.
		cfg.ListenPort = 0
		torrentClient, err = torrent.NewClient(cfg)
		if err != nil {
			err = xerrors.Errorf("starting torrent client: %w", err)
			return
		}
	}

	if err := replica.IterUploads(uploadsDir, func(mi *metainfo.MetaInfo, err error) {
		if err != nil {
			replicaLogger.Printf("error while iterating uploads: %v", err)
			return
		}
		t, err := torrentClient.AddTorrent(mi)
		if err != nil {
			replicaLogger.WithValues(analog.Error).Printf("error adding existing upload to torrent client: %v", err)
		} else {
			replicaLogger.Printf("added previous upload %q to torrent client", t.Name())
		}
	}); err != nil {
		panic(err)
	}

	return &replicaUi.HttpHandler{
		Confluence: confluence.Handler{
			TC: torrentClient,
			MetainfoCacheDir: func() *string {
				s := filepath.Join(userCacheDir, replicaDirElem, "metainfos")
				return &s
			}(),
		},
		TorrentClient: torrentClient,
		DataDir:       replicaDataDir,
		Logger:        replicaLogger,
		UploadsDir:    uploadsDir,
	}, torrentClient.Close, nil
}
