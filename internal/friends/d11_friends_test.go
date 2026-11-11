package friends

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
)

type d11MockUserResolver struct {
	users map[string]struct {
		userID string
		email  string
		handle string
	}
}

func (r *d11MockUserResolver) ResolveUser(ctx context.Context, identifier string) (string, string, string, error) {
	for _, u := range r.users {
		if strings.EqualFold(u.handle, identifier) || strings.EqualFold(u.email, identifier) || u.userID == identifier {
			return u.userID, u.email, u.handle, nil
		}
	}
	return "", "", "", errors.New("user not found")
}

type d11MockBroadcaster struct {
	mu     sync.Mutex
	events []realtime.Event
}

func (b *d11MockBroadcaster) SendToUser(userID string, event realtime.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
	return nil
}

func setupD11FriendsService() (*Service, *Repository, *mockDynamoDB) {
	mockDB := newMockDynamoDB()
	repo := NewRepository(mockDB, "Onusly")
	resolver := &d11MockUserResolver{
		users: map[string]struct {
			userID string
			email  string
			handle string
		}{
			"bob":   {userID: "user-bob", email: "bob@example.com", handle: "bob"},
			"alice": {userID: "user-alice", email: "alice@example.com", handle: "alice"},
		},
	}
	broadcaster := &d11MockBroadcaster{}
	svc := NewService(repo, resolver, broadcaster)
	return svc, repo, mockDB
}

func TestD11_Friends_InvitationExpiry_Configured(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := setupD11FriendsService()

	now := time.Now().UTC()
	req, err := svc.SendRequest(ctx, "user-alice", "alice@example.com", "alice", "bob")
	if err != nil {
		t.Fatalf("expected send request to succeed, got: %v", err)
	}

	if req.ExpiresAt.IsZero() {
		t.Fatalf("expected ExpiresAt to be set, got zero time")
	}

	diff := req.ExpiresAt.Sub(now)
	if diff < 6*24*time.Hour || diff > 8*24*time.Hour {
		t.Fatalf("expected ExpiresAt to be ~7 days in the future, got diff: %v", diff)
	}
}

func TestD11_Friends_ExpiredInvitation_CannotBeAccepted(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := setupD11FriendsService()

	// Create an expired request manually
	reqID := "req-expired-101"
	expiredTime := time.Now().UTC().Add(-2 * time.Hour)
	req := FriendRequest{
		ID:              reqID,
		SenderID:        "user-alice",
		SenderEmail:     "alice@example.com",
		SenderHandle:    "alice",
		RecipientID:     "user-bob",
		RecipientEmail:  "bob@example.com",
		RecipientHandle: "bob",
		Status:          "pending",
		CreatedAt:       expiredTime.Add(-7 * 24 * time.Hour),
		ExpiresAt:       expiredTime,
	}

	if err := repo.SaveFriendRequest(ctx, req); err != nil {
		t.Fatalf("failed to seed expired request: %v", err)
	}

	// Bob tries to accept expired request
	err := svc.RespondRequest(ctx, "user-bob", reqID, "accept")
	if err == nil {
		t.Fatalf("expected error when accepting expired request, but got nil")
	}

	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected error message to mention 'expired', got: %v", err)
	}
}

func TestD11_Friends_ExpiredInvitation_SafeRetry(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := setupD11FriendsService()

	// Seed an expired outgoing request for Alice -> Bob
	reqID := "req-expired-102"
	expiredTime := time.Now().UTC().Add(-1 * time.Hour)
	req := FriendRequest{
		ID:              reqID,
		SenderID:        "user-alice",
		SenderEmail:     "alice@example.com",
		SenderHandle:    "alice",
		RecipientID:     "user-bob",
		RecipientEmail:  "bob@example.com",
		RecipientHandle: "bob",
		Status:          "pending",
		CreatedAt:       expiredTime.Add(-7 * 24 * time.Hour),
		ExpiresAt:       expiredTime,
	}

	if err := repo.SaveFriendRequest(ctx, req); err != nil {
		t.Fatalf("failed to seed expired request: %v", err)
	}

	// Sending a new request should succeed (safe retry) rather than failing with "already pending"
	newReq, err := svc.SendRequest(ctx, "user-alice", "alice@example.com", "alice", "bob")
	if err != nil {
		t.Fatalf("expected retry request to succeed for expired request, got: %v", err)
	}

	if newReq.ID == reqID {
		t.Fatalf("expected a new request ID, got the same: %s", newReq.ID)
	}
	if newReq.ExpiresAt.Before(time.Now().UTC()) {
		t.Fatalf("expected new request to have future expiration, got: %v", newReq.ExpiresAt)
	}
}

func TestD11_Friends_AntiReplay_AtomicConsumption(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := setupD11FriendsService()

	// Seed a valid pending request
	reqID := "req-valid-103"
	req := FriendRequest{
		ID:              reqID,
		SenderID:        "user-alice",
		SenderEmail:     "alice@example.com",
		SenderHandle:    "alice",
		RecipientID:     "user-bob",
		RecipientEmail:  "bob@example.com",
		RecipientHandle: "bob",
		Status:          "pending",
		CreatedAt:       time.Now().UTC(),
		ExpiresAt:       time.Now().UTC().Add(7 * 24 * time.Hour),
	}

	if err := repo.SaveFriendRequest(ctx, req); err != nil {
		t.Fatalf("failed to seed valid request: %v", err)
	}

	// First accept succeeds
	err := svc.RespondRequest(ctx, "user-bob", reqID, "accept")
	if err != nil {
		t.Fatalf("expected first accept to succeed, got: %v", err)
	}

	// Second accept (replay) MUST fail because incoming request item was atomically deleted
	err = svc.RespondRequest(ctx, "user-bob", reqID, "accept")
	if err == nil {
		t.Fatalf("expected replay accept to fail, but succeeded")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected error to indicate request not found or consumed, got: %v", err)
	}
}

func TestD11_Friends_TransactWriteItems_IncludesAttributeExists(t *testing.T) {
	ctx := context.Background()

	// Validate that the TransactWriteItems call constructed by AcceptFriendRequest
	// explicitly enforces ConditionExpression: attribute_exists(PK) on Delete
	var capturedInput *dynamodb.TransactWriteItemsInput
	recordingDB := &conditionRecordingMockDB{
		onTransact: func(input *dynamodb.TransactWriteItemsInput) {
			capturedInput = input
		},
	}

	repo := NewRepository(recordingDB, "Onusly")
	req := FriendRequest{
		ID:          "test-req-cond",
		SenderID:    "user-alice",
		RecipientID: "user-bob",
	}

	_ = repo.AcceptFriendRequest(ctx, req)

	if capturedInput == nil {
		t.Fatalf("expected TransactWriteItems to be invoked")
	}

	var foundCondition bool
	for _, item := range capturedInput.TransactItems {
		if item.Delete != nil {
			if item.Delete.ConditionExpression != nil && *item.Delete.ConditionExpression == "attribute_exists(PK)" {
				foundCondition = true
				break
			}
		}
	}

	if !foundCondition {
		t.Fatalf("expected Delete in AcceptFriendRequest transaction to enforce attribute_exists(PK) condition")
	}
}

type conditionRecordingMockDB struct {
	onTransact func(input *dynamodb.TransactWriteItemsInput)
}

func (c *conditionRecordingMockDB) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	if c.onTransact != nil {
		c.onTransact(params)
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (c *conditionRecordingMockDB) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return &dynamodb.QueryOutput{}, nil
}

func (c *conditionRecordingMockDB) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return &dynamodb.DeleteItemOutput{}, nil
}

func (c *conditionRecordingMockDB) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return &dynamodb.UpdateItemOutput{}, nil
}

func (c *conditionRecordingMockDB) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, nil
}

func (c *conditionRecordingMockDB) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{
		Item: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#user-bob"},
			"SK": &types.AttributeValueMemberS{Value: "FRIEND_REQ_IN#test-req-cond"},
		},
	}, nil
}
