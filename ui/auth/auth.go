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

type AuthMethod func(params *models.UserParams) (*api.AuthResponse, error)

// authHandler is the HTTP handler used by the login and registration endpoints.
// It creates a new SRP client from the user params in the request
func (h AuthHandler) authHandler(authenticate AuthMethod) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params, err := getUserParams(w, r)
		if err != nil {
			return
		}
		authResp, err := authenticate(params)
		if err != nil {
			var e interface{}
			if authResp != nil && len(authResp.Errors) > 0 {
				e = authResp.Errors
			} else {
				e = err
			}
			handler.ErrorHandler(w, e, http.StatusBadRequest)
		} else {
			handler.HandleAuthResponse(authResp, w, err)
		}
	})
}

// accountStatusHandler is an HTTP handler used for checking if a Lantern Pro
// user already has an existing Lantern user account given an email and
// lantern user ID. If so, a success response is returned.
func (h AuthHandler) accountStatusHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	vals := r.URL.Query()
	email := vals.Get("email")
	lanternUserID := vals.Get("lanternUserID")
	if lanternUserID == "" {
		err = fmt.Errorf("missing Lantern User ID")
	} else if email == "" {
		err = fmt.Errorf("missing Lantern email")
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

// sessionHandler is used to fetch the current Lantern user session
func (h AuthHandler) sessionHandler(w http.ResponseWriter, r *http.Request) {
	isAuthenticated, err := h.authClient.IsAuthenticated()
	if err != nil || !isAuthenticated {
		if err == nil {
			err = fmt.Errorf("no active session")
		}
		log.Errorf("Error retrieving account status: %v", err)
		handler.ErrorHandler(w, err, http.StatusUnauthorized)
	} else {
		handler.SuccessResponse(w)
	}
}

// signOutHandler is the handler used for destroying user sessions
func (h AuthHandler) signOutHandler(w http.ResponseWriter, r *http.Request) {
	authResp, err := h.authClient.SignOut()
	handler.HandleAuthResponse(authResp, w, err)
}

// ConfigureRoutes returns an http.Handler for the auth-based routes
func (h AuthHandler) ConfigureRoutes() http.Handler {
	r := handler.NewRouter()
	r.Group(func(r chi.Router) {
		r.Post("/login", h.authHandler(h.authClient.SignIn))
		r.Post("/register", h.authHandler(h.authClient.Register))
		r.Get("/session", h.sessionHandler)
		r.Get("/account/status", h.accountStatusHandler)
		r.Post("/logout", h.signOutHandler)
	})
	return r
}
