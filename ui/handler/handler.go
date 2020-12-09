package handler

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/getlantern/auth-server/api"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-server/common"
	"github.com/gorilla/mux"
)

// successKey is the name of the boolean field
// a successful API response is populated with
const successKey = "success"

var (
	log = golog.LoggerFor("flashlight.ui.handler")
	// successResponse is the default args returned
	// with a successful API response
	successResponse = map[string]interface{}{
		successKey: true,
	}
)

// UIHandler is an interface UI handlers must implement
type UIHandler interface {
	// ConfigureRoutes is used to setup a collection of routes
	// used by the given UI handler. It returns an http.Handler
	ConfigureRoutes() http.Handler
}

// Handler  is a representation of a group of handlers
// related to a specific product (i.e. Yinbi)
type Handler struct {
	UIHandler
	http.Handler
	authAddr   string
	yinbiAddr  string
	HTTPClient *http.Client
}

// NewHandler creates a new UI Handler
func NewHandler(params api.APIParams) Handler {
	return Handler{
		authAddr:   params.AuthServerAddr,
		yinbiAddr:  params.YinbiServerAddr,
		HTTPClient: params.HTTPClient,
	}
}

// NewRouter creates and returns a new mux.Router
// instance, configured to use the default UI middleware
func NewRouter() *mux.Router {
	r := mux.NewRouter()
	r.Use(BodyParser)
	return r
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
	return h.HTTPClient.Do(req)
}

// proxyHandler is a HTTP handler used to proxy requests
// to the Lantern authentication server
func (h Handler) ProxyHandler(url string, req *http.Request, w http.ResponseWriter,
	onResponse common.HandleResponseFunc,
) error {
	return common.ProxyHandler(url, h.HTTPClient, req, w,
		onResponse)
}

// SuccessResponse is an API response used to inform the
// UI a request succeeded. It includes `success: true`
func SuccessResponse(w http.ResponseWriter, vargs ...map[string]interface{}) {
	var args map[string]interface{}
	if len(vargs) == 0 {
		args = successResponse
	} else {
		args = vargs[0]
		args[successKey] = true
	}
	common.WriteJSON(w, http.StatusOK, args)
}

// ErrorHandler is an error handler that takes an error or Errors and writes the
// encoded JSON response to the client
func ErrorHandler(w http.ResponseWriter, err interface{}, errorCode int) {
	var resp api.ApiResponse
	switch err.(type) {
	case string:
		resp.Error = err.(string)
	case error:
		resp.Error = err.(error).Error()
	case api.Errors:
		resp.Errors = err.(api.Errors)
	}
	log.Errorf("Unable to process HTTP request: %v", resp)
	common.WriteJSON(w, errorCode, &resp)
}
