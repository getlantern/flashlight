package desktopReplica

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/golog/testlog"
	"github.com/getlantern/replica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUploadAndDelete makes sure we can upload and then subsequently delete a given file.
func TestUploadAndDelete(t *testing.T) {
	stopCapture := testlog.Capture(t)
	defer stopCapture()

	dir := t.TempDir()
	input := NewHttpHandlerInput{}
	input.SetDefaults()
	input.DefaultReplicaClient = replica.ServiceClient{
		ReplicaServiceEndpoint: replica.GlobalChinaDefaultServiceUrl,
		HttpClient:             http.DefaultClient,
	}
	if false {
		// I'm not sure about letting a unit test connect directly to a prod server, but we *are*
		// deleting the content afterward, so we don't really make any changes overall (assuming the
		// test passes). This connects to a local instance of replica-rust instead.
		input.DefaultReplicaClient.ReplicaServiceEndpoint = &url.URL{Scheme: "http", Host: "localhost:8080"}
	}
	input.ConfigDir = dir
	handler, err := NewHTTPHandler(input)
	require.NoError(t, err)
	defer handler.Close()

	uploadsDir := handler.uploadsDir
	files, err := ioutil.ReadDir(uploadsDir)
	assert.NoError(t, err)
	assert.Empty(t, files)

	w := httptest.NewRecorder()
	rw := ops.InitInstrumentedResponseWriter(w, "replicatest")
	fileName := "testfile"
	r := httptest.NewRequest("POST", "http://dummy.com/upload?name="+fileName, strings.NewReader("file content"))
	err = handler.handleUpload(rw, r)
	require.NoError(t, err)

	var uploadedObjectInfo objectInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &uploadedObjectInfo))

	files, err = ioutil.ReadDir(uploadsDir)
	assert.NoError(t, err)
	// We expect a token file and metainfo.
	assert.Equal(t, 2, len(files))

	magnetLink := uploadedObjectInfo.Link

	// We're bypassing the actual HTTP server route handling, so the domain and path can be anything
	// here.
	url_ := (&url.URL{
		Scheme:   "http",
		Host:     "dummy.com",
		Path:     "/upload",
		RawQuery: url.Values{"link": {magnetLink}}.Encode(),
	}).String()
	log.Debugf("delete url: %q", url_)

	d := httptest.NewRequest("GET", url_, nil)
	err = handler.handleDelete(rw, d)
	require.NoError(t, err)

	files, err = ioutil.ReadDir(uploadsDir)
	assert.NoError(t, err)
	// We expect the delete handler to have removed the token and metainfo files.
	assert.Empty(t, files)
}
