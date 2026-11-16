package chat

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

func getTestChatDynamoClient(t *testing.T) (*dynamodb.Client, string) {
	t.Helper()
	tableName := "Onusly"
	db := shared.NewDynamoClient()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		t.Skipf("DynamoDB table %q is not accessible (%v). Skipping live test.", tableName, err)
	}

	return db, tableName
}

func cleanupChatItems(t *testing.T, db *dynamodb.Client, tableName string, userIDs []string, communityIDs []string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for _, uid := range userIDs {
			out, err := db.Query(ctx, &dynamodb.QueryInput{
				TableName:              aws.String(tableName),
				KeyConditionExpression: aws.String("PK = :pk"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":pk": &types.AttributeValueMemberS{Value: "USER#" + uid},
				},
			})
			if err == nil && out != nil {
				for _, it := range out.Items {
					if sk, ok := it["SK"].(*types.AttributeValueMemberS); ok {
						_, _ = db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
							TableName: aws.String(tableName),
							Key: map[string]types.AttributeValue{
								"PK": &types.AttributeValueMemberS{Value: "USER#" + uid},
								"SK": sk,
							},
						})
					}
				}
			}
		}

		for _, cid := range communityIDs {
			out, err := db.Query(ctx, &dynamodb.QueryInput{
				TableName:              aws.String(tableName),
				KeyConditionExpression: aws.String("PK = :pk"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":pk": &types.AttributeValueMemberS{Value: "COMMUNITY#" + cid},
				},
			})
			if err == nil && out != nil {
				for _, it := range out.Items {
					if sk, ok := it["SK"].(*types.AttributeValueMemberS); ok {
						_, _ = db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
							TableName: aws.String(tableName),
							Key: map[string]types.AttributeValue{
								"PK": &types.AttributeValueMemberS{Value: "COMMUNITY#" + cid},
								"SK": sk,
							},
						})
					}
				}
			}
		}
	})
}

// TestChat_Live_OneOnOneMessageLifecycle tests atomic SaveMessage, GetConversationHistory,
// and ClearConversation on real AWS DynamoDB.
func TestChat_Live_OneOnOneMessageLifecycle(t *testing.T) {
	db, tableName := getTestChatDynamoClient(t)

	repo := NewRepository(db, tableName)

	userA := "chat-user-a-" + uuid.NewString()
	userB := "chat-user-b-" + uuid.NewString()
	msgID := uuid.NewString()

	cleanupChatItems(t, db, tableName, []string{userA, userB}, nil)

	ctx := context.Background()

	// 1. SaveMessage (A sends to B)
	now := time.Now().UTC().Format(time.RFC3339)
	msg := realtime.StoredMessage{
		ID:              msgID,
		SenderID:        userA,
		SenderEmail:     "a@example.com",
		SenderHandle:    "usera",
		RecipientID:     userB,
		RecipientEmail:  "b@example.com",
		RecipientHandle: "userb",
		Text:            "Hello from AWS DynamoDB test!",
		Time:            now,
	}

	if err := repo.SaveMessage(ctx, msg, true); err != nil {
		t.Fatalf("SaveMessage failed on live DynamoDB: %v", err)
	}

	// Read on cold repository instance
	coldRepo := NewRepository(db, tableName)

	// 2. Sender history (A viewing conversation with B)
	historyA, err := coldRepo.GetConversationHistory(ctx, userA, userB, 20)
	if err != nil {
		t.Fatalf("GetConversationHistory for A failed: %v", err)
	}
	if len(historyA) != 1 || historyA[0].ID != msgID {
		t.Fatalf("expected 1 message %s in A's history, got %+v", msgID, historyA)
	}
	if historyA[0].Text != msg.Text {
		t.Errorf("expected text %q, got %q", msg.Text, historyA[0].Text)
	}

	// 3. Recipient history (B viewing conversation with A)
	historyB, err := coldRepo.GetConversationHistory(ctx, userB, userA, 20)
	if err != nil {
		t.Fatalf("GetConversationHistory for B failed: %v", err)
	}
	if len(historyB) != 1 || historyB[0].ID != msgID {
		t.Fatalf("expected 1 message %s in B's history, got %+v", msgID, historyB)
	}

	// 4. ClearConversation for A
	if err := repo.ClearConversation(ctx, userA, userB); err != nil {
		t.Fatalf("ClearConversation failed: %v", err)
	}

	coldRepo2 := NewRepository(db, tableName)
	afterClearA, err := coldRepo2.GetConversationHistory(ctx, userA, userB, 20)
	if err != nil {
		t.Fatalf("GetConversationHistory after clear failed: %v", err)
	}
	if len(afterClearA) != 0 {
		t.Errorf("expected 0 messages for A after clear, got %d", len(afterClearA))
	}
}

// TestChat_Live_CommunityHistory tests SaveCommunityMessage and GetCommunityHistory on DynamoDB.
func TestChat_Live_CommunityHistory(t *testing.T) {
	db, tableName := getTestChatDynamoClient(t)

	repo := NewRepository(db, tableName)

	userA := "comm-user-a-" + uuid.NewString()
	commID := "comm-chat-" + uuid.NewString()
	msgID := uuid.NewString()

	cleanupChatItems(t, db, tableName, []string{userA}, []string{commID})

	ctx := context.Background()

	// 1. SaveCommunityMessage
	now := time.Now().UTC().Format(time.RFC3339)
	msg := realtime.StoredMessage{
		ID:           msgID,
		SenderID:     userA,
		SenderEmail:  "comm_user@example.com",
		SenderHandle: "commuser",
		Text:         "Community announcement on DynamoDB!",
		Time:         now,
	}

	if err := repo.SaveCommunityMessage(ctx, commID, msg); err != nil {
		t.Fatalf("SaveCommunityMessage failed: %v", err)
	}

	// 2. Read community history on cold instance
	coldRepo := NewRepository(db, tableName)
	history, err := coldRepo.GetCommunityHistory(ctx, commID, 20)
	if err != nil {
		t.Fatalf("GetCommunityHistory failed: %v", err)
	}
	if len(history) != 1 || history[0].ID != msgID {
		t.Fatalf("expected 1 message %s in community history, got %+v", msgID, history)
	}
	if history[0].Text != msg.Text {
		t.Errorf("expected text %q, got %q", msg.Text, history[0].Text)
	}
}
