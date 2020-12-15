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
)

const (
	userKey = iota

	pathPrefix = "/user"

	authEndpoint          = "/auth"
	loginEndpoint         = "/login"
	signOutEndpoint       = "/logout"
	registrationEndpoint  = "/register"
	balanceEndpoint       = "/balance"
	createAccountEndpoint = "/account/new"
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

// GetPathPrefix returns the top-level route prefix used
// by the AuthHandler
func (h AuthHandler) GetPathPrefix() string {
	return pathPrefix
}

// ConfigureRoutes returns an http.Handler for the auth-based routes
func (h AuthHandler) ConfigureRoutes() http.Handler {
	authHandler := func(w http.ResponseWriter, r *http.Request) {
		// HTTP handler used by the login and
		// registration endpoints. It creates a new SRP client from
		// the user params in the request
		var params models.UserParams
		// extract user credentials from HTTP request to send to AuthClient
		err := handler.DecodeJSONRequest(w, r, &params)
		if err != nil {
			log.Errorf("Couldn't create SRP client from request: %v", err)
			return
		}
		endpoint := html.EscapeString(r.URL.Path)
		if strings.Contains(endpoint, loginEndpoint) {
			_, err = h.authClient.SignIn(params.Username, params.Password)
		} else {
			_, err = h.authClient.Register(params.Username, params.Password)
		}
		if err != nil {
			handler.ErrorHandler(w, err, http.StatusBadRequest)
		}
	}
	s := handler.NewRouter()
	s.Post(loginEndpoint, authHandler)
	s.Post(registrationEndpoint, authHandler)
	s.Post(signOutEndpoint, func(w http.ResponseWriter, r *http.Request) {
		var params models.UserParams
		// extract user credentials from HTTP request to send to AuthClient
		err := handler.DecodeJSONRequest(w, r, &params)
		if err != nil {
			return
		}
		_, err = h.authClient.SignOut(params.Username)
		if err != nil {
			handler.ErrorHandler(w, err, http.StatusBadRequest)
			return
		}
		log.Debugf("User %s successfully signed out", params.Username)
	})

	return s
}
