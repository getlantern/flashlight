package chained

import (
	"net"
	"reflect"
	"testing"

	"github.com/getlantern/common/config"
	"github.com/stretchr/testify/require"
)

func TestTLSFragConn(t *testing.T) {
	var conn net.Conn = &net.TCPConn{}

	pc := &config.ProxyConfig{
		PluggableTransportSettings: map[string]string{},
	}

	result := tlsFragConn(conn, pc)
	resultType := reflect.TypeOf(result)
	require.Equal(t, "*net.TCPConn", resultType.String())

	pc.PluggableTransportSettings["tlsfrag_split_index"] = "1"
	result = tlsFragConn(conn, pc)
	resultType = reflect.TypeOf(result)
	require.Equal(t, "*transport.duplexConnAdaptor", resultType.String())

	// Test UDP conns too
	conn = &net.UDPConn{}
	pc = &config.ProxyConfig{
		PluggableTransportSettings: map[string]string{},
	}

	result = tlsFragConn(conn, pc)
	resultType = reflect.TypeOf(result)
	require.Equal(t, "*net.UDPConn", resultType.String())

	pc.PluggableTransportSettings["tlsfrag_split_index"] = "1"
	result = tlsFragConn(conn, pc)
	resultType = reflect.TypeOf(result)
	require.Equal(t, "*net.UDPConn", resultType.String())
}
