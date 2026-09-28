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

func main() {

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", health)

	auth.RegisterRoutes(mux)

	log.Println("Server running on :8080")

	log.Fatal(http.ListenAndServe(":8080", mux))
}