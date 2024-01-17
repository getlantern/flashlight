package chained

import (
	"net"
	"strconv"

	"github.com/Jigsaw-Code/outline-sdk/transport/tlsfrag"
	"github.com/getlantern/common/config"
)

func tlsFragConn(conn net.Conn, pc *config.ProxyConfig) net.Conn {
	splitHello, index := splitHelloInfo(pc)
	if splitHello {
		if conn, ok := conn.(*net.TCPConn); !ok {
			return conn
		}
		tlsFragConn, err := tlsfrag.WrapConnFunc(conn.(*net.TCPConn), func(record []byte) (n int) {
			return index
		})
		if err != nil {
			return conn
		}
		return tlsFragConn
	}
	return conn
}

func splitHelloInfo(pc *config.ProxyConfig) (bool, int) {
	indexStr, ok := pc.PluggableTransportSettings["tlsfrag_split_index"]
	if !ok {
		return false, 0
	}

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		log.Errorf("invalid tlsfrag option: %v. It should be a number", indexStr)
		return false, 0
	}
	return true, index
}
