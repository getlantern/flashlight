package replica

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/flashlight/ops"
	"github.com/kennygrant/sanitize"
	"golang.org/x/xerrors"

	"github.com/anacrolix/confluence/confluence"
	analog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/getlantern/replica"
)

type httpHandler struct {
	// Used to handle non-Replica specific routes. (Some of the hard work has been done!). This will
	// probably go away soon, as I pick out the parts we actually need.
	confluence    confluence.Handler
	torrentClient *torrent.Client
	// Where to store torrent client data.
	dataDir    string
	uploadsDir string
	logger     analog.Logger
	mux        http.ServeMux
}

// NewHTTPHandler creates a new http.Handler for calls to replica.
func NewHTTPHandler() (_ http.Handler, exitFunc func(), err error) {
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
	cfg.DisableIPv6 = true
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
		replicaLogger.Printf("Error creating client: %v", err)
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

	if err := replica.IterUploads(uploadsDir, func(iu replica.IteredUpload) {
		if iu.Err != nil {
			replicaLogger.Printf("error while iterating uploads: %v", iu.Err)
			return
		}
		t, err := torrentClient.AddTorrent(iu.MetaInfo)
		if err != nil {
			replicaLogger.WithValues(analog.Error).Printf("error adding existing upload to torrent client: %v", err)
		} else {
			replicaLogger.Printf("added previous upload %q to torrent client", t.Name())
		}
	}); err != nil {
		panic(err)
	}

	handler := &httpHandler{
		confluence: confluence.Handler{
			TC: torrentClient,
			MetainfoCacheDir: func() *string {
				s := filepath.Join(userCacheDir, replicaDirElem, "metainfos")
				return &s
			}(),
		},
		torrentClient: torrentClient,
		dataDir:       replicaDataDir,
		logger:        replicaLogger,
		uploadsDir:    uploadsDir,
	}
	handler.mux.HandleFunc("/upload", handler.handleUpload)
	handler.mux.HandleFunc("/uploads", handler.handleUploads)
	handler.mux.HandleFunc("/view", handler.handleView)
	handler.mux.HandleFunc("/download", handler.handleDownload)
	handler.mux.HandleFunc("/delete", handler.handleDelete)
	handler.mux.HandleFunc("/debug/dht", func(w http.ResponseWriter, r *http.Request) {
		for _, ds := range torrentClient.DhtServers() {
			ds.WriteStatus(w)
		}
	})
	// TODO(anacrolix): Actually not much of Confluence is used now, probably none of the
	// routes, so this might go away soon.
	handler.mux.Handle("/", &handler.confluence)

	return handler, torrentClient.Close, nil
}

func (me *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	me.logger.WithValues(analog.Debug).Printf("replica server request path: %q", r.URL.Path)
	// TODO(anacrolix): Check this is correct and secure. We might want to be given valid origins
	// and apply them to appropriate routes, or only allow anything from localhost for example.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	me.mux.ServeHTTP(w, r)
}

func (me *httpHandler) handleUpload(rw http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	w := ops.InitInstrumentedResponseWriter(rw, "replica_upload")
	defer w.Finish()

	name := r.URL.Query().Get("name")
	s3Key := replica.NewPrefix()
	if name != "" {
		s3Key += "/" + name
	}
	me.logger.WithValues(analog.Debug).Printf("uploading replica key %q", s3Key)
	var cw countWriter
	replicaUploadReader := io.TeeReader(r.Body, &cw)
	tmpFile, tmpFileErr := func() (*os.File, error) {
		if true {
			return ioutil.TempFile("", "")
		}
		return nil, errors.New("sike")
	}()
	if tmpFileErr == nil {
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()
		replicaUploadReader = io.TeeReader(replicaUploadReader, tmpFile)
	} else {
		// This isn't good, but as long as we can add the torrent file metainfo to the local client,
		// we can still spread the metadata, and S3 can take care of the data.
		me.logger.WithValues(analog.Error).Printf("error creating temporary file: %v", tmpFileErr)
	}
	err := replica.Upload(replicaUploadReader, s3Key)
	me.logger.WithValues(analog.Debug).Printf("uploaded %d bytes", cw.bytesWritten)
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
		err = os.Rename(tmpFile.Name(), filepath.Join(me.dataDir, info.Name))
		if err != nil {
			// Not fatal: See above, we only really need the metainfo to be added to the torrent.
			me.logger.WithValues(analog.Error).Printf("error renaming file: %v", err)
		}
	}
	_, err = me.torrentClient.AddTorrent(mi)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	je := json.NewEncoder(w)
	je.SetIndent("", "  ")
	var oi objectInfo
	oi.fromS3UploadMetaInfo(mi, time.Now())
	je.Encode(oi)
}

func (me *httpHandler) handleUploads(w http.ResponseWriter, r *http.Request) {
	resp := []objectInfo{} // Ensure not nil: I don't like 'null' as a response.
	err := replica.IterUploads(me.uploadsDir, func(iu replica.IteredUpload) {
		mi := iu.MetaInfo
		err := iu.Err
		if err != nil {
			me.logger.Printf("error iterating uploads: %v", err)
			return
		}
		var oi objectInfo
		oi.fromS3UploadMetaInfo(mi, iu.FileInfo.ModTime())
		resp = append(resp, oi)
	})
	if err != nil {
		me.logger.Printf("error walking uploads dir: %v", err)
	}
	b, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "%s\n", b)
}

func (me *httpHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
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
	t, ok := me.torrentClient.Torrent(m.InfoHash)
	if !ok {
		http.Error(w, "torrent not in client", http.StatusBadRequest)
		return
	}
	info := t.Info()
	err = replica.DeleteFile(s3KeyFromInfoName(info.Name))
	if err != nil {
		me.logger.Printf("error deleting s3 object: %v", err)
		http.Error(w, "couldn't delete replica object", http.StatusInternalServerError)
		return
	}
	t.Drop()
	os.Remove(filepath.Join(me.dataDir, info.Name))
	os.Remove(me.uploadMetainfoPath(info))
}
func (me *httpHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	me.handleViewWith(w, r, "attachment")
}

func (me *httpHandler) handleView(w http.ResponseWriter, r *http.Request) {
	me.handleViewWith(w, r, "inline")
}

func (me *httpHandler) handleViewWith(rw http.ResponseWriter, r *http.Request, inlineType string) {
	w := ops.InitInstrumentedResponseWriter(rw, fmt.Sprintf("replica_view_%s", inlineType))
	defer w.Finish()

	link := r.URL.Query().Get("link")
	w.Op.Set("link", link)
	m, err := metainfo.ParseMagnetURI(link)
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing magnet link: %v", err.Error()), http.StatusBadRequest)
		return
	}
	s3Key, err := s3KeyFromMagnet(m)
	if err != nil {
		me.logger.Printf("error getting s3 key from magnet: %v", err)
	} else if s3Key == "" {
		me.logger.Printf("s3 key not found in view link %q", link)
	}
	t, _, release := me.confluence.GetTorrent(m.InfoHash)
	defer release()

	// TODO: Perhaps we only want to do this if we're unable to get the metainfo from S3, to avoid
	// bad parties injecting stuff into magnet links and sharing those.
	t.AddTrackers([][]string{m.Trackers})
	if m.DisplayName != "" {
		t.SetDisplayName(m.DisplayName)
	}

	if t.Info() == nil && s3Key != "" {
		// Get another reference to the torrent that lasts until we're done fetching the metainfo.
		_, _, release := me.confluence.GetTorrent(m.InfoHash)
		go func() {
			defer release()
			tob, err := replica.GetObjectTorrent(s3Key)
			if err != nil {
				me.logger.Printf("error getting metainfo for %q from s3: %v", s3Key, err)
				return
			}
			defer tob.Close()
			mi, err := metainfo.Load(tob)
			if err != nil {
				me.logger.Print(err)
				return
			}
			err = me.confluence.PutMetainfo(t, mi)
			if err != nil {
				me.logger.Printf("error putting metainfo from s3: %v", err)
			}
			me.logger.Printf("added metainfo for %q from s3", s3Key)
		}()
	}

	selectOnly, err := strconv.ParseUint(m.Params.Get("so"), 10, 0)
	// Assume that it should be present, as it'll be added going forward where possible. When it's
	// missing, zero is a perfectly adequate default for now.
	if err != nil {
		me.logger.Printf("error parsing so field: %v", err)
	}
	select {
	case <-r.Context().Done():
		return
	case <-t.GotInfo():
	}
	filename := firstNonEmptyString(
		// Note that serving the torrent implies waiting for the info, and we could get a better
		// name for it after that. Torrent.Name will also allow us to reuse previously given 'dn'
		// values, if we don't have one now.
		m.DisplayName,
		displayNameFromInfoName(t.Name()),
	)
	if filename != "" {
		ext := path.Ext(filename)
		if ext != "" {
			filename = sanitize.BaseName(strings.TrimSuffix(filename, ext)) + ext
		}
		w.Header().Set("Content-Disposition", inlineType+"; filename*=UTF-8''"+url.QueryEscape(filename))
	}
	torrentFile := t.Files()[selectOnly]
	fileReader := torrentFile.NewReader()
	confluence.ServeTorrentReader(w, r, fileReader, torrentFile.Path())
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

const uploadDirPerms = 0750

// r is over the metainfo bytes.
func storeUploadedTorrent(r io.Reader, path string) error {
	err := os.MkdirAll(filepath.Dir(path), uploadDirPerms)
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

func (me *httpHandler) uploadMetainfoPath(info *metainfo.Info) string {
	return filepath.Join(me.uploadsDir, info.Name+".torrent")
}
