package ui

import (
	"fmt"
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

func (s *Server) corsHandler(next http.Handler) http.Handler {
	uiAddr := fmt.Sprintf("http://%s", s.listener.accessAddr)
	corsOrigins = append(corsOrigins, uiAddr)
	cors := cors.New(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedHeaders:   corsAllowedHeaders,
		AllowCredentials: true,
		Debug:            true,
	})
	return cors.Handler(next)
}
