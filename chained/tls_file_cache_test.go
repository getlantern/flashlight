package chained

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/getlantern/tlsdialer/v3"
	"github.com/getlantern/tlsresumption"
	tls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/assert"
)

func TestPersistSessionStates(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "persistSessionStatesTest")
	if !assert.NoError(t, err) {
		return
	}

	defer os.RemoveAll(tmpDir)

	currentSessionStatesMx.Lock()
	saveSessionStateCh = make(chan sessionStateForServer, 100)
	currentSessionStates = make(map[string]sessionStateForServer)
	currentSessionStatesMx.Unlock()

	persistSessionStates(tmpDir, 250*time.Millisecond)
	cache := tls.NewLRUClientSessionCache(10)
	td := &tlsdialer.Dialer{
		DoDial:         net.DialTimeout,
		Timeout:        10 * time.Second,
		SendServerName: true,
		ClientHelloID:  tls.HelloChrome_Auto,
		Config: &tls.Config{
			ClientSessionCache: cache,
		},
	}
	host, port := "tls-v1-2.badssl.com", "1012"
	result, err := td.DialForTimings("tcp", net.JoinHostPort(host, port))
	if !assert.NoError(t, err) {
		return
	}
	defer result.Conn.Close()
	log.Debug(result.Conn.RemoteAddr())

	ss1, ok := cache.Get(host)
	assert.True(t, ok)
	expectedTS := time.Now()
	saveSessionState("myserver", ss1, expectedTS)
	close(saveSessionStateCh)

	time.Sleep(1 * time.Second)

	currentSessionStatesMx.Lock()
	saveSessionStateCh = make(chan sessionStateForServer, 100)
	currentSessionStates = make(map[string]sessionStateForServer)
	currentSessionStatesMx.Unlock()

	persistSessionStates(tmpDir, 250*time.Millisecond)

	time.Sleep(1 * time.Second)
	ss2, ts := persistedSessionStateFor("myserver")
	if assert.NotNil(t, ss2) {
		_ss1, _ := tlsresumption.SerializeClientSessionState(ss1)
		_ss2, _ := tlsresumption.SerializeClientSessionState(ss2)
		assert.EqualValues(t, _ss1, _ss2)
		assert.EqualValues(t, expectedTS.Truncate(time.Second), ts.Truncate(time.Second))
	}
}
