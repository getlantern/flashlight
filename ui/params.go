package ui

import (
	"errors"
	"net/http"
)

var (
	ErrMissingDestination   = errors.New("Payment destination is required")
	ErrPaymentInvalidAmount = errors.New("Payment amount must be greater than zero")
	ErrMissingPaymentAmount = errors.New("Payment amount is missing")
	ErrMissingPassword      = errors.New("Password is required")
	ErrMissingUsername      = errors.New("Username is required")
)

type Errors map[string]string

type Response struct {
	Error  string `json:"error,omitempty"`
	Errors Errors `json:"errors,omitempty"`
}

type RedeemParams struct {
	Codes []string `json:"codes"`
}

// ServerParams specifies the parameters to use
// when creating new UI server
type ServerParams struct {
	ExtURL         string
	AuthServerAddr string
	LocalHTTPToken string
	Standalone     bool
	HTTPClient     *http.Client
}

// AuthParams specifies the necessary params for requests that require a
// user's credentials
type AuthParams struct {
	Email    string `json:"email,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (p AuthParams) Validate() Errors {
	errors := make(Errors)
	if p.Password == "" {
		errors["password"] = ErrMissingPassword.Error()
	}
	if p.Username == "" {
		errors["username"] = ErrMissingUsername.Error()
	}
	return errors
}
