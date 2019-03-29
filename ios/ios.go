package ios

import (
	"context"
	"io"
	"net"
	"net/http"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/packetforward"
	"github.com/getlantern/proxy"
	"github.com/getlantern/proxy/filters"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"

	"github.com/dustin/go-humanize"
)

const (
	memLimitInMiB   = 12
	memLimitInBytes = memLimitInMiB * 1024 * 1024
)

var (
	log = golog.LoggerFor("ios")
)

type Writer interface {
	Write([]byte) bool
}

type writerAdapter struct {
	Writer
}

func (wa *writerAdapter) Write(b []byte) (int, error) {
	ok := wa.Writer.Write(b)
	if !ok {
		return 0, errors.New("error writing")
	}
	return len(b), nil
}

type WriteCloser interface {
	Write([]byte) error

	Close() error
}

type wc struct {
	io.Writer
	bal *balancer.Balancer
}

func (c *wc) Write(b []byte) error {
	_, err := c.Writer.Write(b)
	return err
}

func (c *wc) Close() error {
	c.bal.Close()
	return nil
}

type client struct {
	proxy proxy.Proxy
}

// ClientOpts configures a client
type ClientOpts struct {
	ConfigDir string
	MTU       int
}

func Client(packetsOut Writer, opts *ClientOpts) (WriteCloser, error) {
	go trackAndLimitMemory()

	dialers, err := loadDialers(opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	bal := balancer.New(30*time.Second, dialers...)

	w := packetforward.Client(&writerAdapter{packetsOut}, opts.MTU, 30*time.Second, func(ctx context.Context) (net.Conn, error) {
		return bal.DialContext(ctx, "connect", "127.0.0.1:3000")
	})
	return &wc{w, bal}, nil
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

func loadDialers(configDir string) ([]balancer.Dialer, error) {
	c := &configurer{configFolderPath: configDir}
	proxies := make(map[string]*chained.ChainedServerInfo)
	_, _, err := c.openConfig(proxiesYaml, proxies, []byte{})
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

// func loadProxies() (map[string]*chained.ChainedServerInfo, error) {
// 	proxies := make(map[string]*chained.ChainedServerInfo, 1)
// 	err := yaml.Unmarshal([]byte(proxyConfig), proxies)
// 	if err != nil {
// 		return nil, errors.New("Unable to unmarshal proxyConfig: %v", err)
// 	}
// 	return proxies, nil
// }

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
// fp-cloudcompile-tom:
//   addr: "69.55.55.226:443"
//   cert: "-----BEGIN CERTIFICATE-----\nMIIDHzCCAgegAwIBAgIIFPWhFLs4/GMwDQYJKoZIhvcNAQELBQAwHzEQMA4GA1UE\nChMHTGFudGVybjELMAkGA1UEAxMCOjowHhcNMTcxMDEwMDUwNjAwWhcNMjcxMTEw\nMDUwNTU4WjAfMRAwDgYDVQQKEwdMYW50ZXJuMQswCQYDVQQDEwI6OjCCASIwDQYJ\nKoZIhvcNAQEBBQADggEPADCCAQoCggEBAM+1qr0v/5droAUt6P2QRX7C60S4kRdT\nm5M5xPmAMF19bOf+Pk4gKH36/9gzMc3ESTOt5Ij4/3r01vKux7+tMJSLaJtGaB2w\n55NdHOkVyoQf6wnznI6XC+YtE3AN63WzDZnMXw5ADIaRgUPe8qojp5+sWCuXsGvi\nBQZClga/GMmXNLMT4EIjNqC+MfkTttJccRPqahUY1AbUHgUDRy5M0zfIR2XYHmOc\n6mtogbkdalGIKRxzBe/Db1hKgBzojuT+Pxt5L8boWp4MxgGIqBbFxqgvnuVh9Jmj\nlqVUMRqg1pK3MqGHNuAl8Q/e14Y8nmpFjQAFiFnhwtCNP4ZjHaF3oCMCAwEAAaNf\nMF0wDgYDVR0PAQH/BAQDAgKkMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD\nAjAPBgNVHRMBAf8EBTADAQH/MBsGA1UdEQQUMBKHEAAAAAAAAAAAAAAAAAAAAAAw\nDQYJKoZIhvcNAQELBQADggEBAMK5YYj6KiXe/hmC0SF3DdkdW8ZqU8/LlQUuTO6O\nblDqiubvuscz1B2h+TRB5A2ebWukYDoBCurNIbFOmQA1TdBdjF5EvGAVj5QJm3QJ\nrXdWwbVfhdIy8VMrNag1qhqM7XDsLOVA7VKssU78u0nDINun/4cVngbcsqmjjyeM\nYVWgsfwm5mGU01gDYG1Wg7XIb+JNT5ynAv2DnoNCwvLp3UUBJY3sP9r1GD8gem/T\nfyQ8GtB73BYLRSDaocASwVNx/9putdsdkiMx8l4T3z4owqVQ6gGxFUFJRWs0Idww\nsiRLNHqPVZEg32jr++Tm4yBclMNJc62/8FUqAa5G0KtYmjg=\n-----END CERTIFICATE-----"
//   authtoken: "pj6mWPafKzP26KZvUf7FIs24eB2ubjUKFvXktodqgUzZULhGeRUT0mwhyHb9jY2b"
//   trusted: true`

var proxyConfig = `
fp-cloudcompile-tom-lampshade:
  addr: "69.55.55.226:14443"
  cert: "-----BEGIN CERTIFICATE-----\nMIIDHzCCAgegAwIBAgIIFPWhFLs4/GMwDQYJKoZIhvcNAQELBQAwHzEQMA4GA1UE\nChMHTGFudGVybjELMAkGA1UEAxMCOjowHhcNMTcxMDEwMDUwNjAwWhcNMjcxMTEw\nMDUwNTU4WjAfMRAwDgYDVQQKEwdMYW50ZXJuMQswCQYDVQQDEwI6OjCCASIwDQYJ\nKoZIhvcNAQEBBQADggEPADCCAQoCggEBAM+1qr0v/5droAUt6P2QRX7C60S4kRdT\nm5M5xPmAMF19bOf+Pk4gKH36/9gzMc3ESTOt5Ij4/3r01vKux7+tMJSLaJtGaB2w\n55NdHOkVyoQf6wnznI6XC+YtE3AN63WzDZnMXw5ADIaRgUPe8qojp5+sWCuXsGvi\nBQZClga/GMmXNLMT4EIjNqC+MfkTttJccRPqahUY1AbUHgUDRy5M0zfIR2XYHmOc\n6mtogbkdalGIKRxzBe/Db1hKgBzojuT+Pxt5L8boWp4MxgGIqBbFxqgvnuVh9Jmj\nlqVUMRqg1pK3MqGHNuAl8Q/e14Y8nmpFjQAFiFnhwtCNP4ZjHaF3oCMCAwEAAaNf\nMF0wDgYDVR0PAQH/BAQDAgKkMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD\nAjAPBgNVHRMBAf8EBTADAQH/MBsGA1UdEQQUMBKHEAAAAAAAAAAAAAAAAAAAAAAw\nDQYJKoZIhvcNAQELBQADggEBAMK5YYj6KiXe/hmC0SF3DdkdW8ZqU8/LlQUuTO6O\nblDqiubvuscz1B2h+TRB5A2ebWukYDoBCurNIbFOmQA1TdBdjF5EvGAVj5QJm3QJ\nrXdWwbVfhdIy8VMrNag1qhqM7XDsLOVA7VKssU78u0nDINun/4cVngbcsqmjjyeM\nYVWgsfwm5mGU01gDYG1Wg7XIb+JNT5ynAv2DnoNCwvLp3UUBJY3sP9r1GD8gem/T\nfyQ8GtB73BYLRSDaocASwVNx/9putdsdkiMx8l4T3z4owqVQ6gGxFUFJRWs0Idww\nsiRLNHqPVZEg32jr++Tm4yBclMNJc62/8FUqAa5G0KtYmjg=\n-----END CERTIFICATE-----"
  authtoken: "pj6mWPafKzP26KZvUf7FIs24eB2ubjUKFvXktodqgUzZULhGeRUT0mwhyHb9jY2b"
  trusted: true
  pluggabletransport: lampshade
  pluggabletransportsettings:
    windowsize: 250`

// var proxyConfig = `
// fp-cloudcompile:
//   addr: "67.205.172.79:443"
//   multiplexedaddr: "67.205.172.79:444"
//   cert: "-----BEGIN CERTIFICATE-----\nMIIDHzCCAgegAwIBAgIIFPWhFLs4/GMwDQYJKoZIhvcNAQELBQAwHzEQMA4GA1UE\nChMHTGFudGVybjELMAkGA1UEAxMCOjowHhcNMTcxMDEwMDUwNjAwWhcNMjcxMTEw\nMDUwNTU4WjAfMRAwDgYDVQQKEwdMYW50ZXJuMQswCQYDVQQDEwI6OjCCASIwDQYJ\nKoZIhvcNAQEBBQADggEPADCCAQoCggEBAM+1qr0v/5droAUt6P2QRX7C60S4kRdT\nm5M5xPmAMF19bOf+Pk4gKH36/9gzMc3ESTOt5Ij4/3r01vKux7+tMJSLaJtGaB2w\n55NdHOkVyoQf6wnznI6XC+YtE3AN63WzDZnMXw5ADIaRgUPe8qojp5+sWCuXsGvi\nBQZClga/GMmXNLMT4EIjNqC+MfkTttJccRPqahUY1AbUHgUDRy5M0zfIR2XYHmOc\n6mtogbkdalGIKRxzBe/Db1hKgBzojuT+Pxt5L8boWp4MxgGIqBbFxqgvnuVh9Jmj\nlqVUMRqg1pK3MqGHNuAl8Q/e14Y8nmpFjQAFiFnhwtCNP4ZjHaF3oCMCAwEAAaNf\nMF0wDgYDVR0PAQH/BAQDAgKkMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD\nAjAPBgNVHRMBAf8EBTADAQH/MBsGA1UdEQQUMBKHEAAAAAAAAAAAAAAAAAAAAAAw\nDQYJKoZIhvcNAQELBQADggEBAMK5YYj6KiXe/hmC0SF3DdkdW8ZqU8/LlQUuTO6O\nblDqiubvuscz1B2h+TRB5A2ebWukYDoBCurNIbFOmQA1TdBdjF5EvGAVj5QJm3QJ\nrXdWwbVfhdIy8VMrNag1qhqM7XDsLOVA7VKssU78u0nDINun/4cVngbcsqmjjyeM\nYVWgsfwm5mGU01gDYG1Wg7XIb+JNT5ynAv2DnoNCwvLp3UUBJY3sP9r1GD8gem/T\nfyQ8GtB73BYLRSDaocASwVNx/9putdsdkiMx8l4T3z4owqVQ6gGxFUFJRWs0Idww\nsiRLNHqPVZEg32jr++Tm4yBclMNJc62/8FUqAa5G0KtYmjg=\n-----END CERTIFICATE-----"
//   authtoken: "pj6mWPafKzP26KZvUf7FIs24eB2ubjUKFvXktodqgUzZULhGeRUT0mwhyHb9jY2b"
//   trusted: true`

// var proxyConfig = `
// fp-cloudcompile-lampshade:
//   addr: "67.205.172.79:14443"
//   cert: "-----BEGIN CERTIFICATE-----\nMIIDHzCCAgegAwIBAgIIFPWhFLs4/GMwDQYJKoZIhvcNAQELBQAwHzEQMA4GA1UE\nChMHTGFudGVybjELMAkGA1UEAxMCOjowHhcNMTcxMDEwMDUwNjAwWhcNMjcxMTEw\nMDUwNTU4WjAfMRAwDgYDVQQKEwdMYW50ZXJuMQswCQYDVQQDEwI6OjCCASIwDQYJ\nKoZIhvcNAQEBBQADggEPADCCAQoCggEBAM+1qr0v/5droAUt6P2QRX7C60S4kRdT\nm5M5xPmAMF19bOf+Pk4gKH36/9gzMc3ESTOt5Ij4/3r01vKux7+tMJSLaJtGaB2w\n55NdHOkVyoQf6wnznI6XC+YtE3AN63WzDZnMXw5ADIaRgUPe8qojp5+sWCuXsGvi\nBQZClga/GMmXNLMT4EIjNqC+MfkTttJccRPqahUY1AbUHgUDRy5M0zfIR2XYHmOc\n6mtogbkdalGIKRxzBe/Db1hKgBzojuT+Pxt5L8boWp4MxgGIqBbFxqgvnuVh9Jmj\nlqVUMRqg1pK3MqGHNuAl8Q/e14Y8nmpFjQAFiFnhwtCNP4ZjHaF3oCMCAwEAAaNf\nMF0wDgYDVR0PAQH/BAQDAgKkMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD\nAjAPBgNVHRMBAf8EBTADAQH/MBsGA1UdEQQUMBKHEAAAAAAAAAAAAAAAAAAAAAAw\nDQYJKoZIhvcNAQELBQADggEBAMK5YYj6KiXe/hmC0SF3DdkdW8ZqU8/LlQUuTO6O\nblDqiubvuscz1B2h+TRB5A2ebWukYDoBCurNIbFOmQA1TdBdjF5EvGAVj5QJm3QJ\nrXdWwbVfhdIy8VMrNag1qhqM7XDsLOVA7VKssU78u0nDINun/4cVngbcsqmjjyeM\nYVWgsfwm5mGU01gDYG1Wg7XIb+JNT5ynAv2DnoNCwvLp3UUBJY3sP9r1GD8gem/T\nfyQ8GtB73BYLRSDaocASwVNx/9putdsdkiMx8l4T3z4owqVQ6gGxFUFJRWs0Idww\nsiRLNHqPVZEg32jr++Tm4yBclMNJc62/8FUqAa5G0KtYmjg=\n-----END CERTIFICATE-----"
//   authtoken: "pj6mWPafKzP26KZvUf7FIs24eB2ubjUKFvXktodqgUzZULhGeRUT0mwhyHb9jY2b"
//   trusted: true
//   pluggabletransport: lampshade`
