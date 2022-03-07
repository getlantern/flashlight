// This binary provides an easy way to smoke test a quicproxy.QuicReverseProxy.
// Run it alongside cmd/quicforwardproxy.
//
// Usage: go run ./cmd/quicreverseproxy/main.go \
// 		-port=3223
//
// Now, see ./cmd/quicforwardproxy for instructions on how to proxy traffic
// through this proxy.
package main

import (
	"flag"

	"github.com/getlantern/flashlight/quicproxy"
	"github.com/getlantern/golog"
)

var (
	log                = golog.LoggerFor("quic-reverse-proxy")
	portFlag           = flag.String("port", "8080", "port to run client proxy on")
	testPemEncodedCert = []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`)
	testPemEncodedPrivKey = []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49
AwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q
EKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==
-----END EC PRIVATE KEY-----`)
)

func main() {
	if err := mainErr(); err != nil {
		log.Fatalf(err.Error())
	}

}

func mainErr() error {
	flag.Parse()

	errChan := make(chan error)
	srv, err := quicproxy.NewReverseProxy(
		":"+*portFlag, // addr
		testPemEncodedCert,
		testPemEncodedPrivKey,
		true, // verbose
		errChan,
	)
	if err != nil {
		return log.Errorf(" %v", err)
	}
	log.Debugf("Running proxy on %v", srv.Port)
	return <-errChan
}
