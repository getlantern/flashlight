package ui

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/anacrolix/confluence/confluence"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/google/uuid"

	analog "github.com/anacrolix/log"
	"github.com/getlantern/replica"
)

type ReplicaHttpServer struct {
	// This is the S3 key prefix used to group uploads for listing.
	InstancePrefix string
	// Used to handle non-Replica specific routes. (All the hard work has been done!).
	Confluence    confluence.Handler
	TorrentClient *torrent.Client
	// Where to store torrent client data.
	StorageDirectory string
	Logger           analog.Logger
}

type countWriter struct {
	bytesWritten int64
}

func (me *countWriter) Write(b []byte) (int, error) {
	me.bytesWritten += int64(len(b))
	return len(b), nil
}

func (me ReplicaHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	me.Logger.WithValues(analog.Debug).Printf("replica server request path: %q", r.URL.Path)
	switch r.URL.Path {
	case "/upload":
		u, err := uuid.NewRandom()
		if err != nil {
			panic(err)
		}
		name := r.URL.Query().Get("name")
		s3Key := fmt.Sprintf("/%s/%s/%s", me.InstancePrefix, u.String(), name)
		me.Logger.WithValues(analog.Debug).Printf("uploading replica key %q", s3Key)
		var cw countWriter
		replicaUploadReader := io.TeeReader(r.Body, &cw)
		f, err := ioutil.TempFile("", "")
		if err == nil {
			defer os.Remove(f.Name())
			defer f.Close()
			replicaUploadReader = io.TeeReader(replicaUploadReader, f)
		} else {
			// This isn't good, but as long as we can add the torrent file metainfo to the local
			// client, we can still spread the metadata, and S3 can take care of the data.
			me.Logger.WithValues(analog.Error).Printf("error creating temporary file: %v", err)
		}
		err = replica.Upload(replicaUploadReader, s3Key)
		me.Logger.WithValues(analog.Debug).Printf("uploaded %d bytes", cw.bytesWritten)
		if err != nil {
			panic(err)
		}
		t, err := replica.GetObjectTorrent(s3Key)
		if err != nil {
			panic(err)
		}
		defer t.Close()
		mi, err := metainfo.Load(t)
		if err != nil {
			panic(err)
		}
		info, err := mi.UnmarshalInfo()
		if err != nil {
			panic(err)
		}
		err = os.Rename(f.Name(), filepath.Join(me.StorageDirectory, info.Name))
		if err != nil {
			// Not fatal: See above, we only really need the metainfo to be added to the torrent.
			me.Logger.WithValues(analog.Error).Printf("error renaming file: %v", err)
		}
		_, err = me.TorrentClient.AddTorrent(mi)
		if err != nil {
			panic(err)
		}
		w.Header().Set("Content-Type", "application/x-bittorrent")
		fmt.Fprintf(w, "%s\n", mi.Magnet(name, mi.HashInfoBytes()))
	default:
		me.Confluence.ServeHTTP(w, r)
	}
}
