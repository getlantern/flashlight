package replicaUi

import (
	"bytes"
	"encoding/json"
	"errors"
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

type HttpHandler struct {
	// Used to handle non-Replica specific routes. (Some of the hard work has been done!). This will
	// probably go away soon, as I pick out the parts we actually need.
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

func (me *HttpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	me.Logger.WithValues(analog.Debug).Printf("replica server request path: %q", r.URL.Path)
	me.initMuxOnce.Do(func() {
		me.mux.HandleFunc("/upload", me.handleUpload)
		me.mux.HandleFunc("/uploads", me.handleUploads)
		me.mux.HandleFunc("/view", me.handleView)
		me.mux.HandleFunc("/delete", me.handleDelete)
		// TODO(anacrolix): Actually not much of Confluence is used now, probably none of the
		// routes, so this might go away soon.
		me.mux.Handle("/", &me.Confluence)
	})
	// TODO(anacrolix): Check this is correct and secure. We might want to be given valid origins
	// and apply them to appropriate routes, or only allow anything from localhost for example.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	me.mux.ServeHTTP(w, r)
}

func (me *HttpHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
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
	tmpFile, tmpFileErr := func() (*os.File, error) {
		if true {
			return ioutil.TempFile("", "")
		} else {
			return nil, errors.New("sike")
		}
	}()
	if tmpFileErr == nil {
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()
		replicaUploadReader = io.TeeReader(replicaUploadReader, tmpFile)
	} else {
		// This isn't good, but as long as we can add the torrent file metainfo to the local client,
		// we can still spread the metadata, and S3 can take care of the data.
		me.Logger.WithValues(analog.Error).Printf("error creating temporary file: %v", tmpFileErr)
	}
	err = replica.Upload(replicaUploadReader, s3Key)
	me.Logger.WithValues(analog.Debug).Printf("uploaded %d bytes", cw.bytesWritten)
	if err != nil {
		panic(err)
	}
	otr, err := replica.GetObjectTorrent(s3Key)
	if err != nil {
		panic(err)
	}
	b, err := ioutil.ReadAll(otr)
	otr.Close()
	if err != nil {
		panic(err)
	}
	mi, err := metainfo.Load(bytes.NewReader(b))
	if err != nil {
		panic(err)
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		panic(err)
	}
	err = storeUploadedTorrent(bytes.NewReader(b), me.uploadMetainfoPath(&info))
	if err != nil {
		panic(err)
	}
	if tmpFileErr == nil {
		// Windoze might complain if we don't close the handle before moving the file, plus it's
		// considered good practice to check for close errors after writing to a file. (I'm not
		// closing it, but at least I'm flushing anything, if it's incomplete at this point, the
		// torrent client will complete it as required.
		tmpFile.Close()
		// Move the temporary file, which contains the upload body, to the data directory for the
		// torrent client, in the location it expects.
		err = os.Rename(tmpFile.Name(), filepath.Join(me.DataDir, info.Name))
		if err != nil {
			// Not fatal: See above, we only really need the metainfo to be added to the torrent.
			me.Logger.WithValues(analog.Error).Printf("error renaming file: %v", err)
		}
	}
	tt, err := me.TorrentClient.AddTorrent(mi)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "text/uri-list")
	fmt.Fprintln(w, createLink(tt.InfoHash(), s3KeyFromInfoName(info.Name), name))
}

func (me *HttpHandler) handleUploads(w http.ResponseWriter, r *http.Request) {
	type upload struct {
		Link                  string
		FileName              string
		FileExtensionMimeType string
	}
	resp := []upload{} // Ensure not nil: I don't like 'null' as a response.
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

func (me *HttpHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	link := r.URL.Query().Get("link")
	m, err := metainfo.ParseMagnetURI(link)
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing magnet link: %v", err.Error()), http.StatusBadRequest)
		return
	}
	// For simplicity, let's assume that all uploads are loaded in the local torrent client and have
	// their info, which is true at the point of writing unless the initial upload failed to
	// retrieve the object torrent and the torrent client hasn't already obtained the info on its
	// own.
	t, ok := me.TorrentClient.Torrent(m.InfoHash)
	if !ok {
		http.Error(w, "torrent not in client", http.StatusBadRequest)
		return
	}
	info := t.Info()
	err = replica.DeleteFile(s3KeyFromInfoName(info.Name))
	if err != nil {
		me.Logger.Printf("error deleting s3 object: %v", err)
		http.Error(w, "couldn't delete replica object", http.StatusInternalServerError)
		return
	}
	t.Drop()
	os.Remove(filepath.Join(me.DataDir, info.Name))
	os.Remove(me.uploadMetainfoPath(info))
}

func (me *HttpHandler) handleView(w http.ResponseWriter, r *http.Request) {
	link := r.URL.Query().Get("link")
	m, err := metainfo.ParseMagnetURI(link)
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing magnet link: %v", err.Error()), http.StatusBadRequest)
		return
	}
	s3Key, err := s3KeyFromMagnet(m)
	if err != nil {
		me.Logger.Printf("error getting s3 key from magnet: %v", err)
	} else if s3Key == "" {
		me.Logger.Printf("s3 key not found in view link %q", link)
	}
	t, new, release := me.Confluence.GetTorrent(m.InfoHash)
	defer release()

	// TODO: Perhaps we only want to do this if we're unable to get the metainfo from S3, to avoid
	// bad parties injecting stuff into magnet links and sharing those.
	t.AddTrackers([][]string{m.Trackers})
	if m.DisplayName != "" {
		t.SetDisplayName(m.DisplayName)
	}

	if new && t.Info() == nil && s3Key != "" {
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
				me.Logger.Printf("error putting metainfo from s3: %v", err)
			}
			me.Logger.Printf("added metainfo for %q from s3", s3Key)
		}()
	}
	filename := firstNonEmptyString(
		// Note that serving the torrent implies waiting for the info, and we could get a better
		// name for it after that. Torrent.Name will also allow us to reuse previously given 'dn'
		// values, if we don't have one now.
		displayNameFromInfoName(t.Name()),
		m.DisplayName,
	)
	if filename != "" {
		w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.QueryEscape(filename))
	}

	confluence.ServeTorrent(w, r, t)
}

// What a bad language.
func firstNonEmptyString(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func (me *HttpHandler) uploadsDir() string {
	return me.UploadsDir
}

const UploadDirPerms = 0750

// r is over the metainfo bytes.
func storeUploadedTorrent(r io.Reader, path string) error {
	err := os.MkdirAll(filepath.Dir(path), UploadDirPerms)
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
			// This might technically be more correct, but I couldn't find any torrent client that
			// supports it. Make sure to change any assumptions about "xs" before changing it.
			//"xs": {"https://getlantern-replica.s3-ap-southeast-1.amazonaws.com" + s3Key + "?torrent"},
		},
	}.String()
}

// This reverses s3 key to info name change that AWS makes in its ObjectTorrent metainfos.
func s3KeyFromInfoName(name string) string {
	return "/" + strings.Replace(name, "_", "/", 1)
}

// Retrieve the original, user or file-system provided file name, before changes made by AWS.
func displayNameFromInfoName(name string) string {
	ss := strings.SplitN(name, "_", 2)
	if len(ss) > 1 {
		return ss[1]
	}
	return ss[0]
}

// See createLink.
func s3KeyFromMagnet(m metainfo.Magnet) (string, error) {
	u, err := url.Parse(m.Params.Get("xs"))
	if err != nil {
		return "", err
	}
	return u.Path, nil
}

func (me *HttpHandler) uploadMetainfoPath(info *metainfo.Info) string {
	return filepath.Join(me.uploadsDir(), info.Name+".torrent")
}
