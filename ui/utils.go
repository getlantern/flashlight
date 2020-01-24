package ui

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/getlantern/lantern-server/models"
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

func writeJSON(w http.ResponseWriter, code int, i interface{}) {
	w.Header().Set(HeaderContentType, MIMEApplicationJSON)
	w.WriteHeader(code)
	err := json.NewEncoder(w).Encode(i)
	if err != nil {
		log.Error(err)
	}
}

func decodeAuthResponse(body []byte) (*models.AuthResponse, error) {
	authResp := new(models.AuthResponse)
	err := json.Unmarshal(body, authResp)
	return authResp, err
}

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

func (s *Server) errorHandler(w http.ResponseWriter, err error, errorCode int) {
	log.Error(err)
	e := map[string]interface{}{
		"error": err.Error(),
	}
	js, err := json.Marshal(e)
	if err != nil {
		log.Error(err)
		return
	}
	w.WriteHeader(errorCode)
	w.Header().Set(HeaderContentType, MIMEApplicationJSON)
	w.Write(js)
}
