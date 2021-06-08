package desktopReplica

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/torrent/storage"
	sqliteStorage "github.com/anacrolix/torrent/storage/sqlite"
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

var (
	log = golog.LoggerFor("flashlight.desktop.replica")
)

type HttpHandler struct {
	// Used to handle non-Replica specific routes. (Some of the hard work has been done!). This will
	// probably go away soon, as I pick out the parts we actually need.
	confluence    confluence.Handler
	torrentClient *torrent.Client
	// Where to store torrent client data.
	dataDir     string
	uploadsDir  string
	mux         http.ServeMux
	searchProxy http.Handler
	NewHttpHandlerInput
	uploadStorage  storage.ClientImplCloser
	defaultStorage storage.ClientImplCloser
}

type NewHttpHandlerInput struct {
	ConfigDir  string
	UserConfig common.UserConfig
	HttpClient *http.Client
	// For uploads, deletes, and other behaviour serviced by replica-rust using an API in the
	// replica repo.
	ReplicaServiceClient replica.ServiceClient
	GaSession            analytics.Session
	// Doing this might be a privacy concern, since users could be singled out for being the
	// first/only uploader for content.
	AddUploadsToTorrentClient bool
	// Retain a copy of upload file data. This would save downloading our own uploaded content if we
	// intend to seed it.
	StoreUploadsLocally bool

	GlobalConfig *GlobalConfig
}

func (me *NewHttpHandlerInput) SetDefaults() {
	// Why don't we set ConfigDir?

	me.UserConfig = &common.NullUserConfig{}
	me.GaSession = &analytics.NullSession{}
	me.HttpClient = http.DefaultClient
}

// NewHTTPHandler creates a new http.Handler for calls to replica.
func NewHTTPHandler(
	input NewHttpHandlerInput,
) (_ *HttpHandler, err error) {

	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Errorf("accessing the user cache dir, fallback to temp dir: %w", err)
		userCacheDir = os.TempDir()
	}
	const replicaDirElem = "replica"
	replicaCacheDir := filepath.Join(userCacheDir, common.AppName, replicaDirElem)
	_ = os.MkdirAll(replicaCacheDir, 0700)
	uploadsDir := filepath.Join(input.ConfigDir, replicaDirElem, "uploads")
	_ = os.MkdirAll(uploadsDir, 0700)
	replicaDataDir := filepath.Join(replicaCacheDir, "data")
	_ = os.MkdirAll(replicaDataDir, 0700)
	cfg := torrent.NewDefaultClientConfig()
	cfg.DisableIPv6 = true
	// This should not be used, we're specifying our own storage for uploads and general views
	// respectively.
	cfg.DataDir = "\x00"
	cfg.Seed = true
	cfg.HeaderObfuscationPolicy.Preferred = true
	cfg.HeaderObfuscationPolicy.RequirePreferred = true
	//cfg.Debug = true
	cfg.Logger = analog.Default.WithFilter(func(m analog.Msg) bool {
		return !m.HasValue("upnp-discover")
	})
	var opts sqliteStorage.NewDirectStorageOpts
	opts.Path = filepath.Join(replicaCacheDir, "storage-cache.db")
	os.Remove(opts.Path)
	opts.Capacity = 5 << 30
	defaultStorage, err := sqliteStorage.NewDirectStorage(opts)
	if err != nil {
		err = fmt.Errorf("creating torrent storage cache: %w", err)
		return
	}
	defer func() {
		if err != nil {
			defaultStorage.Close()
		}
	}()
	cfg.DefaultStorage = defaultStorage
	cfg.Callbacks.ReceivedUsefulData = append(cfg.Callbacks.ReceivedUsefulData, func(event torrent.ReceivedUsefulDataEvent) {
		op := ops.Begin("replica_torrent_peer_sent_data")
		op.Set("remote_addr", event.Peer.RemoteAddr.String())
		op.Set("remote_network", event.Peer.Network)
		op.SetMetricSum("useful_bytes_count", float64(len(event.Message.Piece)))
		op.End()
		log.Tracef("reported %v bytes from %v over %v",
			len(event.Message.Piece),
			event.Peer.RemoteAddr.String(),
			event.Peer.Network)
	})
	torrentClient, err := torrent.NewClient(cfg)
	if err != nil {
		log.Errorf("Error creating client: %v", err)
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
				s := filepath.Join(replicaCacheDir, "metainfos")
				return &s
			}(),
		},
		torrentClient: torrentClient,
		dataDir:       replicaDataDir,
		uploadsDir:    uploadsDir,
		searchProxy: http.StripPrefix("/search", proxyHandler(
			input.UserConfig,
			input.ReplicaServiceClient.ReplicaServiceEndpoint,
			nil)),
		// I think the standard file-storage implementation is sufficient here because we guarantee
		// unique info name/prefixes for uploads (which the default file implementation does not).
		// There's another implementation that injects the infohash as a prefix to ensure uniqueness
		// of final file names.
		uploadStorage:  storage.NewFile(replicaDataDir),
		defaultStorage: defaultStorage,

		NewHttpHandlerInput: input,
	}
	handler.mux.HandleFunc("/search", handler.wrapHandlerError("replica_search", handler.handleSearch))
	handler.mux.HandleFunc("/search/serp_web", handler.wrapHandlerError("replica_search", handler.handleSearch))
	handler.mux.HandleFunc("/thumbnail", handler.wrapHandlerError("replica_thumbnail", handler.handleMetadata("thumbnail")))
	handler.mux.HandleFunc("/duration", handler.wrapHandlerError("replica_duration", handler.handleMetadata("duration")))
	handler.mux.HandleFunc("/upload", handler.wrapHandlerError("replica_upload", handler.handleUpload))
	handler.mux.HandleFunc("/uploads", handler.wrapHandlerError("replica_uploads", handler.handleUploads))
	handler.mux.HandleFunc("/view", handler.wrapHandlerError("replica_view", handler.handleView))
	handler.mux.HandleFunc("/download", handler.wrapHandlerError("replica_view", handler.handleDownload))
	handler.mux.HandleFunc("/delete", handler.wrapHandlerError("replica_delete", handler.handleDelete))
	handler.mux.HandleFunc("/object_info", handler.wrapHandlerError("replica_object_info", handler.handleObjectInfo))
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

	if input.AddUploadsToTorrentClient {
		if err := replica.IterUploads(uploadsDir, func(iu replica.IteredUpload) {
			if iu.Err != nil {
				log.Errorf("error while iterating uploads: %v", iu.Err)
				return
			}
			err := handler.addUploadTorrent(iu.Metainfo.MetaInfo, true)
			if err != nil {
				log.Errorf("error adding existing upload from %q to torrent client: %v", iu.FileInfo.Name(), err)
			} else {
				log.Debugf("added previous upload %q to torrent client from file %q", iu.Metainfo.Upload, iu.FileInfo.Name())
			}
		}); err != nil {
			handler.Close()
			return nil, fmt.Errorf("iterating through uploads: %w", err)
		}
	}
	return handler, nil
}

func (me *HttpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debugf("replica server request path: %q", r.URL.Path)
	common.ProcessCORS(w.Header(), r)
	me.mux.ServeHTTP(w, r)
}

func (me *HttpHandler) Close() {
	me.torrentClient.Close()
	me.uploadStorage.Close()
	me.defaultStorage.Close()
}

// handlerError is just a small wrapper around errors so that we can more easily return them from
// handlers and then inspect them in our handler wrapper
type handlerError struct {
	statusCode int
	error
}

// encoderError is a small wrapper around error so we know that this is an error
// during encoding/writing a response and we can avoid trying to re-write headers
type encoderWriterError struct {
	error
}

func (me *HttpHandler) wrapHandlerError(
	opName string,
	handler func(*ops.InstrumentedResponseWriter, *http.Request) error,
) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		w := ops.InitInstrumentedResponseWriter(rw, opName)
		defer w.Finish()
		if err := handler(w, r); err != nil {
			log.Errorf("in %q handler: %v", opName, err)
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
				log.Errorf("error writing json: %v", e)
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
				log.Errorf("error writing json error response: %v", writingEncodingErr)
			}
		}
	}
}

func (me *HttpHandler) writeNewUploadAuthTokenFile(auth string, prefix replica.Prefix) error {
	tokenFilePath := me.uploadTokenPath(prefix)
	f, err := os.OpenFile(
		tokenFilePath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_TRUNC,
		0440,
	)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(auth)
	if err != nil {
		return err
	}
	log.Debugf("wrote %q to %q", auth, tokenFilePath)
	return f.Close()
}

func (me *HttpHandler) handleUpload(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
	// Set status code to 204 to handle preflight CORS check
	if r.Method == "OPTIONS" {
		rw.WriteHeader(http.StatusNoContent)
		return nil
	}

	switch r.Method {
	default:
		return handlerError{
			http.StatusMethodNotAllowed,
			fmt.Errorf("expected method supporting request body"),
		}
	case http.MethodPost, http.MethodPut:
		// Maybe we can differentiate form handling based on the method.
	}

	fileName := r.URL.Query().Get("name")
	var fileReader io.Reader
	{
		// There are streaming ways and helpers for temporary files for this if size becomes an issue.
		formFile, fileHeader, err := r.FormFile("file")
		if err == nil {
			fileReader = formFile
			if fileName == "" {
				fileName = fileHeader.Filename
			}
		} else {
			log.Debugf("error getting upload file as form file: %v", err)
			fileReader = r.Body
		}
	}

	scrubbedReader, err := metascrubber.GetScrubber(fileReader)
	if err != nil {
		return fmt.Errorf("getting metascrubber: %w", err)
	}

	var cw replica.CountWriter
	replicaUploadReader := io.TeeReader(scrubbedReader, &cw)

	var (
		tmpFile    *os.File
		tmpFileErr error
	)
	if me.StoreUploadsLocally {
		tmpFile, tmpFileErr = func() (*os.File, error) {
			// This is for testing temp file failures.
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
			// This isn't good, but as long as we can add the torrent file metainfo to the local
			// client, we can still spread the metadata, and S3 can take care of the data.
			log.Errorf("error creating temporary file: %v", tmpFileErr)
		}
	}

	output, err := me.ReplicaServiceClient.Upload(replicaUploadReader, fileName)
	me.GaSession.EventWithLabel("replica", "upload", path.Ext(fileName))
	log.Debugf("uploaded %d bytes", cw.BytesWritten)
	if err != nil {
		return fmt.Errorf("uploading with replica client: %w", err)
	}
	upload := output.Upload
	log.Debugf("uploaded replica key %q", upload)
	rw.Op.Set("upload_s3_key", upload.PrefixString())

	{
		var metainfoBytes bytes.Buffer
		err = output.MetaInfo.Write(&metainfoBytes)
		if err != nil {
			return fmt.Errorf("writing metainfo: %w", err)
		}
		err = storeUploadedTorrent(&metainfoBytes, me.uploadMetainfoPath(upload))
		if err != nil {
			return fmt.Errorf("storing uploaded torrent: %w", err)
		}
	}
	if err := me.writeNewUploadAuthTokenFile(*output.AuthToken, upload.Prefix); err != nil {
		log.Errorf("error writing upload auth token file: %v", err)
	}

	if tmpFileErr == nil && me.StoreUploadsLocally {
		// Windoze might complain if we don't close the handle before moving the file, plus it's
		// considered good practice to check for close errors after writing to a file. (I'm not
		// closing it, but at least I'm flushing anything, if it's incomplete at this point, the
		// torrent client will complete it as required.
		tmpFile.Close()
		// Move the temporary file, which contains the upload body, to the data directory for the
		// torrent client, in the location it expects.
		dst := filepath.Join(append([]string{me.dataDir, upload.String()}, output.Info.UpvertedFiles()[0].Path...)...)
		_ = os.MkdirAll(filepath.Dir(dst), 0700)
		err = os.Rename(tmpFile.Name(), dst)
		if err != nil {
			// Not fatal: See above, we only really need the metainfo to be added to the torrent.
			log.Errorf("error renaming file: %v", err)
		}
	}
	if me.AddUploadsToTorrentClient {
		err = me.addUploadTorrent(output.MetaInfo, true)
		if err != nil {
			return fmt.Errorf("adding torrent: %w", err)
		}
	}
	var oi objectInfo
	err = oi.FromUploadMetainfo(output.UploadMetainfo, time.Now())
	if err != nil {
		return fmt.Errorf("getting objectInfo from upload metainfo: %w", err)
	}
	// We can clobber with what should be a superior link directly from the upload service endpoint.
	if output.Link != nil {
		oi.Link = *output.Link
	}
	return encodeJsonResponse(rw, oi)
}

func (me *HttpHandler) addUploadTorrent(mi *metainfo.MetaInfo, concealUploaderIdentity bool) error {
	spec := torrent.TorrentSpecFromMetaInfo(mi)
	spec.Storage = me.uploadStorage
	t, new, err := me.torrentClient.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	if !new {
		panic("adding an upload should always be a new torrent")
	}
	// I think we're trying to avoid touching the network at all here, including announces etc. This
	// feature is currently supported in anacrolix/torrent? We could serve directly from the local
	// filesystem instead of going through the torrent storage if that was more appropriate?
	// TODO(anacrolix): Prevent network being touched at all, or bypass torrent client entirely.
	if concealUploaderIdentity {
		t.DisallowDataUpload()
	} else {
		me.addImplicitTrackers(t)
	}
	return nil
}

func (me *HttpHandler) handleUploads(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
	resp := []objectInfo{} // Ensure not nil: I don't like 'null' as a response.
	err := replica.IterUploads(me.uploadsDir, func(iu replica.IteredUpload) {
		mi := iu.Metainfo
		err := iu.Err
		if err != nil {
			log.Errorf("error iterating uploads: %v", err)
			return
		}
		var oi objectInfo
		err = oi.FromUploadMetainfo(mi, iu.FileInfo.ModTime())
		if err != nil {
			log.Errorf("error parsing upload metainfo for %q: %v", iu.FileInfo.Name(), err)
			return
		}
		resp = append(resp, oi)
	})
	if err != nil {
		log.Errorf("error walking uploads dir: %v", err)
	}
	return encodeJsonResponse(rw, resp)
}

func (me *HttpHandler) handleDelete(rw *ops.InstrumentedResponseWriter, r *http.Request) (err error) {
	link := r.URL.Query().Get("link")
	m, err := metainfo.ParseMagnetUri(link)
	if err != nil {
		return handlerError{http.StatusBadRequest, fmt.Errorf("parsing magnet link: %v", err)}
	}

	var upload replica.Upload
	if err := upload.FromMagnet(m); err != nil {
		log.Errorf("error getting upload spec from magnet link %q: %v", m, err)
		return handlerError{http.StatusBadRequest, fmt.Errorf("parsing replica uri: %w", err)}
	}

	uploadAuthFilePath := me.uploadTokenPath(upload)
	authBytes, readAuthErr := ioutil.ReadFile(uploadAuthFilePath)

	metainfoFilePath := me.uploadMetainfoPath(upload)
	_, loadMetainfoErr := metainfo.LoadFromFile(metainfoFilePath)

	if readAuthErr == nil || loadMetainfoErr == nil {
		log.Debugf("deleting %q (auth=%q, haveMetainfo=%t)",
			upload.Prefix,
			string(authBytes),
			loadMetainfoErr == nil && os.IsNotExist(readAuthErr))
		// We're not inferring the endpoint from the link, should we?
		err := me.ReplicaServiceClient.DeleteUpload(
			upload.Prefix,
			string(authBytes),
			loadMetainfoErr == nil && os.IsNotExist(readAuthErr),
		)
		if err != nil {
			// It could be possible to unpack the service response status code and relay that.
			return fmt.Errorf("deleting upload: %w", err)
		}
		t, ok := me.torrentClient.Torrent(m.InfoHash)
		if ok {
			t.Drop()
		}
		os.RemoveAll(filepath.Join(me.dataDir, upload.String()))
		os.Remove(metainfoFilePath)
		os.Remove(uploadAuthFilePath)
	}
	if os.IsNotExist(loadMetainfoErr) && os.IsNotExist(readAuthErr) {
		return handlerError{http.StatusGone, errors.New("no upload tokens found")}
	}
	if !os.IsNotExist(readAuthErr) {
		return readAuthErr
	}
	return loadMetainfoErr
}

func (me *HttpHandler) handleMetadata(category string) func(*ops.InstrumentedResponseWriter, *http.Request) error {
	return func(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
		query := r.URL.Query()
		replicaLink := query.Get("replicaLink")
		m, err := metainfo.ParseMagnetUri(replicaLink)
		if err != nil {
			return err
		}
		so := m.Params.Get("so")
		fileIndex, err := strconv.ParseUint(so, 10, 0)
		if err != nil {
			err = fmt.Errorf("parsing so field: %w", err)
			log.Errorf("error %v", err)
			fileIndex = 0
		}
		mr := &http.Request{
			Method: http.MethodGet,
			Header: make(http.Header),
		}
		for _, h := range []string{
			"Accept",
			"Accept-Encoding",
			"Accept-Language",
			"Range",
		} {
			for _, v := range r.Header[h] {
				mr.Header.Add(h, v)
			}
		}
		key := fmt.Sprintf("%s/%s/%d", m.InfoHash.HexString(), category, fileIndex)
		resp, err := doFirst(mr, me.HttpClient, func(r *http.Response) bool {
			return r.StatusCode/100 == 2
		}, func() (ret []string) {
			for _, s := range me.GlobalConfig.MetadataBaseUrls {
				ret = append(ret, s+key)
			}
			return
		}())
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		// TODO: Should we filter for successful response here?
		rw.Header().Set("Cache-Control", "public, max-age=604800, immutable")

		for h, vv := range resp.Header {
			for _, v := range vv {
				rw.Header().Add(h, v)
			}
		}
		rw.WriteHeader(resp.StatusCode)
		_, err = io.Copy(rw, resp.Body)
		return err
	}
}

// Returns the first response for which the filter returns true. Otherwise a result is selected at
// random.
func doFirst(
	req *http.Request,
	client *http.Client,
	filter func(*http.Response) bool,
	urls []string,
) (*http.Response, error) {
	type result struct {
		urlIndex int
		*http.Response
		error
	}
	results := make(chan result, len(urls))
	contextCancels := make([]func(), 0, len(urls))
	for i, url_ := range urls {
		ctx, cancel := context.WithCancel(req.Context())
		contextCancels = append(contextCancels, cancel)
		go func(i int, url_ string) {
			resp, err := func() (_ *http.Response, err error) {
				u, err := url.Parse(url_)
				if err != nil {
					return
				}
				req := req.Clone(ctx)
				req.RequestURI = ""
				req.URL = u
				req.Host = ""
				return client.Do(req)
			}()
			results <- result{i, resp, err}
		}(i, url_)
	}
	retChan := make(chan result)
	go func() {
		var rejected []result
		gotOne := false
		for range urls {
			res := <-results
			if !gotOne && res.error == nil && filter(res.Response) {
				retChan <- res
				gotOne = true
			} else {
				rejected = append(rejected, res)
			}
		}
		if !gotOne {
			// Return a randomly rejected result, and remove it from the slice.
			i := rand.Intn(len(rejected))
			retChan <- rejected[i]
			rejected[i] = rejected[len(rejected)-1]
			rejected = rejected[:len(rejected)-1]
		}
		// Cancel and close out all unused results.
		for _, res := range rejected {
			if res.error == nil {
				res.Body.Close()
			}
		}
	}()
	ret := <-retChan
	go func() {
		close(retChan)
		// Cancel everything we're not using
		for i, f := range contextCancels {
			if i != ret.urlIndex {
				f()
			}
		}
	}()
	return ret.Response, ret.error
}

func (me *HttpHandler) handleSearch(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
	searchTerm := r.URL.Query().Get("s")

	rw.Op.Set("search_term", searchTerm)
	me.GaSession.EventWithLabel("replica", "search", searchTerm)

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

	m, err := metainfo.ParseMagnetUri(link)
	if err != nil {
		return handlerError{http.StatusBadRequest, fmt.Errorf("parsing magnet link: %w", err)}
	}

	rw.Op.Set("info_hash", m.InfoHash)

	t, _, release := me.confluence.GetTorrent(m.InfoHash)
	defer release()

	spec := &torrent.TorrentSpec{
		Trackers:    [][]string{me.GlobalConfig.Trackers},
		InfoHash:    m.InfoHash,
		DisplayName: m.DisplayName,
		Webseeds:    me.GlobalConfig.WebseedUrls(m.InfoHash.HexString()),
		PeerAddrs:   me.GlobalConfig.StaticPeerAddrs,
		Sources:     me.GlobalConfig.MetainfoUrls(m.InfoHash.HexString()),
	}

	// TODO: Remove this when infohash prefixing is used throughout.
	var uploadSpec replica.Upload
	unwrapUploadSpecErr := uploadSpec.FromMagnet(m)
	if unwrapUploadSpecErr == nil {
		spec.Sources = append(spec.Sources, me.GlobalConfig.MetainfoUrls(uploadSpec.PrefixString())...)
		spec.Webseeds = append(spec.Webseeds, me.GlobalConfig.WebseedUrls(uploadSpec.PrefixString())...)
	} else {
		log.Debugf("unwrapping upload spec from magnet link: %v", unwrapUploadSpecErr)
	}

	if err := t.MergeSpec(spec); err != nil {
		return fmt.Errorf("merging spec: %w", err)
	}

	selectOnly, err := strconv.ParseUint(m.Params.Get("so"), 10, 0)
	// Assume that it should be present, as it'll be added going forward where possible. When it's
	// missing, zero is a perfectly adequate default for now.
	if err != nil {
		log.Errorf("error parsing so field: %v", err)
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
	ext := path.Ext(filename)
	if ext != "" {
		filename = sanitize.BaseName(strings.TrimSuffix(filename, ext)) + ext
	}
	if filename != "" {
		rw.Header().Set("Content-Disposition", inlineType+"; filename*=UTF-8''"+url.QueryEscape(filename))
	}

	rw.Op.Set("download_filename", filename)
	switch inlineType {
	case "inline":
		me.GaSession.EventWithLabel("replica", "view", ext)
	case "attachment":
		me.GaSession.EventWithLabel("replica", "download", ext)
	}

	torrentFile := t.Files()[selectOnly]
	fileReader := torrentFile.NewReader()
	rw.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	confluence.ServeTorrentReader(rw, r, fileReader, torrentFile.Path())
	return nil
}

func (me *HttpHandler) handleObjectInfo(rw *ops.InstrumentedResponseWriter, r *http.Request) error {
	query := r.URL.Query()
	replicaLink := query.Get("replicaLink")
	m, err := metainfo.ParseMagnetUri(replicaLink)
	if err != nil {
		return handlerError{http.StatusBadRequest, fmt.Errorf("parsing magnet link: %w", err)}
	}
	// TODO: Update this to be the same as how handleViewWith does it.
	source := m.Params.Get("as")
	resp, err := me.ReplicaServiceClient.HttpClient.Get(source)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	mi, err := metainfo.Load(resp.Body)
	if err != nil {
		return err
	}
	date := time.Unix(mi.CreationDate, 0).Format(time.RFC3339Nano)
	rw.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	return encodeJsonResponse(rw, map[string]interface{}{"creationDate": date})
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

func (me *HttpHandler) uploadMetainfoPath(upload replica.Upload) string {
	return filepath.Join(me.uploadsDir, upload.String()+".torrent")
}

func (me *HttpHandler) uploadTokenPath(prefix replica.Prefix) string {
	return filepath.Join(me.uploadsDir, prefix.PrefixString()+".token")
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
	t.AddTrackers([][]string{me.GlobalConfig.Trackers})
}
