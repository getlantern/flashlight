package ios

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/go-socks5"
	"github.com/getlantern/golog"
	"github.com/getlantern/gotun"
	"github.com/getlantern/hidden"
	"github.com/getlantern/packetforward"
	"github.com/getlantern/proxy"
	"github.com/getlantern/proxy/filters"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/status"

	"github.com/dustin/go-humanize"
)

const (
	memLimitInMiB   = 12
	memLimitInBytes = memLimitInMiB * 1024 * 1024
)

var (
	log = golog.LoggerFor("ios")
)

type client struct {
	proxy proxy.Proxy
}

func Start(fd int, addr string, gw string) error {
	dev, err := tun.WrapTunDevice(fd, addr, gw)
	if err != nil {
		return log.Errorf("Unable to wrap tun device: %v", err)
	}

	return StartWithDevice(dev)
}

func StartWithDevice(dev tun.TUNDevice) error {
	go trackAndLimitMemory()

	dialers, err := loadDialers()
	if err != nil {
		return err
	}
	bal := balancer.New(30*time.Second, dialers...)

	pr, _ := proxy.New(&proxy.Opts{
		IdleTimeout:  chained.IdleTimeout - 5*time.Second,
		BufferSource: buffers.Pool,
		Filter:       filters.FilterFunc(filter),
		OnError:      errorResponse,
		Dial: func(ctx context.Context, isConnect bool, network, addr string) (conn net.Conn, err error) {
			return bal.DialContext(ctx, "connect", addr)
		},
	})

	cl := &client{pr}
	socksAddr, err := cl.ListenAndServeSOCKS5()
	if err != nil {
		bal.Close()
		return err
	}

	go func() {
		defer bal.Close()
		err := packetforward.To(socksAddr, "127.0.0.1:3000", dev, 1500)
		if err != nil {
			log.Fatalf("Error forwarding packets: %v", err)
		}
	}()

	return nil
}

func filter(ctx filters.Context, req *http.Request, next filters.Next) (*http.Response, filters.Context, error) {
	// Add the scheme back for CONNECT requests. It is cleared
	// intentionally by the standard library, see
	// https://golang.org/src/net/http/request.go#L938. The easylist
	// package and httputil.DumpRequest require the scheme to be present.
	req.URL.Scheme = "http"
	req.URL.Host = req.Host

	req.Header.Set(common.VersionHeader, common.Version)

	return next(ctx, req)
}

func (client *client) ListenAndServeSOCKS5() (string, error) {
	var err error
	var l net.Listener
	if l, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
		return "", errors.New("Unable to listen SOCKS5: %v", err)
	}
	listenAddr := l.Addr().String()

	conf := &socks5.Config{
		HandleConnect: func(ctx context.Context, conn net.Conn, req *socks5.Request, replySuccess func(boundAddr net.Addr) error, replyError func(err error) error) error {
			addr := fmt.Sprintf("%v:%v", req.DestAddr.IP, req.DestAddr.Port)
			errOnReply := replySuccess(nil)
			if errOnReply != nil {
				return log.Errorf("Unable to reply success to SOCKS5 client: %v", errOnReply)
			}
			return client.proxy.Connect(ctx, req.BufConn, conn, addr)
		},
	}
	server, err := socks5.New(conf)
	if err != nil {
		return "", errors.New("Unable to create SOCKS5 server: %v", err)
	}

	log.Debugf("About to start SOCKS5 client proxy at %v", listenAddr)
	go server.Serve(l)
	return l.Addr().String(), nil
}

func errorResponse(ctx filters.Context, req *http.Request, read bool, err error) *http.Response {
	var htmlerr []byte

	if req == nil {
		return nil
	}

	// If the request has an 'Accept' header preferring HTML, or
	// doesn't have that header at all, render the error page.
	switch req.Header.Get("Accept") {
	case "text/html":
		fallthrough
	case "application/xhtml+xml":
		fallthrough
	case "":
		// It is likely we will have lots of different errors to handle but for now
		// we will only return a ErrorAccessingPage error.  This prevents the user
		// from getting just a blank screen.
		htmlerr, err = status.ErrorAccessingPage(req.Host, err)
		if err != nil {
			log.Debugf("Got error while generating status page: %q", err)
		}
	}

	if htmlerr == nil {
		// Default value for htmlerr
		htmlerr = []byte(hidden.Clean(err.Error()))
	}

	return &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBuffer(htmlerr)),
		StatusCode: http.StatusServiceUnavailable,
	}
}

func loadDialers() ([]balancer.Dialer, error) {
	proxies, err := loadProxies()
	if err != nil {
		return nil, err
	}

	dialers := make([]balancer.Dialer, 0, len(proxies))
	for name, s := range proxies {
		if s.PluggableTransport == "obfs4-tcp" {
			log.Debugf("Ignoring obfs4-tcp server: %v", name)
			// Ignore obfs4-tcp as these are already included as plain obfs4
			continue
		}
		dialer, err := chainedDialer(name, s)
		if err != nil {
			log.Errorf("Unable to configure chained server %v. Received error: %v", name, err)
			continue
		}
		log.Debugf("Adding chained server: %v", dialer.JustifiedLabel())
		dialers = append(dialers, dialer)
	}

	chained.TrackStatsFor(dialers)

	return dialers, nil
}

// chainedDialer creates a *balancer.Dialer backed by a chained server.
func chainedDialer(name string, si *chained.ChainedServerInfo) (balancer.Dialer, error) {
	// Copy server info to allow modifying
	sic := &chained.ChainedServerInfo{}
	*sic = *si
	// Backwards-compatibility for clients that still have old obfs4
	// configurations on disk.
	if sic.PluggableTransport == "obfs4-tcp" {
		sic.PluggableTransport = "obfs4"
	}

	return chained.CreateDialer(name, sic, common.NewUserConfigData("~~~~~~", 0, "", nil, "en_US"))
}

func loadProxies() (map[string]*chained.ChainedServerInfo, error) {
	proxies := make(map[string]*chained.ChainedServerInfo, 1)
	err := yaml.Unmarshal([]byte(proxyConfig), proxies)
	if err != nil {
		return nil, errors.New("Unable to unmarshal proxyConfig: %v", err)
	}
	return proxies, nil
}

func trackAndLimitMemory() {
	for {
		time.Sleep(5 * time.Second)
		memstats := &runtime.MemStats{}
		runtime.ReadMemStats(memstats)
		log.Debugf("Memory InUse: %v    Alloc: %v    Sys: %v",
			humanize.Bytes(memstats.HeapInuse),
			humanize.Bytes(memstats.Alloc),
			humanize.Bytes(memstats.Sys))
		runtime.GC()
		debug.FreeOSMemory()
	}
}

// var proxyConfig = `
// fp-cloudcompile:
//   addr: "67.205.172.79:443"
//   multiplexedaddr: "67.205.172.79:444"
//   cert: "-----BEGIN CERTIFICATE-----\nMIIDHzCCAgegAwIBAgIIFPWhFLs4/GMwDQYJKoZIhvcNAQELBQAwHzEQMA4GA1UE\nChMHTGFudGVybjELMAkGA1UEAxMCOjowHhcNMTcxMDEwMDUwNjAwWhcNMjcxMTEw\nMDUwNTU4WjAfMRAwDgYDVQQKEwdMYW50ZXJuMQswCQYDVQQDEwI6OjCCASIwDQYJ\nKoZIhvcNAQEBBQADggEPADCCAQoCggEBAM+1qr0v/5droAUt6P2QRX7C60S4kRdT\nm5M5xPmAMF19bOf+Pk4gKH36/9gzMc3ESTOt5Ij4/3r01vKux7+tMJSLaJtGaB2w\n55NdHOkVyoQf6wnznI6XC+YtE3AN63WzDZnMXw5ADIaRgUPe8qojp5+sWCuXsGvi\nBQZClga/GMmXNLMT4EIjNqC+MfkTttJccRPqahUY1AbUHgUDRy5M0zfIR2XYHmOc\n6mtogbkdalGIKRxzBe/Db1hKgBzojuT+Pxt5L8boWp4MxgGIqBbFxqgvnuVh9Jmj\nlqVUMRqg1pK3MqGHNuAl8Q/e14Y8nmpFjQAFiFnhwtCNP4ZjHaF3oCMCAwEAAaNf\nMF0wDgYDVR0PAQH/BAQDAgKkMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD\nAjAPBgNVHRMBAf8EBTADAQH/MBsGA1UdEQQUMBKHEAAAAAAAAAAAAAAAAAAAAAAw\nDQYJKoZIhvcNAQELBQADggEBAMK5YYj6KiXe/hmC0SF3DdkdW8ZqU8/LlQUuTO6O\nblDqiubvuscz1B2h+TRB5A2ebWukYDoBCurNIbFOmQA1TdBdjF5EvGAVj5QJm3QJ\nrXdWwbVfhdIy8VMrNag1qhqM7XDsLOVA7VKssU78u0nDINun/4cVngbcsqmjjyeM\nYVWgsfwm5mGU01gDYG1Wg7XIb+JNT5ynAv2DnoNCwvLp3UUBJY3sP9r1GD8gem/T\nfyQ8GtB73BYLRSDaocASwVNx/9putdsdkiMx8l4T3z4owqVQ6gGxFUFJRWs0Idww\nsiRLNHqPVZEg32jr++Tm4yBclMNJc62/8FUqAa5G0KtYmjg=\n-----END CERTIFICATE-----"
//   authtoken: "pj6mWPafKzP26KZvUf7FIs24eB2ubjUKFvXktodqgUzZULhGeRUT0mwhyHb9jY2b"
//   trusted: true`

var proxyConfig = `
fp-cloudcompile-lampshade:
  addr: "67.205.172.79:14443"
  cert: "-----BEGIN CERTIFICATE-----\nMIIDHzCCAgegAwIBAgIIFPWhFLs4/GMwDQYJKoZIhvcNAQELBQAwHzEQMA4GA1UE\nChMHTGFudGVybjELMAkGA1UEAxMCOjowHhcNMTcxMDEwMDUwNjAwWhcNMjcxMTEw\nMDUwNTU4WjAfMRAwDgYDVQQKEwdMYW50ZXJuMQswCQYDVQQDEwI6OjCCASIwDQYJ\nKoZIhvcNAQEBBQADggEPADCCAQoCggEBAM+1qr0v/5droAUt6P2QRX7C60S4kRdT\nm5M5xPmAMF19bOf+Pk4gKH36/9gzMc3ESTOt5Ij4/3r01vKux7+tMJSLaJtGaB2w\n55NdHOkVyoQf6wnznI6XC+YtE3AN63WzDZnMXw5ADIaRgUPe8qojp5+sWCuXsGvi\nBQZClga/GMmXNLMT4EIjNqC+MfkTttJccRPqahUY1AbUHgUDRy5M0zfIR2XYHmOc\n6mtogbkdalGIKRxzBe/Db1hKgBzojuT+Pxt5L8boWp4MxgGIqBbFxqgvnuVh9Jmj\nlqVUMRqg1pK3MqGHNuAl8Q/e14Y8nmpFjQAFiFnhwtCNP4ZjHaF3oCMCAwEAAaNf\nMF0wDgYDVR0PAQH/BAQDAgKkMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD\nAjAPBgNVHRMBAf8EBTADAQH/MBsGA1UdEQQUMBKHEAAAAAAAAAAAAAAAAAAAAAAw\nDQYJKoZIhvcNAQELBQADggEBAMK5YYj6KiXe/hmC0SF3DdkdW8ZqU8/LlQUuTO6O\nblDqiubvuscz1B2h+TRB5A2ebWukYDoBCurNIbFOmQA1TdBdjF5EvGAVj5QJm3QJ\nrXdWwbVfhdIy8VMrNag1qhqM7XDsLOVA7VKssU78u0nDINun/4cVngbcsqmjjyeM\nYVWgsfwm5mGU01gDYG1Wg7XIb+JNT5ynAv2DnoNCwvLp3UUBJY3sP9r1GD8gem/T\nfyQ8GtB73BYLRSDaocASwVNx/9putdsdkiMx8l4T3z4owqVQ6gGxFUFJRWs0Idww\nsiRLNHqPVZEg32jr++Tm4yBclMNJc62/8FUqAa5G0KtYmjg=\n-----END CERTIFICATE-----"
  authtoken: "pj6mWPafKzP26KZvUf7FIs24eB2ubjUKFvXktodqgUzZULhGeRUT0mwhyHb9jY2b"
  trusted: true
  pluggabletransport: lampshade`
