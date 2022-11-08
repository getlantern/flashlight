package rpcclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/getlantern/borda/client"
	"github.com/getlantern/golog"
	"github.com/getlantern/withtimeout"
	"github.com/getlantern/zenodb/rpc"
)

var log = golog.LoggerFor("rpcclient")

const (
	defaultRPCTimeoutPerMeasurement = 100 * time.Millisecond
)

// Default creates a new borda.Client that connects to borda.lantern.io
// using gRPC if possible, or falling back to HTTPS if it can't dial out with
// gRPC.
func Default(batchInterval time.Duration) *client.Client {
	log.Debugf("Creating borda client that submits every %v", batchInterval)

	opts := &client.Options{
		BatchInterval: batchInterval,
	}

	clientSessionCache := tls.NewLRUClientSessionCache(10000)
	clientTLSConfig := &tls.Config{
		ServerName:         "borda.getlantern.org",
		ClientSessionCache: clientSessionCache,
	}

	rc, err := rpc.Dial("borda.getlantern.org:17712", &rpc.ClientOpts{
		Dialer: func(addr string, timeout time.Duration) (net.Conn, error) {
			log.Debug("Dialing borda with gRPC")
			_conn, _, err := withtimeout.Do(30*time.Second, func() (interface{}, error) {
				conn, dialErr := net.DialTimeout("tcp", addr, timeout)
				if dialErr != nil {
					return nil, dialErr
				}
				tlsConn := tls.Client(conn, clientTLSConfig)
				handshakeErr := tlsConn.Handshake()
				if handshakeErr != nil {
					log.Errorf("Error TLS handshaking with borda: %v", handshakeErr)
					conn.Close()
				}
				return tlsConn, handshakeErr
			})
			if err != nil {
				log.Errorf("Failed to dial borda with gRPC: %v", err)
				return nil, err
			}
			return _conn.(net.Conn), nil
		},
	})
	if err != nil {
		log.Errorf("Unable to dial borda, will not use gRPC: %v", err)
	} else {
		log.Debug("Using gRPC to communicate with borda")
		opts.Sender = buildSender(rc, defaultRPCTimeoutPerMeasurement)
	}

	return client.NewClient(opts)
}

// if opts.RPCTimeoutPerMeasurement <= 0 {
//   log.Debugf("Defaulting per measurement RPC timeout to %v", defaultRPCTimeoutPerMeasurement)
//   opts.RPCTimeoutPerMeasurement = defaultRPCTimeoutPerMeasurement
// }

func buildSender(rc rpc.Client, rpcTimeoutPerMeasurement time.Duration) func(batch map[string][]*client.Measurement) (int, error) {
	return func(batch map[string][]*client.Measurement) (int, error) {
		log.Debug("Sending batch with RPC")

		numMeasurements := 0
		for _, measurements := range batch {
			numMeasurements += len(measurements)
		}

		timeout := time.Duration(numMeasurements) * rpcTimeoutPerMeasurement
		log.Debugf("Setting timeout of %v for submitting measurements via RPC", timeout)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// TODO: right now we send everything to "inbound", might be nice to
		// separate streams where we can.
		inserter, err := rc.NewInserter(ctx, "inbound")
		if err != nil {
			return 0, fmt.Errorf("Unable to get inserter: %v", err)
		}

		for _, measurements := range batch {
			for _, m := range measurements {
				err = inserter.Insert(m.Ts, m.TypedDimensions, func(cb func(string, interface{})) {
					for key, val := range m.Values {
						cb(key, val.Get())
					}
				})
				if err != nil {
					inserter.Close()
					return 0, fmt.Errorf("Error inserting: %v", err)
				}
			}
		}

		report, err := inserter.Close()
		if err != nil {
			return 0, fmt.Errorf("Error closing inserter: %v", err)
		}

		return report.Succeeded, nil
	}
}
