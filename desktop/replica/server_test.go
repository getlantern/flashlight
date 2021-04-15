package desktopReplica

import (
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/golog/testlog"
	"github.com/getlantern/replica"
	"github.com/stretchr/testify/assert"
)

// TestUploadAndDelete makes sure we can upload and then subsequently delete a given file.
func TestUploadAndDelete(t *testing.T) {
	stopCapture := testlog.Capture(t)
	defer stopCapture()

	dir, err := ioutil.TempDir(os.TempDir(), "replicauploadtest")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)
	handler, err := NewHTTPHandler(
		dir,
		&common.NullUserConfig{},
		replica.Client{
			Storage:  replica.S3Storage{},
			Endpoint: replica.DefaultEndpoint,
		},
		// TODO: make this configurable for Iran
		replica.Client{
			Storage: replica.S3Storage{},
			Endpoint: replica.Endpoint{
				StorageProvider: "s3",
				BucketName:      "replica-metadata",
				Region:          "ap-southeast-1",
			}},
		&analytics.NullSession{},
		DefaultNewHttpHandlerOpts(),
	)
	if err != nil {
		log.Errorf("error creating replica http server: %v", err)
		return
	}
	defer handler.Close()

	uploadsDir := dir + "/replica/uploads"
	files, err := ioutil.ReadDir(uploadsDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(files))

	w := httptest.NewRecorder()
	rw := ops.InitInstrumentedResponseWriter(w, "replicatest")
	fileName := "testfile"
	r := httptest.NewRequest("POST", "http://dummy.com/upload?name="+fileName, strings.NewReader("file content"))
	err = handler.handleUpload(rw, r)
	assert.NoError(t, err)

	// The delete call requires a magnet link. To get that, we have to parse the torrents
	// in the upload directory.

	files, err = ioutil.ReadDir(uploadsDir)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(files))
	magnetLink := ""
	err = replica.IterUploads(uploadsDir, func(iu replica.IteredUpload) {
		mi := iu.Metainfo
		err := iu.Err
		if err != nil {
			log.Errorf("error iterating uploads: %v", err)
			return
		}

		var oi objectInfo
		err = oi.fromS3UploadMetaInfo(mi, iu.FileInfo.ModTime())
		if err != nil {
			log.Errorf("error parsing upload metainfo for %q: %v", iu.FileInfo.Name(), err)
			return
		}
		log.Debugf("OI: %#v", oi)
		magnetLink = oi.Link
	})

	// We're bypassing the actual HTTP server route handling, so the domain and path
	// can be anything here.
	u, err := url.Parse("http://dummy.com/upload")
	assert.NoError(t, err)
	q := u.Query()
	q.Set("link", magnetLink)
	u.RawQuery = q.Encode()
	url := u.String()
	log.Debugf("URL: %v", url)

	d := httptest.NewRequest("GET", url, nil)
	err = handler.handleDelete(rw, d)
	assert.NoError(t, err)
}
