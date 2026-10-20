package friends

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
)

type mockDynamoDB struct {
	mu    sync.Mutex
	items map[string]map[string]types.AttributeValue // key: PK+SK -> item
}

func newMockDynamoDB() *mockDynamoDB {
	return &mockDynamoDB{
		items: make(map[string]map[string]types.AttributeValue),
	}
}

func (m *mockDynamoDB) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, action := range params.TransactItems {
		if action.Put != nil {
			pk := action.Put.Item["PK"].(*types.AttributeValueMemberS).Value
			sk := action.Put.Item["SK"].(*types.AttributeValueMemberS).Value
			m.items[pk+"#"+sk] = action.Put.Item
		}
		if action.Delete != nil {
			pk := action.Delete.Key["PK"].(*types.AttributeValueMemberS).Value
			sk := action.Delete.Key["SK"].(*types.AttributeValueMemberS).Value
			delete(m.items, pk+"#"+sk)
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (m *mockDynamoDB) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	targetPK := params.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
	prefixSK := ""
	if p, ok := params.ExpressionAttributeValues[":skPrefix"].(*types.AttributeValueMemberS); ok {
		prefixSK = p.Value
	}

	var results []map[string]types.AttributeValue
	for _, item := range m.items {
		pk := item["PK"].(*types.AttributeValueMemberS).Value
		sk := item["SK"].(*types.AttributeValueMemberS).Value
		if pk == targetPK && strings.HasPrefix(sk, prefixSK) {
			results = append(results, item)
		}
	}
	return &dynamodb.QueryOutput{Items: results}, nil
}

func (m *mockDynamoDB) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pk := params.Key["PK"].(*types.AttributeValueMemberS).Value
	sk := params.Key["SK"].(*types.AttributeValueMemberS).Value
	delete(m.items, pk+"#"+sk)
	return &dynamodb.DeleteItemOutput{}, nil
}

func (m *mockDynamoDB) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return &dynamodb.UpdateItemOutput{}, nil
}

func (m *mockDynamoDB) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pk := params.Item["PK"].(*types.AttributeValueMemberS).Value
	sk := params.Item["SK"].(*types.AttributeValueMemberS).Value
	m.items[pk+"#"+sk] = params.Item
	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockDynamoDB) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pk := params.Key["PK"].(*types.AttributeValueMemberS).Value
	sk := params.Key["SK"].(*types.AttributeValueMemberS).Value
	item, ok := m.items[pk+"#"+sk]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: item}, nil
}

type mockUserResolver struct {
	users map[string]struct {
		id     string
		email  string
		handle string
	}
}

func (r *mockUserResolver) ResolveUser(ctx context.Context, identifier string) (string, string, string, error) {
	clean := strings.TrimPrefix(strings.TrimSpace(identifier), "@")
	if u, ok := r.users[clean]; ok {
		return u.id, u.email, u.handle, nil
	}
	if u, ok := r.users[identifier]; ok {
		return u.id, u.email, u.handle, nil
	}
	return "", "", "", nil
}

type mockBroadcaster struct {
	mu     sync.Mutex
	events []realtime.Event
}

func (b *mockBroadcaster) SendToUser(userID string, event realtime.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
	return nil
}

func TestFriendRequestFlow(t *testing.T) {
	db := newMockDynamoDB()
	repo := NewRepository(db, "TestTable")
	resolver := &mockUserResolver{
		users: map[string]struct {
			id     string
			email  string
			handle string
		}{
			"bob":   {id: "user-bob-123", email: "bob@example.com", handle: "bob"},
			"alice": {id: "user-alice-456", email: "alice@example.com", handle: "alice"},
		},
	}
	broadcaster := &mockBroadcaster{}
	svc := NewService(repo, resolver, broadcaster)
	handler := NewHandler(svc)

	ctxAlice := context.WithValue(context.Background(), middleware.UserIDKey, "user-alice-456")
	ctxAlice = context.WithValue(ctxAlice, middleware.UserEmailKey, "alice@example.com")
	ctxAlice = context.WithValue(ctxAlice, middleware.UserHandleKey, "alice")

	ctxBob := context.WithValue(context.Background(), middleware.UserIDKey, "user-bob-123")
	ctxBob = context.WithValue(ctxBob, middleware.UserEmailKey, "bob@example.com")
	ctxBob = context.WithValue(ctxBob, middleware.UserHandleKey, "bob")

	// 1. Alice sends friend request to herself -> Expect Error
	_, err := svc.SendRequest(ctxAlice, "user-alice-456", "alice@example.com", "alice", "@alice")
	if err == nil || !strings.Contains(err.Error(), "cannot send friend request to yourself") {
		t.Fatalf("expected self-request error, got %v", err)
	}

	// 2. Alice sends friend request to Bob -> Success
	req, err := svc.SendRequest(ctxAlice, "user-alice-456", "alice@example.com", "alice", "@bob")
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if req.RecipientID != "user-bob-123" {
		t.Fatalf("expected recipient user-bob-123, got %s", req.RecipientID)
	}

	// 3. Bob checks incoming requests -> Should see Alice's request
	incoming, err := svc.GetIncomingRequests(ctxBob, "user-bob-123")
	if err != nil {
		t.Fatalf("get incoming requests failed: %v", err)
	}
	if len(incoming) != 1 || incoming[0].SenderID != "user-alice-456" {
		t.Fatalf("expected 1 incoming request from Alice, got %v", incoming)
	}

	// 4. Bob accepts Alice's friend request
	err = svc.RespondRequest(ctxBob, "user-bob-123", req.ID, "accept")
	if err != nil {
		t.Fatalf("accept request failed: %v", err)
	}

	// 5. Verify Alice is in Bob's friend list
	bobFriends, err := svc.GetFriends(ctxBob, "user-bob-123")
	if err != nil {
		t.Fatalf("get bob friends failed: %v", err)
	}
	if len(bobFriends) != 1 || bobFriends[0].FriendID != "user-alice-456" {
		t.Fatalf("expected Alice in Bob's friends, got %v", bobFriends)
	}

	// 6. Verify Bob is in Alice's friend list
	aliceFriends, err := svc.GetFriends(ctxAlice, "user-alice-456")
	if err != nil {
		t.Fatalf("get alice friends failed: %v", err)
	}
	if len(aliceFriends) != 1 || aliceFriends[0].FriendID != "user-bob-123" {
		t.Fatalf("expected Bob in Alice's friends, got %v", aliceFriends)
	}

	// 7. Verify incoming requests for Bob is now 0
	incomingAfter, _ := svc.GetIncomingRequests(ctxBob, "user-bob-123")
	if len(incomingAfter) != 0 {
		t.Fatalf("expected 0 incoming requests after accept, got %d", len(incomingAfter))
	}

	// 8. Test HTTP Handler GET /friends for Bob
	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest("GET", "/friends", nil).WithContext(ctxBob)
	handler.GetFriends(rec, httpReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var returnedFriends []Friend
	_ = json.Unmarshal([]byte(rec.Body.String()), &returnedFriends)
	if !strings.Contains(rec.Body.String(), "alice") {
		t.Fatalf("expected body to contain alice, got %s", rec.Body.String())
	}

	// 9. Community: Bob joins early risers
	joinReq := httptest.NewRequest("POST", "/communities/join", strings.NewReader(`{"communityId":"comm-early-risers"}`)).WithContext(ctxBob)
	joinRec := httptest.NewRecorder()
	handler.JoinCommunity(joinRec, joinReq)
	if joinRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for join, got %d (body: %s)", joinRec.Code, joinRec.Body.String())
	}

	// 10. Trending communities should show comm-early-risers as isJoined=true for Bob
	trending, err := svc.GetTrendingCommunities(ctxBob, "user-bob-123")
	if err != nil {
		t.Fatalf("get trending failed: %v", err)
	}
	foundJoined := false
	for _, c := range trending {
		if c.ID == "comm-early-risers" && c.IsJoined {
			foundJoined = true
			break
		}
	}
	if !foundJoined {
		t.Fatalf("expected comm-early-risers to be joined for Bob")
	}
}
