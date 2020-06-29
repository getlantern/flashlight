package desktopReplica

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
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/replica"
)

type HttpHandler struct {
	// Used to handle non-Replica specific routes. (Some of the hard work has been done!). This will
	// probably go away soon, as I pick out the parts we actually need.
	confluence    confluence.Handler
	torrentClient *torrent.Client
	// Where to store torrent client data.
	dataDir       string
	uploadsDir    string
	logger        analog.Logger
	mux           http.ServeMux
	replicaClient *replica.Client
}

// NewHTTPHandler creates a new http.Handler for calls to replica.
func NewHTTPHandler(uc common.UserConfig, replicaClient *replica.Client) (_ *HttpHandler, exitFunc func(), err error) {
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
	cfg.HeaderObfuscationPolicy.Preferred = true
	cfg.HeaderObfuscationPolicy.RequirePreferred = true
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

	handler := &HttpHandler{
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
		replicaClient: replicaClient,
	}
	handler.mux.Handle("/search", http.StripPrefix("/search", searchHandler(uc)))
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

	if err := handler.replicaClient.IterUploads(uploadsDir, func(iu replica.IteredUpload) {
		if iu.Err != nil {
			replicaLogger.Printf("error while iterating uploads: %v", iu.Err)
			return
		}
		err := handler.addTorrent(iu.Metainfo.MetaInfo, true)
		if err != nil {
			replicaLogger.WithValues(analog.Error).Printf("error adding existing upload from %q to torrent client: %v", iu.FileInfo.Name(), err)
		} else {
			replicaLogger.Printf("added previous upload %q to torrent client from file %q", iu.Metainfo.Upload, iu.FileInfo.Name())
		}
	}); err != nil {
		panic(err)
	}

	return handler, torrentClient.Close, nil
}

func (me *HttpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	me.logger.WithValues(analog.Debug).Printf("replica server request path: %q", r.URL.Path)
	// TODO(anacrolix): Check this is correct and secure. We might want to be given valid origins
	// and apply them to appropriate routes, or only allow anything from localhost for example.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	me.mux.ServeHTTP(w, r)
}

func (me *HttpHandler) handleUpload(rw http.ResponseWriter, r *http.Request) {
	// Set status code to 204 to handle preflight cors check
	if r.Method == "OPTIONS" {
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != "POST" {
		http.Error(rw, "expected POST", http.StatusMethodNotAllowed)
		return
	}
	w := ops.InitInstrumentedResponseWriter(rw, "replica_upload")
	defer w.Finish()

	var cw replica.CountWriter
	replicaUploadReader := io.TeeReader(r.Body, &cw)

	tmpFile, tmpFileErr := func() (*os.File, error) {
		const forceTempFileFailure = false
		if forceTempFileFailure {
			return nil, errors.New("sike")
		}
		return ioutil.TempFile("", "")
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

	output, err := me.replicaClient.Upload(replicaUploadReader, r.URL.Query().Get("name"))
	s3Prefix := output.Upload
	me.logger.WithValues(analog.Debug).Printf("uploaded replica key %q", s3Prefix)
	w.Op.Set("upload_s3_key", s3Prefix)
	me.logger.WithValues(analog.Debug).Printf("uploaded %d bytes", cw.BytesWritten)
	if err != nil {
		panic(err)
	}
	info := output.Info
	var metainfoBytes bytes.Buffer
	err = output.MetaInfo.Write(&metainfoBytes)
	if err != nil {
		panic(err)
	}
	err = storeUploadedTorrent(&metainfoBytes, me.uploadMetainfoPath(s3Prefix))
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
		dst := filepath.Join(append([]string{me.dataDir, s3Prefix.String()}, info.UpvertedFiles()[0].Path...)...)
		os.MkdirAll(filepath.Dir(dst), 0700)
		err = os.Rename(tmpFile.Name(), dst)
		if err != nil {
			// Not fatal: See above, we only really need the metainfo to be added to the torrent.
			me.logger.WithValues(analog.Error).Printf("error renaming file: %v", err)
		}
	}
	err = me.addTorrent(output.MetaInfo, true)
	if err != nil {
		panic(err)
	}
	var oi objectInfo
	err = oi.fromS3UploadMetaInfo(output.UploadMetainfo, time.Now())
	if err != nil {
		panic(err)
	}
	encodeJsonResponse(w, oi)
}

func (me *HttpHandler) addTorrent(mi *metainfo.MetaInfo, concealUploaderIdentity bool) error {
	t, err := me.torrentClient.AddTorrent(mi)
	if err != nil {
		return err
	}
	if concealUploaderIdentity {
		t.DisallowDataUpload()
	} else {
		me.addImplicitTrackers(t)
	}
	return nil
}

func (me *HttpHandler) handleUploads(w http.ResponseWriter, r *http.Request) {
	resp := []objectInfo{} // Ensure not nil: I don't like 'null' as a response.
	err := me.replicaClient.IterUploads(me.uploadsDir, func(iu replica.IteredUpload) {
		mi := iu.Metainfo
		err := iu.Err
		if err != nil {
			me.logger.Printf("error iterating uploads: %v", err)
			return
		}
		var oi objectInfo
		err = oi.fromS3UploadMetaInfo(mi, iu.FileInfo.ModTime())
		if err != nil {
			me.logger.Printf("error parsing upload metainfo for %q: %v", iu.FileInfo.Name(), err)
			return
		}
		resp = append(resp, oi)
	})
	if err != nil {
		me.logger.Printf("error walking uploads dir: %v", err)
	}
	encodeJsonResponse(w, resp)
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
	t, ok := me.torrentClient.Torrent(m.InfoHash)
	if !ok {
		http.Error(w, "torrent not in client", http.StatusBadRequest)
		return
	}
	var s3Prefix replica.Upload
	err = s3Prefix.FromMagnet(m)
	if err != nil {
		me.logger.Printf("error getting s3 prefix from magnet link %q: %v", m, err)
		http.Error(w, fmt.Sprintf("error parsing replica uri: %v", err.Error()), http.StatusBadRequest)
		return
	}

	if errs := me.replicaClient.DeleteUpload(s3Prefix, func() (ret [][]string) {
		for _, f := range t.Info().Files {
			ret = append(ret, f.Path)
		}
		return
	}()...); len(errs) != 0 {
		for _, e := range errs {
			me.logger.Printf("error deleting prefix %q: %v", s3Prefix, e)
		}
		http.Error(w, "couldn't delete replica prefix", http.StatusInternalServerError)
		return
	}
	t.Drop()
	os.RemoveAll(filepath.Join(me.dataDir, s3Prefix.String()))
	os.Remove(me.uploadMetainfoPath(s3Prefix))
}

func (me *HttpHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	me.handleViewWith(w, r, "attachment")
}

func (me *HttpHandler) handleView(w http.ResponseWriter, r *http.Request) {
	me.handleViewWith(w, r, "inline")
}

func (me *HttpHandler) handleViewWith(rw http.ResponseWriter, r *http.Request, inlineType string) {
	w := ops.InitInstrumentedResponseWriter(rw, "replica_view")
	defer w.Finish()

	w.Op.Set("inline_type", inlineType)

	link := r.URL.Query().Get("link")

	m, err := metainfo.ParseMagnetURI(link)
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing magnet link: %v", err.Error()), http.StatusBadRequest)
		return
	}

	w.Op.Set("info_hash", m.InfoHash)

	var s3Prefix replica.Upload
	unwrapUploadSpecErr := s3Prefix.FromMagnet(m)
	if unwrapUploadSpecErr != nil {
		me.logger.Printf("error getting s3 key from magnet: %v", err)
	}

	t, _, release := me.confluence.GetTorrent(m.InfoHash)
	defer release()

	// The trackers for a torrent should overlap with those used by replica-peer. Also
	// replica-search might provide this dynamically via the magnet links (except that uploaders
	// will need a way to use the same list). As an ad-hoc practice, we'll use trackers provided via
	// the magnet link in the first tier, and our forcibly injected ones in the second.
	if err := t.MergeSpec(func() *torrent.TorrentSpec {
		// We're unnecessary parsing a Magnet here.
		spec, err := torrent.TorrentSpecFromMagnetURI(link)
		if err != nil {
			panic(err)
		}
		// Override the use of "xs", as this is replica-specific here.
		spec.Sources = m.Params["as"]
		return spec
	}()); err != nil {
		panic(err)
	}
	me.addImplicitTrackers(t)

	if t.Info() == nil && unwrapUploadSpecErr == nil {
		// Get another reference to the torrent that lasts until we're done fetching the metainfo.
		_, _, release := me.confluence.GetTorrent(m.InfoHash)
		go func() {
			defer release()
			tob, err := me.replicaClient.GetMetainfo(s3Prefix)
			if err != nil {
				me.logger.Printf("error getting metainfo for %q from s3 API: %v", s3Prefix, err)
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
			me.logger.Printf("added metainfo for %q from s3 API", s3Prefix)
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
		t.Name(),
	)
	if filename != "" {
		ext := path.Ext(filename)
		if ext != "" {
			filename = sanitize.BaseName(strings.TrimSuffix(filename, ext)) + ext
		}
		w.Header().Set("Content-Disposition", inlineType+"; filename*=UTF-8''"+url.QueryEscape(filename))
	}

	w.Op.Set("download_filename", filename)
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

func (me *HttpHandler) uploadMetainfoPath(s3Prefix replica.Upload) string {
	return filepath.Join(me.uploadsDir, s3Prefix.String()+".torrent")
}

func encodeJsonResponse(w http.ResponseWriter, resp interface{}) {
	w.Header().Set("Content-Type", "application/json")
	je := json.NewEncoder(w)
	je.SetIndent("", "  ")
	je.SetEscapeHTML(false)
	err := je.Encode(resp)
	if err != nil {
		panic(err)
	}
}

func (me *HttpHandler) addImplicitTrackers(t *torrent.Torrent) {
	t.AddTrackers([][]string{
		nil,
		replica.Trackers(),
	})
}
