package params

import "github.com/getlantern/auth-server/models"

type Response struct {
	Success bool          `json:"success,omitempty"`
	Error   string        `json:"error,omitempty"`
	Errors  models.Errors `json:"errors,omitempty"`
}

func NewResponse(err string, errors models.Errors) Response {
	return Response{
		false,
		err,
		errors,
	}
}
