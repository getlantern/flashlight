package auth

import (
	"errors"
	"html"
	"net/http"
	"strings"

	"github.com/getlantern/auth-server/api"
	"github.com/getlantern/auth-server/client"
	"github.com/getlantern/auth-server/models"
	"github.com/getlantern/flashlight/ui/handler"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-server/common"
	"github.com/gorilla/mux"
)

const (
	userKey               = iota
	authEndpoint          = "/auth"
	loginEndpoint         = "/login"
	signOutEndpoint       = "/user/logout"
	registrationEndpoint  = "/register"
	balanceEndpoint       = "/user/balance"
	createAccountEndpoint = "/user/account/new"
)

var (
	ErrInvalidCredentials = errors.New("The supplied user credentials were invalid")
	ErrBadRequest         = errors.New("The request parameters were invalid")
	ErrSRPKeysDifferent   = errors.New("SRP client and server keys do not match")
	log                   = golog.LoggerFor("flashlight.ui.auth")
)

type AuthHandler struct {
	handler.Handler
	authClient *client.AuthClient
}

// New creates a new auth handler
func New(params api.APIParams) AuthHandler {
	return AuthHandler{
		handler.NewHandler(params),
		client.New(params.AuthServerAddr, params.HTTPClient),
	}
}

func (h AuthHandler) ConfigureRoutes(r *mux.Router) (next func(http.Handler) http.Handler) {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			head, _ := h.GetPath(r.URL.Path)
			switch head {
			case loginEndpoint:
			case registrationEndpoint:
				// HTTP handler used by the login and
				// registration endpoints. It creates a new SRP client from
				// the user params in the request
				var params models.UserParams
				// extract user credentials from HTTP request to send to AuthClient
				err := common.DecodeJSONRequest(r, &params)
				if err != nil {
					log.Errorf("Couldn't create SRP client from request: %v", err)
					h.ErrorHandler(w, err, http.StatusBadRequest)
					return
				}
				endpoint := html.EscapeString(r.URL.Path)
				if strings.Contains(endpoint, loginEndpoint) {
					_, err = h.authClient.SignIn(params.Username, params.Password)
				} else {
					_, err = h.authClient.Register(params.Username, params.Password)
				}
				if err != nil {
					h.ErrorHandler(w, err, http.StatusBadRequest)
				}
			case signOutEndpoint:
				var params models.UserParams
				// extract user credentials from HTTP request to send to AuthClient
				err := common.DecodeJSONRequest(r, &params)
				if err != nil {
					h.ErrorHandler(w, err, http.StatusBadRequest)
					return
				}
				_, err = h.authClient.SignOut(params.Username)
				if err != nil {
					h.ErrorHandler(w, err, http.StatusBadRequest)
					return
				}
				log.Debugf("User %s successfully signed out", params.Username)
			default:
				// no default case
			}
			next.ServeHTTP(w, r)
		})
	}
}
