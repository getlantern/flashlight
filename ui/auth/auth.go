package auth

import (
	"context"
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

func withUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey, userID)
}

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

func (h AuthHandler) Routes() []handler.Route {
	authHandler := handler.WrapMiddleware(
		h.authHandler(),
	)
	signOutHandler := handler.WrapMiddleware(
		h.signOutHandler(),
	)
	authRoutes := []handler.Route{
		handler.Route{
			loginEndpoint,
			common.POST,
			authHandler,
		},
		handler.Route{
			registrationEndpoint,
			common.POST,
			authHandler,
		},
		handler.Route{
			signOutEndpoint,
			common.POST,
			signOutHandler,
		},
	}
	return authRoutes
}

// authHandler is the HTTP handler used by the login and
// registration endpoints. It creates a new SRP client from
// the user params in the request
func (h AuthHandler) authHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		params, err := h.getUserParams(req)
		if err != nil {
			log.Errorf("Couldn't create SRP client from request: %v", err)
			h.ErrorHandler(w, err, http.StatusBadRequest)
			return
		}
		endpoint := html.EscapeString(req.URL.Path)
		if strings.Contains(endpoint, loginEndpoint) {
			_, err = h.authClient.SignIn(params.Username, params.Password)
		} else {
			_, err = h.authClient.Register(params.Username, params.Password)
		}
		if err != nil {
			h.ErrorHandler(w, err, http.StatusBadRequest)
		}
	}
}

// getUserParams extracts user credentials from the HTTP request
// and passes those to the auth client based on the endpoint specified
func (h AuthHandler) getUserParams(req *http.Request) (*models.UserParams, error) {
	var params models.UserParams
	err := common.DecodeJSONRequest(req, &params)
	return &params, err
}

func (h AuthHandler) signOutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		url := h.GetAuthAddr(signOutEndpoint)
		log.Debugf("Sending sign out request to %s", url)
		h.ProxyHandler(url, req, w, nil)
	}
}
