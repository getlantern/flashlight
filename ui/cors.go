package ui

import (
	"fmt"
	"net/http"

	"github.com/rs/cors"
)

var defaultCorsOrigins = []string{
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

func (s *Server) corsHandler(next http.Handler) http.Handler {
	corsOrigins := defaultCorsOrigins
	corsOrigins = append(corsOrigins, fmt.Sprintf("http://%s", s.listenAddr))
	cors := cors.New(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedHeaders:   corsAllowedHeaders,
		AllowCredentials: true,
		Debug:            true,
	})
	return cors.Handler(next)
}
