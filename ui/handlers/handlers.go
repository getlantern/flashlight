package handlers

import (
	"fmt"
	"net/http"

	"github.com/getlantern/flashlight/ui/params"
	"github.com/getlantern/lantern-server/common"
)

type HandlerFunc func(http.ResponseWriter, *http.Request)

type UIHandler interface {
	Routes() map[string]HandlerFunc
}

type Handler struct {
	UIHandler
	authServerAddr string
	HttpClient     *http.Client
}

type Params struct {
	AuthServerAddr string
	HttpClient     *http.Client
}

func New(params Params) Handler {
	return Handler{
		authServerAddr: params.AuthServerAddr,
		HttpClient:     params.HttpClient,
	}
}

// getAPIAddr combines the given uri with the authentication server address
func (h Handler) GetAPIAddr(uri string) string {
	return fmt.Sprintf("%s%s", h.authServerAddr, uri)
}

// ErrorHandler is an error handler that takes an error or Errors and writes the
// encoded JSON response to the client
func (h Handler) ErrorHandler(w http.ResponseWriter, err interface{}, errorCode int) {
	var resp params.Response
	switch err.(type) {
	case error:
		resp.Error = err.(error).Error()
	case params.Errors:
		resp.Errors = err.(params.Errors)
	}
	common.WriteJSON(w, errorCode, &resp)
}
