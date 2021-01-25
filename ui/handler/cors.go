package handler

import (
	"net/http"

	"github.com/rs/cors"
)

var corsOrigins = []string{
	"http://localhost:2000",
	"http://localhost:8080",
	"https://localhost:2000",
}

var corsAllowedHeaders = []string{
	"Origin",
	"Accept",
	"Content-Type",
	"X-Requested-With",
	"Cache",
}

func CORSMiddleware(next http.Handler) http.Handler {
	cors := cors.New(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedHeaders:   corsAllowedHeaders,
		AllowCredentials: true,
		Debug:            true,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		cors.Handler(next).ServeHTTP(w, req)
	})
}
