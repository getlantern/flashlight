package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/getlantern/lantern-server/models"
)

const (
	userKey               = iota
	authEndpoint          = "/auth"
	loginEndpoint         = "/user/login"
	registrationEndpoint  = "/user/register"
	balanceEndpoint       = "/user/balance"
	createAccountEndpoint = "/user/account/new"
)

var (
	ErrInvalidSRPProof = errors.New("The SRP proof supplied by the server is invalid")
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

func (s *Server) getAPIAddr(uri string) string {
	return fmt.Sprintf("%s%s", s.authServerAddr, uri)
}

// proxyHandler
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
	if resp.StatusCode != http.StatusOK {
		err = errors.New("Received an invalid response code")
		return err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if onResp != nil {
		err = onResp(body)
		if err != nil {
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

func (s *Server) sendRequest(method, url string, requestBody []byte) (*http.Response, error) {
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

func (s *Server) sendAuthRequest(method, url string, requestBody []byte) (*models.AuthResponse, error) {
	resp, err := s.sendRequest(method, url, requestBody)
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

func (s *Server) authHandler(w http.ResponseWriter, req *http.Request) {
	params, srpClient, err := s.getSRPClient(req)
	if err != nil {
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(requestBody))

	onResp := func(body []byte) error {
		resp, err := decodeAuthResponse(body)
		if err != nil {
			return err
		}
		resp, err = s.sendMutualAuth(srpClient,
			resp.Credentials, params.Username)
		if err != nil {
			return err
		}
		ok := srpClient.ServerOk(resp.Proof)
		if !ok {
			return ErrInvalidSRPProof
		}
		return nil
	}

	err = s.proxyHandler(req, w, onResp)
	if err != nil {
		s.errorHandler(w, err, http.StatusInternalServerError)
		return
	}
}
