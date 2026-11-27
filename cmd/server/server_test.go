package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

func TestWebSocketRouteIntegration(t *testing.T) {
	jwtSecret := "server-test-secret"
	hub := realtime.NewHub()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /ws", realtime.ServeWS(hub, jwtSecret))
	mux.HandleFunc("GET /ws/", realtime.ServeWS(hub, jwtSecret))

	handler := cors("development", []string{"http://localhost:5173", "http://127.0.0.1:5173"})(mux)
	server := httptest.NewServer(handler)
	defer server.Close()

	// 1. Test /health
	resp, err := http.Get(server.URL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /health, got status %v, err %v", resp.StatusCode, err)
	}

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"
	u.Path = "/ws"

	claims := jwt.MapClaims{
		"sub": "test-server-user",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokString, _ := tok.SignedString([]byte(jwtSecret))

	// 2. Test /ws with query param token -> 401 Unauthorized (token URL leakage prevented)
	wsURLQuery := u.String() + "?token=" + tokString
	_, resp, err = websocket.DefaultDialer.Dial(wsURLQuery, nil)
	if err == nil {
		t.Fatalf("expected query parameter token to be rejected")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for query parameter token, got %d", resp.StatusCode)
	}

	// 3. Test /ws with first-message handshake -> 101 Switching Protocols + auth.success
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial /ws: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}

	_ = conn.WriteJSON(map[string]string{
		"type":  "auth",
		"token": tokString,
	})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, authMsg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read handshake response: %v", err)
	}
	if !strings.Contains(string(authMsg), "auth.success") {
		t.Fatalf("expected auth.success in handshake, got: %s", string(authMsg))
	}

	// Check user is registered in hub
	time.Sleep(50 * time.Millisecond)
	if !hub.IsUserOnline("test-server-user") {
		t.Fatalf("expected test-server-user to be online in hub")
	}

	// 4. Test /ws with Authorization header directly
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokString)
	connHeader, respHeader, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		t.Fatalf("failed to dial with Authorization header: %v", err)
	}
	defer connHeader.Close()
	if respHeader.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols with header, got %d", respHeader.StatusCode)
	}
}

func TestDownloadsRouteIntegration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)

	downloadsDir := "../../frontend/public/downloads"
	fileServer := http.FileServer(http.Dir(downloadsDir))
	mux.Handle("GET /downloads/", http.StripPrefix("/downloads/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".crx") {
			w.Header().Set("Content-Type", "application/x-chrome-extension")
		} else if strings.HasSuffix(r.URL.Path, ".xml") {
			w.Header().Set("Content-Type", "application/xml")
		}
		fileServer.ServeHTTP(w, r)
	})))

	server := httptest.NewServer(mux)
	defer server.Close()

	// 1. Test updates.xml
	resp, err := http.Get(server.URL + "/downloads/updates.xml")
	if err != nil {
		t.Fatalf("failed to GET updates.xml: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for updates.xml, got %v", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Errorf("expected xml Content-Type, got %v", ct)
	}

	// 2. Test onusly.crx
	respCRX, err := http.Get(server.URL + "/downloads/onusly.crx")
	if err != nil {
		t.Fatalf("failed to GET onusly.crx: %v", err)
	}
	defer respCRX.Body.Close()
	if respCRX.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for onusly.crx, got %v", respCRX.StatusCode)
	}
	if ct := respCRX.Header.Get("Content-Type"); ct != "application/x-chrome-extension" {
		t.Errorf("expected application/x-chrome-extension, got %v", ct)
	}
}

func TestValidateAndResolveBindAddress(t *testing.T) {
	// 1. Default fallback (empty host and port)
	addr, err := validateAndResolveBindAddress("", "")
	if err != nil {
		t.Fatalf("expected default bind address to succeed, got: %v", err)
	}
	if addr != "127.0.0.1:8080" {
		t.Errorf("expected 127.0.0.1:8080, got %s", addr)
	}

	// 2. Localhost resolution
	addr, err = validateAndResolveBindAddress("localhost", "9000")
	if err != nil {
		t.Fatalf("expected localhost to succeed, got: %v", err)
	}
	if addr != "127.0.0.1:9000" {
		t.Errorf("expected 127.0.0.1:9000, got %s", addr)
	}

	// 3. IPv6 loopback
	addr, err = validateAndResolveBindAddress("::1", "8080")
	if err != nil {
		t.Fatalf("expected ::1 loopback to succeed, got: %v", err)
	}
	if addr != "[::1]:8080" {
		t.Errorf("expected [::1]:8080, got %s", addr)
	}

	// 4. Reject unspecified 0.0.0.0
	_, err = validateAndResolveBindAddress("0.0.0.0", "8080")
	if err == nil {
		t.Fatalf("expected 0.0.0.0 to be rejected as security violation")
	}
	if !strings.Contains(err.Error(), "strictly forbidden") {
		t.Errorf("expected error message to mention strictly forbidden, got: %v", err)
	}

	// 5. Reject unspecified ::
	_, err = validateAndResolveBindAddress("::", "8080")
	if err == nil {
		t.Fatalf("expected :: to be rejected as security violation")
	}

	// 6. Reject non-loopback IP without override
	_, err = validateAndResolveBindAddress("192.168.1.50", "8080")
	if err == nil {
		t.Fatalf("expected non-loopback IP to be rejected without ALLOW_INSECURE_BIND")
	}

	// 7. Allow non-loopback IP with ALLOW_INSECURE_BIND=true
	t.Setenv("ALLOW_INSECURE_BIND", "true")
	addr, err = validateAndResolveBindAddress("192.168.1.50", "8080")
	if err != nil {
		t.Fatalf("expected ALLOW_INSECURE_BIND=true to allow bind, got: %v", err)
	}
	if addr != "192.168.1.50:8080" {
		t.Errorf("expected 192.168.1.50:8080, got %s", addr)
	}

	// 8. Reject invalid IP format
	t.Setenv("ALLOW_INSECURE_BIND", "false")
	_, err = validateAndResolveBindAddress("not.a.valid.ip", "8080")
	if err == nil {
		t.Fatalf("expected malformed IP to be rejected")
	}
}

type mockTableDescriber struct {
	calls atomic.Int32
	err   error
}

func (m *mockTableDescriber) DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	m.calls.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	return &dynamodb.DescribeTableOutput{}, nil
}

func TestWarmHealthHandler(t *testing.T) {
	// 1. Loopback healthy + cache deduplication
	mockDB := &mockTableDescriber{}
	handler := newWarmHealthHandler(mockDB, "Onusly", 30*time.Second)

	req1 := httptest.NewRequest(http.MethodGet, "/health/warm", nil)
	req1.RemoteAddr = "127.0.0.1:54321"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (%s)", rec1.Code, rec1.Body.String())
	}
	var body1 map[string]any
	if err := json.Unmarshal(rec1.Body.Bytes(), &body1); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if body1["status"] != "ok" || body1["dynamodb"] != "warm" || body1["cached"] != false {
		t.Errorf("unexpected response payload: %+v", body1)
	}
	if mockDB.calls.Load() != 1 {
		t.Errorf("expected 1 DescribeTable call, got %d", mockDB.calls.Load())
	}

	// Second call within cache TTL must return cached=true without another DescribeTable
	req2 := httptest.NewRequest(http.MethodGet, "/health/warm", nil)
	req2.RemoteAddr = "[::1]:54322"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected cached 200 OK, got %d", rec2.Code)
	}
	var body2 map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &body2)
	if body2["cached"] != true {
		t.Errorf("expected cached=true on second call, got %+v", body2)
	}
	if mockDB.calls.Load() != 1 {
		t.Errorf("expected DescribeTable to remain cached at 1 call, got %d", mockDB.calls.Load())
	}

	// 2. Non-loopback RemoteAddr or X-Forwarded-For must be forbidden
	reqExt := httptest.NewRequest(http.MethodGet, "/health/warm", nil)
	reqExt.RemoteAddr = "203.0.113.10:12345"
	recExt := httptest.NewRecorder()
	handler.ServeHTTP(recExt, reqExt)
	if recExt.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-loopback RemoteAddr, got %d", recExt.Code)
	}

	reqXFF := httptest.NewRequest(http.MethodGet, "/health/warm", nil)
	reqXFF.RemoteAddr = "127.0.0.1:12345"
	reqXFF.Header.Set("X-Forwarded-For", "198.51.100.42")
	recXFF := httptest.NewRecorder()
	handler.ServeHTTP(recXFF, reqXFF)
	if recXFF.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-loopback X-Forwarded-For, got %d", recXFF.Code)
	}

	// 3. Unhealthy DynamoDB returns 503 without leaking internal error text
	failingDB := &mockTableDescriber{err: errors.New("secret internal aws endpoint error")}
	failingHandler := newWarmHealthHandler(failingDB, "Onusly", 0)
	reqFail := httptest.NewRequest(http.MethodGet, "/health/warm", nil)
	reqFail.RemoteAddr = "127.0.0.1:54323"
	recFail := httptest.NewRecorder()
	failingHandler.ServeHTTP(recFail, reqFail)

	if recFail.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 Service Unavailable, got %d", recFail.Code)
	}
	if strings.Contains(recFail.Body.String(), "secret internal") {
		t.Errorf("response leaked internal error message: %s", recFail.Body.String())
	}
}

type mockUploadGoalLookup struct {
	goals map[string]*goals.Goal
}

func (m *mockUploadGoalLookup) GetByID(_ context.Context, ownerID, goalID string) (*goals.Goal, error) {
	if g, ok := m.goals[goalID]; ok {
		if ownerID == "" || g.OwnerID == ownerID {
			return g, nil
		}
	}
	return nil, errors.New("goal not found")
}

func TestUploadsACLAuthorization(t *testing.T) {
	uploadsDir := t.TempDir()
	jwtSecret := "upload-acl-secret"
	makeTok := func(uid, email, handle string) string {
		claims := jwt.MapClaims{
			"sub":           uid,
			"email":         email,
			"handle":        handle,
			"token_type":    "user",
			"token_version": 1,
			"exp":           time.Now().Add(time.Hour).Unix(),
		}
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(jwtSecret))
		return tok
	}

	aliceToken := makeTok("alice-id", "alice@example.com", "alice")
	bobToken := makeTok("bob-id", "bob@example.com", "bob")
	eveToken := makeTok("eve-id", "eve@example.com", "eve")

	goalLookup := &mockUploadGoalLookup{
		goals: map[string]*goals.Goal{
			"goal-1": {
				ID:            "goal-1",
				OwnerID:       "alice-id",
				ApproverID:    "bob-id",
				ApproverEmail: "bob@example.com",
				Status:        "proof_submitted",
			},
		},
	}

	uploadCreate := middleware.Auth(jwtSecret)(http.HandlerFunc(newUploadCreateHandler(uploadsDir)))
	uploadServe := newUploadsServeHandler(uploadsDir, jwtSecret, nil, goalLookup, nil)

	// Upload a valid 1x1 PNG as Alice bound to goal-1
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("goalId", "goal-1")
	fw, _ := mw.CreateFormFile("files", "proof.png")
	_, _ = fw.Write(pngHeader)
	_ = mw.Close()

	reqUp := httptest.NewRequest(http.MethodPost, "/uploads", &body)
	reqUp.Header.Set("Content-Type", mw.FormDataContentType())
	reqUp.Header.Set("Authorization", "Bearer "+aliceToken)
	recUp := httptest.NewRecorder()
	uploadCreate.ServeHTTP(recUp, reqUp)
	if recUp.Code != http.StatusOK {
		t.Fatalf("expected upload 200 OK, got %d (%s)", recUp.Code, recUp.Body.String())
	}

	var uploaded []UploadedFileItem
	if err := json.Unmarshal(recUp.Body.Bytes(), &uploaded); err != nil || len(uploaded) != 1 {
		t.Fatalf("expected 1 uploaded item, err=%v body=%s", err, recUp.Body.String())
	}
	fileURL := uploaded[0].URL

	// 1. Unauthenticated read -> 401
	recUnauth := httptest.NewRecorder()
	uploadServe.ServeHTTP(recUnauth, httptest.NewRequest(http.MethodGet, fileURL, nil))
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated upload fetch, got %d", recUnauth.Code)
	}

	// 2. Owner Alice can fetch -> 200
	reqAlice := httptest.NewRequest(http.MethodGet, fileURL, nil)
	reqAlice.Header.Set("Authorization", "Bearer "+aliceToken)
	recAlice := httptest.NewRecorder()
	uploadServe.ServeHTTP(recAlice, reqAlice)
	if recAlice.Code != http.StatusOK {
		t.Errorf("expected 200 for owner Alice, got %d (%s)", recAlice.Code, recAlice.Body.String())
	}

	// 3. Assigned approver Bob can fetch -> 200
	reqBob := httptest.NewRequest(http.MethodGet, fileURL, nil)
	reqBob.Header.Set("Authorization", "Bearer "+bobToken)
	recBob := httptest.NewRecorder()
	uploadServe.ServeHTTP(recBob, reqBob)
	if recBob.Code != http.StatusOK {
		t.Errorf("expected 200 for assigned approver Bob, got %d (%s)", recBob.Code, recBob.Body.String())
	}

	// 4. Unrelated authenticated user Eve -> 403 Forbidden (SECURITY-001)
	reqEve := httptest.NewRequest(http.MethodGet, fileURL, nil)
	reqEve.Header.Set("Authorization", "Bearer "+eveToken)
	recEve := httptest.NewRecorder()
	uploadServe.ServeHTTP(recEve, reqEve)
	if recEve.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for unrelated user Eve, got %d", recEve.Code)
	}

	// 5. Dotfile metadata sidecar cannot be fetched even by Alice
	filename := strings.TrimPrefix(fileURL, "/uploads/proofs/")
	reqMeta := httptest.NewRequest(http.MethodGet, "/uploads/proofs/.meta/"+filename+".json", nil)
	reqMeta.Header.Set("Authorization", "Bearer "+aliceToken)
	recMeta := httptest.NewRecorder()
	uploadServe.ServeHTTP(recMeta, reqMeta)
	if recMeta.Code != http.StatusBadRequest && recMeta.Code != http.StatusForbidden {
		t.Errorf("expected 400/403 for hidden .meta read, got %d", recMeta.Code)
	}
}

type mockUploadUserResolver struct {
	users map[string]struct{ id, email, handle string }
}

func (m *mockUploadUserResolver) ResolveUser(_ context.Context, identifier string) (string, string, string, error) {
	if u, ok := m.users[strings.ToLower(strings.TrimPrefix(identifier, "@"))]; ok {
		return u.id, u.email, u.handle, nil
	}
	return "", "", "", errors.New("user not found")
}

type mockUploadCommunityChecker struct {
	members map[string]map[string]bool
}

func (m *mockUploadCommunityChecker) IsCommunityMember(_ context.Context, userID, _, _, communityID string) bool {
	return m.members[communityID][userID]
}

func TestUploadsChatPDFAndExtendedTypes(t *testing.T) {
	uploadsDir := t.TempDir()
	jwtSecret := "upload-chat-secret"
	makeTok := func(uid, email, handle string) string {
		claims := jwt.MapClaims{
			"sub":           uid,
			"email":         email,
			"handle":        handle,
			"token_type":    "user",
			"token_version": 1,
			"exp":           time.Now().Add(time.Hour).Unix(),
		}
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(jwtSecret))
		return tok
	}

	aliceToken := makeTok("alice-id", "alice@example.com", "alice")
	bobToken := makeTok("bob-id", "bob@example.com", "bob")
	eveToken := makeTok("eve-id", "eve@example.com", "eve")

	resolver := &mockUploadUserResolver{
		users: map[string]struct{ id, email, handle string }{
			"bob": {id: "bob-id", email: "bob@example.com", handle: "bob"},
		},
	}
	commChecker := &mockUploadCommunityChecker{
		members: map[string]map[string]bool{},
	}

	uploadCreate := middleware.Auth(jwtSecret)(http.HandlerFunc(newUploadCreateHandlerWithResolver(uploadsDir, resolver)))
	uploadServe := newUploadsServeHandlerWithChat(uploadsDir, jwtSecret, nil, nil, nil, commChecker)

	// 1. Upload valid PDF scoped to DM with Bob
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("scope", "chat")
	_ = mw.WriteField("recipient", "@bob")
	fw, _ := mw.CreateFormFile("files", "study-notes.pdf")
	_, _ = fw.Write([]byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF\n"))
	_ = mw.Close()

	reqUp := httptest.NewRequest(http.MethodPost, "/uploads", &body)
	reqUp.Header.Set("Content-Type", mw.FormDataContentType())
	reqUp.Header.Set("Authorization", "Bearer "+aliceToken)
	recUp := httptest.NewRecorder()
	uploadCreate.ServeHTTP(recUp, reqUp)
	if recUp.Code != http.StatusOK {
		t.Fatalf("expected chat PDF upload 200 OK, got %d (%s)", recUp.Code, recUp.Body.String())
	}

	var uploaded []UploadedFileItem
	if err := json.Unmarshal(recUp.Body.Bytes(), &uploaded); err != nil || len(uploaded) != 1 {
		t.Fatalf("expected 1 uploaded PDF item, err=%v body=%s", err, recUp.Body.String())
	}
	pdfURL := uploaded[0].URL
	if !strings.HasPrefix(pdfURL, "/uploads/chat/") || !uploaded[0].IsPdf {
		t.Fatalf("unexpected chat PDF metadata: %+v", uploaded[0])
	}

	// Recipient Bob can fetch inline PDF without 'sandbox' CSP blocking Chrome viewer
	reqBob := httptest.NewRequest(http.MethodGet, pdfURL, nil)
	reqBob.Header.Set("Authorization", "Bearer "+bobToken)
	recBob := httptest.NewRecorder()
	uploadServe.ServeHTTP(recBob, reqBob)
	if recBob.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for recipient Bob, got %d (%s)", recBob.Code, recBob.Body.String())
	}
	if csp := recBob.Header().Get("Content-Security-Policy"); strings.Contains(csp, "sandbox") {
		t.Errorf("PDF response must not include sandbox CSP directive, got %q", csp)
	}
	if cd := recBob.Header().Get("Content-Disposition"); cd != "inline" {
		t.Errorf("expected inline Content-Disposition for PDF, got %q", cd)
	}

	// Unrelated Eve must get 403 Forbidden
	reqEve := httptest.NewRequest(http.MethodGet, pdfURL, nil)
	reqEve.Header.Set("Authorization", "Bearer "+eveToken)
	recEve := httptest.NewRecorder()
	uploadServe.ServeHTTP(recEve, reqEve)
	if recEve.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for unrelated Eve, got %d", recEve.Code)
	}

	// 2. Upload valid .docx (ZIP magic header) succeeds and forces attachment + sandbox CSP
	var docxBody bytes.Buffer
	mwDocx := multipart.NewWriter(&docxBody)
	_ = mwDocx.WriteField("scope", "chat")
	_ = mwDocx.WriteField("recipient", "@bob")
	fwDocx, _ := mwDocx.CreateFormFile("files", "plan.docx")
	_, _ = fwDocx.Write([]byte("PK\x03\x04\x14\x00\x00\x00fake-docx-zip-payload"))
	_ = mwDocx.Close()

	reqDocx := httptest.NewRequest(http.MethodPost, "/uploads", &docxBody)
	reqDocx.Header.Set("Content-Type", mwDocx.FormDataContentType())
	reqDocx.Header.Set("Authorization", "Bearer "+aliceToken)
	recDocx := httptest.NewRecorder()
	uploadCreate.ServeHTTP(recDocx, reqDocx)
	if recDocx.Code != http.StatusOK {
		t.Fatalf("expected valid .docx upload 200 OK, got %d (%s)", recDocx.Code, recDocx.Body.String())
	}

	var uploadedDocx []UploadedFileItem
	_ = json.Unmarshal(recDocx.Body.Bytes(), &uploadedDocx)
	reqFetchDocx := httptest.NewRequest(http.MethodGet, uploadedDocx[0].URL, nil)
	reqFetchDocx.Header.Set("Authorization", "Bearer "+aliceToken)
	recFetchDocx := httptest.NewRecorder()
	uploadServe.ServeHTTP(recFetchDocx, reqFetchDocx)
	if recFetchDocx.Code != http.StatusOK {
		t.Fatalf("expected 200 OK fetching own docx, got %d", recFetchDocx.Code)
	}
	if cd := recFetchDocx.Header().Get("Content-Disposition"); cd != "attachment" {
		t.Errorf("expected attachment Content-Disposition for .docx, got %q", cd)
	}
	if csp := recFetchDocx.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Errorf("expected sandbox CSP for non-inline attachment, got %q", csp)
	}

	// 3. Disguised executable or HTML payload must be rejected cleanly with 400
	var badBody bytes.Buffer
	mwBad := multipart.NewWriter(&badBody)
	fwBad, _ := mwBad.CreateFormFile("files", "fake.docx")
	_, _ = fwBad.Write([]byte("MZ\x90\x00not-really-docx"))
	_ = mwBad.Close()
	reqBad := httptest.NewRequest(http.MethodPost, "/uploads", &badBody)
	reqBad.Header.Set("Content-Type", mwBad.FormDataContentType())
	reqBad.Header.Set("Authorization", "Bearer "+aliceToken)
	recBad := httptest.NewRecorder()
	uploadCreate.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for spoofed .docx MZ payload, got %d", recBad.Code)
	}
}
