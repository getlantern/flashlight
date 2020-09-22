package auth

import (
	"errors"

	"github.com/getlantern/auth-server/models"
)

var (
	ErrMissingPassword = errors.New("Password is required")
	ErrMissingUsername = errors.New("Username is required")
)

// AuthParams specifies the necessary params for requests that require a
// user's credentials
type AuthParams struct {
	Email    string `json:"email,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (p AuthParams) Validate() models.Errors {
	errors := make(models.Errors)
	if p.Password == "" {
		errors["password"] = ErrMissingPassword.Error()
	}
	if p.Username == "" {
		errors["username"] = ErrMissingUsername.Error()
	}
	return errors
}
