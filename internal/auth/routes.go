package auth

import (
	"net/http"

	"github.com/devanshbaghel18/ONUSLY/internal/config"
)

func RegisterRoutes(mux *http.ServeMux) {

	repo := NewRepository()
	cfg := config.Load()
	service := NewService(repo, cfg)
	handler := NewHandler(service)

	mux.HandleFunc("POST /auth/google", handler.GoogleLogin)
	mux.HandleFunc("GET /auth/google/callback", handler.GoogleCallback)
}