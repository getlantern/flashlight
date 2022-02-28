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
		true,
		errChan,
	)
	if err != nil {
		return log.Errorf(" %v", err)
	}
	srv.SetReverseProxyUrl(*reverseProxyAddrFlag)
	log.Debugf("Running proxy on %v", srv.Port)
	return <-errChan
}
