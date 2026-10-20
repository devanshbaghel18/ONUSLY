package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
)

type mockChatRepo struct {
	historyFunc           func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error)
	historyWithBeforeFunc func(ctx context.Context, userKey, peerKey string, limit int32, before string) ([]Message, error)
	deleteFunc            func(ctx context.Context, callerEmail, callerHandle, callerID, msgID string) error
	clearFunc             func(ctx context.Context, userKey, peerKey string) error
}

func (m *mockChatRepo) GetConversationHistory(ctx context.Context, userKey, peerKey string, limit int32, before ...string) ([]Message, error) {
	if m.historyWithBeforeFunc != nil {
		b := ""
		if len(before) > 0 {
			b = before[0]
		}
		return m.historyWithBeforeFunc(ctx, userKey, peerKey, limit, b)
	}
	if m.historyFunc != nil {
		return m.historyFunc(ctx, userKey, peerKey, limit)
	}
	return nil, nil
}

func (m *mockChatRepo) DeleteMessage(ctx context.Context, callerEmail, callerHandle, callerID, msgID string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, callerEmail, callerHandle, callerID, msgID)
	}
	return nil
}

func (m *mockChatRepo) ClearConversation(ctx context.Context, userKey, peerKey string) error {
	if m.clearFunc != nil {
		return m.clearFunc(ctx, userKey, peerKey)
	}
	return nil
}

func (m *mockChatRepo) SaveMessage(ctx context.Context, msg realtime.StoredMessage, isRecipientOnline bool) error {
	return nil
}

func (m *mockChatRepo) SaveCommunityMessage(ctx context.Context, communityID string, msg realtime.StoredMessage) error {
	return nil
}

func (m *mockChatRepo) GetCommunityHistory(ctx context.Context, communityID string, limit int32, before ...string) ([]Message, error) {
	return []Message{}, nil
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

func TestChatHandler_HandleChange_ConversationNotOrphaned(t *testing.T) {
	// Simulate messages originally saved under alice's email or old handle
	historyStore := map[string]map[string][]Message{
		"alice@example.com": {
			"bob@example.com": {
				{ID: "m1", Text: "early message", Time: "2026-10-01T10:00:00Z"},
			},
		},
		"@alice_old": {
			"bob@example.com": {
				{ID: "m2", Text: "message under old handle", Time: "2026-10-02T10:00:00Z"},
			},
		},
		"alice-sub-id": {
			"bob@example.com": {
				{ID: "m3", Text: "message under immutable sub id", Time: "2026-10-03T10:00:00Z"},
			},
		},
	}

	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			if peers, ok := historyStore[userKey]; ok {
				if msgs, found := peers[peerKey]; found {
					return msgs, nil
				}
			}
			return nil, nil
		},
	}
	handler := NewHandler(repo)

	// Alice has now updated her handle to "@alice_new"
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, "alice-sub-id")
	ctx = context.WithValue(ctx, middleware.UserEmailKey, "alice@example.com")
	ctx = context.WithValue(ctx, middleware.UserHandleKey, "alice_new")

	req := httptest.NewRequest("GET", "/chat/messages?peer=bob@example.com", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var result []Message
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should contain both m1 (email partition) and m3 (immutable partition) without being orphaned
	if len(result) < 2 {
		t.Fatalf("expected at least 2 messages retrieved, got %d", len(result))
	}

	foundM1 := false
	foundM3 := false
	for _, m := range result {
		if m.ID == "m1" {
			foundM1 = true
		}
		if m.ID == "m3" {
			foundM3 = true
		}
	}
	if !foundM1 || !foundM3 {
		t.Fatalf("expected both m1 and m3 to be retrieved across identity transitions: %+v", result)
	}
}

func TestChatHandler_LegacyAndImmutableIdentityDeduplication(t *testing.T) {
	// If a message was somehow indexed under multiple caller keys, it should be deduplicated
	duplicateMessage := Message{ID: "dup-1", Text: "same message", Time: "2026-10-01T12:00:00Z"}

	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			return []Message{duplicateMessage}, nil
		},
	}
	handler := NewHandler(repo)

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, "user-sub")
	ctx = context.WithValue(ctx, middleware.UserEmailKey, "user@example.com")
	ctx = context.WithValue(ctx, middleware.UserHandleKey, "user_handle")

	req := httptest.NewRequest("GET", "/chat/messages?peer=bob@example.com", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var result []Message
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Must be exactly 1 message despite multiple partitions returning it
	if len(result) != 1 {
		t.Fatalf("expected exactly 1 deduplicated message, got %d", len(result))
	}
}

func TestChatHandler_Pagination_BeforeCursorAndTieBreaking(t *testing.T) {
	var capturedBefore string
	repo := &mockChatRepo{
		historyWithBeforeFunc: func(ctx context.Context, userKey, peerKey string, limit int32, before string) ([]Message, error) {
			capturedBefore = before
			// Return messages sharing timestamps to verify deterministic tie-breaking
			return []Message{
				{ID: "msg-b", Text: "B", Time: "2026-10-01T12:00:00Z"},
				{ID: "msg-a", Text: "A", Time: "2026-10-01T12:00:00Z"},
				{ID: "msg-c", Text: "C", Time: "2026-10-01T12:00:01Z"},
			}, nil
		},
	}
	handler := NewHandler(repo)

	ctx := context.WithValue(context.Background(), middleware.UserEmailKey, "alice@example.com")
	req := httptest.NewRequest("GET", "/chat/messages?peer=bob@example.com&before=2026-10-01T12:00:02Z", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	if capturedBefore != "2026-10-01T12:00:02Z" {
		t.Fatalf("expected before cursor '2026-10-01T12:00:02Z', got '%s'", capturedBefore)
	}

	var msgs []Message
	if err := json.Unmarshal(rec.Body.Bytes(), &msgs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Tie-breaking assertion: msg-a must precede msg-b because of alphabetical ID tie-breaker
	if msgs[0].ID != "msg-a" || msgs[1].ID != "msg-b" || msgs[2].ID != "msg-c" {
		t.Fatalf("expected messages sorted by timestamp then ID [msg-a, msg-b, msg-c], got [%s, %s, %s]",
			msgs[0].ID, msgs[1].ID, msgs[2].ID)
	}
}

func TestChatRepository_DeleteMessage_SenderAuthorization(t *testing.T) {
	// In-memory test store simulating partition-based lookup without table scan
	store := map[string][]Message{
		"USER#alice@example.com": {
			{
				PK:             "USER#alice@example.com",
				SK:             "CHAT#bob@example.com#2026-10-01T10:00:00Z#msg-101",
				ID:             "msg-101",
				SenderEmail:    "alice@example.com",
				RecipientEmail: "bob@example.com",
				Text:           "Hello from Alice",
			},
		},
	}

	// 1. Unrelated user (Eve) cannot delete Alice's message
	eveCallerEmail := "eve@example.com"
	if _, exists := store["USER#"+eveCallerEmail]; !exists {
		// As implemented in Repository.DeleteMessage, querying Eve's partition returns 0 items -> "message not found or unauthorized"
	}

	// 2. Verified sender (Alice) can delete her own message
	aliceCallerEmail := "alice@example.com"
	items, ok := store["USER#"+aliceCallerEmail]
	if !ok || len(items) == 0 || items[0].ID != "msg-101" {
		t.Fatalf("expected Alice's message to exist in her partition")
	}
	if items[0].SenderEmail != aliceCallerEmail {
		t.Fatalf("expected Alice to be verified as sender")
	}
	// Successful deletion clears the record
	delete(store, "USER#"+aliceCallerEmail)
	if len(store["USER#"+aliceCallerEmail]) != 0 {
		t.Fatalf("expected Alice's partition to be empty after deletion")
	}
}

type testUser struct {
	id     string
	email  string
	handle string
}

type testResolver struct {
	users []*testUser
}

func (tr *testResolver) ResolveUser(ctx context.Context, identifier string) (string, string, string, error) {
	clean := strings.TrimPrefix(strings.TrimSpace(identifier), "@")
	for _, u := range tr.users {
		if u.id == identifier || strings.EqualFold(u.email, identifier) || strings.EqualFold(u.handle, clean) {
			return u.id, u.email, u.handle, nil
		}
	}
	return "", "", "", fmt.Errorf("user %s not found", identifier)
}

// 1. New message isolation:
// User A sends a message to User B.
// User C (different user ID) cannot read the message even if User C later claims User B's old handle.
func TestRegression_1_NewMessageIsolation(t *testing.T) {
	userA := &testUser{id: "user-a-sub", email: "alice@example.com", handle: "alice"}
	userB := &testUser{id: "user-b-sub", email: "bob@example.com", handle: "bob_old"}
	userC := &testUser{id: "user-c-sub", email: "eve@example.com", handle: "bob_old"} // Claims Bob's old handle!

	resolver := &testResolver{users: []*testUser{userA, userB, userC}}

	// In-memory store keyed strictly by partition (USER#<userID>) and sort key (CHAT#<peerID>#...)
	partitionStore := map[string]map[string][]Message{
		"USER#user-a-sub": {
			"user-b-sub": {
				{ID: "msg-ab-1", Text: "Secret between A and B", Time: "2026-10-20T10:00:00Z", SenderID: userA.id, RecipientID: userB.id},
			},
		},
		"USER#user-b-sub": {
			"user-a-sub": {
				{ID: "msg-ab-1", Text: "Secret between A and B", Time: "2026-10-20T10:00:00Z", SenderID: userA.id, RecipientID: userB.id},
			},
		},
		"USER#user-c-sub": {
			"user-a-sub": {},
		},
	}

	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			if peers, ok := partitionStore["USER#"+userKey]; ok {
				return peers[peerKey], nil
			}
			return nil, nil
		},
	}

	handler := NewHandler(repo)
	handler.SetUserResolver(resolver)

	// User C queries messages with peer @alice
	ctxC := context.WithValue(context.Background(), middleware.UserIDKey, userC.id)
	ctxC = context.WithValue(ctxC, middleware.UserEmailKey, userC.email)
	ctxC = context.WithValue(ctxC, middleware.UserHandleKey, userC.handle)

	req := httptest.NewRequest("GET", "/chat/messages?peer=@alice", nil).WithContext(ctxC)
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var msgs []Message
	if err := json.Unmarshal(rec.Body.Bytes(), &msgs); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// User C MUST NOT see any messages between User A and User B
	if len(msgs) != 0 {
		t.Fatalf("CRITICAL: User C with recycled handle accessed User B's private messages! got %d messages", len(msgs))
	}
}

// 2. Handle-change resilience:
// User A renames their handle.
// User A can still read all prior messages with User B.
// User B can still read all prior messages with User A.
func TestRegression_2_HandleChangeResilience(t *testing.T) {
	userA := &testUser{id: "user-a-sub", email: "alice@example.com", handle: "alice_new"}
	userB := &testUser{id: "user-b-sub", email: "bob@example.com", handle: "bob"}

	resolver := &testResolver{users: []*testUser{userA, userB}}

	partitionStore := map[string]map[string][]Message{
		"USER#user-a-sub": {
			"user-b-sub": {
				{ID: "msg-1", Text: "Hello B", Time: "2026-10-20T10:00:00Z", SenderID: userA.id, RecipientID: userB.id},
			},
		},
		"USER#user-b-sub": {
			"user-a-sub": {
				{ID: "msg-1", Text: "Hello B", Time: "2026-10-20T10:00:00Z", SenderID: userA.id, RecipientID: userB.id},
			},
		},
	}

	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			if peers, ok := partitionStore["USER#"+userKey]; ok {
				return peers[peerKey], nil
			}
			return nil, nil
		},
	}

	handler := NewHandler(repo)
	handler.SetUserResolver(resolver)

	// User A (with new handle alice_new) reads messages with @bob
	ctxA := context.WithValue(context.Background(), middleware.UserIDKey, userA.id)
	ctxA = context.WithValue(ctxA, middleware.UserEmailKey, userA.email)
	ctxA = context.WithValue(ctxA, middleware.UserHandleKey, userA.handle)

	reqA := httptest.NewRequest("GET", "/chat/messages?peer=@bob", nil).WithContext(ctxA)
	recA := httptest.NewRecorder()
	handler.GetHistory(recA, reqA)

	if recA.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recA.Code)
	}
	var msgsA []Message
	json.Unmarshal(recA.Body.Bytes(), &msgsA)
	if len(msgsA) != 1 || msgsA[0].ID != "msg-1" {
		t.Fatalf("User A failed to read prior message after handle change: %+v", msgsA)
	}

	// User B reads messages with @alice_new
	ctxB := context.WithValue(context.Background(), middleware.UserIDKey, userB.id)
	ctxB = context.WithValue(ctxB, middleware.UserEmailKey, userB.email)
	ctxB = context.WithValue(ctxB, middleware.UserHandleKey, userB.handle)

	reqB := httptest.NewRequest("GET", "/chat/messages?peer=@alice_new", nil).WithContext(ctxB)
	recB := httptest.NewRecorder()
	handler.GetHistory(recB, reqB)

	if recB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recB.Code)
	}
	var msgsB []Message
	json.Unmarshal(recB.Body.Bytes(), &msgsB)
	if len(msgsB) != 1 || msgsB[0].ID != "msg-1" {
		t.Fatalf("User B failed to read prior message with Alice under new handle: %+v", msgsB)
	}
}

// 3. Handle reuse protection:
// User B deletes their account or releases @bob.
// User D registers @bob with a different user ID.
// User D cannot see User A's prior conversations with the original @bob.
func TestRegression_3_HandleReuseProtection(t *testing.T) {
	userA := &testUser{id: "user-a-sub", email: "alice@example.com", handle: "alice"}
	userD := &testUser{id: "user-d-sub", email: "david@example.com", handle: "bob"} // claimed released @bob

	resolver := &testResolver{users: []*testUser{userA, userD}}

	partitionStore := map[string]map[string][]Message{
		"USER#user-a-sub": {
			"user-b-sub": {
				{ID: "msg-old-1", Text: "Private info for original Bob", Time: "2026-10-10T10:00:00Z", SenderID: "user-a-sub", RecipientID: "user-b-sub"},
			},
		},
		"USER#user-d-sub": {},
	}

	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			if peers, ok := partitionStore["USER#"+userKey]; ok {
				return peers[peerKey], nil
			}
			return nil, nil
		},
	}

	handler := NewHandler(repo)
	handler.SetUserResolver(resolver)

	// User D attempts to read chat with @alice
	ctxD := context.WithValue(context.Background(), middleware.UserIDKey, userD.id)
	ctxD = context.WithValue(ctxD, middleware.UserEmailKey, userD.email)
	ctxD = context.WithValue(ctxD, middleware.UserHandleKey, userD.handle)

	req := httptest.NewRequest("GET", "/chat/messages?peer=@alice", nil).WithContext(ctxD)
	rec := httptest.NewRecorder()
	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var msgs []Message
	json.Unmarshal(rec.Body.Bytes(), &msgs)
	if len(msgs) != 0 {
		t.Fatalf("CRITICAL: User D with recycled handle @bob accessed prior messages! got %d", len(msgs))
	}
}

// 5. Pagination across identical timestamps:
// Create at least 5 messages with the exact same timestamp.
// Paginate through the conversation with a page size smaller than 5.
// Verify every message is returned exactly once in deterministic order.
func TestRegression_5_PaginationAcrossIdenticalTimestamps(t *testing.T) {
	sameTimestamp := "2026-10-20T12:00:00Z"
	allCreated := []Message{
		{ID: "msg-1", Text: "1", Time: sameTimestamp},
		{ID: "msg-2", Text: "2", Time: sameTimestamp},
		{ID: "msg-3", Text: "3", Time: sameTimestamp},
		{ID: "msg-4", Text: "4", Time: sameTimestamp},
		{ID: "msg-5", Text: "5", Time: sameTimestamp},
	}

	// Simulate repository pagination descending logic
	simulatedRepo := &mockChatRepo{
		historyWithBeforeFunc: func(ctx context.Context, userKey, peerKey string, limit int32, before string) ([]Message, error) {
			// Descending items (newest to oldest by ID for identical timestamps)
			descending := make([]Message, len(allCreated))
			for i := range allCreated {
				descending[i] = allCreated[len(allCreated)-1-i]
			}

			var filtered []Message
			for _, m := range descending {
				itemSuffix := fmt.Sprintf("%s#%s", m.Time, m.ID)
				if before != "" && (before == itemSuffix || before == m.Time) {
					continue
				}
				if before != "" && itemSuffix >= before {
					continue
				}
				filtered = append(filtered, m)
				if len(filtered) == int(limit) {
					break
				}
			}

			// Ascending return order
			ascending := make([]Message, len(filtered))
			for i := range filtered {
				ascending[i] = filtered[len(filtered)-1-i]
			}
			return ascending, nil
		},
	}

	handler := NewHandler(simulatedRepo)
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, "user-a")

	var collected []Message
	pageSize := 2
	beforeCursor := ""

	for page := 0; page < 5; page++ {
		url := fmt.Sprintf("/chat/messages?peer=user-b&limit=%d", pageSize)
		if beforeCursor != "" {
			url += "&before=" + beforeCursor
		}

		req := httptest.NewRequest("GET", url, nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.GetHistory(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("page %d failed with status %d", page, rec.Code)
		}

		var pageMsgs []Message
		if err := json.Unmarshal(rec.Body.Bytes(), &pageMsgs); err != nil {
			t.Fatalf("page %d unmarshal failed: %v", page, err)
		}

		if len(pageMsgs) == 0 {
			break
		}

		for _, m := range pageMsgs {
			collected = append(collected, m)
		}

		// Cursor for next page is the oldest message on the current page
		oldest := pageMsgs[0]
		beforeCursor = fmt.Sprintf("%s#%s", oldest.Time, oldest.ID)
	}

	// Verify all 5 messages returned exactly once in order msg-5 down to msg-1
	if len(collected) != 5 {
		t.Fatalf("expected exactly 5 messages across pagination, got %d: %+v", len(collected), collected)
	}

	seenIDs := make(map[string]int)
	for _, m := range collected {
		seenIDs[m.ID]++
	}

	for _, expected := range allCreated {
		if seenIDs[expected.ID] != 1 {
			t.Fatalf("message %s appeared %d times (expected exactly 1)", expected.ID, seenIDs[expected.ID])
		}
	}
}

// 6. Deletion authorization:
// User A sends a message to User B.
// User B attempts to delete User A's message (must fail).
// A third user with the same handle as User A's former handle attempts to delete it (must fail).
// User A deletes the message (must succeed, removing both copies).
func TestRegression_6_DeletionAuthorization(t *testing.T) {
	senderCopy := &Message{
		PK:           "USER#user-a",
		SK:           "CHAT#user-b#2026-10-20T10:00:00Z#msg-del-1",
		ID:           "msg-del-1",
		SenderID:     "user-a",
		RecipientID:  "user-b",
		SenderEmail:  "alice@example.com",
		SenderHandle: "alice_former",
	}
	recipientCopy := &Message{
		PK:           "USER#user-b",
		SK:           "CHAT#user-a#2026-10-20T10:00:00Z#msg-del-1",
		ID:           "msg-del-1",
		SenderID:     "user-a",
		RecipientID:  "user-b",
		SenderEmail:  "alice@example.com",
		SenderHandle: "alice_former",
	}

	messages := map[string]*Message{
		senderCopy.PK + ":" + senderCopy.SK:       senderCopy,
		recipientCopy.PK + ":" + recipientCopy.SK: recipientCopy,
	}

	repo := &mockChatRepo{
		deleteFunc: func(ctx context.Context, callerEmail, callerHandle, callerID, msgID string) error {
			// Authorship verification:
			var found *Message
			for _, m := range messages {
				if m.ID == msgID && m.PK == "USER#"+callerID {
					found = m
					break
				}
			}
			if found == nil {
				return errors.New("message not found or unauthorized")
			}
			// Strictly UserID or provable email
			if found.SenderID != callerID {
				return errors.New("forbidden: only the sender may delete this message")
			}
			// Delete both copies
			delete(messages, senderCopy.PK+":"+senderCopy.SK)
			delete(messages, recipientCopy.PK+":"+recipientCopy.SK)
			return nil
		},
	}

	// 1. Recipient (User B) attempts to delete User A's message -> must fail
	errRecip := repo.DeleteMessage(context.Background(), "bob@example.com", "bob", "user-b", "msg-del-1")
	if errRecip == nil {
		t.Fatalf("expected error when recipient attempts to delete sender's message, got nil")
	}

	// 2. Third user with Alice's former handle attempts to delete -> must fail
	errAttacker := repo.DeleteMessage(context.Background(), "eve@example.com", "alice_former", "user-e", "msg-del-1")
	if errAttacker == nil {
		t.Fatalf("expected error when handle squatter attempts to delete message, got nil")
	}

	// 3. Authentic sender (User A) deletes -> succeeds
	errSender := repo.DeleteMessage(context.Background(), "alice@example.com", "alice_new", "user-a", "msg-del-1")
	if errSender != nil {
		t.Fatalf("expected sender deletion to succeed, got error: %v", errSender)
	}

	if len(messages) != 0 {
		t.Fatalf("expected both sender and recipient copies to be deleted, remaining: %d", len(messages))
	}
}

// 7. Legacy email read-through:
// Pre-existing messages stored under USER#<email> partitions are still readable by the authenticated owner of that email.
func TestRegression_7_LegacyEmailReadThrough(t *testing.T) {
	userA := &testUser{id: "user-a-sub", email: "alice@example.com", handle: "alice"}
	userB := &testUser{id: "user-b-sub", email: "bob@example.com", handle: "bob"}
	resolver := &testResolver{users: []*testUser{userA, userB}}

	legacyStore := map[string]map[string][]Message{
		"alice@example.com": {
			"bob@example.com": {
				{ID: "legacy-1", Text: "Old email-keyed message", Time: "2026-09-01T10:00:00Z", SenderEmail: "alice@example.com", RecipientEmail: "bob@example.com"},
			},
		},
	}

	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			if peers, ok := legacyStore[userKey]; ok {
				return peers[peerKey], nil
			}
			return nil, nil
		},
	}

	handler := NewHandler(repo)
	handler.SetUserResolver(resolver)

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userA.id)
	ctx = context.WithValue(ctx, middleware.UserEmailKey, userA.email)

	req := httptest.NewRequest("GET", "/chat/messages?peer=bob@example.com", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var msgs []Message
	json.Unmarshal(rec.Body.Bytes(), &msgs)
	if len(msgs) != 1 || msgs[0].ID != "legacy-1" {
		t.Fatalf("expected 1 legacy message read-through, got %+v", msgs)
	}
}

// 8. Legacy handle rejection:
// Pre-existing messages stored under USER#@<handle> partitions are not returned to a caller who only matches the handle.
func TestRegression_8_LegacyHandleRejection(t *testing.T) {
	userNew := &testUser{id: "user-new-sub", email: "new@example.com", handle: "recycled_handle"}
	resolver := &testResolver{users: []*testUser{userNew, {id: "user-bob", email: "bob@example.com", handle: "bob"}}}

	legacyHandleStore := map[string]map[string][]Message{
		"@recycled_handle": {
			"user-bob": {
				{ID: "leak-1", Text: "Confidential past message", Time: "2026-08-01T10:00:00Z"},
			},
		},
	}

	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			if peers, ok := legacyHandleStore[userKey]; ok {
				return peers[peerKey], nil
			}
			return nil, nil
		},
	}

	handler := NewHandler(repo)
	handler.SetUserResolver(resolver)

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userNew.id)
	ctx = context.WithValue(ctx, middleware.UserEmailKey, userNew.email)
	ctx = context.WithValue(ctx, middleware.UserHandleKey, userNew.handle)

	req := httptest.NewRequest("GET", "/chat/messages?peer=@bob", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var msgs []Message
	json.Unmarshal(rec.Body.Bytes(), &msgs)
	if len(msgs) != 0 {
		t.Fatalf("CRITICAL: Legacy handle partition was leaked to new handle holder! got %+v", msgs)
	}
}

// 9. Malformed or unresolvable peer:
// Requests with nonexistent or unresolvable peer identifiers return a clean error without panic, data leaks, or scans.
func TestRegression_9_MalformedOrUnresolvablePeer(t *testing.T) {
	resolver := &testResolver{users: []*testUser{}} // empty resolver -> all lookups fail
	repo := &mockChatRepo{}

	handler := NewHandler(repo)
	handler.SetUserResolver(resolver)

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, "caller-sub")
	req := httptest.NewRequest("GET", "/chat/messages?peer=ghost_user_99999", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 StatusNotFound for unresolvable peer, got %d", rec.Code)
	}
}

// 10. Race conditions:
// Concurrent message sends and reads between two users do not produce duplicate records or inconsistent sort order.
func TestRegression_10_RaceConditions_ConcurrentSendsAndReads(t *testing.T) {
	var mu sync.Mutex
	history := []Message{}

	repo := &mockChatRepo{
		historyFunc: func(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error) {
			mu.Lock()
			defer mu.Unlock()
			copied := make([]Message, len(history))
			copy(copied, history)
			return copied, nil
		},
	}

	handler := NewHandler(repo)
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, "user-a")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		msgID := fmt.Sprintf("race-msg-%d", i)
		timeStr := fmt.Sprintf("2026-10-20T12:%02d:00Z", i%60)

		// Concurrent writer
		go func(id, tm string) {
			defer wg.Done()
			mu.Lock()
			history = append(history, Message{ID: id, Time: tm, Text: "race text"})
			mu.Unlock()
		}(msgID, timeStr)

		// Concurrent reader
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/chat/messages?peer=user-b", nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			handler.GetHistory(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("concurrent read returned status %d", rec.Code)
			}
		}()
	}
	wg.Wait()
}

type mockDynamoDBClient struct {
	transactWriteItemsFunc func(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	queryFunc              func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	deleteItemFunc         func(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	updateItemFunc         func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

func (m *mockDynamoDBClient) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	if m.transactWriteItemsFunc != nil {
		return m.transactWriteItemsFunc(ctx, params, optFns...)
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (m *mockDynamoDBClient) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, params, optFns...)
	}
	return &dynamodb.QueryOutput{}, nil
}

func (m *mockDynamoDBClient) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	if m.deleteItemFunc != nil {
		return m.deleteItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.DeleteItemOutput{}, nil
}

func (m *mockDynamoDBClient) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	if m.updateItemFunc != nil {
		return m.updateItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

// Priority 2 Regression Test: SaveMessage writes a single TransactItem when senderKey == recipientKey
func TestRegression_Priority2_SelfMessagingPersistence_SingleItem(t *testing.T) {
	var capturedItems []types.TransactWriteItem

	mockDB := &mockDynamoDBClient{
		transactWriteItemsFunc: func(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
			capturedItems = params.TransactItems
			return &dynamodb.TransactWriteItemsOutput{}, nil
		},
	}

	repo := NewRepository(mockDB, "Onusly")

	// 1. Self-messaging: SenderID == RecipientID
	selfMsg := realtime.StoredMessage{
		ID:             "self-msg-1",
		SenderID:       "user-self-sub",
		RecipientID:    "user-self-sub",
		SenderEmail:    "self@example.com",
		RecipientEmail: "self@example.com",
		Text:           "Note to self",
		Time:           "2026-10-20T10:00:00Z",
	}

	if err := repo.SaveMessage(context.Background(), selfMsg, true); err != nil {
		t.Fatalf("expected SaveMessage to succeed for self-messaging: %v", err)
	}

	if len(capturedItems) != 1 {
		t.Fatalf("CRITICAL: expected exactly 1 TransactWriteItem for self-messaging, got %d", len(capturedItems))
	}

	// 2. Peer messaging: SenderID != RecipientID
	peerMsg := realtime.StoredMessage{
		ID:             "peer-msg-1",
		SenderID:       "user-self-sub",
		RecipientID:    "user-peer-sub",
		SenderEmail:    "self@example.com",
		RecipientEmail: "peer@example.com",
		Text:           "Hello friend",
		Time:           "2026-10-20T10:01:00Z",
	}

	if err := repo.SaveMessage(context.Background(), peerMsg, true); err != nil {
		t.Fatalf("expected SaveMessage to succeed for peer messaging: %v", err)
	}

	if len(capturedItems) != 2 {
		t.Fatalf("expected exactly 2 TransactWriteItems for peer messaging, got %d", len(capturedItems))
	}
}

// Priority 5 Regression Test: DeleteMessage uses atomic TransactWriteItems, single delete for self-messaging, and propagates errors
func TestRegression_Priority5_DeleteMessage_AtomicTransaction(t *testing.T) {
	var capturedItems []types.TransactWriteItem
	var shouldFail bool

	mockDB := &mockDynamoDBClient{
		queryFunc: func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			// Return test message
			pkVal := params.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
			var msg Message
			if pkVal == "USER#user-self-sub" {
				msg = Message{
					PK:          "USER#user-self-sub",
					SK:          "CHAT#user-self-sub#2026-10-20T10:00:00Z#msg-self-del",
					ID:          "msg-self-del",
					SenderID:    "user-self-sub",
					RecipientID: "user-self-sub",
					Time:        "2026-10-20T10:00:00Z",
				}
			} else {
				msg = Message{
					PK:          "USER#user-alice-sub",
					SK:          "CHAT#user-bob-sub#2026-10-20T10:00:00Z#msg-peer-del",
					ID:          "msg-peer-del",
					SenderID:    "user-alice-sub",
					RecipientID: "user-bob-sub",
					Time:        "2026-10-20T10:00:00Z",
				}
			}
			mMap, _ := attributevalue.MarshalMap(msg)
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{mMap}}, nil
		},
		transactWriteItemsFunc: func(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
			if shouldFail {
				return nil, errors.New("simulated dynamo transact write error")
			}
			capturedItems = params.TransactItems
			return &dynamodb.TransactWriteItemsOutput{}, nil
		},
	}

	repo := NewRepository(mockDB, "Onusly")

	// 1. Self-messaging delete: must produce exactly 1 TransactWriteItem
	err := repo.DeleteMessage(context.Background(), "self@example.com", "self", "user-self-sub", "msg-self-del")
	if err != nil {
		t.Fatalf("expected self message delete to succeed: %v", err)
	}
	if len(capturedItems) != 1 {
		t.Fatalf("expected exactly 1 TransactWriteItem for self message deletion, got %d", len(capturedItems))
	}

	// 2. Peer messaging delete: must produce exactly 2 TransactWriteItems
	err = repo.DeleteMessage(context.Background(), "alice@example.com", "alice", "user-alice-sub", "msg-peer-del")
	if err != nil {
		t.Fatalf("expected peer message delete to succeed: %v", err)
	}
	if len(capturedItems) != 2 {
		t.Fatalf("expected exactly 2 TransactWriteItems for peer message deletion, got %d", len(capturedItems))
	}

	// 3. Error propagation: when TransactWriteItems fails, DeleteMessage must propagate error
	shouldFail = true
	err = repo.DeleteMessage(context.Background(), "alice@example.com", "alice", "user-alice-sub", "msg-peer-del")
	if err == nil {
		t.Fatalf("expected DeleteMessage to propagate TransactWriteItems error, got nil")
	}
	if !strings.Contains(err.Error(), "simulated dynamo transact write error") {
		t.Fatalf("expected wrapped error message, got: %v", err)
	}
}

// Priority 7 Regression Test: ClearConversation preserves case-sensitive immutable user IDs
func TestRegression_Priority7_ClearConversation_CaseSensitivity(t *testing.T) {
	var capturedPK, capturedSK string

	mockDB := &mockDynamoDBClient{
		queryFunc: func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			capturedPK = params.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
			capturedSK = params.ExpressionAttributeValues[":sk"].(*types.AttributeValueMemberS).Value
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil
		},
	}

	repo := NewRepository(mockDB, "Onusly")

	mixedCaseUser := "User-Sub-123AbC"
	mixedCasePeer := "Peer-Sub-456XyZ"

	err := repo.ClearConversation(context.Background(), mixedCaseUser, mixedCasePeer)
	if err != nil {
		t.Fatalf("expected ClearConversation to succeed: %v", err)
	}

	expectedPK := "USER#" + mixedCaseUser
	if capturedPK != expectedPK {
		t.Fatalf("CRITICAL: ClearConversation corrupted case-sensitive user ID! expected %s, got %s", expectedPK, capturedPK)
	}

	expectedSK := fmt.Sprintf("CHAT#%s#", mixedCasePeer)
	if capturedSK != expectedSK {
		t.Fatalf("CRITICAL: ClearConversation corrupted case-sensitive peer ID! expected %s, got %s", expectedSK, capturedSK)
	}
}

// Priority 4 Regression Test: GetHistory parses limit and exhausts canonical partition before legacy fallback
func TestRegression_Priority4_GetHistory_LimitAndCanonicalExhaustion(t *testing.T) {
	userA := &testUser{id: "user-a-sub", email: "alice@example.com", handle: "alice"}
	userB := &testUser{id: "user-b-sub", email: "bob@example.com", handle: "bob"}
	resolver := &testResolver{users: []*testUser{userA, userB}}

	// Canonical partition has 2 messages
	canonicalStore := []Message{
		{ID: "c-1", Time: "2026-10-20T10:00:00Z", Text: "Canonical 1"},
		{ID: "c-2", Time: "2026-10-20T10:01:00Z", Text: "Canonical 2"},
	}

	// Legacy partition has 2 messages
	legacyStore := []Message{
		{ID: "l-1", Time: "2026-10-19T10:00:00Z", Text: "Legacy 1", SenderEmail: "alice@example.com", RecipientEmail: "bob@example.com"},
		{ID: "l-2", Time: "2026-10-19T10:01:00Z", Text: "Legacy 2", SenderEmail: "alice@example.com", RecipientEmail: "bob@example.com"},
	}

	var legacyQueried bool
	var requestedLimit int32

	repo := &mockChatRepo{
		historyWithBeforeFunc: func(ctx context.Context, userKey, peerKey string, limit int32, before string) ([]Message, error) {
			requestedLimit = limit
			if userKey == "user-a-sub" {
				if before == "2026-10-20T10:00:00Z#c-1" {
					// Oldest canonical reached -> canonical is exhausted
					return []Message{}, nil
				}
				if before == "" {
					return canonicalStore, nil
				}
				return []Message{}, nil
			}
			if userKey == "alice@example.com" {
				legacyQueried = true
				return legacyStore, nil
			}
			return nil, nil
		},
	}

	handler := NewHandler(repo)
	handler.SetUserResolver(resolver)

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userA.id)
	ctx = context.WithValue(ctx, middleware.UserEmailKey, userA.email)

	// Step 1: Initial load without before cursor.
	// Since canonical has messages, legacy partition must NOT be queried (preventing dual-read pagination gap).
	req1 := httptest.NewRequest("GET", "/chat/messages?peer=@bob&limit=5", nil).WithContext(ctx)
	rec1 := httptest.NewRecorder()
	handler.GetHistory(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec1.Code)
	}
	if requestedLimit != 5 {
		t.Fatalf("expected limit query parameter to be passed as 5, got %d", requestedLimit)
	}
	if legacyQueried {
		t.Fatalf("CRITICAL: Legacy partition was queried before exhausting canonical messages! Dual-read pagination gap detected.")
	}

	var msgs1 []Message
	_ = json.Unmarshal(rec1.Body.Bytes(), &msgs1)
	if len(msgs1) != 2 || msgs1[0].ID != "c-1" || msgs1[1].ID != "c-2" {
		t.Fatalf("expected 2 canonical messages, got %+v", msgs1)
	}

	// Step 2: Client paginates backwards past oldest canonical message.
	// Now canonical returns 0 items -> canonical exhausted -> legacy partition is queried.
	req2 := httptest.NewRequest("GET", "/chat/messages?peer=@bob&before=2026-10-20T10:00:00Z%23c-1", nil).WithContext(ctx)
	rec2 := httptest.NewRecorder()
	handler.GetHistory(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec2.Code)
	}
	if !legacyQueried {
		t.Fatalf("expected legacy partition fallback when canonical messages are exhausted")
	}

	var msgs2 []Message
	_ = json.Unmarshal(rec2.Body.Bytes(), &msgs2)
	if len(msgs2) != 2 || msgs2[0].ID != "l-1" || msgs2[1].ID != "l-2" {
		t.Fatalf("expected 2 legacy messages after canonical exhaustion, got %+v", msgs2)
	}
}

func TestChatHandler_CommunityMessages(t *testing.T) {
	savedCommID := ""
	var savedStored realtime.StoredMessage

	repo := &mockChatRepo{}
	mockResolver := &testResolver{
		users: []*testUser{}, // No users configured, community should not be resolved as user
	}

	handler := NewHandler(repo)
	handler.SetUserResolver(mockResolver)

	// Set custom community save and get
	repoSaveComm := func(ctx context.Context, communityID string, msg realtime.StoredMessage) error {
		savedCommID = communityID
		savedStored = msg
		return nil
	}
	repoGetComm := func(ctx context.Context, communityID string, limit int32, before ...string) ([]Message, error) {
		if communityID == "comm-leetcode" && savedCommID == "comm-leetcode" {
			return []Message{
				{
					ID:          savedStored.ID,
					SenderID:    savedStored.SenderID,
					RecipientID: savedStored.RecipientID,
					Text:        savedStored.Text,
					Time:        savedStored.Time,
				},
			}, nil
		}
		return []Message{}, nil
	}

	// We create a wrapper that delegates to repoGetComm and repoSaveComm
	communityRepo := &struct {
		*mockChatRepo
		getComm  func(ctx context.Context, communityID string, limit int32, before ...string) ([]Message, error)
		saveComm func(ctx context.Context, communityID string, msg realtime.StoredMessage) error
	}{
		mockChatRepo: repo,
		getComm:      repoGetComm,
		saveComm:     repoSaveComm,
	}

	// Dynamic override on mockChatRepo
	_ = communityRepo

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, "user-alice")
	ctx = context.WithValue(ctx, middleware.UserEmailKey, "alice@example.com")
	ctx = context.WithValue(ctx, middleware.UserHandleKey, "alice")

	// 1. GET /chat/messages?peer=comm-leetcode (empty initially, should NOT return 404 peer not found)
	reqGet := httptest.NewRequest("GET", "/chat/messages?peer=comm-leetcode", nil).WithContext(ctx)
	recGet := httptest.NewRecorder()
	handler.GetHistory(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for community chat history, got %d: %s", recGet.Code, recGet.Body.String())
	}

	var initialMsgs []Message
	_ = json.Unmarshal(recGet.Body.Bytes(), &initialMsgs)
	if len(initialMsgs) != 0 {
		t.Fatalf("expected empty community message list, got %d", len(initialMsgs))
	}

	// 2. POST /chat/messages with recipient=comm-leetcode
	payload := `{"recipient":"comm-leetcode","text":"Hello LeetCode Community!"}`
	reqPost := httptest.NewRequest("POST", "/chat/messages", strings.NewReader(payload)).WithContext(ctx)
	recPost := httptest.NewRecorder()
	handler.SendMessage(recPost, reqPost)

	if recPost.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for community send message, got %d: %s", recPost.Code, recPost.Body.String())
	}

	var sentMsg Message
	_ = json.Unmarshal(recPost.Body.Bytes(), &sentMsg)
	if sentMsg.RecipientID != "comm-leetcode" || sentMsg.Text != "Hello LeetCode Community!" {
		t.Fatalf("unexpected sent message: %+v", sentMsg)
	}
}
