package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/devanshbaghel18/ONUSLY/internal/auth"
)

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5174")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", health)

	auth.RegisterRoutes(mux)

	log.Println("Server running on :8080")

	log.Fatal(http.ListenAndServe(":8080", cors(mux)))
}