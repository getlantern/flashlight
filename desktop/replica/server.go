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

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/golog"
	"github.com/getsentry/sentry-go"
	"github.com/kennygrant/sanitize"

	"github.com/anacrolix/confluence/confluence"
	analog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/getlantern/flashlight/common"
	metascrubber "github.com/getlantern/meta-scrubber"
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
	logger        golog.Logger
	mux           http.ServeMux
	replicaClient *replica.Client
	searchProxy   http.Handler
	gaSession     analytics.Session
}

// NewHTTPHandler creates a new http.Handler for calls to replica.
func NewHTTPHandler(configDir string, uc common.UserConfig, replicaClient *replica.Client, gaSession analytics.Session) (_ *HttpHandler, exitFunc func(), err error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		err = fmt.Errorf("accessing the user cache dir: %w", err)
		return
	}

	logger := golog.LoggerFor("replica.server")
	const replicaDirElem = "replica"
	replicaConfigDir := common.InConfigDir(configDir, replicaDirElem)
	uploadsDir := filepath.Join(replicaConfigDir, "uploads")
	replicaDataDir := filepath.Join(userCacheDir, replicaDirElem, "data")
	cfg := torrent.NewDefaultClientConfig()
	cfg.DisableIPv6 = true
	cfg.DataDir = replicaDataDir
	cfg.Seed = true
	cfg.HeaderObfuscationPolicy.Preferred = true
	cfg.HeaderObfuscationPolicy.RequirePreferred = true
	//cfg.Debug = true
	cfg.Logger = analog.Default.WithFilter(func(m analog.Msg) bool {
		return !m.HasValue("upnp-discover")
	})
	// DHT disabled pending fixes to transaction query concurrency.	 Also using S3 as a
	// fallback means we have a great tracker to rely on.
	//cfg.NoDHT = true
	// Household users may be behind a NAT/bad router, or on a limited device like a mobile. We
	// don't want to overload their networks, so ensure the default connection tracking
	// behaviour.
	//cfg.ConnTracker = conntrack.NewInstance()
	torrentClient, err := torrent.NewClient(cfg)
	if err != nil {
		logger.Errorf("Error creating client: %v", err)
		if torrentClient != nil {
			torrentClient.Close()
		}
		// Try an ephemeral port in case there was an error binding.
		cfg.ListenPort = 0
		torrentClient, err = torrent.NewClient(cfg)
		if err != nil {
			err = fmt.Errorf("starting torrent client: %w", err)
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
		logger:        logger,
		uploadsDir:    uploadsDir,
		replicaClient: replicaClient,
		searchProxy:   http.StripPrefix("/search", searchHandler(uc)),
		gaSession:     gaSession,
	}
	handler.mux.HandleFunc("/search", handler.wrapHandlerError("replica_search", handler.handleSearch))
	handler.mux.HandleFunc("/upload", handler.wrapHandlerError("replica_upload", handler.handleUpload))
	handler.mux.HandleFunc("/uploads", handler.wrapHandlerError("replica_uploads", handler.handleUploads))
	handler.mux.HandleFunc("/view", handler.wrapHandlerError("replica_view", handler.handleView))
	handler.mux.HandleFunc("/download", handler.wrapHandlerError("replica_view", handler.handleDownload))
	handler.mux.HandleFunc("/delete", handler.wrapHandlerError("replica_delete", handler.handleDelete))
	handler.mux.HandleFunc("/debug/dht", func(w http.ResponseWriter, r *http.Request) {
		for _, ds := range torrentClient.DhtServers() {
			ds.WriteStatus(w)
		}
	})
	// Helps debug connection tracking, for best configuring DHT and other limits.
	handler.mux.HandleFunc("/debug/conntrack", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		cfg.ConnTracker.PrintStatus(w)
	})
	// TODO(anacrolix): Actually not much of Confluence is used now, probably none of the
	// routes, so this might go away soon.
	handler.mux.Handle("/", &handler.confluence)

	if err := handler.replicaClient.IterUploads(uploadsDir, func(iu replica.IteredUpload) {
		if iu.Err != nil {
			logger.Errorf("error while iterating uploads: %v", iu.Err)
			return
		}
		err := handler.addTorrent(iu.Metainfo.MetaInfo, true)
		if err != nil {
			logger.Errorf("error adding existing upload from %q to torrent client: %v", iu.FileInfo.Name(), err)
		} else {
			logger.Debugf("added previous upload %q to torrent client from file %q", iu.Metainfo.Upload, iu.FileInfo.Name())
		}
	}); err != nil {
		return nil, nil, fmt.Errorf("iterating through uploads: %w", err)
	}

	return handler, torrentClient.Close, nil
}

func (me *HttpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	me.logger.Debugf("replica server request path: %q", r.URL.Path)
	// TODO(anacrolix): Check this is correct and secure. We might want to be given valid origins
	// and apply them to appropriate routes, or only allow anything from localhost for example.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	me.mux.ServeHTTP(w, r)
}

// handlerError is just a small wrapper around errors so that we can more easily
// return them from handlers and then inspect them in our handler wrapper
type handlerError struct {
	statusCode int
	error
}

// encoderError is a small wrapper around error so we know that this is an error
// during encoding/writing a response and we can avoid trying to re-write headers
type encoderWriterError struct {
	error
}

func (me *HttpHandler) wrapHandlerError(opName string, handler func(*ops.InstrumentedResponseWriter, *http.Request) error) func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		w := ops.InitInstrumentedResponseWriter(rw, opName)
		defer w.Finish()
		if err := handler(w, r); err != nil {
			me.logger.Errorf("in replica handler: %v", err)
			w.Op.FailIf(err)

			// we may want to only ultimately only report server errors here (ie >=500)
			// but since we're also the client I think this makes some sense for now
			sentry.ConfigureScope(func(scope *sentry.Scope) {
				scope.SetLevel(sentry.LevelError)
			})
			sentry.CaptureException(err)

			// if it's an error during encoding+writing, don't attempt to
			// write new headers and body
			if e, ok := err.(encoderWriterError); ok {
				me.logger.Errorf("error writing json: %v", e)
				return
			}

			var statusCode int
			if e, ok := err.(handlerError); ok {
				statusCode = e.statusCode
			} else {
				statusCode = http.StatusInternalServerError
			}

			resp := map[string]interface{}{
				"statusCode": statusCode,
				"error":      err.Error(),
			}

			var writingEncodingErr error
			writingEncodingErr = encodeJsonErrorResponse(rw, resp, statusCode)

			if writingEncodingErr != nil {
				me.logger.Errorf("error writing json error response: %v", writingEncodingErr)
			}
		}
	}
}

func (me *HttpHandler) handleUpload(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
	// Set status code to 204 to handle preflight cors check
	if r.Method == "OPTIONS" {
		rw.WriteHeader(http.StatusNoContent)
		return nil
	}

	if r.Method != "POST" {
		return handlerError{http.StatusMethodNotAllowed, fmt.Errorf("expected POST")}
	}

	scrubbedReader, err := metascrubber.GetScrubber(r.Body)
	if err != nil {
		return fmt.Errorf("getting metascrubber: %w", err)
	}

	var cw replica.CountWriter
	replicaUploadReader := io.TeeReader(scrubbedReader, &cw)

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
		me.logger.Errorf("error creating temporary file: %v", tmpFileErr)
	}

	output, err := me.replicaClient.Upload(replicaUploadReader, r.URL.Query().Get("name"))
	s3Prefix := output.Upload
	me.logger.Debugf("uploaded replica key %q", s3Prefix)
	rw.Op.Set("upload_s3_key", s3Prefix)
	me.gaSession.Event("replica", "upload")
	me.logger.Debugf("uploaded %d bytes", cw.BytesWritten)
	if err != nil {
		return fmt.Errorf("uploading with replicaClient: %w", err)
	}
	info := output.Info
	var metainfoBytes bytes.Buffer
	err = output.MetaInfo.Write(&metainfoBytes)
	if err != nil {
		return fmt.Errorf("writing metainfo: %w", err)
	}
	err = storeUploadedTorrent(&metainfoBytes, me.uploadMetainfoPath(s3Prefix))
	if err != nil {
		return fmt.Errorf("storing uploaded torrent: %w", err)
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
			me.logger.Errorf("error renaming file: %v", err)
		}
	}
	err = me.addTorrent(output.MetaInfo, true)
	if err != nil {
		return fmt.Errorf("adding torrent: %w", err)
	}
	var oi objectInfo
	err = oi.fromS3UploadMetaInfo(output.UploadMetainfo, time.Now())
	if err != nil {
		return fmt.Errorf("getting objectInfo from s3 upload metainfo: %w", err)
	}
	return encodeJsonResponse(rw, oi)
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

func (me *HttpHandler) handleUploads(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
	resp := []objectInfo{} // Ensure not nil: I don't like 'null' as a response.
	err := me.replicaClient.IterUploads(me.uploadsDir, func(iu replica.IteredUpload) {
		mi := iu.Metainfo
		err := iu.Err
		if err != nil {
			me.logger.Errorf("error iterating uploads: %v", err)
			return
		}
		var oi objectInfo
		err = oi.fromS3UploadMetaInfo(mi, iu.FileInfo.ModTime())
		if err != nil {
			me.logger.Errorf("error parsing upload metainfo for %q: %v", iu.FileInfo.Name(), err)
			return
		}
		resp = append(resp, oi)
	})
	if err != nil {
		me.logger.Errorf("error walking uploads dir: %v", err)
	}
	return encodeJsonResponse(rw, resp)
}

func (me *HttpHandler) handleDelete(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
	link := r.URL.Query().Get("link")
	m, err := metainfo.ParseMagnetURI(link)
	if err != nil {
		return handlerError{http.StatusBadRequest, fmt.Errorf("parsing magnet link: %v", err)}
	}
	// For simplicity, let's assume that all uploads are loaded in the local torrent client and have
	// their info, which is true at the point of writing unless the initial upload failed to
	// retrieve the object torrent and the torrent client hasn't already obtained the info on its
	// own.
	t, ok := me.torrentClient.Torrent(m.InfoHash)
	if !ok {
		return handlerError{http.StatusBadRequest, fmt.Errorf("torrent not in client")}
	}
	var s3Prefix replica.Upload
	err = s3Prefix.FromMagnet(m)
	if err != nil {
		me.logger.Errorf("error getting s3 prefix from magnet link %q: %v", m, err)
		return handlerError{http.StatusBadRequest, fmt.Errorf("parsing replica uri: %w", err)}
	}

	if errs := me.replicaClient.DeleteUpload(s3Prefix, func() (ret [][]string) {
		for _, f := range t.Info().Files {
			ret = append(ret, f.Path)
		}
		return
	}()...); len(errs) != 0 {
		for _, e := range errs {
			me.logger.Errorf("error deleting prefix %q: %v", s3Prefix, e)
		}

		return fmt.Errorf("couldn't delete replica prefix")
	}
	t.Drop()
	os.RemoveAll(filepath.Join(me.dataDir, s3Prefix.String()))
	os.Remove(me.uploadMetainfoPath(s3Prefix))
	return nil
}

func (me *HttpHandler) handleSearch(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
	searchTerm := r.URL.Query().Get("s")

	rw.Op.Set("search_term", searchTerm)
	me.gaSession.EventWithLabel("replica", "search", searchTerm)

	me.searchProxy.ServeHTTP(rw, r)
	return nil
}

func (me *HttpHandler) handleDownload(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
	return me.handleViewWith(rw, r, "attachment")
}

func (me *HttpHandler) handleView(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
	return me.handleViewWith(rw, r, "inline")
}

func (me *HttpHandler) handleViewWith(rw *ops.InstrumentedResponseWriter, r *http.Request, inlineType string) error {
	rw.Op.Set("inline_type", inlineType)

	link := r.URL.Query().Get("link")

	m, err := metainfo.ParseMagnetURI(link)
	if err != nil {
		return handlerError{http.StatusBadRequest, fmt.Errorf("parsing magnet link: %w", err)}
	}

	rw.Op.Set("info_hash", m.InfoHash)

	var s3Prefix replica.Upload
	unwrapUploadSpecErr := s3Prefix.FromMagnet(m)
	if unwrapUploadSpecErr != nil {
		me.logger.Errorf("error getting s3 key from magnet: %v", unwrapUploadSpecErr)
	}

	t, _, release := me.confluence.GetTorrent(m.InfoHash)
	defer release()

	// The trackers for a torrent should overlap with those used by replica-peer. Also
	// replica-search might provide this dynamically via the magnet links (except that uploaders
	// will need a way to use the same list). As an ad-hoc practice, we'll use trackers provided via
	// the magnet link in the first tier, and our forcibly injected ones in the second.

	// We're unnecessary parsing a Magnet here.
	spec, err := torrent.TorrentSpecFromMagnetURI(link)
	if err != nil {
		return fmt.Errorf("getting spec from magnet URI: %w", err)
	}
	// Override the use of "xs", as this is replica-specific here.
	spec.Sources = m.Params["as"]
	if err := t.MergeSpec(spec); err != nil {
		return fmt.Errorf("merging spec: %w", err)
	}

	me.addImplicitTrackers(t)

	if t.Info() == nil && unwrapUploadSpecErr == nil {
		// Get another reference to the torrent that lasts until we're done fetching the metainfo.
		_, _, release := me.confluence.GetTorrent(m.InfoHash)
		go func() {
			defer release()
			tob, err := me.replicaClient.GetMetainfo(s3Prefix)
			if err != nil {
				me.logger.Errorf("error getting metainfo for %q from s3 API: %v", s3Prefix, err)
				return
			}
			defer tob.Close()
			mi, err := metainfo.Load(tob)
			if err != nil {
				me.logger.Errorf("error loading metainfo: %v", err)
				return
			}
			err = me.confluence.PutMetainfo(t, mi)
			if err != nil {
				me.logger.Errorf("error putting metainfo from s3: %v", err)
			}
			me.logger.Debugf("added metainfo for %q from s3 API", s3Prefix)
		}()
	}

	selectOnly, err := strconv.ParseUint(m.Params.Get("so"), 10, 0)
	// Assume that it should be present, as it'll be added going forward where possible. When it's
	// missing, zero is a perfectly adequate default for now.
	if err != nil {
		me.logger.Errorf("error parsing so field: %v", err)
	}
	select {
	case <-r.Context().Done():
		return nil
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
		rw.Header().Set("Content-Disposition", inlineType+"; filename*=UTF-8''"+url.QueryEscape(filename))
	}

	rw.Op.Set("download_filename", filename)
	me.gaSession.EventWithLabel("replica", "view", filename)

	torrentFile := t.Files()[selectOnly]
	fileReader := torrentFile.NewReader()
	confluence.ServeTorrentReader(rw, r, fileReader, torrentFile.Path())
	return nil
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

func encodeJsonResponse(rw http.ResponseWriter, resp interface{}) error {
	rw.Header().Set("Content-Type", "application/json")
	je := json.NewEncoder(rw)
	je.SetIndent("", "  ")
	je.SetEscapeHTML(false)
	if err := je.Encode(resp); err != nil {
		return encoderWriterError{err}
	}
	return nil
}

func encodeJsonErrorResponse(rw http.ResponseWriter, resp interface{}, statusCode int) error {
	// necessary here because of header writing ordering requirements
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(statusCode)
	return encodeJsonResponse(rw, resp)
}

func (me *HttpHandler) addImplicitTrackers(t *torrent.Torrent) {
	t.AddTrackers([][]string{
		nil,
		replica.Trackers(),
	})
}
