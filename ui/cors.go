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

func (s *Server) corsHandler(next http.HandlerFunc) http.HandlerFunc {
	uiAddr := fmt.Sprintf("http://%s", s.listenAddr)
	origins := append(corsOrigins, uiAddr)
	cors := cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowedHeaders:   corsAllowedHeaders,
		AllowCredentials: true,
		Debug:            true,
	})
	return func(w http.ResponseWriter, req *http.Request) {
		cors.HandlerFunc(w, req)
	}
}
