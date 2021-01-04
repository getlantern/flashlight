package bandwidth

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/common"
	"github.com/stretchr/testify/assert"
)

func TestRoundTrip(t *testing.T) {
	fromChannel := eventual.NewValue()
	go func() {
		for q := range Updates {
			fromChannel.Set(q)
		}
	}()

	q, tracked := GetQuota()
	assert.Nil(t, q)
	assert.False(t, tracked)

	Track(build(5, 500, 2, 20))
	Track(build(6, 500, 3, 30))
	Track(build(4, 500, 1, 10))

	q, tracked = GetQuota()
	time.Sleep(50 * time.Millisecond)
	_q2, ok := fromChannel.Get(1 * time.Second)
	if assert.True(t, ok, "Should have gotten quota from channel") {
		q2 := _q2.(*Quota)
		assert.EqualValues(t, 500, q.MiBAllowed)
		assert.EqualValues(t, 500, q2.MiBAllowed)
		assert.EqualValues(t, 6, q.MiBUsed)
		assert.EqualValues(t, 6, q2.MiBUsed)
		asof := epoch.Add(3 * time.Second)
		assert.EqualValues(t, asof, q.AsOf)
		assert.EqualValues(t, asof, q2.AsOf)
		assert.EqualValues(t, 30, q2.TTLSeconds)
	}

}

func build(used uint64, allowed uint64, asof int64, ttlSeconds int64) *http.Response {
	resp := &http.Response{
		Header: make(http.Header),
	}
	resp.Header.Set(common.XBQHeaderv2, fmt.Sprintf("%d/%d/%d/%d", used, allowed, asof, ttlSeconds))
	return resp
}
