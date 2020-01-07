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

func writeJSON(w http.ResponseWriter,
	code int, i interface{}) error {
	w.Header().Set(HeaderContentType, MIMEApplicationJSON)
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(i)
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
