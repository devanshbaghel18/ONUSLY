package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

func getTestAuthDynamoClient(t *testing.T) (*dynamodb.Client, string) {
	t.Helper()
	tableName := "Onusly"
	db := shared.NewDynamoClient()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		t.Skipf("DynamoDB table %q is not accessible (%v). Skipping live DynamoDB test.", tableName, err)
	}

	return db, tableName
}

func cleanupAuthItems(t *testing.T, db *dynamodb.Client, tableName, userID string, handles ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if userID != "" {
			_, _ = db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: aws.String(tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
					"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
				},
			})
		}
		for _, h := range handles {
			clean := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(h)), "@")
			if clean != "" {
				_, _ = db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
					TableName: aws.String(tableName),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "HANDLE#" + clean},
						"SK": &types.AttributeValueMemberS{Value: "RESERVATION"},
					},
				})
			}
		}
	})
}

// TestDynamoRepository_Live_UserLifecycle verifies user creation, GSI1 email lookup,
// handle reservation, handle update, and atomic token version counter on real DynamoDB.
func TestDynamoRepository_Live_UserLifecycle(t *testing.T) {
	db, tableName := getTestAuthDynamoClient(t)

	repo := NewRepositoryWithTable(db, tableName).(*DynamoRepository)

	uid := "test-user-" + uuid.NewString()
	email := "test-" + uuid.NewString() + "@example.com"
	handle := "usr" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	newHandle := "new" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]

	cleanupAuthItems(t, db, tableName, uid, handle, newHandle)

	ctx := context.Background()

	// 1. Create User
	testUser := &User{
		ID:           uid,
		Email:        email,
		Handle:       handle,
		Name:         "AWS Test User",
		Picture:      "https://example.com/avatar.png",
		TokenVersion: 1,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	if err := repo.CreateUser(ctx, testUser); err != nil {
		t.Fatalf("CreateUser failed on live DynamoDB: %v", err)
	}

	// Clear local memory cache on a new instance to force reading strictly from DynamoDB
	coldRepo := NewRepositoryWithTable(db, tableName).(*DynamoRepository)

	// 2. GetUserByID
	fetchedByID, err := coldRepo.GetUserByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetUserByID failed on live DynamoDB: %v", err)
	}
	if fetchedByID == nil {
		t.Fatalf("expected user %s, got nil", uid)
	}
	if fetchedByID.Email != email {
		t.Errorf("expected email %s, got %s", email, fetchedByID.Email)
	}
	if fetchedByID.Handle != handle {
		t.Errorf("expected handle %s, got %s", handle, fetchedByID.Handle)
	}

	// 3. GetUserByEmail (queries GSI1 index)
	fetchedByEmail, err := coldRepo.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetUserByEmail (GSI1) failed on live DynamoDB: %v", err)
	}
	if fetchedByEmail == nil {
		t.Fatalf("expected user by email %s, got nil", email)
	}
	if fetchedByEmail.ID != uid {
		t.Errorf("expected ID %s, got %s", uid, fetchedByEmail.ID)
	}

	// 4. GetUserByHandle (resolves via HANDLE#<handle> reservation item)
	fetchedByHandle, err := coldRepo.GetUserByHandle(ctx, handle)
	if err != nil {
		t.Fatalf("GetUserByHandle failed on live DynamoDB: %v", err)
	}
	if fetchedByHandle == nil {
		t.Fatalf("expected user by handle %s, got nil", handle)
	}
	if fetchedByHandle.ID != uid {
		t.Errorf("expected ID %s, got %s", uid, fetchedByHandle.ID)
	}

	// 5. UpdateUserHandle
	if err := repo.UpdateUserHandle(ctx, uid, handle, newHandle); err != nil {
		t.Fatalf("UpdateUserHandle failed on live DynamoDB: %v", err)
	}

	coldRepo2 := NewRepositoryWithTable(db, tableName).(*DynamoRepository)
	updatedUser, err := coldRepo2.GetUserByID(ctx, uid)
	if err != nil || updatedUser == nil {
		t.Fatalf("GetUserByID after handle update failed: %v", err)
	}
	if updatedUser.Handle != newHandle {
		t.Errorf("expected handle %s, got %s", newHandle, updatedUser.Handle)
	}

	// Verify old handle cannot resolve anymore
	oldLookup, _ := coldRepo2.GetUserByHandle(ctx, handle)
	if oldLookup != nil && oldLookup.ID == uid && oldLookup.Handle == handle {
		t.Errorf("old handle %s should not resolve after update", handle)
	}

	// 6. IncrementTokenVersion (atomic counter ADD TokenVersion :inc)
	v1, err := repo.IncrementTokenVersion(ctx, uid)
	if err != nil {
		t.Fatalf("IncrementTokenVersion failed on live DynamoDB: %v", err)
	}
	if v1 <= 1 {
		t.Errorf("expected TokenVersion > 1, got %d", v1)
	}

	coldRepo3 := NewRepositoryWithTable(db, tableName).(*DynamoRepository)
	vRead, err := coldRepo3.GetTokenVersion(ctx, uid)
	if err != nil {
		t.Fatalf("GetTokenVersion failed: %v", err)
	}
	if vRead != v1 {
		t.Errorf("expected TokenVersion %d from live DynamoDB, got %d", v1, vRead)
	}
}

// TestDynamoRepository_Live_HandleCollision verifies that DynamoDB TransactWriteItems
// prevents two users from claiming the exact same handle.
func TestDynamoRepository_Live_HandleCollision(t *testing.T) {
	db, tableName := getTestAuthDynamoClient(t)

	repo := NewRepositoryWithTable(db, tableName).(*DynamoRepository)

	uid1 := "test-collision-1-" + uuid.NewString()
	uid2 := "test-collision-2-" + uuid.NewString()
	email1 := "c1-" + uuid.NewString() + "@example.com"
	email2 := "c2-" + uuid.NewString() + "@example.com"
	handle := "coll" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]

	cleanupAuthItems(t, db, tableName, uid1, handle)
	cleanupAuthItems(t, db, tableName, uid2)

	ctx := context.Background()

	user1 := &User{
		ID:        uid1,
		Email:     email1,
		Handle:    handle,
		Name:      "First Claimer",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := repo.CreateUser(ctx, user1); err != nil {
		t.Fatalf("User 1 failed to claim handle: %v", err)
	}

	// User 2 attempts to claim the exact same handle
	user2 := &User{
		ID:        uid2,
		Email:     email2,
		Handle:    handle,
		Name:      "Second Claimer",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	err := repo.CreateUser(ctx, user2)
	if err == nil {
		t.Fatalf("expected CreateUser to fail with conflict on duplicate handle, got nil")
	}
}
