package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type mockChatRepo struct {
	historyFunc func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error)
	clearFunc   func(ctx context.Context, userKey, peerKey string) error
}

func (m *mockChatRepo) GetConversationHistory(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
	if m.historyFunc != nil {
		return m.historyFunc(ctx, userKey, peerKey, limit)
	}
	return nil, nil
}

func (m *mockChatRepo) ClearConversation(ctx context.Context, userKey, peerKey string) error {
	if m.clearFunc != nil {
		return m.clearFunc(ctx, userKey, peerKey)
	}
	return nil
}

func TestChatHandler_UnauthorizedWithoutToken(t *testing.T) {
	repo := &mockChatRepo{}
	handler := NewHandler(repo)

	// GET /chat/messages without auth
	reqGet := httptest.NewRequest("GET", "/chat/messages?peer=test@example.com", nil)
	recGet := httptest.NewRecorder()
	handler.GetHistory(recGet, reqGet)
	if recGet.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized for GET, got %d", recGet.Code)
	}

	// DELETE /chat/messages without auth
	reqDel := httptest.NewRequest("DELETE", "/chat/messages?peer=test@example.com", nil)
	recDel := httptest.NewRecorder()
	handler.ClearHistory(recDel, reqDel)
	if recDel.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized for DELETE, got %d", recDel.Code)
	}
}

func TestChatHandler_MissingPeerQueryParam(t *testing.T) {
	repo := &mockChatRepo{}
	handler := NewHandler(repo)

	// Context with caller identity
	reqGet := httptest.NewRequest("GET", "/chat/messages", nil)
	reqGet = reqGet.WithContext(context.WithValue(reqGet.Context(), middleware.UserEmailKey, "alice@example.com"))
	recGet := httptest.NewRecorder()
	handler.GetHistory(recGet, reqGet)
	if recGet.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request for GET without peer, got %d", recGet.Code)
	}

	reqDel := httptest.NewRequest("DELETE", "/chat/messages", nil)
	reqDel = reqDel.WithContext(context.WithValue(reqDel.Context(), middleware.UserEmailKey, "alice@example.com"))
	recDel := httptest.NewRecorder()
	handler.ClearHistory(recDel, reqDel)
	if recDel.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request for DELETE without peer, got %d", recDel.Code)
	}
}

func TestChatHandler_GetHistory_BareHandleFallback(t *testing.T) {
	queriedPeers := []string{}
	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			queriedPeers = append(queriedPeers, peerKey)
			if peerKey == "@bob" {
				return []Message{{ID: "msg-1", Text: "hello from @bob"}}, nil
			}
			return nil, nil
		},
	}
	handler := NewHandler(repo)

	req := httptest.NewRequest("GET", "/chat/messages?peer=bob", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserEmailKey, "alice@example.com"))
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var msgs []Message
	if err := json.Unmarshal(rec.Body.Bytes(), &msgs); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(msgs) != 1 || msgs[0].ID != "msg-1" {
		t.Fatalf("expected msg-1, got %+v", msgs)
	}

	// Verify fallback queried both bare and @-prefixed variant
	hasBare := false
	hasAt := false
	for _, p := range queriedPeers {
		if p == "bob" {
			hasBare = true
		}
		if p == "@bob" {
			hasAt = true
		}
	}
	if !hasBare || !hasAt {
		t.Fatalf("expected queries for both 'bob' and '@bob', got: %v", queriedPeers)
	}
}

func TestChatHandler_GetHistory_AtPrefixedHandle(t *testing.T) {
	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			if peerKey == "@bob" {
				return []Message{{ID: "msg-2", Text: "found @bob directly"}}, nil
			}
			return nil, nil
		},
	}
	handler := NewHandler(repo)

	req := httptest.NewRequest("GET", "/chat/messages?peer=@bob", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserEmailKey, "alice@example.com"))
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var msgs []Message
	json.Unmarshal(rec.Body.Bytes(), &msgs)
	if len(msgs) != 1 || msgs[0].ID != "msg-2" {
		t.Fatalf("expected msg-2, got %+v", msgs)
	}
}

func TestChatHandler_GetHistory_EmptyResults(t *testing.T) {
	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			return []Message{}, nil
		},
	}
	handler := NewHandler(repo)

	req := httptest.NewRequest("GET", "/chat/messages?peer=nonexistent", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserEmailKey, "alice@example.com"))
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for empty results, got %d", rec.Code)
	}

	var msgs []Message
	if err := json.Unmarshal(rec.Body.Bytes(), &msgs); err != nil {
		t.Fatalf("failed to decode empty array: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestChatHandler_GetHistory_DatabaseError_Primary(t *testing.T) {
	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			return nil, errors.New("dynamodb read failed")
		},
	}
	handler := NewHandler(repo)

	req := httptest.NewRequest("GET", "/chat/messages?peer=bob", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserEmailKey, "alice@example.com"))
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", rec.Code)
	}
}

func TestChatHandler_GetHistory_DatabaseError_Fallback(t *testing.T) {
	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			if peerKey == "bob" {
				return nil, nil // primary query empty
			}
			return nil, errors.New("dynamodb fallback timeout")
		},
	}
	handler := NewHandler(repo)

	req := httptest.NewRequest("GET", "/chat/messages?peer=bob", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserEmailKey, "alice@example.com"))
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when fallback query errors, got %d", rec.Code)
	}
}

func TestChatHandler_ClearHistory_BareAndAtPrefixed(t *testing.T) {
	clearedCalls := make(map[string]bool)
	repo := &mockChatRepo{
		clearFunc: func(ctx context.Context, userKey, peerKey string) error {
			clearedCalls[userKey+":"+peerKey] = true
			return nil
		},
	}
	handler := NewHandler(repo)

	ctx := context.WithValue(context.Background(), middleware.UserEmailKey, "alice@example.com")
	ctx = context.WithValue(ctx, middleware.UserHandleKey, "alice")

	req := httptest.NewRequest("DELETE", "/chat/messages?peer=bob", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ClearHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify both bare "bob" and "@bob" were deleted for both caller keys
	if !clearedCalls["alice@example.com:bob"] || !clearedCalls["alice@example.com:@bob"] {
		t.Errorf("missing email-based peer clear calls: %v", clearedCalls)
	}
	if !clearedCalls["@alice:bob"] || !clearedCalls["@alice:@bob"] {
		t.Errorf("missing handle-based peer clear calls: %v", clearedCalls)
	}
}

func TestChatHandler_ClearHistory_DatabaseError(t *testing.T) {
	repo := &mockChatRepo{
		clearFunc: func(ctx context.Context, userKey, peerKey string) error {
			return errors.New("dynamodb delete failed")
		},
	}
	handler := NewHandler(repo)

	ctx := context.WithValue(context.Background(), middleware.UserEmailKey, "alice@example.com")
	req := httptest.NewRequest("DELETE", "/chat/messages?peer=bob", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ClearHistory(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when ClearConversation returns an error, got %d", rec.Code)
	}
}

func TestChatHandler_CrossUserAuthorization_OnlyCallerPartitionQueried(t *testing.T) {
	var queriedCallerKey string
	var queriedPeerKey string

	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			queriedCallerKey = userKey
			queriedPeerKey = peerKey
			return []Message{}, nil
		},
	}
	handler := NewHandler(repo)

	// Eve tries to query chat history, attempting to inject Alice as user/sender via query params
	ctx := context.WithValue(context.Background(), middleware.UserEmailKey, "eve@example.com")
	req := httptest.NewRequest("GET", "/chat/messages?peer=bob@example.com&user=alice@example.com&sender=alice@example.com", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	// Verify that the repository was queried ONLY for Eve's partition, never Alice's
	if queriedCallerKey != "eve@example.com" {
		t.Fatalf("expected caller partition to be strictly 'eve@example.com' from JWT, got '%s'", queriedCallerKey)
	}
	if queriedPeerKey != "bob@example.com" {
		t.Fatalf("expected peer key 'bob@example.com', got '%s'", queriedPeerKey)
	}
}

func TestNormalizeHandleKey(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"bob", "@bob"},
		{"@bob", "@bob"},
		{"  @alice  ", "@alice"},
	}

	for _, c := range cases {
		actual := normalizeHandleKey(c.input)
		if actual != c.expected {
			t.Errorf("normalizeHandleKey(%q) = %q, expected %q", c.input, actual, c.expected)
		}
	}
}

func TestGetPeerVariants(t *testing.T) {
	// Bare handle
	hVariants := getPeerVariants("bob")
	if len(hVariants) != 2 || hVariants[0] != "bob" || hVariants[1] != "@bob" {
		t.Errorf("expected ['bob', '@bob'], got %v", hVariants)
	}

	// Prefixed handle
	atVariants := getPeerVariants("@alice")
	if len(atVariants) != 2 || atVariants[0] != "@alice" || atVariants[1] != "alice" {
		t.Errorf("expected ['@alice', 'alice'], got %v", atVariants)
	}

	// Email
	emailVariants := getPeerVariants("user@example.com")
	if len(emailVariants) != 1 || emailVariants[0] != "user@example.com" {
		t.Errorf("expected ['user@example.com'], got %v", emailVariants)
	}

	// Empty
	if empty := getPeerVariants("  "); empty != nil {
		t.Errorf("expected nil for empty input, got %v", empty)
	}
}

func TestChatMessageModel(t *testing.T) {
	msg := Message{
		ID:             "msg-123",
		SenderEmail:    "alice@example.com",
		SenderID:       "user-alice",
		RecipientEmail: "bob@example.com",
		Text:           "test message",
		Time:           "2026-10-06T19:00:00Z",
		Delivered:      true,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal message: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	if decoded.ID != "msg-123" || decoded.Text != "test message" {
		t.Fatalf("mismatched decoded message content: %+v", decoded)
	}
}
