package chained

import (
	"encoding/hex"
	"math/rand"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/transport/tlsfrag"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
)

func wrapTLSFrag(conn net.Conn, pc *config.ProxyConfig) net.Conn {
	fragFunc, ok := makeFragFunc(pc)
	if ok {
		tlsFragConn, err := tlsfrag.WrapConnFunc(&streamConn{conn}, fragFunc)
		if err != nil {
			log.Errorf("tlsfrag: wrapping error: %w", err)
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
	// Example: "index:3" or "regex:foo.*" or "regex:test" or "rand:10,80"
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
		regex, err := regexp.Compile(hex.EncodeToString([]byte(cfg)))
		if err != nil {
			log.Errorf("tlsfrag: bad regex specifier: %v", err)
			return nil, false
		}
		return func(b []byte) int {
			loc := regex.FindIndex([]byte(hex.EncodeToString(b)))
			if loc == nil {
				return 0
			}
			log.Debugf("tlsfrag: regex match at index: %v", loc[0]/2)
			return loc[0] / 2
		}, true
	case "rand":
		min, max, err := parseRange(cfg)
		if err != nil {
			log.Errorf("tlsfrag: bad rand specifier: %v", err)
			return nil, false
		}
		return func(_ []byte) int {
			randIndex := min + rand.Intn(max-min)
			log.Debugf("tlsfrag: rand index: %v", randIndex)
			return randIndex
		}, true
	default:
		log.Errorf("tlsfrag: unrecognized func type '%s'", funcType)
		return nil, false
	}
}

func parseRange(cfg string) (int, int, error) {
	parts := strings.Split(cfg, ",")
	if len(parts) != 2 {
		return 0, 0, errors.New("expected two parts")
	}
	min, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, errors.New("bad min: %v", err)
	}
	max, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, errors.New("bad max: %v", err)
	}
	if min > max {
		return 0, 0, errors.New("min > max")
	}
	return min, max, nil
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
