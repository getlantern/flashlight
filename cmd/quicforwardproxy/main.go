// This binary provides an easy way to smoke test a quicproxy.QuicForwardProxy.
// Run it alongside cmd/quicreverseproxy.
//
// Usage: go run ./cmd/quicforwardproxy/main.go \
// 		-port=3223 \
// 		-reverse-proxy-addr=http://whatever:whatever
//
// Now, point your browser to http://127.0.0.1:3223 and traffic will be
// proxied from this quicproxy.QuicForwardProxy to the
// quicproxy.QuicReverseProxy specified in "-reverse-proxy-addr" flag
package main

import (
	"flag"

	"github.com/getlantern/flashlight/quicproxy"
	"github.com/getlantern/golog"
)

var (
	log                  = golog.LoggerFor("quic-forward-proxy")
	portFlag             = flag.String("port", "", "")
	reverseProxyAddrFlag = flag.String("reverse-proxy-addr", "", "")
)

func main() {
	if err := mainErr(); err != nil {
		log.Fatalf(err.Error())
	}
}

func mainErr() error {
	flag.Parse()

	errChan := make(chan error)
	srv, err := quicproxy.NewForwardProxy(
		*portFlag, // addr
		true,      // verbose
		true,      // insecureSkipVerify
		errChan,
	)
	if err != nil {
		return log.Errorf(" %v", err)
	}
	srv.SetReverseProxyUrl(*reverseProxyAddrFlag)
	log.Debugf("Running proxy on %v", srv.Port)
	return <-errChan
}
