package upnp

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpnp(t *testing.T) {
	port := 32223
	l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	require.NoError(t, err)
	doneChan := make(chan struct{})
	go func() {
		c, err := l.Accept()
		require.NoError(t, err)
		t.Log("Accepted conn")
		// var b [256]byte
		// _, err = c.Read(b[:])
		// require.NoError(t, err)
		_, err = c.Write([]byte("replica"))
		require.NoError(t, err)
		c.Close()
		t.Log("Done")
		close(doneChan)
	}()
	okAtleastOnce, _ := ForwardPortWithUpnp(context.Background(), uint16(port))
	require.True(t, okAtleastOnce)
	select {
	case <-doneChan:
	case <-time.After(30 * time.Second):
		require.Fail(t, "Timeout")
	}
}
