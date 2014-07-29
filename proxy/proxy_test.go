package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"
	"time"

	"code.google.com/p/go-uuid/uuid"
	"github.com/getlantern/enproxy"
)

const (
	HOST          = "127.0.0.1"
	CF_PORT       = 19871
	CF_ADDR       = HOST + ":19871"
	CLIENT_PORT   = 19872
	CLIENT_ADDR   = HOST + ":19872"
	SERVER_PORT   = 19873
	SERVER_ADDR   = HOST + ":19873"
	HTTP_ADDR     = HOST + ":19874"
	HTTPS_ADDR    = HOST + ":19875"
	MASQUERADE_AS = HOST

	EXPECTED_BODY    = "This is some stuff that goes in the body\n"
	FORWARDED_FOR_IP = "192.168.1.1"
)

// TestCloudFlare tests to make sure that a client and server can communicate
// with each other to proxy traffic for an HTTP client using the CloudFlare
// protocol.  This does not test actually running through CloudFlare and just
// uses a local HTTP server to serve the test content.
func TestCloudFlare(t *testing.T) {
	// Set up a mock HTTP server
	mockServer := &MockServer{}
	err := mockServer.init()
	if err != nil {
		t.Fatalf("Unable to init mock HTTP(S) server: %s", err)
	}
	defer mockServer.deleteCerts()

	mockServer.run(t)
	waitForServer(HTTP_ADDR, 2*time.Second, t)
	waitForServer(HTTPS_ADDR, 2*time.Second, t)

	// Set up a mock CloudFlare
	cf := &MockCloudFlare{}
	err = cf.init()
	if err != nil {
		t.Fatalf("Unable to init mock CloudFlare: %s", err)
	}
	defer cf.deleteCerts()

	go func() {
		err := cf.run(t)
		if err != nil {
			t.Fatalf("Unable to run mock CloudFlare: %s", err)
		}
	}()
	waitForServer(CF_ADDR, 2*time.Second, t)

	// Set up common certContext for proxies
	certContext := &CertContext{
		PKFile:         randomTempPath(),
		ServerCertFile: randomTempPath(),
	}
	defer os.Remove(certContext.PKFile)
	defer os.Remove(certContext.ServerCertFile)

	// Run server proxy
	server := &Server{
		ProxyConfig: ProxyConfig{
			Addr:         SERVER_ADDR,
			ReadTimeout:  0, // don't timeout
			WriteTimeout: 0,
		},
		CertContext:                certContext,
		AllowNonGlobalDestinations: true,
	}
	go func() {
		err := server.Run()
		if err != nil {
			t.Fatalf("Unable to run server: %s", err)
		}
	}()
	waitForServer(SERVER_ADDR, 2*time.Second, t)

	client := &Client{
		ProxyConfig: ProxyConfig{
			Addr:         CLIENT_ADDR,
			ReadTimeout:  0, // don't timeout
			WriteTimeout: 0,
		},
		EnproxyConfig: &enproxy.Config{
			DialProxy: func(addr string) (net.Conn, error) {
				return tls.Dial("tcp", CF_ADDR, &tls.Config{
					RootCAs: cf.certContext.serverCert.PoolContainingCert(),
				})
			},
			NewRequest: func(host string, method string, body io.Reader) (req *http.Request, err error) {
				if host == "" {
					host = SERVER_ADDR
				}
				return http.NewRequest(method, "http://"+host, body)
			},
		},
	}
	go func() {
		err := client.Run()
		if err != nil {
			t.Fatalf("Unable to run client: %s", err)
		}
	}()
	waitForServer(CLIENT_ADDR, 2*time.Second, t)

	// Test various scenarios
	certPool := mockServer.certContext.serverCert.PoolContainingCert()
	testRequest("Plain Text Request", t, mockServer.requests, false, certPool, 200, nil)
	testRequest("HTTPS Request", t, mockServer.requests, true, certPool, 200, nil)
	testRequest("HTTPS Request without server Cert", t, mockServer.requests, true, nil, 200, fmt.Errorf("Get https://"+HTTPS_ADDR+": x509: certificate signed by unknown authority"))
}

// testRequest tests an individual request, either HTTP or HTTPS, making sure
// that the response status and body match the expected values.  If the request
// was successful, it also tests to make sure that the outbound request didn't
// leak any Lantern or CloudFlare headers.
func testRequest(testCase string, t *testing.T, requests chan *http.Request, https bool, certPool *x509.CertPool, expectedStatus int, expectedErr error) {
	httpClient := &http.Client{Transport: &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse("http://" + CLIENT_ADDR)
		},

		TLSClientConfig: &tls.Config{
			RootCAs: certPool,
		},
	}}

	var destURL string
	if https {
		destURL = "https://" + HTTPS_ADDR
	} else {
		destURL = "http://" + HTTP_ADDR
	}
	req, err := http.NewRequest("GET", destURL, nil)
	if err != nil {
		t.Fatalf("Unable to construct request: %s", err)
	}
	resp, err := httpClient.Do(req)

	requestSuccessful := err == nil
	gotCorrectError := expectedErr == nil && err == nil ||
		expectedErr != nil && err != nil && err.Error() == expectedErr.Error()
	if !gotCorrectError {
		t.Errorf("%s: Wrong error.\nExpected: %s\nGot     : %s", testCase, expectedErr, err)
	} else if requestSuccessful {
		defer resp.Body.Close()
		if resp.StatusCode != expectedStatus {
			t.Errorf("%s: Wrong response status. Expected %d, got %d", testCase, expectedStatus, resp.StatusCode)
		} else {
			// Check body
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("%s: Unable to read response body: %s", testCase, err)
			} else if string(body) != EXPECTED_BODY {
				t.Errorf("%s: Body didn't contain expected text.\nExpected: %s\nGot     : '%s'", testCase, EXPECTED_BODY, string(body))
			}
		}
	}
}

// MockServer is an HTTP+S server that serves up simple responses
type MockServer struct {
	certContext *CertContext
	requests    chan *http.Request // publishes received requests
}

func (server *MockServer) init() error {
	server.certContext = &CertContext{
		PKFile:         randomTempPath(),
		ServerCertFile: randomTempPath(),
	}

	err := server.certContext.initServerCert(HOST)
	if err != nil {
		fmt.Errorf("Unable to initialize mock server cert: %s", err)
	}

	server.requests = make(chan *http.Request, 100)
	return nil
}

func (server *MockServer) deleteCerts() {
	os.Remove(server.certContext.PKFile)
	os.Remove(server.certContext.ServerCertFile)
}

func (server *MockServer) run(t *testing.T) {
	httpServer := &http.Server{
		Addr:    HTTP_ADDR,
		Handler: http.HandlerFunc(server.handle),
	}

	httpsServer := &http.Server{
		Addr:    HTTPS_ADDR,
		Handler: http.HandlerFunc(server.handle),
	}

	go func() {
		t.Logf("About to start mock HTTP at: %s", httpServer.Addr)
		err := httpServer.ListenAndServe()
		if err != nil {
			t.Errorf("Unable to start HTTP server: %s", err)
		}
	}()

	go func() {
		t.Logf("About to start mock HTTPS at: %s", httpsServer.Addr)
		err := httpsServer.ListenAndServeTLS(server.certContext.ServerCertFile, server.certContext.PKFile)
		if err != nil {
			t.Errorf("Unable to start HTTP server: %s", err)
		}
	}()
}

func (server *MockServer) handle(resp http.ResponseWriter, req *http.Request) {
	resp.Write([]byte(EXPECTED_BODY))
	server.requests <- req
}

// MockCloudFlare is a ReverseProxy that pretends to be CloudFlare
type MockCloudFlare struct {
	certContext *CertContext
}

func (cf *MockCloudFlare) init() error {
	cf.certContext = &CertContext{
		PKFile:         randomTempPath(),
		ServerCertFile: randomTempPath(),
	}

	err := cf.certContext.initServerCert(HOST)
	if err != nil {
		fmt.Errorf("Unable to initialize mock CloudFlare server cert: %s", err)
	}
	return nil
}

func (cf *MockCloudFlare) deleteCerts() {
	os.Remove(cf.certContext.PKFile)
	os.Remove(cf.certContext.ServerCertFile)
}

func (cf *MockCloudFlare) run(t *testing.T) error {
	httpServer := &http.Server{
		Addr: CF_ADDR,
		Handler: &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = "https"
				req.URL.Host = SERVER_ADDR
				req.Host = SERVER_ADDR
			},
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					// Real CloudFlare doesn't verify our cert, so mock doesn't
					// either
					InsecureSkipVerify: true,
				},
			},
		},
	}

	t.Logf("About to start mock CloudFlare at: %s", httpServer.Addr)
	return httpServer.ListenAndServeTLS(cf.certContext.ServerCertFile, cf.certContext.PKFile)
}

// randomTempPath creates a random file path in the temp folder (doesn't create
// a file)
func randomTempPath() string {
	return os.TempDir() + string(os.PathSeparator) + uuid.New()
}

// waitForServer waits for a TCP server to start at the given address, waiting
// up to the given limit and reporting an error to the given testing.T if the
// server didn't start within the time limit.
func waitForServer(addr string, limit time.Duration, t *testing.T) {
	cutoff := time.Now().Add(limit)
	for {
		if time.Now().After(cutoff) {
			t.Errorf("Server never came up at address %s", addr)
			return
		}
		c, err := net.DialTimeout("tcp", addr, limit)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
