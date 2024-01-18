package chained

import (
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/transport/tlsfrag"

	"github.com/getlantern/common/config"
)

func wrapTLSFrag(conn net.Conn, pc *config.ProxyConfig) net.Conn {
	fragFunc, ok := makeFragFunc(pc)
	if ok {
		tlsFragConn, err := tlsfrag.WrapConnFunc(&streamConn{conn}, fragFunc)
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
	// Example: "index:3" or "regex:foo.*" or "regex:test"
	funcType, cfg, hasCfg := strings.Cut(fragStr, ":")
	if !hasCfg {
		log.Error("tlsfrag: missing config specifier")
		return nil, false
	}
	if cfg == "" {
		log.Error("tlsfrag: empty config specifier")
		return nil, false
	}
	switch funcType {
	case "index":
		index, err := strconv.Atoi(cfg)
		if err != nil {
			log.Errorf("tlsfrag: bad index specifier: %v", err)
			return nil, false
		}
		return func(_ []byte) int { return index }, true
	case "regex":
		regex, err := regexp.Compile(cfg)
		if err != nil {
			log.Errorf("tlsfrag: bad regex specifier: %v", err)
			return nil, false
		}
		return func(b []byte) int {
			loc := regex.FindIndex(b)
			if loc == nil {
				return 0
			}
			log.Debugf("tlsfrag: regex match at index: %v", loc[0])
			return loc[0]
		}, true

	default:
		log.Errorf("tlsfrag: unrecognized func type '%s'", funcType)
		return nil, false
	}
}

type streamConn struct {
	net.Conn
}

func (sc *streamConn) CloseRead() error {
	return sc.Close()
}

func (sc *streamConn) CloseWrite() error {
	return sc.Close()
}

var _ transport.StreamConn = (*streamConn)(nil)
