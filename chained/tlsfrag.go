package chained

import (
	"net"
	"strconv"
	"strings"

	"github.com/Jigsaw-Code/outline-sdk/transport/tlsfrag"

	"github.com/getlantern/common/config"
)

func tlsFragConn(conn net.Conn, pc *config.ProxyConfig) net.Conn {
	fragFunc, ok := makeFragFunc(pc)
	if ok {
		if _, ok := conn.(*net.TCPConn); !ok {
			return conn
		}
		tlsFragConn, err := tlsfrag.WrapConnFunc(conn.(*net.TCPConn), fragFunc)
		if err != nil {
			return conn
		}
		return tlsFragConn
	}
	return conn
}

func makeFragFunc(pc *config.ProxyConfig) (tlsfrag.FragFunc, bool) {
	fragStr, ok := pc.PluggableTransportSettings["tlsfrag"]
	if !ok {
		return nil, false
	}

	// fragStr should be of the form <frag func> or <frag func>:<func config>.
	funcType, cfg, hasCfg := strings.Cut(fragStr, ":")
	switch funcType {
	case "index":
		if !hasCfg {
			log.Error("tlsfrag: missing index specifier")
			return nil, false
		}

		index, err := strconv.Atoi(cfg)
		if err != nil {
			log.Errorf("tlsfrag: bad index specifier: %v", err)
		}
		return func(_ []byte) int { return index }, true

	default:
		log.Errorf("tlsfrag: unrecognized func type '%s'", funcType)
		return nil, false
	}
}
