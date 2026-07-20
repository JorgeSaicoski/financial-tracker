package api

import (
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/interfaces/api/handlers"
)

func NewRouter(movementHandler *handlers.MovementHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /movements", movementHandler.CreateMovement)
	mux.HandleFunc("GET /movements", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("id") {
			movementHandler.GetMovement(w, r)
		} else {
			movementHandler.ListMovements(w, r)
		}
	})

	return withCORS(mux)
}

// withCORS is a permissive, dev-only CORS layer so the Svelte app (running
// on its own port) can call this API directly. Tighten this before any
// non-local deployment.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
