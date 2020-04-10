package params

type Errors map[string]string

type Response struct {
	Error  string `json:"error,omitempty"`
	Errors Errors `json:"errors,omitempty"`
}

func NewResponse(err string, errors Errors) Response {
	return Response{
		err,
		errors,
	}
}
