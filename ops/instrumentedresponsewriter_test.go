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
		actualByteCount := ctx["testing_bytes"]
		expectedFirstByteTime := []float64{1000.0}
		actualFirstByteTime := ctx["testing_first_byte_ms"].(borda.Ptile)
		expectedResponseTime := []float64{2000.0}
		actualResponseTime := ctx["testing_response_time_ms"].(borda.Ptile)
		expectedResponseRate := []float64{((expectedByteCount / 1000000 / expectedResponseTime[0]) * 8.0 * 1000)}
		actualResponseRate := ctx["testing_response_rate_mbps"].(borda.Ptile)

		assert.Equal(t, "testing", ctx["op"])
		assert.Equal(t, borda.Sum(expectedByteCount), actualByteCount)
		assert.InDeltaSlice(t, expectedFirstByteTime, actualFirstByteTime, 5.0)
		assert.InDeltaSlice(t, expectedResponseTime, actualResponseTime, 5.0)
		assert.InDeltaSlice(t, expectedResponseRate, actualResponseRate, 0.01)
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
