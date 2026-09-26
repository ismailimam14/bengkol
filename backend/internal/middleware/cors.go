package middleware

import (
	"net/http"

	"github.com/bengkol/backend/config"
	"github.com/go-chi/cors"
)

// CORS creates a configured CORS middleware handler.
func CORS(cfg config.CORSConfig) func(next http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   cfg.AllowedMethods,
		AllowedHeaders:   cfg.AllowedHeaders,
		AllowCredentials: true,
		MaxAge:           300,
	})
}
