package ui

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/getlantern/lantern-server/models"
	"github.com/getlantern/lantern-server/srp"
)

const (
	userKey               = iota
	authEndpoint          = "/auth"
	loginEndpoint         = "/login"
	registrationEndpoint  = "/register"
	balanceEndpoint       = "/user/balance"
	createAccountEndpoint = "/user/account/new"
)

var (
	ErrInvalidCredentials = errors.New("The supplied user credentials were invalid")
	ErrSRPKeysDifferent   = errors.New("SRP client and server keys do not match")
)

var forwardHeaders = map[string]bool{
	"authorization":   true,
	"cookie":          true,
	"accept-encoding": true,
	"content-type":    true,
	"accept":          true,
}

// Hop headers removed prior to the request being sent
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true, // canonicalized version of "TE"
	"trailers":            true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

func withUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey, userID)
}

// getAPIAddr combines the given uri with the authentication server address
func (s *Server) getAPIAddr(uri string) string {
	return fmt.Sprintf("%s%s", s.authServerAddr, uri)
}

// proxyHandler is a HTTP handler used to proxy requests
// to the Lantern authentication server
func (s *Server) proxyHandler(req *http.Request, w http.ResponseWriter,
	onResp func(body []byte) error,
) error {
	url := s.getAPIAddr(html.EscapeString(req.URL.Path))
	proxyReq, err := http.NewRequest(req.Method, url, req.Body)
	if err != nil {
		return err
	}
	proxyReq.Header = make(http.Header)
	for k, v := range req.Header {
		if _, ok := forwardHeaders[strings.ToLower(k)]; ok {
			proxyReq.Header[k] = v
		}
	}
	clientIP, _, err := net.SplitHostPort(req.RemoteAddr)
	if err == nil {
		proxyReq.Header.Set("X-Forwarded-For", clientIP)
	}
	resp, err := s.httpClient.Do(proxyReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK && onResp != nil {
		err = onResp(body)
		if err != nil {
			log.Error(err)
			return err
		}
	}

	headers := w.Header()
	for k, values := range resp.Header {
		if _, ok := hopHeaders[strings.ToLower(k)]; ok {
			// skip hop headers
			continue
		}
		for _, v := range values {
			headers.Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
	return nil
}

// doRequest creates and sends a new HTTP request to the given url
// with an optional requestBody. It returns an HTTP response
func (s *Server) doRequest(method, url string,
	requestBody []byte) (*http.Response, error) {
	log.Debugf("Sending new request to url %s", url)
	var req *http.Request
	var err error
	if requestBody != nil {
		req, err = http.NewRequest(method, url,
			bytes.NewBuffer(requestBody))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set(HeaderContentType, MIMEApplicationJSON)
	return s.httpClient.Do(req)
}

func (s *Server) sendAuthRequest(method, url string,
	requestBody []byte) (*models.AuthResponse, error) {
	resp, err := s.doRequest(method, url, requestBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return decodeAuthResponse(body)
}

func decodeAuthResponse(body []byte) (*models.AuthResponse, error) {
	authResp := new(models.AuthResponse)
	err := json.Unmarshal(body, authResp)
	return authResp, err
}

// authHandler is the HTTP handler used by the login and
// registration endpoints. It creates a new SRP client from
// the user params in the request and sends the
// SRP params (i.e. verifier) generated to the authentication
// server, establishing a fully authenticated session
func (s *Server) authHandler(w http.ResponseWriter, req *http.Request) {
	params, srpClient, err := s.getSRPClient(req)
	if err != nil {
		return
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		return
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(requestBody))

	onResp := func(body []byte) error {
		resp, err := decodeAuthResponse(body)
		if err != nil {
			return err
		}
		// client generates a mutual auth and sends it to the server
		resp, err = s.sendMutualAuth(srpClient,
			resp.Credentials, params.Username)
		if err != nil {
			return err
		}
		// Verify the server's proof
		ok := srpClient.ServerOk(resp.Proof)
		if !ok {
			return ErrInvalidCredentials
		}
		srv, err := srp.UnmarshalServer(resp.Server)
		if err != nil {
			s.errorHandler(w, err, http.StatusInternalServerError)
			return err
		}
		// Client and server are successfully authenticated to each other
		kc := srpClient.RawKey()
		ks := srv.RawKey()
		if 1 != subtle.ConstantTimeCompare(kc, ks) {
			return ErrSRPKeysDifferent
		}
		log.Debug("Successfully created new SRP session")
		return nil
	}

	err = s.proxyHandler(req, w, onResp)
	if err != nil {
		s.errorHandler(w, err, http.StatusUnauthorized)
		return
	}
}
