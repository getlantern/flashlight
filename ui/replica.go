package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
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
	DataDir     string
	UploadsDir  string
	Logger      analog.Logger
	mux         http.ServeMux
	initMuxOnce sync.Once
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
		me.mux.HandleFunc("/view", me.handleView)
		me.mux.Handle("/", &me.Confluence)
	})
	// TODO(anacrolix): Check this is correct and secure. We might want to be given valid origins
	// and apply them to appropriate routes, or only allow anything from localhost for example.
	w.Header().Set("Access-Control-Allow-Origin", "*")

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
	err = os.Rename(f.Name(), filepath.Join(me.DataDir, info.Name))
	if err != nil {
		// Not fatal: See above, we only really need the metainfo to be added to the torrent.
		me.Logger.WithValues(analog.Error).Printf("error renaming file: %v", err)
	}
	tt, err := me.TorrentClient.AddTorrent(mi)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "text/uri-list")
	fmt.Fprintln(w, createLink(tt.InfoHash(), s3Key, name))
}

func (me ReplicaHttpServer) handleUploads(w http.ResponseWriter, r *http.Request) {
	type upload struct {
		Link                  string
		FileName              string
		FileExtensionMimeType string
	}
	resp := []upload{}
	err := replica.IterUploads(me.uploadsDir(), func(mi *metainfo.MetaInfo, err error) {
		if err != nil {
			me.Logger.Printf("error iterating uploads: %v", err)
			return
		}
		info, err := mi.UnmarshalInfo()
		if err != nil {
			me.Logger.WithValues(analog.Warning).Printf("error unmarshalling info: %v", err)
			return
		}

		dn := displayNameFromInfoName(info.Name)
		resp = append(resp, upload{
			Link:                  createLink(mi.HashInfoBytes(), s3KeyFromInfoName(info.Name), dn),
			FileName:              dn,
			FileExtensionMimeType: mime.TypeByExtension(path.Ext(dn)),
		})
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

func (me ReplicaHttpServer) handleView(w http.ResponseWriter, r *http.Request) {
	m, err := metainfo.ParseMagnetURI(r.URL.Query().Get("link"))
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing magnet link: %v", err.Error()), http.StatusBadRequest)
		return
	}
	s3Key, err := s3KeyFromMagnet(m)
	if err != nil {
		me.Logger.Printf("error getting s3 key from magnet: %v", err)
	}
	t, new, release := me.Confluence.GetTorrent(m.InfoHash)
	defer release()
	if new && t.Info() == nil {
		// Get another reference to the torrent that lasts until we're done fetching the metainfo.
		_, _, release := me.Confluence.GetTorrent(m.InfoHash)
		go func() {
			defer release()
			tob, err := replica.GetObjectTorrent(s3Key)
			if err != nil {
				me.Logger.Printf("error getting metainfo for %q from s3: %v", s3Key, err)
				return
			}
			defer tob.Close()
			mi, err := metainfo.Load(tob)
			if err != nil {
				me.Logger.Print(err)
				return
			}
			err = me.Confluence.PutMetainfo(t, mi)
			if err != nil {
				me.Logger.Printf("error putting metainfo from s3: %v")
			}
			me.Logger.Printf("added metainfo for %q from s3", s3Key)
		}()
	}
	confluence.ServeTorrent(w, r, t)
}

func (me ReplicaHttpServer) uploadsDir() string {
	return me.UploadsDir
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

func createLink(ih torrent.InfoHash, s3Key, name string) string {
	return metainfo.Magnet{
		InfoHash:    ih,
		DisplayName: name,
		Trackers:    []string{"http://s3-tracker.ap-southeast-1.amazonaws.com:6969/announce"},
		Params: url.Values{
			"as": {"https://getlantern-replica.s3-ap-southeast-1.amazonaws.com" + s3Key},
			"xs": {(&url.URL{Scheme: "replica", Path: s3Key}).String()},
		},
	}.String()
}

func s3KeyFromInfoName(name string) string {
	return "/" + strings.Replace(name, "_", "/", 1)
}

func displayNameFromInfoName(name string) string {
	ss := strings.SplitN(name, "_", 2)
	if len(ss) > 1 {
		return ss[1]
	}
	return ss[0]
}

func s3KeyFromMagnet(m metainfo.Magnet) (string, error) {
	u, err := url.Parse(m.Params.Get("xs"))
	if err != nil {
		return "", err
	}
	return u.Path, nil
}
