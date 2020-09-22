package yinbi

import (
	"errors"
	"strconv"

	"github.com/getlantern/auth-server/models"
)

var (
	ErrMissingDestination   = errors.New("Payment destination is required")
	ErrPaymentInvalidAmount = errors.New("Payment amount must be greater than zero")
	ErrMissingPaymentAmount = errors.New("Payment amount is missing")
	ErrMissingPassword      = errors.New("Password is required")
	ErrMissingUsername      = errors.New("Username is required")
)

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
	Email         string `json:"email,omitempty"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Address       string `json:"address"`
	Authenticated bool   `json:"authenticated,omitempty"`
}

type ImportWalletParams = AuthParams

// Validate validates the payment params and returns
// a map of param names to errors
func (p PaymentParams) Validate() models.Errors {
	errors := make(models.Errors)
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
