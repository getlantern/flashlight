package goproxy

import (
	"bufio"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"sync/atomic"
)

const GoproxyNoMetricsHeader = "Goproxy-No-Metrics"

// The basic proxy type. Implements http.Handler.
type ProxyHttpServer struct {
	// An identifier used to identify the proxy. Can be empty, but helpful in
	// debugging sessions
	ID string
	// session variable must be aligned in i386
	// see http://golang.org/src/pkg/sync/atomic/doc.go#L41
	sess int64
	// KeepDestinationHeaders indicates the proxy should retain any headers present in the http.Response before proxying
	KeepDestinationHeaders bool
	// setting Verbose to true will log information on each request sent to the proxy
	Verbose bool
	Logger  Logger
	// NonproxyHandler handles non-proxy requests - e.g. requests to the proxy itself
	// Optional. If nil, a default handler is used.
	// Return the size of the request body if it is known, or 0 otherwise
	NonproxyHandler      func(http.ResponseWriter, *http.Request) int64
	CustomConnectHandler http.Handler
	reqHandlers          []ReqHandler
	respHandlers         []RespHandler
	httpsHandlers        []HttpsHandler
	Tr                   *http.Transport
	// ConnectDial will be used to create TCP connections for CONNECT requests
	// if nil Tr.Dial will be used
	ConnectDial func(
		originalConnectReq *http.Request,
		network string, addr string) (net.Conn, error)
	CertStore  CertStorage
	KeepHeader bool
	// All errors during reading and writing to different net.Conn objects
	// (almost exclusively used during HTTP CONNECT requests) will be piped
	// here, if this is not nil. Else, errors will be logged with a warning
	// level
	ReadWriteErrChan     chan error
	BytesReadCallback    func(host string, n int64)
	BytesWrittenCallback func(host string, n int64)
}

var hasPort = regexp.MustCompile(`:\d+$`)

func copyHeaders(dst, src http.Header, keepDestHeaders bool) {
	if !keepDestHeaders {
		for k := range dst {
			dst.Del(k)
		}
	}
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func isEof(r *bufio.Reader) bool {
	_, err := r.Peek(1)
	if err == io.EOF {
		return true
	}
	return false
}

func (proxy *ProxyHttpServer) filterRequest(r *http.Request, ctx *ProxyCtx) (req *http.Request, resp *http.Response) {
	req = r
	for _, h := range proxy.reqHandlers {
		req, resp = h.Handle(r, ctx)
		// non-nil resp means the handler decided to skip sending the request
		// and return canned response instead.
		if resp != nil {
			break
		}
	}
	return
}
func (proxy *ProxyHttpServer) filterResponse(respOrig *http.Response, ctx *ProxyCtx) (resp *http.Response) {
	resp = respOrig
	for _, h := range proxy.respHandlers {
		ctx.Resp = resp
		resp = h.Handle(resp, ctx)
	}
	return
}

func removeProxyHeaders(ctx *ProxyCtx, r *http.Request) {
	r.RequestURI = "" // this must be reset when serving a request with the client
	ctx.Logf("Sending request %v %v", r.Method, r.URL.String())
	// If no Accept-Encoding header exists, Transport will add the headers it can accept
	// and would wrap the response body with the relevant reader.
	r.Header.Del("Accept-Encoding")
	// curl can add that, see
	// https://jdebp.eu./FGA/web-proxy-connection-header.html
	r.Header.Del("Proxy-Connection")
	r.Header.Del("Proxy-Authenticate")
	r.Header.Del("Proxy-Authorization")
	// Connection, Authenticate and Authorization are single hop Header:
	// http://www.w3.org/Protocols/rfc2616/rfc2616.txt
	// 14.10 Connection
	//   The Connection general-header field allows the sender to specify
	//   options that are desired for that particular connection and MUST NOT
	//   be communicated by proxies over further connections.

	// When server reads http request it sets req.Close to true if
	// "Connection" header contains "close".
	// https://github.com/golang/go/blob/master/src/net/http/request.go#L1080
	// Later, transfer.go adds "Connection: close" back when req.Close is true
	// https://github.com/golang/go/blob/master/src/net/http/transfer.go#L275
	// That's why tests that checks "Connection: close" removal fail
	if r.Header.Get("Connection") == "close" {
		r.Close = false
	}
	r.Header.Del("Connection")
}

// Standard net/http function. Shouldn't be used directly, http.Serve will use it.
func (proxy *ProxyHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// r.Header["X-Forwarded-For"] = []string{r.RemoteAddr}
	if r.Method == "CONNECT" {
		proxy.handleHttps(w, r)
	} else {
		ctx := &ProxyCtx{Req: r, Session: atomic.AddInt64(&proxy.sess, 1), Proxy: proxy}

		var err error
		ctx.Logf("Got request %v %v %v %v", r.URL.Path, r.Host, r.Method, r.URL.String())
		if !r.URL.IsAbs() {
			reqBytes := proxy.NonproxyHandler(w, r)
			// Respect the no-metrics header
			if r.Header.Get(GoproxyNoMetricsHeader) == "1" {
				return
			}
			if reqBytes > 0 && proxy.BytesWrittenCallback != nil {
				proxy.BytesWrittenCallback(r.Host, reqBytes)
			}
			return
		}
		r, resp := proxy.filterRequest(r, ctx)

		if resp == nil {
			if isWebSocketRequest(r) {
				ctx.Logf("Request looks like websocket upgrade.")
				proxy.serveWebsocket(ctx, w, r)
			}

			if !proxy.KeepHeader {
				removeProxyHeaders(ctx, r)
			}
			resp, err = ctx.RoundTrip(r)
			if err != nil {
				ctx.Error = err
				resp = proxy.filterResponse(nil, ctx)

			}
			if resp != nil {
				ctx.Logf("Received response %v", resp.Status)
			}
		}

		var origBody io.ReadCloser

		if resp != nil {
			origBody = resp.Body
			defer origBody.Close()
		}

		resp = proxy.filterResponse(resp, ctx)

		if resp == nil {
			var errorString string
			if ctx.Error != nil {
				errorString = "error read response " + r.URL.Host + " : " + ctx.Error.Error()
				ctx.Logf(errorString)
				http.Error(w, ctx.Error.Error(), 500)
			} else {
				errorString = "error read response " + r.URL.Host
				ctx.Logf(errorString)
				http.Error(w, errorString, 500)
			}
			return
		}
		ctx.Logf("Copying response to client %v [%d]", resp.Status, resp.StatusCode)
		// http.ResponseWriter will take care of filling the correct response length
		// Setting it now, might impose wrong value, contradicting the actual new
		// body the user returned.
		// We keep the original body to remove the header only if things changed.
		// This will prevent problems with HEAD requests where there's no body, yet,
		// the Content-Length header should be set.
		if origBody != resp.Body {
			resp.Header.Del("Content-Length")
		}
		copyHeaders(w.Header(), resp.Header, proxy.KeepDestinationHeaders)
		w.WriteHeader(resp.StatusCode)
		nr, err := io.Copy(w, resp.Body)
		if err := resp.Body.Close(); err != nil {
			ctx.Warnf("Can't close response body %v", err)
		}
		ctx.Logf("Copied %v bytes to client error=%v", nr, err)
	}
}

// NewProxyHttpServer creates and returns a proxy server, logging to stderr by default
func NewProxyHttpServer() *ProxyHttpServer {
	proxy := ProxyHttpServer{
		Logger:        log.New(os.Stderr, "", log.LstdFlags),
		reqHandlers:   []ReqHandler{},
		respHandlers:  []RespHandler{},
		httpsHandlers: []HttpsHandler{},
		NonproxyHandler: func(w http.ResponseWriter, req *http.Request) int64 {
			http.Error(w, "This is a proxy server. Does not respond to non-proxy requests.", 500)
			return 0
		},
		Tr: &http.Transport{TLSClientConfig: tlsClientSkipVerify, Proxy: http.ProxyFromEnvironment},
	}

	proxy.ConnectDial = dialerFromEnv(&proxy)
	return &proxy
}