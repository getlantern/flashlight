package handler

import (
	"net/http"

	"github.com/getlantern/flashlight/common"
	"github.com/rs/cors"
)

var corsAllowedHeaders = []string{
	"Origin",
	"Accept",
	"Content-Type",
	"X-Requested-With",
	"Cache",
}

func CORSMiddleware(next http.Handler) http.Handler {
	cors := cors.New(cors.Options{
		AllowOriginFunc:  common.IsOriginAllowed,
		AllowedHeaders:   corsAllowedHeaders,
		AllowCredentials: true,
		Debug:            true,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		cors.Handler(next).ServeHTTP(w, req)
	})
}
