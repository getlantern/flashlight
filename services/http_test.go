package services

import (
	"bytes"
	"io"
	"math"
	mrand "math/rand/v2"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/getlantern/flashlight/v7/apipb"

	"github.com/getlantern/flashlight/v7/common"
)

func TestPost(t *testing.T) {
	sdr := &sender{}
	rt := &mockRoundTripper{
		status: http.StatusOK,
		sleep:  mrand.IntN(10),
	}
	user := common.NullUserConfig{}
	_, sleep, err := sdr.post("http://example.com", nil, rt, user)
	require.NoError(t, err)

	assert.Equal(t, rt.sleep, int(sleep), "response sleep value does not match")

	testBackoff := func(t *testing.T, rt *mockRoundTripper) {
		sdr := &sender{}
		for i := 0; i < 5; i++ {
			wait := time.Duration(math.Pow(2, float64(i))) * retryWaitSeconds
			want := int64(wait.Seconds())
			_, sleep, err = sdr.post("http://example.com", nil, rt, user)
			assert.Equal(t, want, sleep, "returned sleep value does not follow an exponential backoff")
		}
	}

	t.Run("backoff on error", func(t *testing.T) {
		rt = &mockRoundTripper{shouldErr: true}
		testBackoff(t, rt)
	})

	t.Run("backoff on bad StatusCode", func(t *testing.T) {
		rt = &mockRoundTripper{status: http.StatusBadRequest}
		testBackoff(t, rt)
	})
}

func TestDoPost(t *testing.T) {
	sdr := &sender{}
	rt := &mockRoundTripper{}
	_, err := sdr.doPost("http://example.com", nil, rt, common.NullUserConfig{})
	assert.NoError(t, err)
	assert.True(t, rt.req.Close, "request.Close should be set to true before calling RoundTrip")
}

type mockRoundTripper struct {
	req       *http.Request
	status    int
	sleep     int
	config    *apipb.ConfigResponse
	shouldErr bool
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.req = req
	if m.shouldErr {
		return nil, assert.AnError
	}

	buf, _ := proto.Marshal(m.config)
	body := io.NopCloser(bytes.NewReader(buf))

	header := http.Header{}
	header.Add(common.SleepHeader, strconv.Itoa(m.sleep))
	if m.status == 0 {
		m.status = http.StatusOK
	}

	return &http.Response{
		StatusCode: m.status,
		Header:     header,
		Body:       body,
	}, nil
}
