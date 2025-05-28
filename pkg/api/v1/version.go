// Package v1 contains the V1 API for ToolHive.
package v1

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/cors"

	"github.com/stacklok/toolhive/pkg/versions"
)

// VersionRouter sets up the version route.
func VersionRouter() http.Handler {
	// Create a permissive CORS handler
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},                                       // Allow all origins
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}, // Allow common HTTP methods
		AllowedHeaders:   []string{"*"},                                       // Allow all headers
		AllowCredentials: true,                                                // Allow cookies
		MaxAge:           300,                                                 // Maximum cache age (in seconds)
	})

	r := chi.NewRouter()
	r.Get("/", getVersion)

	// Wrap the router with CORS middleware
	return corsHandler.Handler(r)
}

type versionResponse struct {
	Version string `json:"version"`
}

//	 getVersion
//		@Summary		Get server version
//		@Description	Returns the current version of the server
//		@Tags			version
//		@Produce		json
//		@Success		200	{object}	versionResponse
//		@Router			/api/v1beta/version [get]
func getVersion(w http.ResponseWriter, _ *http.Request) {
	versionInfo := versions.GetVersionInfo()
	err := json.NewEncoder(w).Encode(versionResponse{Version: versionInfo.Version})
	if err != nil {
		http.Error(w, "Failed to marshal version info", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
}
