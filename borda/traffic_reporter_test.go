package borda

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/measured"
	"github.com/stretchr/testify/assert"
)

type mockProxy struct {
	chained.BaseProxy
	addr string
}

func (m mockProxy) DialServer() (net.Conn, error) {
	return net.Dial("tcp", m.addr)
}

type mockRT struct {
	ch chan []byte
}

func (r mockRT) RoundTrip(req *http.Request) (*http.Response, error) {
	b, _ := ioutil.ReadAll(req.Body)
	r.ch <- b
	return nil, errors.New("fail intentionally")
}

func TestReportTraffic(t *testing.T) {
	ch := make(chan []byte, 1)
	bc := borda.NewClient(&borda.Options{
		BatchInterval: 1 * time.Second,
		Client: &http.Client{
			Transport: &mockRT{ch},
		},
	})
	tr, wrapper := newTrafficReporter(bc, 100*time.Millisecond, func() bool { return true })
	defer tr.Stop()
	defer bc.Flush()
	l, _ := net.Listen("tcp", ":0")
	go func() {
		c, _ := l.Accept()
		io.Copy(c, c)
	}()
	// allow sometime for goroutine to start
	time.Sleep(10 * time.Millisecond)
	p := wrapper(&mockProxy{addr: l.Addr().String()})
	conn, err := p.DialServer()
	assert.NoError(t, err, "should dial server")
	_, ok := conn.(*measured.Conn)
	assert.True(t, ok, "should wrap correctly")
	n, _ := conn.Write([]byte("abcd"))
	assert.Equal(t, 4, n, "should write to conn")
	var buf [4]byte
	n, _ = conn.Read(buf[:])
	assert.Equal(t, 4, n, "should read from conn")
	assert.EqualValues(t, buf[:], []byte("abcd"), "should read anything")
	conn.Close()
	payload := <-ch
	t.Log(string(payload))
	var obj []map[string]interface{}
	err = json.Unmarshal(payload, &obj)
	assert.NoError(t, err, "should unmarshal")
	assert.Equal(t, "client_results", obj[0]["name"])
	values := obj[0]["values"].(map[string]interface{})
	assert.EqualValues(t, 4, values["client_bytes_recv"])
	assert.EqualValues(t, 4, values["client_bytes_sent"])
}
