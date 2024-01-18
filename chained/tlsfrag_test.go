package chained

import (
	"net"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/common/config"
	"github.com/stretchr/testify/require"
)

// this is a hello packet with localhost encoded in the SNI field
var hello = []byte{22, 3, 1, 0, 135, 1, 0, 0, 131, 3, 3, 48, 53, 69, 203, 240, 58, 77, 56, 159, 42, 153, 251, 106, 51, 38, 204, 75, 107, 173, 175, 235, 47, 66, 133, 56, 177, 148, 100, 25, 71, 144, 144, 0, 0, 26, 192, 47, 192, 43, 192, 17, 192, 7, 192, 19, 192, 9, 192, 20, 192, 10, 0, 5, 0, 47, 0, 53, 192, 18, 0, 10, 1, 0, 0, 64, 0, 0, 0, 14, 0, 12, 0, 0, 9, 108, 111, 99, 97, 108, 104, 111, 115, 116, 0, 5, 0, 5, 1, 0, 0, 0, 0, 0, 10, 0, 8, 0, 6, 0, 23, 0, 24, 0, 25, 0, 11, 0, 2, 1, 0, 0, 13, 0, 10, 0, 8, 4, 1, 4, 3, 2, 1, 2, 3, 255, 1, 0, 1, 0}

func TestTLSFragConn(t *testing.T) {
	var conn net.Conn = &net.TCPConn{}

	pc := &config.ProxyConfig{
		PluggableTransportSettings: map[string]string{},
	}

	fragConn := tlsFragConn(conn, pc)
	resultType := reflect.TypeOf(fragConn)
	require.Equal(t, "*net.TCPConn", resultType.String())

	pc.PluggableTransportSettings["tlsfrag"] = "index:1"
	fragConn = tlsFragConn(conn, pc)
	resultType = reflect.TypeOf(fragConn)
	require.NotEqual(t, "*net.TCPConn", resultType.String())

	pc.PluggableTransportSettings["tlsfrag"] = "regex:1"
	fragConn = tlsFragConn(conn, pc)
	resultType = reflect.TypeOf(fragConn)
	require.NotEqual(t, "*net.TCPConn", resultType.String())

	pc.PluggableTransportSettings["tlsfrag"] = "incorrect:1"
	fragConn = tlsFragConn(conn, pc)
	resultType = reflect.TypeOf(fragConn)
	require.Equal(t, "*net.TCPConn", resultType.String())

	pc.PluggableTransportSettings["tlsfrag"] = "regex:"
	fragConn = tlsFragConn(conn, pc)
	resultType = reflect.TypeOf(fragConn)
	require.Equal(t, "*net.TCPConn", resultType.String())

	// Test UDP conns too
	conn = &net.UDPConn{}
	pc = &config.ProxyConfig{
		PluggableTransportSettings: map[string]string{},
	}

	fragConn = tlsFragConn(conn, pc)
	resultType = reflect.TypeOf(fragConn)
	require.Equal(t, "*net.UDPConn", resultType.String())

	pc.PluggableTransportSettings["tlsfrag"] = "1"
	fragConn = tlsFragConn(conn, pc)
	resultType = reflect.TypeOf(fragConn)
	require.Equal(t, "*net.UDPConn", resultType.String())

	server, client := net.Pipe()
	var serverRead atomic.Int32
	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := server.Read(buf)
			if err != nil {
				break
			}
			serverRead.Add(int32(n))
		}
	}()

	pc.PluggableTransportSettings["tlsfrag"] = "regex:localhost"
	_, err := client.Write(hello)
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, len(hello), int(serverRead.Load()))
	serverRead.Store(0)

	fragConn = tlsFragConn(client, pc)
	_, err = fragConn.Write(hello)
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond)
	require.True(t, int(serverRead.Load()) > len(hello))
	client.Close()
	server.Close()
}
