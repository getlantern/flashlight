package client

import (
	"crypto/tls"
	"fmt"
	"git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/obfs4"
	"github.com/getlantern/cmux"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/keyman"
	"github.com/getlantern/netx"
	"github.com/getlantern/snappyconn"
	"github.com/getlantern/tlsdialer"
	"github.com/xtaci/kcp-go"
	"net"
	"time"
)

type dialFN func() (net.Conn, error)
type dialFactory func(*ChainedServerInfo, string) (dialFN, error)

var pluggableTransports = map[string]dialFactory{
	"":          defaultDialFactory,
	"obsf4-tcp": tcpOBFS4DialFactory,
	"obfs4-kcp": kcpOBFS4DialFactory,
}

var (
	chainedDialTimeout = 10 * time.Second
)

func defaultDialFactory(s *ChainedServerInfo, deviceID string) (dialFN, error) {
	forceProxy := ForceChainedProxyAddr != ""
	addr := s.Addr
	if forceProxy {
		log.Debugf("Forcing proxying to server at %v instead of configured server at %v", ForceChainedProxyAddr, s.Addr)
		addr = ForceChainedProxyAddr
	}

	var dial dialFN

	if s.Cert == "" && !forceProxy {
		log.Error("No Cert configured for chained server, will dial with plain tcp")
		dial = func() (net.Conn, error) {
			op := ops.Begin("dial_to_chained").ChainedProxy(s.Addr, "http")
			defer op.End()
			start := time.Now()
			conn, err := netx.DialTimeout("tcp", addr, chainedDialTimeout)
			op.DialTime(start, err)
			return conn, op.FailIf(err)
		}
	} else {
		log.Trace("Cert configured for chained server, will dial with tls over tcp")
		cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
		if err != nil {
			return nil, log.Errorf("Unable to parse certificate: %s", err)
		}
		x509cert := cert.X509()
		sessionCache := tls.NewLRUClientSessionCache(1000)
		dial = func() (net.Conn, error) {
			op := ops.Begin("dial_to_chained").ChainedProxy(s.Addr, "https")
			defer op.End()

			start := time.Now()
			conn, err := tlsdialer.DialTimeout(netx.DialTimeout, chainedDialTimeout,
				"tcp", addr, false, &tls.Config{
					ClientSessionCache: sessionCache,
					InsecureSkipVerify: true,
				})
			op.DialTime(start, err)
			if err != nil {
				return nil, op.FailIf(err)
			}
			if !forceProxy && !conn.ConnectionState().PeerCertificates[0].Equal(x509cert) {
				if closeErr := conn.Close(); closeErr != nil {
					log.Debugf("Error closing chained server connection: %s", closeErr)
				}
				return nil, op.FailIf(log.Errorf("Server's certificate didn't match expected! Server had\n%v\nbut expected:\n%v",
					conn.ConnectionState().PeerCertificates[0], x509cert))
			}
			return conn, op.FailIf(err)
		}
	}

	return dial, nil
}

func tcpOBFS4DialFactory(s *ChainedServerInfo, deviceID string) (dialFN, error) {
	return obfs4DialFactory(s, deviceID, netx.Dial)
}

func kcpOBFS4DialFactory(s *ChainedServerInfo, deviceID string) (dialFN, error) {
	// TODO: parameterize inputs to KCP
	var dial func(network, addr string) (net.Conn, error) = cmux.Dialer(&cmux.DialerOpts{
		Dial: dialKCP,
	})
	return obfs4DialFactory(s, deviceID, dial)
}

func obfs4DialFactory(s *ChainedServerInfo, deviceID string, dial func(network, error string) (net.Conn, error)) (dialFN, error) {
	if s.Cert == "" {
		return nil, fmt.Errorf("No Cert configured for obfs4 server, can't connect")
	}

	tr := obfs4.Transport{}
	cf, err := tr.ClientFactory("")
	if err != nil {
		return nil, log.Errorf("Unable to create obfs4 client factory: %v", err)
	}

	ptArgs := &pt.Args{}
	ptArgs.Add("cert", s.Cert)
	ptArgs.Add("iat-mode", s.PluggableTransportSettings["iat-mode"])

	args, err := cf.ParseArgs(ptArgs)
	if err != nil {
		return nil, log.Errorf("Unable to parse client args: %v", err)
	}

	return func() (net.Conn, error) {
		op := ops.Begin("dial_to_chained").ChainedProxy(s.Addr, "obfs4")
		defer op.End()
		start := time.Now()
		conn, err := cf.Dial("tcp", s.Addr, dial, args)
		op.DialTime(start, err)
		return conn, op.FailIf(err)
	}, nil
}

func dialKCP(network, addr string) (net.Conn, error) {
	block, err := kcp.NewNoneBlockCrypt(nil)
	if err != nil {
		return nil, errors.New("Unable to initialize AES-128 cipher: %v", err)
	}
	// TODO: the below options are hardcoded based on the defaults in kcptun.
	// At some point, it would be nice to make these tunable via the server pt
	// properties, but these defaults work well for now.
	conn, err := kcp.DialWithOptions(addr, block, 10, 3)
	if err != nil {
		return nil, err
	}
	conn.SetStreamMode(true)
	conn.SetNoDelay(0, 20, 2, 1)
	conn.SetWindowSize(128, 1024)
	conn.SetMtu(1350)
	conn.SetACKNoDelay(false)
	conn.SetKeepAlive(10)
	conn.SetDSCP(0)
	conn.SetReadBuffer(4194304)
	conn.SetWriteBuffer(4194304)
	return snappyconn.Wrap(conn), nil
}
