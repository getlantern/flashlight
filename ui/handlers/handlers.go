package handlers

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/getlantern/auth-server/models"
	"github.com/getlantern/flashlight/ui/params"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-server/common"
)

var (
	log = golog.LoggerFor("flashlight.ui.handlers")
)

type HandlerFunc func(http.ResponseWriter, *http.Request)

// UIHandler is an interface UI handlers must implement
type UIHandler interface {
	Routes() map[string]HandlerFunc
}

// Handler  is a representation of a group of handlers
// related to a specific product (i.e. Yinbi)
type Handler struct {
	UIHandler
	authServerAddr  string
	yinbiServerAddr string
	HttpClient      *http.Client
}

// Params represents the parameters handlers are configured with
type Params struct {
	AuthServerAddr  string
	YinbiServerAddr string
	HttpClient      *http.Client
}

func New(params Params) Handler {
	return Handler{
		authServerAddr:  params.AuthServerAddr,
		yinbiServerAddr: params.YinbiServerAddr,
		HttpClient:      params.HttpClient,
	}
}

// GetAuthAddr combines the given uri with the Lantern authentication server address
func (h Handler) GetAuthAddr(uri string) string {
	return fmt.Sprintf("%s%s", h.authServerAddr, uri)
}

// GetYinbiAddr combines the given uri with the Yinbi server address
func (h Handler) GetYinbiAddr(uri string) string {
	return fmt.Sprintf("%s%s", h.yinbiServerAddr, uri)
}

// DoRequest creates and sends a new HTTP request to the given url
// with an optional requestBody. It returns an HTTP response
func (h Handler) DoRequest(method, url string,
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
	req.Header.Set(common.HeaderContentType, common.MIMEApplicationJSON)
	return h.HttpClient.Do(req)
}

// proxyHandler is a HTTP handler used to proxy requests
// to the Lantern authentication server
func (h Handler) ProxyHandler(url string, req *http.Request, w http.ResponseWriter,
	onResponse common.HandleResponseFunc,
) error {
	return common.ProxyHandler(url, h.HttpClient, req, w,
		onResponse)
}

// ErrorHandler is an error handler that takes an error or Errors and writes the
// encoded JSON response to the client
func (h Handler) ErrorHandler(w http.ResponseWriter, err interface{}, errorCode int) {
	var resp params.Response
	switch err.(type) {
	case error:
		resp.Error = err.(error).Error()
	case models.Errors:
		resp.Errors = err.(models.Errors)
	}
	common.WriteJSON(w, errorCode, &resp)
}
