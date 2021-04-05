package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/getlantern/auth-server/api"
	"github.com/getlantern/auth-server/client"
	"github.com/getlantern/auth-server/models"
	"github.com/getlantern/flashlight/ui/handler"
	"github.com/getlantern/golog"
	"github.com/go-chi/chi"
)

var (
	ErrInvalidCredentials = errors.New("The supplied user credentials were invalid")
	ErrBadRequest         = errors.New("The request parameters were invalid")
	ErrSRPKeysDifferent   = errors.New("SRP client and server keys do not match")
	log                   = golog.LoggerFor("flashlight.ui.auth")
)

type AuthHandler struct {
	handler.Handler
	authClient client.AuthClient
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
	return "/user"
}

// getUserParams is used to unmarshal JSON from the given request r into
// the the user params type
func getUserParams(w http.ResponseWriter, r *http.Request) (*models.UserParams, error) {
	var params models.UserParams
	return &params, handler.GetParams(w, r, &params)
}

type AuthMethod func(params *models.UserParams) (api.AuthResponse, error)

// authHandler is the HTTP handler used by the login and registration endpoints.
// It creates a new SRP client from the user params in the request
func (h AuthHandler) authHandler(authenticate AuthMethod) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params, err := getUserParams(w, r)
		if err != nil {
			return
		}
		authResp, err := authenticate(params)
		handler.HandleAuthResponse(authResp, w, err)
	})
}

func (h AuthHandler) statusHandler(w http.ResponseWriter, r *http.Request) {
	args := handler.GetQueryParams(r, "lanternUserID", "email")
	var err error
	email, lanternUserID := args["email"], args["lanternUserID"]
	if lanternUserID == "" {
		err = fmt.Errorf("Missing Lantern User ID")
	} else if email == "" {
		err = fmt.Errorf("Missing Lantern email")
	}
	if err != nil {
		handler.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	userID, _ := strconv.ParseInt(lanternUserID, 10, 64)
	_, err = h.authClient.AccountStatus(&models.UserParams{
		LanternUserID: userID,
		Email:         email,
	})
	if err != nil {
		log.Errorf("Error retrieving account status: %v", err)
		handler.ErrorHandler(w, err, http.StatusBadRequest)
	} else {
		handler.SuccessResponse(w)
	}
}

// ConfigureRoutes returns an http.Handler for the auth-based routes
func (h AuthHandler) ConfigureRoutes() http.Handler {
	r := handler.NewRouter()
	r.Group(func(r chi.Router) {
		r.Post("/login", h.authHandler(h.authClient.SignIn))
		r.Post("/register", h.authHandler(h.authClient.Register))
		r.Get("/account/status", h.statusHandler)
		r.Post("/logout", func(w http.ResponseWriter, r *http.Request) {
			params, err := getUserParams(w, r)
			if err != nil {
				return
			}
			authResp, err := h.authClient.SignOut(params.Username)
			if err != nil {
				log.Debugf("User %s successfully signed out", params.Username)
			}
			handler.HandleAuthResponse(authResp, w, err)
		})
	})
	return r
}
