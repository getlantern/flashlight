package ui

import (
	"errors"
	"net/http"
	"strconv"
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

// PaymentParams specifies the necessary parameters for
// sending a YNB payment
type PaymentParams struct {
	AuthParams
	Destination string `json:"destination"`
	Amount      string `json:"amount"`
	Asset       string `json:"asset"`
}

// AuthParams specifies the necessary params for requests that require a
// user's credentials
type AuthParams struct {
	Email    string `json:"email,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ImportWalletResponse struct {
	Username string `json:"username"`
	Address  string `json:"address"`
	Salt     string `json:"salt"`
	Seed     string `json:"seed"`
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

// Validate validates the payment params and returns
// a map of param names to errors
func (p PaymentParams) Validate() Errors {
	errors := make(Errors)
	if p.Password == "" {
		errors["password"] = ErrMissingPassword.Error()
	}
	if p.Username == "" {
		errors["username"] = ErrMissingUsername.Error()
	}
	if p.Destination == "" {
		errors["destination"] = ErrMissingDestination.Error()
	}
	if p.Amount == "" {
		errors["amount"] = ErrMissingPaymentAmount.Error()
	}
	amount, err := strconv.Atoi(p.Amount)
	if err != nil || amount < 0 {
		errors["amount"] = ErrPaymentInvalidAmount.Error()
	}
	return errors
}
