package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/devanshbaghel18/ONUSLY/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/internal/config"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
	"github.com/devanshbaghel18/ONUSLY/internal/shared"
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {

	cfg := config.Load()

	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("GET /health", health)
	auth.RegisterRoutes(mux)

	// DynamoDB
	db := shared.NewDynamoClient()

	// Goals
	goalRepo := goals.NewRepository(db, "Onusly")
	goalService := goals.NewService(goalRepo)
	goalHandler := goals.NewHandler(goalService)
	proofRepo := proof.NewRepository(db, "Onusly")
	proofService := proof.NewService(proofRepo, goalService)
	proofHandler := proof.NewHandler(proofService)

	// Protected Goals routes
	goalRoutes := http.NewServeMux()

	goalRoutes.HandleFunc("POST /goals", goalHandler.Create)
	goalRoutes.HandleFunc("GET /goals", goalHandler.List)
	goalRoutes.HandleFunc("GET /goals/{id}", goalHandler.GetByID)
	goalRoutes.HandleFunc("DELETE /goals/{id}", goalHandler.Delete)
	goalRoutes.HandleFunc("PATCH /goals/{id}", goalHandler.Update)
	goalRoutes.HandleFunc("POST /goals/{id}/proofs", proofHandler.Submit)
	goalRoutes.HandleFunc("GET /goals/{id}/proofs", proofHandler.ListByGoal)
	goalRoutes.HandleFunc("GET /goals/{id}/proofs/{proofId}", proofHandler.GetByID)
	protectedGoals := middleware.Auth(cfg.JWTSecret)(goalRoutes)

	mux.Handle("/goals", protectedGoals)
	mux.Handle("/goals/", protectedGoals)

	log.Println("Server running on :8080")

	log.Fatal(http.ListenAndServe(":8080", cors(mux)))
}
