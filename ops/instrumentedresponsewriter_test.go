package ops

import (
	"crypto/rand"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	borda "github.com/getlantern/borda/client"

	"github.com/stretchr/testify/assert"
)

func TestInstrumentedResponseWriterMetrics(t *testing.T) {

	RegisterReporter(func(failure error, ctx map[string]interface{}) {
		expectedByteCount := 10000.0
		expectedFirstByteTime := 1000.0
		expectedResponseTime := 2000.0
		assert.Equal(t, "testing", ctx["op"])
		assert.Equal(t, borda.Sum(expectedByteCount), ctx["testing_bytes"])
		assert.Equal(t, borda.Percentile(expectedFirstByteTime), ctx["testing_first_byte_ms"])
		assert.Equal(t, borda.Percentile(expectedResponseTime), ctx["testing_response_time_ms"])
		assert.Equal(t, borda.Percentile((expectedByteCount/1000000/expectedResponseTime)*8.0*1000), ctx["testing_response_rate_mbps"])
	})

	handler := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		io.CopyN(w, rand.Reader, 1)
		time.Sleep(1 * time.Second)
		io.CopyN(w, rand.Reader, 9999)
	}

	req := httptest.NewRequest("GET", "http://testing.com", nil)

	recorder := httptest.NewRecorder()

	instrumentedResponseWriter := InitInstrumentedResponseWriter(recorder, "testing")
	handler(instrumentedResponseWriter, req)

	resp := recorder.Result()
	ioutil.ReadAll(resp.Body)

	instrumentedResponseWriter.Finish()
}
