// Package proxybench provides a mechanism for benchmarking proxies that are
// running with the -bench flag.
package proxybench

import (
	"crypto/rsa"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/enproxy"
	"github.com/getlantern/golog"
	"github.com/getlantern/keyman"
	"github.com/getlantern/lampshade"
	"github.com/getlantern/mtime"
	"github.com/getlantern/netx"
	"github.com/getlantern/ops"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"gitlab.com/yawning/obfs4.git/transports/obfs4"
)

const (
	serverCert = "-----BEGIN CERTIFICATE-----\nMIIDHzCCAgegAwIBAgIIFKPif4XAzmkwDQYJKoZIhvcNAQELBQAwHzEQMA4GA1UE\nChMHTGFudGVybjELMAkGA1UEAxMCOjowHhcNMTcwMTE2MjE0MjE5WhcNMjcwMjE2\nMjE0MjE4WjAfMRAwDgYDVQQKEwdMYW50ZXJuMQswCQYDVQQDEwI6OjCCASIwDQYJ\nKoZIhvcNAQEBBQADggEPADCCAQoCggEBAKL8CVzRcPMuU75ymNhT+76Xt9ewvBUK\nBfQQ272zHT0qqK8qZ3LG3oS/I06y9w1ZB2tBci+BOIRbMD5HBLgJynZGEUvO4wVg\n/8pCftSWiJQrcAVtXKFeEDzrccs6IPWH5f6Xxlws3GBqSbmMEsnsL0+Hlv01pwSH\np9xxew+d9FvPLilKC95rbSJBYoNNDw/iS+gpGWiRwU0SwMinuvGlKHY03my1tRU5\naFNEw1XU/y4Lm8uF10/+s6A0abdhKvNhCib73JFegRRuWUc5Z/mSlozWG2PXr5sc\nuJiOCJs2IGS2WVWWSzcXULcpsTVU0CWcUtvboQ0g/AqjKZPNkp6ZBikCAwEAAaNf\nMF0wDgYDVR0PAQH/BAQDAgKkMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcD\nAjAPBgNVHRMBAf8EBTADAQH/MBsGA1UdEQQUMBKHEAAAAAAAAAAAAAAAAAAAAAAw\nDQYJKoZIhvcNAQELBQADggEBAF84xZiezhR3QX4ikkWnuGCgrCGEMOqhi70st0/7\nXSnmkhJeju59TS+s2M+zdjGwBH2MkOx27u6apVvXb9McvhgkVwDqMWJkgFOpjd9f\nd8ksXNqTbc+dQwZ2oTB1ZRc+g0c+l6vuTLPY/UcXixvM13yMitTE8wKA8Pf9W9za\nHqDWy5kXdM+YBqK5Kf6jPfWpz953If1qHO+pRcNEUoswpD+oIxxInXVZ1ajSi9g0\nK7drghpGkRs52+nhyQs3Yi/RItRwZJDuM3u0Or1fD34OH/olsUlrFNi5njqL+5h8\nu1qLOIc4sSXdbMhkH/me7SUxsGV+qhaBzSn5Hi+W1rZNHgE=\n-----END CERTIFICATE-----"
)

const (
	direct = "direct"
	cdn    = "cdn"
)

var (
	log                = golog.LoggerFor("proxybench")
	buffers            = lampshade.NewBufferPool(1024768)
	supportedProtocols = map[string]bool{
		"https":     true,
		"obfs4":     true,
		"lampshade": true,
		"enproxy":   true,
		cdn:         true,
	}
	testingProxy = ""

	directProxy = &Proxy{&ProxyCfg{Provider: direct, DataCenter: direct}, direct, "", nil}
)

type ProxyCfg struct {
	Addrs      map[string]string `json:"addrs"`
	Provider   string            `json:"provider"`
	DataCenter string            `json:"dataCenter"`
}

func (p *ProxyCfg) withRandomProtocol() *Proxy {
	protocols := make([]string, 0, len(p.Addrs))
	for protocol := range p.Addrs {
		if supportedProtocols[protocol] {
			protocols = append(protocols, protocol)
		}
	}
	sort.Strings(protocols)
	protocol := protocols[rand.Intn(len(protocols))]
	return p.WithProtocolAndDialer(protocol, nil)
}

func (p *ProxyCfg) WithProtocolAndDialer(protocol string, dial func(network, addr string) (net.Conn, error)) *Proxy {
	if dial == nil {
		dial = netx.Dial
	}
	return &Proxy{p, protocol, p.Addrs[protocol], dial}
}

type Proxy struct {
	*ProxyCfg
	protocol string
	addr     string
	dial     func(network, addr string) (net.Conn, error)
}

func (p *Proxy) String() string {
	return fmt.Sprintf("%v - %v (%v)", p.Provider, p.DataCenter, p.protocol)
}

type Opts struct {
	SampleRate   float64 `json:"sampleRate"`
	Period       time.Duration
	PeriodString string      `json:"period"`
	Proxies      []*ProxyCfg `json:"proxies"`
	URLs         []string    `json:"urls"`
	DirectURLs   []string    `json:"directURLs"`
	UpdateURL    string      `json:"updateURL"`
}

func (opts *Opts) applyDefaults() {
	testingMode := testingProxy != ""
	if opts.PeriodString != "" {
		opts.Period, _ = time.ParseDuration(opts.PeriodString)
	}
	if opts.Period <= 0 {
		opts.Period = 1 * time.Hour
	}
	if opts.SampleRate == 0 {
		opts.SampleRate = 0.05 // 5%
	}
	if opts.UpdateURL == "" && !testingMode {
		opts.UpdateURL = "https://s3.amazonaws.com/lantern/proxybench.json"
	}
	if testingMode {
		log.Debug("Overriding urls and proxy in testing mode")
		opts.SampleRate = 1
		opts.URLs = []string{"http://i.ytimg.com/vi/video_id/0.jpg"}
		opts.DirectURLs = []string{
			"http://edition.cnn.com/404",
			"https://edition.cnn.com/404",
		}
		opts.Proxies = []*ProxyCfg{&ProxyCfg{Addrs: map[string]string{"https": testingProxy}, Provider: "testingProvider", DataCenter: "testingDC"}}
	}
}

type ReportFN func(timing time.Duration, ctx map[string]interface{})

// LoadDefaultOpts loads the default options from Amazon S3
func LoadDefaultOpts() (*Opts, error) {
	opts := &Opts{}
	opts.applyDefaults()
	return opts.fetchUpdate()
}

func Start(opts *Opts, report ReportFN) {
	opts.applyDefaults()

	ops.Go(func() {
		var err error
		for {
			opts, err = opts.fetchUpdate()
			if err != nil {
				log.Error(err)
				// Continue processing though, since we still have the old opts
			}
			if rand.Float64() < opts.SampleRate {
				log.Debugf("Running benchmarks")
				bench(opts, report)
			}
			// Add +/- 20% to sleep time
			sleepPeriod := time.Duration(float64(opts.Period) * (1.0 + (rand.Float64()-1.0)/5))
			log.Debugf("Waiting %v before running again", sleepPeriod)
			time.Sleep(sleepPeriod)
		}
	})
}

func bench(opts *Opts, report ReportFN) {
	// Shuffle the list of proxies to so we're not always hitting them in the same
	// order (and thus biasing towards testing earlier proxies more).
	for i := range opts.Proxies {
		j := rand.Intn(i + 1)
		opts.Proxies[i], opts.Proxies[j] = opts.Proxies[j], opts.Proxies[i]
	}

	for _, proxy := range opts.Proxies {
		for _, origin := range opts.URLs {
			request(report, origin, proxy.withRandomProtocol())

		}
	}

	for _, origin := range opts.DirectURLs {
		doRequest(report, origin, directProxy, "")
	}
}

func request(report ReportFN, origin string, proxy *Proxy) {
	// http.Transport can't talk to HTTPS proxies, so we need an intermediary.
	l, err := setupLocalProxy(proxy)
	if err != nil {
		log.Errorf("Unable to set up local proxy for %v: %v", proxy.addr, err)
		return
	}
	doRequest(report, origin, proxy, l.Addr().String())
	l.Close()
}

func doRequest(report ReportFN, origin string, proxy *Proxy, addr string) {
	op := ops.Begin("proxybench").
		Set("url", origin).
		Set("proxy_protocol", proxy.protocol).
		Set("proxy_provider", proxy.Provider).
		Set("proxy_datacenter", proxy.DataCenter)
	defer op.End()
	if proxy.protocol != direct {
		op.Set("proxy_type", "chained")
		host, port, _ := net.SplitHostPort(proxy.addr)
		op.Set("proxy_host", host).Set("proxy_port", port)
	}

	log.Debug("Making request")
	client := &http.Client{
		Timeout: 1 * time.Minute,
	}
	defer op.End()

	fail := func(msg string, params ...interface{}) {
		op.FailIf(log.Errorf(msg, params...))
	}

	if proxy.protocol == cdn {
		host, _, err := net.SplitHostPort(proxy.addr)
		if err != nil {
			fail("Unable to parse proxy addr %s: %v, ", proxy.addr, err)
			return
		}
		alias, err := url.Parse(origin)
		if err != nil {
			fail("Unable to parse origin url %s: %v, ", origin, err)
			return
		}
		alias.Host = host
		alias.Scheme = "https"
		origin = alias.String()

		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	} else if proxy.protocol != direct {
		client.Transport = &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				// Note - we're using HTTP here, but this is talking to the local proxy,
				// which talks HTTPS to the remote proxy.
				return url.Parse("http://" + addr)
			},
			DisableKeepAlives: true,
		}
	}

	elapsed := mtime.Stopwatch()
	req, err := http.NewRequest("GET", origin, nil)
	if err != nil {
		fail("Unable to build request for %v: %v", origin, err)
		return
	}
	op.Set("origin", req.URL.Host).Set("origin_host", req.URL.Host)
	resp, err := client.Do(req)
	if err != nil {
		fail("Error fetching %v from %v: %v", origin, proxy, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		fail("Unexpected status %v fetching %v from %v: %v", resp.Status, origin, proxy, err)
		return
	}
	// Read the full response body
	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		fail("Error reading full response body for %v from %v: %v", origin, proxy, err)
		return
	}
	delta := elapsed()
	log.Debugf("Request succeeded in %v", delta)
	op.Set("proxybench_success", true)
	op.Set("response_time", borda.Avg(float64(delta.Seconds())))
	report(delta, ops.AsMap(op, true))
}

func setupLocalProxy(proxy *Proxy) (net.Listener, error) {
	l, err := net.Listen("tcp", "localhost:")
	if err != nil {
		return nil, err
	}
	go func() {
		in, err := l.Accept()
		if err != nil {
			log.Errorf("Unable to accept connection: %v", err)
			return
		}
		go doLocalProxy(in, proxy)
	}()
	return l, nil
}

func doLocalProxy(in net.Conn, proxy *Proxy) {
	defer in.Close()
	out, err := proxy.Dial()
	if err != nil {
		log.Debugf("Unable to dial proxy %v: %v", proxy, err)
		return
	}
	defer out.Close()
	bufOut := buffers.Get()
	bufIn := buffers.Get()
	defer buffers.Put(bufOut)
	defer buffers.Put(bufIn)
	outErr, inErr := netx.BidiCopy(out, in, bufOut, bufIn)
	if outErr != nil {
		log.Debugf("Error copying to local proxy from %v: %v", proxy, outErr)
	}
	if inErr != nil {
		log.Debugf("Error copying from local proxy to %v: %v", proxy, inErr)
	}
}

func (p *Proxy) Dial() (net.Conn, error) {
	switch p.protocol {
	case "https":
		return p.DialTLS()
	case "obfs4":
		return p.DialOBFS4()
	case "lampshade":
		return p.DialLampshade()
	case "enproxy":
		return p.DialEnproxy()
	default:
		return nil, fmt.Errorf("Unknown protocol %v", p.protocol)
	}
}

func (p *Proxy) DialTLS() (net.Conn, error) {
	conn, err := p.dial("tcp", p.addr)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(conn, &tls.Config{
		InsecureSkipVerify: true,
	})
	return tlsConn, nil
}

func (p *Proxy) DialLampshade() (net.Conn, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(serverCert))
	if err != nil {
		return nil, err
	}
	dialer := lampshade.NewDialer(&lampshade.DialerOpts{
		Pool:            buffers,
		Cipher:          lampshade.ChaCha20Poly1305,
		ServerPublicKey: cert.X509().PublicKey.(*rsa.PublicKey)})
	return dialer.Dial(func() (net.Conn, error) {
		return p.dial("tcp", p.addr)
	})
}

func (p *Proxy) DialOBFS4() (net.Conn, error) {
	tr := obfs4.Transport{}
	cf, err := tr.ClientFactory("")
	if err != nil {
		return nil, log.Errorf("Unable to create obfs4 client factory: %v", err)
	}

	ptArgs := &pt.Args{}
	ptArgs.Add("cert", "1LYfzzTyz7xsu0bTBUJacwDTLN3NU/gNSjC+pfdRVNuh/LYmtbLOlhZwCfNTKyUVvfMTWQ")
	ptArgs.Add("iat-mode", "0")

	args, err := cf.ParseArgs(ptArgs)
	if err != nil {
		return nil, log.Errorf("Unable to parse client args: %v", err)
	}
	return cf.Dial("tcp", p.addr, p.dial, args)
}

func (p *Proxy) DialEnproxy() (net.Conn, error) {

	enproxyConfig := &enproxy.Config{
		DialProxy: func(addr string) (net.Conn, error) {
			return p.DialTLS()
		},
		NewRequest: func(host, path, method string, body io.Reader) (*http.Request, error) {
			if host == "" {
				host = p.addr
			}
			req, err := http.NewRequest(method, fmt.Sprintf("https://%s/%s/", host, path), body)
			if err != nil {
				return nil, fmt.Errorf("dial enproxy: %v", err)
			}
			ephost, _, _ := net.SplitHostPort(p.addr)
			req.Host = ephost
			return req, nil
		},
	}

	// always contact https proxy local to the enproxy server
	epconn, err := enproxy.Dial("localhost:443", enproxyConfig)
	if err != nil {
		return nil, fmt.Errorf("dial enproxy: %v", err)
	}
	tlsConn := tls.Client(epconn, &tls.Config{
		InsecureSkipVerify: true,
	})
	return tlsConn, nil

}

func (opts *Opts) fetchUpdate() (*Opts, error) {
	if opts.UpdateURL == "" {
		log.Debug("Not fetching updated options")
		return opts, nil
	}
	resp, err := http.Get(opts.UpdateURL)
	if err != nil {
		return opts, fmt.Errorf("Unable to fetch updated Opts from %v: %v", opts.UpdateURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return opts, fmt.Errorf("Unexpected response status fetching updated Opts from %v: %v", opts.UpdateURL, resp.Status)
	}
	newOpts := &Opts{}
	err = json.NewDecoder(resp.Body).Decode(newOpts)
	if err != nil {
		return opts, fmt.Errorf("Error decoding JSON for updated Opts from %v: %v", opts.UpdateURL, err)
	}
	newOpts.applyDefaults()
	log.Debug("Applying updated options")
	return newOpts, nil
}
