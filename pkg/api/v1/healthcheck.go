package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/cors"
)

// HealthcheckRouter sets up healthcheck route.
func HealthcheckRouter() http.Handler {
	// Create a permissive CORS handler
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},                                       // Allow all origins
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}, // Allow common HTTP methods
		AllowedHeaders:   []string{"*"},                                       // Allow all headers
		AllowCredentials: true,                                                // Allow cookies
		MaxAge:           300,                                                 // Maximum cache age (in seconds)
	})

	r := chi.NewRouter()
	r.Get("/", getHealthcheck)

	// Wrap the router with CORS middleware
	return corsHandler.Handler(r)
}

func getHealthcheck(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
