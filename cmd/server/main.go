package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/devanshbaghel18/ONUSLY/internal/approval"
	"github.com/devanshbaghel18/ONUSLY/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/internal/chat"
	"github.com/devanshbaghel18/ONUSLY/internal/config"
	"github.com/devanshbaghel18/ONUSLY/internal/enforcement"
	"github.com/devanshbaghel18/ONUSLY/internal/events"
	"github.com/devanshbaghel18/ONUSLY/internal/friends"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/metrics"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/notifications"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
	"github.com/devanshbaghel18/ONUSLY/internal/queue"
	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
	"github.com/devanshbaghel18/ONUSLY/internal/shared"
	"github.com/devanshbaghel18/ONUSLY/internal/wellbeing"
)

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

type tableDescriber interface {
	DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
}

func isLoopbackClient(r *http.Request) bool {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if ip := net.ParseIP(first); ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

func warmDynamo(ctx context.Context, describer tableDescriber, tableName string) error {
	if describer == nil {
		return errors.New("dynamodb client is nil")
	}
	warmCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := describer.DescribeTable(warmCtx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	return err
}

// newWarmHealthHandler returns a loopback-only readiness/keep-warm handler that
// exercises the DynamoDB connection pool via DescribeTable with a bounded cache TTL.
func newWarmHealthHandler(describer tableDescriber, tableName string, cacheTTL time.Duration) http.HandlerFunc {
	var mu sync.Mutex
	var lastChecked time.Time
	var lastHealthy bool

	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackClient(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		now := time.Now()
		mu.Lock()
		if !lastChecked.IsZero() && cacheTTL > 0 && now.Sub(lastChecked) < cacheTTL {
			healthy := lastHealthy
			mu.Unlock()
			writeWarmHealthResponse(w, healthy, true)
			return
		}
		mu.Unlock()

		err := warmDynamo(r.Context(), describer, tableName)
		if err != nil {
			log.Printf("[Warmup] DynamoDB DescribeTable failed for table %q: %v", tableName, err)
		}

		mu.Lock()
		lastChecked = time.Now()
		lastHealthy = err == nil
		healthy := lastHealthy
		mu.Unlock()

		writeWarmHealthResponse(w, healthy, false)
	}
}

func writeWarmHealthResponse(w http.ResponseWriter, healthy, cached bool) {
	w.Header().Set("Content-Type", "application/json")
	if !healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "degraded",
			"dynamodb": "unreachable",
			"cached":   cached,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"dynamodb": "warm",
		"cached":   cached,
	})
}

func isAllowedOrigin(origin, host, envMode string, allowedOrigins []string) bool {
	if origin == "" {
		return true // Direct non-browser clients (CLI, agents, mobile apps)
	}
	origin = strings.TrimSpace(origin)
	originLower := strings.ToLower(origin)

	for _, o := range allowedOrigins {
		if strings.EqualFold(originLower, strings.ToLower(strings.TrimSpace(o))) {
			return true
		}
	}

	// Permit local development origins only outside production
	if !strings.EqualFold(strings.TrimSpace(envMode), "production") {
		if originLower == "http://localhost:3000" ||
			originLower == "http://localhost:5173" ||
			originLower == "http://127.0.0.1:3000" ||
			originLower == "http://127.0.0.1:5173" ||
			originLower == "http://localhost:8080" ||
			originLower == "http://127.0.0.1:8080" {
			return true
		}
	}

	// Same-origin check
	if host != "" {
		hostLower := strings.ToLower(host)
		if originLower == "http://"+hostLower || originLower == "https://"+hostLower {
			return true
		}
	}

	return false
}

func cors(envMode string, allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")
			origin := r.Header.Get("Origin")
			if origin != "" {
				if isAllowedOrigin(origin, r.Host, envMode, allowedOrigins) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				} else {
					if r.Method == http.MethodOptions {
						http.Error(w, "forbidden: origin not allowed", http.StatusForbidden)
						return
					}
				}
			}

			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, ngrok-skip-browser-warning, Bypass-Tunnel-Reminder, If-None-Match, Idempotency-Key, X-Device-ID")
			w.Header().Set("Access-Control-Expose-Headers", "ETag")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// validateAndResolveBindAddress enforces loopback-only binding in production to prevent
// accidental public exposure of the Go backend port (:8080).
func validateAndResolveBindAddress(host, port string) (string, error) {
	if host == "" || strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "8080"
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("invalid HOST IP address: %q", host)
	}

	// Explicitly reject unspecified 0.0.0.0 and :: addresses
	if ip.IsUnspecified() {
		return "", fmt.Errorf("security violation: binding to unspecified address %q (0.0.0.0/::) is strictly forbidden; backend must bind strictly to loopback (127.0.0.1)", host)
	}

	// Enforce loopback binding in production
	if !ip.IsLoopback() {
		allowInsecure := strings.EqualFold(os.Getenv("ALLOW_INSECURE_BIND"), "true")
		if !allowInsecure {
			return "", fmt.Errorf("security violation: backend host %q is not a loopback address (127.0.0.1/::1); external public exposure is forbidden in production. Set ALLOW_INSECURE_BIND=true for explicit development-only testing", host)
		}
		log.Printf("[SECURITY WARNING] Non-loopback binding permitted via explicit development override ALLOW_INSECURE_BIND=true: %s\n", host)
	}

	return net.JoinHostPort(host, port), nil
}

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	mux := http.NewServeMux()

	// DynamoDB table configuration (supports environment isolation e.g. Onusly-Staging)
	tableName := cfg.DynamoTable
	if tableName == "" {
		tableName = "Onusly"
	}

	// DynamoDB
	db := shared.NewDynamoClient()
	go func() {
		start := time.Now()
		if err := warmDynamo(context.Background(), db, tableName); err != nil {
			log.Printf("[Warmup] Initial DynamoDB connection warmup failed (%v): %v", time.Since(start).Round(time.Millisecond), err)
		} else {
			log.Printf("[Warmup] Initial DynamoDB connection pool warmed in %v (table=%s)", time.Since(start).Round(time.Millisecond), tableName)
		}
	}()

	// Auth repository, service & handler
	authRepo := auth.NewRepositoryWithTable(db, tableName)
	authService := auth.NewService(authRepo, cfg)
	authHandler := auth.NewHandler(authService)

	// Rate limiters for sensitive endpoints
	authLimiter := middleware.NewRateLimiter(30, time.Minute)
	pairingLimiter := middleware.NewRateLimiter(20, time.Minute)
	apiLimiter := middleware.NewUserRateLimiter(180, time.Minute)

	// Public routes
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /health/warm", newWarmHealthHandler(db, tableName, 60*time.Second))
	mux.HandleFunc("GET /metrics", metrics.DefaultRegistry.Handler())
	mux.Handle("POST /auth/google", authLimiter.Limit(http.HandlerFunc(authHandler.GoogleLogin)))
	mux.Handle("GET /auth/google/login", authLimiter.Limit(http.HandlerFunc(authHandler.GoogleLoginRedirect)))
	mux.Handle("GET /auth/google/callback", authLimiter.Limit(http.HandlerFunc(authHandler.GoogleCallback)))
	mux.HandleFunc("POST /auth/logout", authHandler.Logout)

	// Downloads & Enterprise Extension CRX / Update Manifest with path-traversal guard
	downloadsDir := "frontend/public/downloads"
	fileServer := http.FileServer(http.Dir(downloadsDir))
	downloadsHandler := http.StripPrefix("/downloads/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimSpace(r.URL.Path)
		if strings.Contains(cleanPath, "..") || strings.Contains(cleanPath, "%2e") || strings.Contains(cleanPath, "\x00") {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		// Extend write deadline beyond global 30s server timeout for binary downloads (.crx/.zip/.exe/.apk)
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Minute))
		if strings.HasSuffix(cleanPath, ".crx") {
			w.Header().Set("Content-Type", "application/x-chrome-extension")
		} else if strings.HasSuffix(cleanPath, ".xml") {
			w.Header().Set("Content-Type", "application/xml")
		} else if strings.HasSuffix(cleanPath, ".apk") {
			w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		}
		fileServer.ServeHTTP(w, r)
	}))
	mux.Handle("GET /downloads/", downloadsHandler)

	// Real-time WebSocket Hub
	wsHub := realtime.NewHub()
	authService.SetSessionDisconnecter(wsHub)
	mux.HandleFunc("GET /ws", realtime.ServeWSWithValidator(wsHub, cfg.JWTSecret, authRepo))
	mux.HandleFunc("GET /ws/", realtime.ServeWSWithValidator(wsHub, cfg.JWTSecret, authRepo))

	// Goals
	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, authRepo)
	goalHandler := goals.NewHandler(goalService)

	// Enforcement
	enforcementRepo := enforcement.NewDynamoRepository(db, tableName)
	enforcementService := enforcement.NewService(goalService)
	enforcementService.SetDeviceRepository(enforcementRepo)
	enforcementService.SetTokenGenerator(authService)
	enforcementHandler := enforcement.NewHandler(enforcementService)

	// Proof
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	proofHandler := proof.NewHandler(proofService)

	// Proof & Media Uploads Static Server (authenticated + owner/approver ACL + path-traversal & XSS guard)
	uploadsDir := "uploads"
	protectedUploads := newUploadsServeHandler(uploadsDir, cfg.JWTSecret, authRepo, goalService, proofRepo)
	mux.Handle("GET /uploads/", protectedUploads)

	// Approval
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(
		approvalRepo,
		goalService,
		proofService,
	)
	approvalService.SetAuthRepo(authRepo)

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
	notifRepo := notifications.NewRepository(db, tableName)
	notifService := notifications.NewService(notifRepo)
	notifService.SetBroadcaster(&serverNotifBroadcaster{hub: wsHub})
	enforcementService.SetNotificationSender(notifService)
	goalService.SetNotificationSender(notifService)
	notifHandler := notifications.NewHandler(notifService)

	// Background Notification Worker
	notifWorker := notifications.NewWorker(eventQueue, notifService, proofQueueName, approvalQueueName, 250*time.Millisecond)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	notifWorker.Start(workerCtx)
	log.Println("[Worker] Notification worker started")

	approvalHandler := approval.NewHandler(approvalService)

	// Chat Repository, Store & Handler
	chatRepo := chat.NewRepository(db, tableName)
	chatResolver := &serverUserResolver{authRepo: authRepo}
	chatHandler := chat.NewHandler(chatRepo)
	chatHandler.SetUserResolver(chatResolver)
	chatHandler.SetBroadcaster(wsHub)
	wsHub.SetMessageStore(chatRepo)
	wsHub.SetGoalVerifier(&serverGoalVerifier{goalService: goalService})
	wsHub.SetUserResolver(chatResolver)

	// Friends & Communities Repository, Service & Handler
	friendsRepo := friends.NewRepository(db, tableName)
	friendsService := friends.NewService(friendsRepo, chatResolver, wsHub)
	friendsService.SetNotificationSender(notifService)
	friendsHandler := friends.NewHandler(friendsService)
	goalService.SetFriendFinder(&serverFriendFinder{friendsRepo: friendsRepo})
	chatHandler.SetCommunityChecker(&serverCommunityChecker{
		friendsRepo: friendsRepo,
		hub:         wsHub,
	})

	// Digital Wellbeing Analytics Repository, Service & Handler
	wellbeingRepo := wellbeing.NewDynamoRepository(db, tableName)
	wellbeingService := wellbeing.NewService(wellbeingRepo)
	wellbeingService.SetLimitNotifier(&serverWellbeingNotifier{hub: wsHub})
	wellbeingHandler := wellbeing.NewHandler(wellbeingService)
	enforcementService.SetWellbeingProvider(wellbeingService)

	// Protected routes (user-only mutations & approvals enforced via RequireUserToken;
	// device enforcement & wellbeing telemetry endpoints permit both user and device JWTs)
	goalRoutes := http.NewServeMux()
	requireUser := func(h http.HandlerFunc) http.Handler {
		return middleware.RequireUserToken(h)
	}

	// Notifications
	goalRoutes.HandleFunc(
		"GET /notifications",
		notifHandler.List,
	)
	goalRoutes.Handle(
		"PATCH /notifications/{id}/read",
		requireUser(notifHandler.MarkAsRead),
	)
	goalRoutes.Handle(
		"POST /notifications/read-all",
		requireUser(notifHandler.MarkAllAsRead),
	)

	// Chat
	goalRoutes.Handle(
		"GET /chat/messages",
		requireUser(chatHandler.GetHistory),
	)
	goalRoutes.Handle(
		"POST /chat/messages",
		requireUser(chatHandler.SendMessage),
	)
	goalRoutes.Handle(
		"DELETE /chat/messages",
		requireUser(chatHandler.ClearHistory),
	)

	// Friends
	goalRoutes.Handle(
		"POST /friends/request",
		requireUser(friendsHandler.SendRequest),
	)
	goalRoutes.Handle(
		"GET /friends/requests",
		requireUser(friendsHandler.GetRequests),
	)
	goalRoutes.Handle(
		"POST /friends/request/respond",
		requireUser(friendsHandler.RespondRequest),
	)
	goalRoutes.Handle(
		"GET /friends",
		requireUser(friendsHandler.GetFriends),
	)
	goalRoutes.Handle(
		"DELETE /friends/",
		requireUser(friendsHandler.DeleteFriend),
	)

	// Communities Discovery & Membership
	goalRoutes.Handle(
		"GET /communities/trending",
		requireUser(friendsHandler.GetTrendingCommunities),
	)
	goalRoutes.Handle(
		"GET /communities/joined",
		requireUser(friendsHandler.GetJoinedCommunities),
	)
	goalRoutes.Handle(
		"POST /communities/join",
		requireUser(friendsHandler.JoinCommunity),
	)
	goalRoutes.Handle(
		"POST /communities/leave",
		requireUser(friendsHandler.LeaveCommunity),
	)

	// Goals (mutations strictly user-only so stolen device JWTs cannot unlock/delete goals)
	goalRoutes.Handle(
		"POST /goals",
		requireUser(goalHandler.Create),
	)

	goalRoutes.Handle(
		"GET /goals",
		requireUser(goalHandler.List),
	)

	goalRoutes.Handle(
		"GET /goals/{id}",
		requireUser(goalHandler.GetByID),
	)

	goalRoutes.Handle(
		"DELETE /goals/{id}",
		requireUser(goalHandler.Delete),
	)

	goalRoutes.Handle(
		"PATCH /goals/{id}",
		requireUser(goalHandler.Update),
	)

	// Proofs
	goalRoutes.Handle(
		"POST /goals/{id}/proofs",
		requireUser(proofHandler.Submit),
	)

	goalRoutes.Handle(
		"GET /goals/{id}/proofs",
		requireUser(proofHandler.ListByGoal),
	)

	goalRoutes.Handle(
		"GET /goals/{id}/proofs/{proofId}",
		requireUser(proofHandler.GetByID),
	)

	// Approval
	goalRoutes.Handle(
		"POST /goals/{id}/proofs/{proofId}/approval",
		requireUser(approvalHandler.Decide),
	)

	goalRoutes.Handle(
		"GET /goals/{id}/approvals",
		requireUser(approvalHandler.ListByGoal),
	)

	goalRoutes.Handle(
		"GET /goals/{id}/approvals/{approvalId}",
		requireUser(approvalHandler.GetByID),
	)

	goalRoutes.Handle(
		"GET /goals/{id}/submissions",
		requireUser(approvalHandler.GetSubmissions),
	)

	goalRoutes.Handle(
		"GET /approvals/inbox",
		requireUser(approvalHandler.GetInbox),
	)

	// Users & Public Profiles
	goalRoutes.Handle(
		"GET /users/search",
		requireUser(authHandler.LookupUser),
	)

	goalRoutes.Handle(
		"PATCH /users/handle",
		requireUser(authHandler.UpdateHandle),
	)

	goalRoutes.Handle(
		"GET /users/me",
		requireUser(authHandler.GetMe),
	)
	goalRoutes.Handle(
		"POST /auth/revoke-all",
		requireUser(authHandler.RevokeAllSessions),
	)

	// Enforcement Blocklist & Device Heartbeats (blocklist & heartbeat accept device or user tokens)
	goalRoutes.HandleFunc(
		"GET /enforcement/blocklist",
		enforcementHandler.GetBlocklist,
	)
	goalRoutes.HandleFunc(
		"POST /enforcement/heartbeat",
		enforcementHandler.Heartbeat,
	)
	goalRoutes.Handle(
		"GET /enforcement/devices",
		requireUser(enforcementHandler.GetDevices),
	)
	goalRoutes.Handle(
		"POST /enforcement/devices/{id}/revoke",
		requireUser(enforcementHandler.RevokeDevice),
	)
	goalRoutes.Handle(
		"GET /goals/{id}/enforcement-status",
		requireUser(enforcementHandler.GetGoalEnforcementStatus),
	)
	goalRoutes.Handle(
		"POST /enforcement/pair/confirm",
		requireUser(enforcementHandler.ConfirmPairing),
	)

	// Digital Wellbeing Ingestion, Summary, Daily Limits & Focus Schedule
	goalRoutes.HandleFunc(
		"POST /wellbeing/ingest",
		wellbeingHandler.Ingest,
	)
	goalRoutes.HandleFunc(
		"GET /wellbeing/summary",
		wellbeingHandler.GetSummary,
	)
	goalRoutes.HandleFunc(
		"GET /wellbeing/rules",
		wellbeingHandler.GetRules,
	)
	goalRoutes.Handle(
		"PUT /wellbeing/limits",
		requireUser(wellbeingHandler.UpsertLimit),
	)
	goalRoutes.Handle(
		"DELETE /wellbeing/limits/{id}",
		requireUser(wellbeingHandler.DeleteLimit),
	)
	goalRoutes.Handle(
		"PUT /wellbeing/schedule",
		requireUser(wellbeingHandler.SaveSchedule),
	)
	goalRoutes.Handle(
		"GET /enforcement/desktop/status",
		requireUser(enforcementHandler.GetDesktopStatus),
	)
	goalRoutes.Handle(
		"POST /enforcement/desktop/dns",
		requireUser(enforcementHandler.ToggleDesktopDNS),
	)
	goalRoutes.Handle(
		"POST /enforcement/desktop/sync",
		requireUser(enforcementHandler.SyncDesktop),
	)
	goalRoutes.Handle(
		"POST /enforcement/desktop/pair",
		requireUser(enforcementHandler.AutoPairDesktop),
	)
	goalRoutes.Handle(
		"POST /enforcement/desktop/reconnect",
		requireUser(enforcementHandler.ReconnectDesktop),
	)

	// Proof Document & Media Upload Handler (supports multi-file PDF, Image, Document uploads up to 32MB)
	goalRoutes.Handle("POST /uploads", requireUser(newUploadCreateHandler(uploadsDir)))

	// Idempotency store for mutation endpoints
	idempotencyStore := middleware.NewMemoryIdempotencyStore(24 * time.Hour)
	idempotentProtected := middleware.Idempotency(idempotencyStore)(goalRoutes)

	// Public Device Pairing Endpoints (protected by rate limiting and idempotency)
	mux.Handle("POST /enforcement/pair/request", pairingLimiter.Limit(middleware.Idempotency(idempotencyStore)(http.HandlerFunc(enforcementHandler.RequestPairing))))
	mux.Handle("POST /enforcement/pair/exchange", pairingLimiter.Limit(middleware.Idempotency(idempotencyStore)(http.HandlerFunc(enforcementHandler.ExchangePairing))))

	// JWT authentication, session version verification, and per-user sliding window rate limiting (180 req/min) for all protected routes
	protectedRoutes := middleware.Auth(cfg.JWTSecret, middleware.WithSessionValidator(authRepo))(apiLimiter.Limit(idempotentProtected))

	mux.Handle("/goals", protectedRoutes)
	mux.Handle("/goals/", protectedRoutes)
	mux.Handle("/approvals", protectedRoutes)
	mux.Handle("/approvals/", protectedRoutes)
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
	mux.Handle("/wellbeing", protectedRoutes)
	mux.Handle("/wellbeing/", protectedRoutes)
	mux.Handle("/uploads", protectedRoutes)
	mux.Handle("POST /auth/revoke-all", protectedRoutes)

	addr, err := validateAndResolveBindAddress(os.Getenv("HOST"), os.Getenv("PORT"))
	if err != nil {
		log.Fatalf("Server bind address configuration error: %v", err)
	}

	// Apply defense-in-depth pipeline to root handler:
	// 1. Protective HTTP security headers (nosniff, DENY, CSP, XSS)
	// 2. Request body size ceiling (50MB) to allow document & high-resolution proof uploads
	// 3. Strict origin-whitelisted CORS
	rootHandler := middleware.SecurityHeaders(
		middleware.MaxBytesLimiter(50 * 1024 * 1024)(
			cors(cfg.Env, cfg.AllowedOrigins)(mux),
		),
	)

	server := &http.Server{
		Addr:         addr,
		Handler:      rootHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Server running on %s\n", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server listen error: %v", err)
		}
	}()

	var fixtureServer *http.Server
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ENABLE_FIXTURE_SERVER")), "true") {
		fixtureMux := http.NewServeMux()
		fixtureMux.Handle("GET /downloads/", downloadsHandler)
		fixtureServer = &http.Server{
			Addr:         "127.0.0.1:8765",
			Handler:      middleware.SecurityHeaders(fixtureMux),
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
		}
		go func() {
			if err := fixtureServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("Secondary downloads listener (:8765) stopped: %v", err)
			}
		}()
	}

	<-stopCtx.Done()
	log.Println("Gracefully stopping server...")

	notifWorker.Stop()
	log.Println("Notification worker drained and stopped.")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if fixtureServer != nil {
		_ = fixtureServer.Shutdown(shutdownCtx)
	}
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped cleanly.")
}

type serverCommunityChecker struct {
	friendsRepo *friends.Repository
	hub         *realtime.Hub
}

func (c *serverCommunityChecker) IsCommunityMember(ctx context.Context, userID, email, handle, communityID string) bool {
	if c.hub != nil && c.hub.IsCommunityMember(ctx, userID, email, handle, communityID) {
		return true
	}
	if c.friendsRepo != nil && userID != "" {
		joined, err := c.friendsRepo.GetJoinedCommunities(ctx, userID)
		if err == nil {
			for _, id := range joined {
				if id == communityID {
					return true
				}
			}
		}
	}
	return false
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

type serverFriendFinder struct {
	friendsRepo *friends.Repository
}

func (f *serverFriendFinder) FindFriends(ctx context.Context, userID string) ([]goals.FriendMatch, error) {
	if f.friendsRepo == nil || userID == "" {
		return nil, nil
	}
	list, err := f.friendsRepo.GetFriends(ctx, userID)
	if err != nil {
		return nil, err
	}
	matches := make([]goals.FriendMatch, 0, len(list))
	for _, item := range list {
		matches = append(matches, goals.FriendMatch{
			ID:     item.FriendID,
			Handle: item.FriendHandle,
			Email:  item.FriendEmail,
			Name:   item.FriendName,
		})
	}
	return matches, nil
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

type serverNotifBroadcaster struct {
	hub *realtime.Hub
}

func (b *serverNotifBroadcaster) SendNotification(userID, eventType string, payload interface{}) error {
	if b.hub == nil {
		return nil
	}
	return b.hub.SendToUser(userID, realtime.Event{
		Type:    eventType,
		Payload: payload,
	})
}

type serverWellbeingNotifier struct {
	hub *realtime.Hub
}

func (n *serverWellbeingNotifier) NotifyLimitReached(_ context.Context, userID string, exceeded []wellbeing.TimeLimitRule) {
	if n.hub == nil || len(exceeded) == 0 {
		return
	}
	_ = n.hub.SendToUser(userID, realtime.Event{
		Type: "wellbeing.limit_reached",
		Payload: map[string]interface{}{
			"userId":   userID,
			"exceeded": exceeded,
		},
	})
}
