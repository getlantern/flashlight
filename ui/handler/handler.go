package handler

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/getlantern/auth-server/api"
	"github.com/getlantern/auth-server/models"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-server/common"
)

var (
	log = golog.LoggerFor("flashlight.ui.handler")
)

// Middleware
type Middleware func(http.HandlerFunc) http.HandlerFunc

// Route defines a structure for UI routes
type Route struct {
	Pattern     string
	Method      string
	HandlerFunc http.HandlerFunc
}

// UIHandler is an interface UI handlers must implement
type UIHandler interface {
	// Routes is a map of UI server paths to handler funcs
	Routes() []Route
}

// Handler  is a representation of a group of handlers
// related to a specific product (i.e. Yinbi)
type Handler struct {
	UIHandler
	authAddr   string
	yinbiAddr  string
	HttpClient *http.Client
}

// wrapMiddleware takes the given http.Handler and optionally wraps it with
// the cors middleware handler
func WrapMiddleware(h http.HandlerFunc, m ...Middleware) http.HandlerFunc {
	if len(m) < 1 {
		return corsHandler(h)
	}
	wrapped := h
	for i := len(m) - 1; i >= 0; i-- {
		wrapped = m[i](wrapped)
	}

	return corsHandler(wrapped)
}

func NewHandler(params api.APIParams) Handler {
	return Handler{
		authAddr:   params.AuthServerAddr,
		yinbiAddr:  params.YinbiServerAddr,
		HttpClient: params.HttpClient,
	}
}

// GetAuthAddr combines the given uri with the Lantern authentication server address
func (h Handler) GetAuthAddr(uri string) string {
	return fmt.Sprintf("%s%s", h.authAddr, uri)
}

// GetYinbiAddr combines the given uri with the Yinbi server address
func (h Handler) GetYinbiAddr(uri string) string {
	return fmt.Sprintf("%s%s", h.yinbiAddr, uri)
}

// DoHTTPRequest creates and sends a new HTTP request to the given url
// with an optional requestBody. It returns an HTTP response
func (h Handler) DoHTTPRequest(method, url string,
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
	var resp api.ApiResponse
	switch err.(type) {
	case string:
		resp.Error = err.(string)
	case error:
		resp.Error = err.(error).Error()
	case models.Errors:
		resp.Errors = err.(models.Errors)
	}
	common.WriteJSON(w, errorCode, &resp)
}
