package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/approval"
	"github.com/devanshbaghel18/ONUSLY/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/internal/chat"
	"github.com/devanshbaghel18/ONUSLY/internal/config"
	"github.com/devanshbaghel18/ONUSLY/internal/enforcement"
	"github.com/devanshbaghel18/ONUSLY/internal/events"
	"github.com/devanshbaghel18/ONUSLY/internal/friends"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/notifications"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
	"github.com/devanshbaghel18/ONUSLY/internal/queue"
	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
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
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, ngrok-skip-browser-warning, Bypass-Tunnel-Reminder, If-None-Match")
		w.Header().Set("Access-Control-Expose-Headers", "ETag")
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
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	mux := http.NewServeMux()

	// DynamoDB
	db := shared.NewDynamoClient()

	// Auth repository, service & handler
	authRepo := auth.NewRepository()
	authService := auth.NewService(authRepo, cfg)
	authHandler := auth.NewHandler(authService)

	// Public routes
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("POST /auth/google", authHandler.GoogleLogin)
	mux.HandleFunc("GET /auth/google/callback", authHandler.GoogleCallback)

	// Real-time WebSocket Hub
	wsHub := realtime.NewHub()
	mux.HandleFunc("GET /ws", realtime.ServeWS(wsHub, cfg.JWTSecret))
	mux.HandleFunc("GET /ws/", realtime.ServeWS(wsHub, cfg.JWTSecret))

	// Goals
	goalRepo := goals.NewRepository(db, "Onusly")
	goalService := goals.NewService(goalRepo, authRepo)
	goalHandler := goals.NewHandler(goalService)

	// Enforcement
	enforcementService := enforcement.NewService(goalService)
	enforcementHandler := enforcement.NewHandler(enforcementService)

	// Proof
	proofRepo := proof.NewRepository(db, "Onusly")
	proofService := proof.NewService(proofRepo, goalService)
	proofHandler := proof.NewHandler(proofService)

	// Approval
	approvalRepo := approval.NewRepository(db, "Onusly")
	approvalService := approval.NewService(
		approvalRepo,
		goalService,
		proofService,
	)

	// Queue configuration
	proofQueueURL := os.Getenv("PROOF_EVENTS_QUEUE_URL")
	approvalQueueURL := os.Getenv("APPROVAL_EVENTS_QUEUE_URL")
	proofQueueName := "proof-events"
	approvalQueueName := "approval-events"

	var eventQueue queue.Queue
	if proofQueueURL != "" && approvalQueueURL != "" {
		sqsClient := shared.NewSQSClient()
		if sqsClient != nil {
			log.Println("[Queue] Using AWS SQS queue adapter with configured queue URLs")
			eventQueue = queue.NewSQSQueue(sqsClient, map[string]string{
				proofQueueName:    proofQueueURL,
				approvalQueueName: approvalQueueURL,
			})
		}
	}
	if eventQueue == nil {
		log.Println("[Queue] Using in-memory queue adapter (set PROOF_EVENTS_QUEUE_URL and APPROVAL_EVENTS_QUEUE_URL to use AWS SQS)")
		eventQueue = queue.NewMemoryQueue()
	}

	// Shared Publisher abstraction (Slice A WebSocket + Slice B Queue)
	wsPublisher := realtime.NewWebSocketPublisher(wsHub)
	queuePublisher := events.NewQueuePublisher(eventQueue, proofQueueName, approvalQueueName)
	domainPublisher := events.NewCompositePublisher(wsPublisher, queuePublisher)

	proofService.SetPublisher(domainPublisher)
	approvalService.SetPublisher(domainPublisher)

	// Notifications
	notifRepo := notifications.NewRepository(db, "Onusly")
	notifService := notifications.NewService(notifRepo)
	notifHandler := notifications.NewHandler(notifService)

	// Background Notification Worker
	notifWorker := notifications.NewWorker(eventQueue, notifService, proofQueueName, approvalQueueName, 250*time.Millisecond)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	notifWorker.Start(workerCtx)
	log.Println("[Worker] Notification worker started")

	approvalHandler := approval.NewHandler(approvalService)

	// Chat Repository, Store & Handler
	chatRepo := chat.NewRepository(db, "Onusly")
	chatResolver := &serverUserResolver{authRepo: authRepo}
	chatHandler := chat.NewHandler(chatRepo)
	chatHandler.SetUserResolver(chatResolver)
	chatHandler.SetBroadcaster(wsHub)
	wsHub.SetMessageStore(chatRepo)
	wsHub.SetGoalVerifier(&serverGoalVerifier{goalService: goalService})
	wsHub.SetUserResolver(chatResolver)

	// Friends & Communities Repository, Service & Handler
	friendsRepo := friends.NewRepository(db, "Onusly")
	friendsService := friends.NewService(friendsRepo, chatResolver, wsHub)
	friendsHandler := friends.NewHandler(friendsService)

	// Protected routes
	goalRoutes := http.NewServeMux()

	// Notifications
	goalRoutes.HandleFunc(
		"GET /notifications",
		notifHandler.List,
	)
	goalRoutes.HandleFunc(
		"PATCH /notifications/{id}/read",
		notifHandler.MarkAsRead,
	)
	goalRoutes.HandleFunc(
		"POST /notifications/read-all",
		notifHandler.MarkAllAsRead,
	)

	// Chat
	goalRoutes.HandleFunc(
		"GET /chat/messages",
		chatHandler.GetHistory,
	)
	goalRoutes.HandleFunc(
		"POST /chat/messages",
		chatHandler.SendMessage,
	)
	goalRoutes.HandleFunc(
		"DELETE /chat/messages",
		chatHandler.ClearHistory,
	)

	// Friends
	goalRoutes.HandleFunc(
		"POST /friends/request",
		friendsHandler.SendRequest,
	)
	goalRoutes.HandleFunc(
		"GET /friends/requests",
		friendsHandler.GetRequests,
	)
	goalRoutes.HandleFunc(
		"POST /friends/request/respond",
		friendsHandler.RespondRequest,
	)
	goalRoutes.HandleFunc(
		"GET /friends",
		friendsHandler.GetFriends,
	)
	goalRoutes.HandleFunc(
		"DELETE /friends/",
		friendsHandler.DeleteFriend,
	)

	// Communities Discovery & Membership
	goalRoutes.HandleFunc(
		"GET /communities/trending",
		friendsHandler.GetTrendingCommunities,
	)
	goalRoutes.HandleFunc(
		"GET /communities/joined",
		friendsHandler.GetJoinedCommunities,
	)
	goalRoutes.HandleFunc(
		"POST /communities/join",
		friendsHandler.JoinCommunity,
	)
	goalRoutes.HandleFunc(
		"POST /communities/leave",
		friendsHandler.LeaveCommunity,
	)

	// Goals
	goalRoutes.HandleFunc(
		"POST /goals",
		goalHandler.Create,
	)

	goalRoutes.HandleFunc(
		"GET /goals",
		goalHandler.List,
	)

	goalRoutes.HandleFunc(
		"GET /goals/{id}",
		goalHandler.GetByID,
	)

	goalRoutes.HandleFunc(
		"DELETE /goals/{id}",
		goalHandler.Delete,
	)

	goalRoutes.HandleFunc(
		"PATCH /goals/{id}",
		goalHandler.Update,
	)

	// Proofs
	goalRoutes.HandleFunc(
		"POST /goals/{id}/proofs",
		proofHandler.Submit,
	)

	goalRoutes.HandleFunc(
		"GET /goals/{id}/proofs",
		proofHandler.ListByGoal,
	)

	goalRoutes.HandleFunc(
		"GET /goals/{id}/proofs/{proofId}",
		proofHandler.GetByID,
	)

	// Approval
	goalRoutes.HandleFunc(
		"POST /goals/{id}/proofs/{proofId}/approval",
		approvalHandler.Decide,
	)

	goalRoutes.HandleFunc(
		"GET /goals/{id}/approvals",
		approvalHandler.ListByGoal,
	)

	goalRoutes.HandleFunc(
		"GET /goals/{id}/approvals/{approvalId}",
		approvalHandler.GetByID,
	)

	// Users & Public Profiles
	goalRoutes.HandleFunc(
		"GET /users/search",
		authHandler.LookupUser,
	)

	goalRoutes.HandleFunc(
		"PATCH /users/handle",
		authHandler.UpdateHandle,
	)

	goalRoutes.HandleFunc(
		"GET /users/me",
		authHandler.GetMe,
	)

	// Enforcement Blocklist
	goalRoutes.HandleFunc(
		"GET /enforcement/blocklist",
		enforcementHandler.GetBlocklist,
	)

	// JWT authentication for all protected routes
	protectedRoutes := middleware.Auth(cfg.JWTSecret)(goalRoutes)

	mux.Handle("/goals", protectedRoutes)
	mux.Handle("/goals/", protectedRoutes)
	mux.Handle("/chat", protectedRoutes)
	mux.Handle("/chat/", protectedRoutes)
	mux.Handle("/users", protectedRoutes)
	mux.Handle("/users/", protectedRoutes)
	mux.Handle("/notifications", protectedRoutes)
	mux.Handle("/notifications/", protectedRoutes)
	mux.Handle("/friends", protectedRoutes)
	mux.Handle("/friends/", protectedRoutes)
	mux.Handle("/communities", protectedRoutes)
	mux.Handle("/communities/", protectedRoutes)
	mux.Handle("/enforcement", protectedRoutes)
	mux.Handle("/enforcement/", protectedRoutes)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      cors(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Println("Server running on :8080")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server listen error: %v", err)
		}
	}()

	<-stopCtx.Done()
	log.Println("Gracefully stopping server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped cleanly.")
}

type serverGoalVerifier struct {
	goalService *goals.Service
}

func (v *serverGoalVerifier) VerifyApprovedGoal(ctx context.Context, callerID, callerEmail, callerHandle, goalID string) (string, string, bool, error) {
	goal, err := v.goalService.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return "", "", false, err
	}
	if !goal.IsApprover(callerID, callerEmail, callerHandle) {
		return "", "", false, nil
	}
	if goal.Status != "completed" {
		return "", "", false, nil
	}
	return goal.OwnerID, goal.Title, true, nil
}

type serverUserResolver struct {
	authRepo auth.Repository
}

func (r *serverUserResolver) ResolveUser(ctx context.Context, identifier string) (string, string, string, error) {
	clean := strings.TrimSpace(identifier)
	if clean == "" {
		return "", "", "", errors.New("empty user identifier")
	}

	// 0. If community identifier, return directly
	if strings.HasPrefix(clean, "comm-") || strings.HasPrefix(clean, "comm_") {
		return clean, "", "", nil
	}

	// 1. If starts with @, resolve by handle
	if strings.HasPrefix(clean, "@") {
		h := strings.TrimPrefix(clean, "@")
		u, err := r.authRepo.GetUserByHandle(ctx, h)
		if err == nil && u != nil {
			return u.ID, u.Email, u.Handle, nil
		}
		return "", "", "", fmt.Errorf("user with handle %s not found", clean)
	}

	// 2. If contains @ and ., resolve by email
	if strings.Contains(clean, "@") && strings.Contains(clean, ".") {
		u, err := r.authRepo.GetUserByEmail(ctx, clean)
		if err == nil && u != nil {
			return u.ID, u.Email, u.Handle, nil
		}
		uH, errH := r.authRepo.GetUserByHandle(ctx, clean)
		if errH == nil && uH != nil {
			return uH.ID, uH.Email, uH.Handle, nil
		}
		return "", "", "", fmt.Errorf("user with email %s not found", clean)
	}

	// 3. Otherwise try ID first, then handle, then email
	u, err := r.authRepo.GetUserByID(ctx, clean)
	if err == nil && u != nil {
		return u.ID, u.Email, u.Handle, nil
	}

	uH, errH := r.authRepo.GetUserByHandle(ctx, clean)
	if errH == nil && uH != nil {
		return uH.ID, uH.Email, uH.Handle, nil
	}

	uE, errE := r.authRepo.GetUserByEmail(ctx, clean)
	if errE == nil && uE != nil {
		return uE.ID, uE.Email, uE.Handle, nil
	}

	return "", "", "", fmt.Errorf("user %s not found", clean)
}
