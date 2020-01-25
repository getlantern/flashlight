package ui

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
)

const (
	HeaderContentType              = "Content-Type"
	HeaderAccessControlAllow       = "Access-Control-Allow-Headers"
	HeaderAuthorization            = "Authorization"
	HeaderAccessControlAllowOrigin = "Access-Control-Allow-Origin"

	POST = "POST"
	GET  = "GET"

	charsetUTF8                    = "charset=UTF-8"
	MIMEApplicationJSON            = "application/json"
	MIMEApplicationJSONCharsetUTF8 = MIMEApplicationJSON + "; " + charsetUTF8
)

var ErrUnsupportedMediaType = errors.New("The request media type is invalid")

// decodeJSONRequest parses the JSON-encoded data in the provided request and
// stores the result in dst
func decodeJSONRequest(req *http.Request, dst interface{}) error {

	ctype := req.Header.Get(HeaderContentType)
	switch {
	case strings.HasPrefix(ctype, MIMEApplicationJSON):
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return err
		}
		err = json.Unmarshal(body, &dst)
		return err
	default:
		return ErrUnsupportedMediaType
	}
}

// errorHandler is an error handler that takes an error or Errors and writes the
// encoded JSON response to the client
func (s *Server) errorHandler(w http.ResponseWriter, err interface{}, errorCode int) {
	var resp Response
	switch err.(type) {
	case error:
		resp.Error = err.(error).Error()
	case Errors:
		resp.Errors = err.(Errors)
	}
	writeJSON(w, errorCode, &resp)
}

// writeJSON writes the encoding of i to the provided http.ResponseWriter
func writeJSON(w http.ResponseWriter, code int, i interface{}) {
	w.Header().Set(HeaderContentType, MIMEApplicationJSON)
	w.WriteHeader(code)
	err := json.NewEncoder(w).Encode(i)
	if err != nil {
		log.Error(err)
	}
}
