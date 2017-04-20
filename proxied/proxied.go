// Package proxied provides  http.Client implementations that use various
// combinations of chained and direct domain-fronted proxies.
//
// Remember to call SetProxyAddr before obtaining an http.Client.
package proxied

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"
	"github.com/getlantern/keyman"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
)

const (
	forceDF           = "FORCE_DOMAINFRONT"
	lanternFrontedURL = "Lantern-Fronted-URL"
)

var (
	log = golog.LoggerFor("flashlight.proxied")

	proxyAddrMutex sync.RWMutex
	proxyAddr      = eventual.DefaultUnsetGetter()

	// ErrChainedProxyUnavailable indicates that we weren't able to find a chained
	// proxy.
	ErrChainedProxyUnavailable = "chained proxy unavailable"

	// Shared client session cache for all connections
	clientSessionCache = tls.NewLRUClientSessionCache(1000)
)

func success(resp *http.Response) bool {
	return resp.StatusCode > 199 && resp.StatusCode < 400
}

// changeUserAgent prepends Lantern version and OSARCH to the User-Agent header
// of req to facilitate debugging on server side.
func changeUserAgent(req *http.Request) {
	secondary := req.Header.Get("User-Agent")
	ua := strings.TrimSpace(fmt.Sprintf("Lantern/%s (%s/%s) %s",
		common.Version, runtime.GOOS, runtime.GOARCH, secondary))
	req.Header.Set("User-Agent", ua)
}

// SetProxyAddr sets the eventual.Getter that's used to determine the proxy's
// address. This MUST be called before attempting to use the proxied package.
func SetProxyAddr(addr eventual.Getter) {
	proxyAddrMutex.Lock()
	proxyAddr = addr
	proxyAddrMutex.Unlock()
}

func getProxyAddr() (string, bool) {
	proxyAddrMutex.RLock()
	addr, ok := proxyAddr(1 * time.Minute)
	proxyAddrMutex.RUnlock()
	if !ok {
		return "", false
	}
	return addr.(string), true
}

// ParallelPreferChained creates a new http.RoundTripper that attempts to send
// requests through both chained and direct fronted routes in parallel. Once a
// chained request succeeds, subsequent requests will only go through Chained
// servers unless and until a request fails, in which case we'll start trying
// fronted requests again.
func ParallelPreferChained() http.RoundTripper {
	cf := &chainedAndFronted{
		parallel: true,
	}
	cf.setFetcher(&dualFetcher{cf})
	return cf
}

// ChainedThenFronted creates a new http.RoundTripper that attempts to send
// requests first through a chained server and then falls back to using a
// direct fronted server if the chained route didn't work.
func ChainedThenFronted() http.RoundTripper {
	cf := &chainedAndFronted{
		parallel: false,
	}
	cf.setFetcher(&dualFetcher{cf})
	return cf
}

// chainedAndFronted fetches HTTP data in parallel using both chained and fronted
// servers.
type chainedAndFronted struct {
	parallel bool
	_fetcher http.RoundTripper
	mu       sync.RWMutex
}

func (cf *chainedAndFronted) getFetcher() http.RoundTripper {
	cf.mu.RLock()
	result := cf._fetcher
	cf.mu.RUnlock()
	return result
}

func (cf *chainedAndFronted) setFetcher(fetcher http.RoundTripper) {
	cf.mu.Lock()
	cf._fetcher = fetcher
	cf.mu.Unlock()
}

// RoundTrip will attempt to execute the specified HTTP request using only a chained fetcher
func (cf *chainedAndFronted) RoundTrip(req *http.Request) (*http.Response, error) {
	op := ops.Begin("chainedandfronted").Request(req)
	defer op.End()

	resp, err := cf.getFetcher().RoundTrip(req)
	op.Response(resp)
	if err != nil {
		log.Error(err)
		// If there's an error, switch back to using the dual fetcher.
		cf.setFetcher(&dualFetcher{cf})
	} else if !success(resp) {
		log.Error(resp.Status)
		cf.setFetcher(&dualFetcher{cf})
	}
	return resp, err
}

type chainedFetcher struct {
}

// RoundTrip will attempt to execute the specified HTTP request using only a chained fetcher
func (cf *chainedFetcher) RoundTrip(req *http.Request) (*http.Response, error) {
	log.Debugf("Using chained fetcher")
	rt, err := ChainedNonPersistent("")
	if err != nil {
		return nil, err
	}
	return rt.RoundTrip(req)
}

type dualFetcher struct {
	cf *chainedAndFronted
}

type frontedRT struct{}

// Use a wrapper for fronted.NewDirect to avoid blocking
// `dualFetcher.RoundTrip` when fronted is not yet available, especially when
// the application is starting up
func (f frontedRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := fronted.NewDirect(5 * time.Minute)
	changeUserAgent(req)
	return rt.RoundTrip(req)
}

// RoundTrip will attempt to execute the specified HTTP request using both
// chained and fronted servers, simply returning the first response to
// arrive. Callers MUST use the Lantern-Fronted-URL HTTP header to
// specify the fronted URL to use.
func (df *dualFetcher) RoundTrip(req *http.Request) (*http.Response, error) {
	if df.cf.parallel && !isIdempotentMethod(req) {
		return nil, errors.New("Use ParallelPreferChained for non-idempotent method")
	}
	directRT, err := ChainedNonPersistent("")
	if err != nil {
		return nil, errors.Wrap(err).Op("DFCreateChainedClient")
	}
	return df.do(req, directRT, frontedRT{})
}

// do will attempt to execute the specified HTTP request using both
// chained and fronted servers. Callers MUST use the Lantern-Fronted-URL HTTP
// header to specify the fronted URL to use.
func (df *dualFetcher) do(req *http.Request, chainedRT http.RoundTripper, ddfRT http.RoundTripper) (*http.Response, error) {
	log.Debugf("Using dual fronter")
	op := ops.Begin("dualfetcher").Request(req)
	defer op.End()

	responses := make(chan *http.Response, 2)
	errs := make(chan error, 2)

	request := func(asIs bool, rt http.RoundTripper, req *http.Request) (*http.Response, error) {
		resp, err := rt.RoundTrip(req)
		if err != nil {
			errs <- err
			return nil, err
		}
		op.Response(resp)
		if asIs {
			log.Debug("Passing response as is")
			responses <- resp
			return resp, nil
		} else if success(resp) {
			log.Debugf("Got successful HTTP call!")
			responses <- resp
			return resp, nil
		}
		// If the local proxy can't connect to any upstream proxies, for example,
		// it will return a 502.
		err = errors.New(resp.Status)
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		errs <- err
		return nil, err
	}

	frontedRTT := int64(100000 * time.Hour)
	chainedRTT := int64(100000 * time.Hour)
	switchToChainedIfRequired := func() {
		// 3 is arbitraily chosen to check if chained speed is reasonable
		// comparing to fronted
		if atomic.LoadInt64(&chainedRTT) <= 3*atomic.LoadInt64(&frontedRTT) {
			log.Debug("Switching to chained fetcher for future requests since it is within 3 times of fronted response time")
			df.cf.setFetcher(&chainedFetcher{})
		}
	}

	// cloneRequestForFronted modifies the req to remove Lantern-Fronted-URL
	// header. We need to call it before make any requests.
	frontedReq, err := cloneRequestForFronted(req)
	if err != nil {
		// Fail immediately as it's a program error.
		return nil, op.FailIf(err)
	}
	doFronted := func() {
		op.ProxyType(ops.ProxyFronted)
		log.Debugf("Sending DDF request. With body? %v", frontedReq.Body != nil)
		start := time.Now()
		if resp, err := request(!df.cf.parallel, ddfRT, frontedReq); err == nil {
			elapsed := time.Since(start)
			log.Debugf("Fronted request succeeded (%s) in %v",
				resp.Status, elapsed)
			// util.DumpResponse(resp) can be called here to examine the response
			atomic.StoreInt64(&frontedRTT, int64(elapsed))
			switchToChainedIfRequired()
		}
	}

	doChained := func() {
		op.ProxyType(ops.ProxyChained)
		log.Debugf("Sending chained request. With body? %v", req.Body != nil)
		start := time.Now()
		if _, err := request(false, chainedRT, req); err == nil {
			elapsed := time.Since(start)
			log.Debugf("Chained request succeeded in %v", elapsed)
			atomic.StoreInt64(&chainedRTT, int64(elapsed))
			switchToChainedIfRequired()
		}
	}

	getResponse := func() (*http.Response, error) {
		select {
		case resp := <-responses:
			return resp, nil
		case err := <-errs:
			return nil, err
		}
	}

	getResponseParallel := func() (*http.Response, error) {
		// Create channels for the final response or error. The response channel will be filled
		// in the case of any successful response as well as a non-error response for the second
		// response received. The error channel will only be filled if the first response is
		// unsuccessful and the second is an error.
		finalResponseCh := make(chan *http.Response, 1)
		finalErrorCh := make(chan error, 1)

		ops.Go(func() {
			readResponses(finalResponseCh, responses, finalErrorCh, errs)
		})

		select {
		case resp := <-finalResponseCh:
			return resp, nil
		case err := <-finalErrorCh:
			return nil, err
		}
	}

	frontOnly, _ := strconv.ParseBool(os.Getenv(forceDF))
	if frontOnly {
		log.Debug("Forcing domain-fronting")
		doFronted()
		resp, err := getResponse()
		return resp, op.FailIf(err)
	}

	if df.cf.parallel {
		ops.Go(doFronted)
		ops.Go(doChained)
		resp, err := getResponseParallel()
		return resp, op.FailIf(err)
	}

	doChained()
	resp, err := getResponse()
	if err != nil {
		log.Errorf("Chained failed, trying fronted: %v", err)
		doFronted()
		resp, err = getResponse()
		log.Debugf("Result of fronting: %v", err)
	}
	return resp, op.FailIf(err)
}

func cloneRequestForFronted(req *http.Request) (*http.Request, error) {
	frontedURL := req.Header.Get(lanternFrontedURL)
	if frontedURL == "" {
		return nil, errors.New("Callers MUST specify the fronted URL in the Lantern-Fronted-URL header")
	}
	req.Header.Del(lanternFrontedURL)

	frontedReq, err := http.NewRequest(req.Method, frontedURL, nil)
	if err != nil {
		return nil, err
	}

	// We need to copy the query parameters from the original.
	frontedReq.URL.RawQuery = req.URL.RawQuery
	// Make a copy of the original request headers to include in the
	// fronted request. This will ensure that things like the caching
	// headers are included in both requests.
	for k, vv := range req.Header {
		// Since we're doing domain fronting don't copy the host just in
		// case it ever makes any difference under the covers.
		if strings.EqualFold("Host", k) {
			continue
		}
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		frontedReq.Header[k] = vv2
	}

	if req.Body != nil {
		//Replicate the body. Attach a new copy to original request as body can
		//only be read once
		buf, _ := ioutil.ReadAll(req.Body)
		_ = req.Body.Close()
		req.Body = ioutil.NopCloser(bytes.NewReader(buf))
		frontedReq.Body = ioutil.NopCloser(bytes.NewReader(buf))
		frontedReq.ContentLength = req.ContentLength
	}

	return frontedReq, nil
}

func readResponses(finalResponse chan *http.Response, responses chan *http.Response, finalErr chan error, errs chan error) {
	waitForSecond := func() {
		// Just use whatever we get from the second response.
		select {
		case resp := <-responses:
			finalResponse <- resp
		case err := <-errs:
			finalErr <- err
		}
	}
	select {
	case resp := <-responses:
		if success(resp) {
			log.Debug("Got good first response")
			finalResponse <- resp

			// Just ignore the second response, but still process it.
			select {
			case response := <-responses:
				log.Debug("Closing second response body")
				_ = response.Body.Close()
				return
			case <-errs:
				log.Debug("Ignoring error on second response")
				return
			}
		} else {
			log.Debugf("Got bad first response -- wait for second")
			_ = resp.Body.Close()
			waitForSecond()
		}
	case err := <-errs:
		log.Debugf("Got an error in first response: %v", err)
		waitForSecond()
	}
}

// ChainedPersistent creates an http.RoundTripper that uses keepalive
// connectionspersists and proxies through chained servers. If rootCA is
// specified, the RoundTripper will validate the server's certificate on TLS
// connections against that RootCA.
func ChainedPersistent(rootCA string) (http.RoundTripper, error) {
	return chained(rootCA, true)
}

// ChainedNonPersistent creates an http.RoundTripper that proxies through
// chained servers and does not use keepalive connections. If rootCA is
// specified, the RoundTripper will validate the server's certificate on TLS
// connections against that RootCA.
func ChainedNonPersistent(rootCA string) (http.RoundTripper, error) {
	return chained(rootCA, false)
}

// chained creates an http.RoundTripper. If rootCA is specified, the
// RoundTripper will validate the server's certificate on TLS connections
// against that RootCA. If persistent is specified, the RoundTripper will use
// keepalive connections across requests.
func chained(rootCA string, persistent bool) (http.RoundTripper, error) {
	tr := &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,

		// This method is typically used for creating a one-off HTTP client
		// that we don't want to keep around for future calls, making
		// persistent connections a potential source of file descriptor
		// leaks. Note the name of this variable is misleading -- it would
		// be clearer to call it DisablePersistentConnections -- i.e. it has
		// nothing to do with TCP keep alives along the lines of the KeepAlive
		// variable in net.Dialer.
		DisableKeepAlives: !persistent,

		TLSClientConfig: &tls.Config{
			// Cache TLS sessions for faster connection
			ClientSessionCache: clientSessionCache,
		},
	}
	if persistent {
		tr.IdleConnTimeout = 30 * time.Second
	}

	if rootCA != "" {
		caCert, err := keyman.LoadCertificateFromPEMBytes([]byte(rootCA))
		if err != nil {
			return nil, errors.Wrap(err).Op("DecodeRootCA")
		}
		tr.TLSClientConfig.RootCAs = caCert.PoolContainingCert()
	}

	tr.Proxy = func(req *http.Request) (*url.URL, error) {
		proxyAddr, ok := getProxyAddr()
		if !ok {
			return nil, errors.New(ErrChainedProxyUnavailable)
		}
		return url.Parse("http://" + proxyAddr)
	}

	return AsRoundTripper(func(req *http.Request) (*http.Response, error) {
		changeUserAgent(req)
		op := ops.Begin("chained").ProxyType(ops.ProxyChained).Request(req)
		defer op.End()
		resp, err := tr.RoundTrip(req)
		op.Response(resp)
		return resp, errors.Wrap(err)
	}), nil
}

// PrepareForFronting prepares the given request to be used with domain-
// fronting.
func PrepareForFronting(req *http.Request, frontedURL string) {
	req.Header.Set(lanternFrontedURL, frontedURL)
}

// AsRoundTripper turns the given function into an http.RoundTripper.
func AsRoundTripper(fn func(req *http.Request) (*http.Response, error)) http.RoundTripper {
	return &rt{fn}
}

type rt struct {
	fn func(*http.Request) (*http.Response, error)
}

func (rt *rt) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt.fn(req)
}

var idempotentMethods = []string{
	"OPTIONS",
	"GET",
}

func isIdempotentMethod(req *http.Request) bool {
	for _, m := range idempotentMethods {
		if req.Method == m {
			return true
		}
	}
	return false
}
