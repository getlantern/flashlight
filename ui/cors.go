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

func corsHandler(next http.HandlerFunc) http.HandlerFunc {
	//uiAddr := fmt.Sprintf("http://%s", s.listenAddr)
	//origins := append(corsOrigins, uiAddr)
	log.Debugf("Cors origins: %v", corsOrigins)
	cors := cors.New(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedHeaders:   corsAllowedHeaders,
		AllowCredentials: true,
		Debug:            true,
	})
	return func(w http.ResponseWriter, req *http.Request) {
		cors.HandlerFunc(w, req)
	}
}
