package api

import "net/http"

type Params struct {
	AuthServerAddr  string
	YinbiServerAddr string
	HttpClient      *http.Client
}
