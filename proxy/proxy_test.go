package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"code.google.com/p/go-uuid/uuid"

	"github.com/getlantern/flashlight/protocol/cloudflare"
)

const (
	HOST          = "localhost"
	CF_PORT       = 19871
	CF_ADDR       = HOST + ":19871"
	CLIENT_PORT   = 19872
	CLIENT_ADDR   = HOST + ":19872"
	SERVER_PORT   = 19873
	SERVER_ADDR   = HOST + ":19873"
	HTTP_ADDR     = HOST + ":19874"
	HTTPS_ADDR    = HOST + ":19875"
	MASQUERADE_AS = "localhost"

	EXPECTED_BODY    = "This is some stuff that goes in the body\n"
	FORWARDED_FOR_IP = "192.168.1.1"
)

// TestCloudFlare tests to make sure that a client and server can communicate
// with each other to proxy traffic for an HTTP client other using the
// CloudFlare protocol.  This does not test actually running through CloudFlare.
func TestCloudFlare(t *testing.T) {
	// Set up a mock HTTP server
	ms := &MockServer{}
	err := ms.init()
	if err != nil {
		t.Fatalf("Unable to init mock HTTP(S) server: %s", err)
	}
	defer ms.deleteCerts()

	ms.run()
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
		err := cf.run()
		if err != nil {
			t.Fatalf("Unable to run mock CloudFlare: %s", err)
		}
	}()
	waitForServer(CF_ADDR, 2*time.Second, t)

	// Set up client and server
	certContext := &CertContext{
		PKFile:         randomTempPath(),
		CACertFile:     randomTempPath(),
		ServerCertFile: randomTempPath(),
	}
	defer os.Remove(certContext.PKFile)
	defer os.Remove(certContext.CACertFile)
	defer os.Remove(certContext.ServerCertFile)

	clientProtocol, err := cloudflare.NewClientProtocol(HOST, CF_PORT, MASQUERADE_AS, string(cf.certContext.caCert.PEMEncoded()))
	if err != nil {
		t.Fatalf("Error initializing client protocol: %s", err)
	}
	client := &Client{
		UpstreamHost:        "localhost",
		Protocol:            clientProtocol,
		ShouldProxyLoopback: true,
		ProxyConfig: ProxyConfig{
			Addr:         CLIENT_ADDR,
			ReadTimeout:  0, // don't timeout
			WriteTimeout: 0,
			CertContext:  certContext,
		},
	}
	go func() {
		err := client.Run()
		if err != nil {
			t.Fatalf("Unable to run client: %s", err)
		}
	}()
	waitForServer(CLIENT_ADDR, 2*time.Second, t)

	serverProtocol := cloudflare.NewServerProtocol()
	server := &Server{
		Protocol: serverProtocol,
		ProxyConfig: ProxyConfig{
			Addr:         SERVER_ADDR,
			ReadTimeout:  0, // don't timeout
			WriteTimeout: 0,
			CertContext:  certContext,
		},
		TLSClientConfig: &tls.Config{
			// Trust the mock server's cert
			RootCAs: ms.certContext.caCert.PoolContainingCert(),
		},
	}
	go func() {
		err := server.Run()
		if err != nil {
			t.Fatalf("Unable to run server: %s", err)
		}
	}()
	waitForServer(SERVER_ADDR, 2*time.Second, t)

	certPool := certContext.caCert.PoolContainingCert()
	testRequest("Plain Text Request", t, ms.requests, false, certPool, 200, nil)
	testRequest("HTTPS Request", t, ms.requests, true, certPool, 200, nil)
	testRequest("HTTPS Request without MITM Cert", t, ms.requests, true, nil, 200, fmt.Errorf("Get https://"+HTTPS_ADDR+": x509: certificate signed by unknown authority"))
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
			// Trust our client's CA for MITM
			RootCAs: certPool,
		},
	}}
	var dest string
	if https {
		dest = "https://" + HTTPS_ADDR
	} else {
		dest = "http://" + HTTP_ADDR
	}
	req, err := http.NewRequest("GET", dest, nil)
	if err != nil {
		t.Fatalf("Unable to construct request: %s", err)
	}
	resp, err := httpClient.Do(req)
	errorsMatch := expectedErr == nil && err == nil ||
		expectedErr != nil && err != nil && err.Error() == expectedErr.Error()
	if !errorsMatch {
		t.Errorf("%s: Wrong error.\nExpected: %s\nGot     : %s", testCase, expectedErr, err)
	} else if err == nil {
		if resp.StatusCode != expectedStatus {
			t.Errorf("%s: Wrong response status. Expected %d, got %d", testCase, expectedStatus, resp.StatusCode)
		} else {
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("%s: Unable to read response body: %s", testCase, err)
			} else if string(body) != EXPECTED_BODY {
				t.Errorf("%s: Body didn't contain expected text.\nExpected: %s\nGot     : '%s'", testCase, EXPECTED_BODY, string(body))
			} else {
				// Make sure none of the request headers contained CloudFlare or
				// Lantern keys
				req := <-requests
				for key, values := range req.Header {
					for _, value := range values {
						if strings.Index(key, cloudflare.CF_PREFIX) >= 0 {
							t.Errorf("%s: Outgoing request contained CloudFlare header %s: %s", testCase, key, value)
						} else if strings.Index(key, cloudflare.X_LANTERN_PREFIX) >= 0 {
							t.Errorf("%s: Outgoing request contained Lantern header %s: %s", testCase, key, value)
						} else if strings.Index(key, "X-Forwarded-For") >= 0 && value == FORWARDED_FOR_IP {
							t.Errorf("%s: Outgoing request contained X-Forwarded-For with original IP %s: %s", testCase, key, value)
						}
					}
				}
			}
		}
	}
}

// MockServer is an HTTP(S) server
type MockServer struct {
	certContext *CertContext
	requests    chan *http.Request // publishes received requests
}

func (server *MockServer) init() error {
	server.certContext = &CertContext{
		PKFile:         randomTempPath(),
		CACertFile:     randomTempPath(),
		ServerCertFile: randomTempPath(),
	}

	err := server.certContext.InitCommonCerts()
	if err != nil {
		return fmt.Errorf("Unable to initialize mock server common certs: %s", err)
	}
	err = server.certContext.initServerCert(HOST)
	if err != nil {
		fmt.Errorf("Unable to initialize mock server cert: %s", err)
	}

	server.requests = make(chan *http.Request, 100)
	return nil
}

func (server *MockServer) deleteCerts() {
	os.Remove(server.certContext.PKFile)
	os.Remove(server.certContext.CACertFile)
	os.Remove(server.certContext.ServerCertFile)
}

func (server *MockServer) run() {
	httpServer := &http.Server{
		Addr:    HTTP_ADDR,
		Handler: http.HandlerFunc(server.handle),
	}

	httpsServer := &http.Server{
		Addr:    HTTPS_ADDR,
		Handler: http.HandlerFunc(server.handle),
	}

	go func() {
		log.Printf("About to start mock HTTP at: %s", httpServer.Addr)
		err := httpServer.ListenAndServe()
		if err != nil {
			log.Printf("Unable to start HTTP server: %s", err)
		}
	}()

	go func() {
		log.Printf("About to start mock HTTPS at: %s", httpsServer.Addr)
		err := httpsServer.ListenAndServeTLS(server.certContext.ServerCertFile, server.certContext.PKFile)
		if err != nil {
			log.Printf("Unable to start HTTP server: %s", err)
		}
	}()
}

func (server *MockServer) handle(resp http.ResponseWriter, req *http.Request) {
	resp.Write([]byte(EXPECTED_BODY))
	server.requests <- req
}

// MockCloudFlare pretends to be CloudFlare
type MockCloudFlare struct {
	certContext *CertContext
}

func (cf *MockCloudFlare) init() error {
	cf.certContext = &CertContext{
		PKFile:         randomTempPath(),
		CACertFile:     randomTempPath(),
		ServerCertFile: randomTempPath(),
	}

	err := cf.certContext.InitCommonCerts()
	if err != nil {
		return fmt.Errorf("Unable to initialize mock CloudFlare common certs: %s", err)
	}
	err = cf.certContext.initServerCert(HOST)
	if err != nil {
		fmt.Errorf("Unable to initialize mock CloudFlare server cert: %s", err)
	}
	return nil
}

func (cf *MockCloudFlare) deleteCerts() {
	os.Remove(cf.certContext.PKFile)
	os.Remove(cf.certContext.CACertFile)
	os.Remove(cf.certContext.ServerCertFile)
}

func (cf *MockCloudFlare) run() error {
	httpServer := &http.Server{
		Addr: CF_ADDR,
		Handler: &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = "https"
				req.URL.Host = SERVER_ADDR
				req.Host = SERVER_ADDR

				// Set some headers like CloudFlare
				req.Header.Set("Cf-A", "A")
				req.Header.Set("Cf-B", "B")
				req.Header.Set(cloudflare.X_FORWARDED_PROTO, "https")
				req.Header.Set(cloudflare.X_FORWARDED_FOR, FORWARDED_FOR_IP)
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

	log.Printf("About to start mock CloudFlare at: %s", httpServer.Addr)
	return httpServer.ListenAndServeTLS(cf.certContext.ServerCertFile, cf.certContext.PKFile)
}

func randomTempPath() string {
	return os.TempDir() + string(os.PathSeparator) + uuid.New()
}

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
