package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/anacrolix/confluence/confluence"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/google/uuid"

	analog "github.com/anacrolix/log"
	"github.com/getlantern/replica"
)

type ReplicaHttpServer struct {
	// This is the S3 key prefix used to group uploads for listing.
	//InstancePrefix string
	// Used to handle non-Replica specific routes. (All the hard work has been done!).
	Confluence    confluence.Handler
	TorrentClient *torrent.Client
	// Where to store torrent client data.
	StorageDirectory string
	Logger           analog.Logger
	mux              http.ServeMux
	initMuxOnce      sync.Once
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
	me.initMuxOnce.Do(func() {
		me.mux.HandleFunc("/upload", me.handleUpload)
		me.mux.HandleFunc("/uploads", me.handleUploads)
		me.mux.Handle("/", me.Confluence)
	})
	me.mux.ServeHTTP(w, r)
}

func (me ReplicaHttpServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	u, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	name := r.URL.Query().Get("name")
	s3Key := u.String()
	if name != "" {
		s3Key += "/" + name
	}
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
	p := filepath.Join(me.uploadsDir(), filepath.FromSlash(s3Key)+".torrent")
	err = storeUploadedTorrent(t, p)
	if err != nil {
		panic(err)
	}
	mi, err := metainfo.LoadFromFile(p)
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
	tt, err := me.TorrentClient.AddTorrent(mi)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "text/uri-list")
	fmt.Fprintln(w, createReplicaUrl(tt.InfoHash(), s3Key))
}

func (me ReplicaHttpServer) handleUploads(w http.ResponseWriter, r *http.Request) {
	var resp []string
	err := filepath.Walk(me.uploadsDir(), func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			me.Logger.Printf("error walking to %q: %v", path, err)
			return nil
		}
		if fi.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".torrent" {
			return nil
		}
		mi, err := metainfo.LoadFromFile(path)
		if err != nil {
			me.Logger.WithValues(analog.Warning).Printf("error loading metainfo from file %q: %v", path, err)
			return nil
		}
		info, err := mi.UnmarshalInfo()
		if err != nil {
			me.Logger.WithValues(analog.Warning).Printf("error unmarshalling info: %v", err)
			return nil
		}
		resp = append(resp, createReplicaUrl(mi.HashInfoBytes(), s3KeyFromInfoName(info.Name)))
		return nil
	})
	if err != nil {
		me.Logger.Printf("error walking uploads dir: %v", err)
	}
	b, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "%s\n", b)
}

func (me ReplicaHttpServer) uploadsDir() string {
	return filepath.Join(me.StorageDirectory, "uploads")
}

func storeUploadedTorrent(r io.Reader, path string) error {
	err := os.MkdirAll(filepath.Dir(path), 0750)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	if err != nil {
		return err
	}
	return f.Close()
}

func createReplicaUrl(ih torrent.InfoHash, s3Key string) string {
	return (&url.URL{
		Scheme: "https",
		Host:   "replica.getlantern.org",
		Path:   s3Key,
		RawQuery: url.Values{
			"ih": {ih.HexString()},
		}.Encode(),
	}).String()
}

func s3KeyFromInfoName(name string) string {
	return strings.Replace(name, "_", "/", 1)
}
