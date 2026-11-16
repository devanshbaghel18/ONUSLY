package friends

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

func getTestFriendsDynamoClient(t *testing.T) (*dynamodb.Client, string) {
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

func cleanupFriendsItems(t *testing.T, db *dynamodb.Client, tableName string, userIDs []string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for _, uid := range userIDs {
			// Query all items for this user partition and delete
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
	})
}

// TestFriends_Live_RequestAndMutualFriendship tests SaveFriendRequest, GetIncomingRequests,
// AcceptFriendRequest, GetFriends, and DeleteFriend on live DynamoDB.
func TestFriends_Live_RequestAndMutualFriendship(t *testing.T) {
	db, tableName := getTestFriendsDynamoClient(t)

	repo := NewRepository(db, tableName)

	userA := "user-a-" + uuid.NewString()
	userB := "user-b-" + uuid.NewString()
	reqID := uuid.NewString()

	cleanupFriendsItems(t, db, tableName, []string{userA, userB})

	ctx := context.Background()

	// 1. Send Friend Request from A to B
	req := FriendRequest{
		ID:              reqID,
		SenderID:        userA,
		SenderEmail:     "a@example.com",
		SenderHandle:    "usera",
		SenderName:      "User A",
		RecipientID:     userB,
		RecipientEmail:  "b@example.com",
		RecipientHandle: "userb",
		RecipientName:   "User B",
		Status:          "pending",
		CreatedAt:       time.Now().UTC(),
	}

	if err := repo.SaveFriendRequest(ctx, req); err != nil {
		t.Fatalf("SaveFriendRequest failed on live DynamoDB: %v", err)
	}

	// Read on cold instance
	coldRepo := NewRepository(db, tableName)

	// 2. Incoming requests for B
	incomingB, err := coldRepo.GetIncomingRequests(ctx, userB)
	if err != nil {
		t.Fatalf("GetIncomingRequests failed: %v", err)
	}
	if len(incomingB) != 1 || incomingB[0].ID != reqID {
		t.Fatalf("expected 1 incoming request %s for B, got %+v", reqID, incomingB)
	}

	// 3. Outgoing requests for A
	outgoingA, err := coldRepo.GetOutgoingRequests(ctx, userA)
	if err != nil {
		t.Fatalf("GetOutgoingRequests failed: %v", err)
	}
	if len(outgoingA) != 1 || outgoingA[0].ID != reqID {
		t.Fatalf("expected 1 outgoing request %s for A, got %+v", reqID, outgoingA)
	}

	// 4. Accept Friend Request
	if err := repo.AcceptFriendRequest(ctx, req); err != nil {
		t.Fatalf("AcceptFriendRequest failed on live DynamoDB: %v", err)
	}

	coldRepo2 := NewRepository(db, tableName)

	// 5. Verify mutual friendship
	friendsA, err := coldRepo2.GetFriends(ctx, userA)
	if err != nil {
		t.Fatalf("GetFriends for A failed: %v", err)
	}
	if len(friendsA) != 1 || friendsA[0].FriendID != userB {
		t.Fatalf("expected B to be friend of A, got %+v", friendsA)
	}

	friendsB, err := coldRepo2.GetFriends(ctx, userB)
	if err != nil {
		t.Fatalf("GetFriends for B failed: %v", err)
	}
	if len(friendsB) != 1 || friendsB[0].FriendID != userA {
		t.Fatalf("expected A to be friend of B, got %+v", friendsB)
	}

	// 6. Delete Friendship
	if err := repo.DeleteFriend(ctx, userA, userB); err != nil {
		t.Fatalf("DeleteFriend failed on live DynamoDB: %v", err)
	}

	coldRepo3 := NewRepository(db, tableName)
	afterDelA, _ := coldRepo3.GetFriends(ctx, userA)
	if len(afterDelA) != 0 {
		t.Errorf("expected 0 friends for A after delete, got %d", len(afterDelA))
	}
}

// TestFriends_Live_CommunityMembership tests JoinCommunity, GetJoinedCommunities,
// and LeaveCommunity on live DynamoDB.
func TestFriends_Live_CommunityMembership(t *testing.T) {
	db, tableName := getTestFriendsDynamoClient(t)

	repo := NewRepository(db, tableName)

	user := "user-comm-" + uuid.NewString()
	commID := "comm-" + uuid.NewString()

	cleanupFriendsItems(t, db, tableName, []string{user})

	ctx := context.Background()

	// 1. Join Community
	if err := repo.JoinCommunity(ctx, user, commID); err != nil {
		t.Fatalf("JoinCommunity failed: %v", err)
	}

	// 2. GetJoinedCommunities on cold instance
	coldRepo := NewRepository(db, tableName)
	joined, err := coldRepo.GetJoinedCommunities(ctx, user)
	if err != nil {
		t.Fatalf("GetJoinedCommunities failed: %v", err)
	}
	if len(joined) != 1 || joined[0] != commID {
		t.Fatalf("expected joined [%s], got %+v", commID, joined)
	}

	// 3. Leave Community
	if err := repo.LeaveCommunity(ctx, user, commID); err != nil {
		t.Fatalf("LeaveCommunity failed: %v", err)
	}

	coldRepo2 := NewRepository(db, tableName)
	afterLeave, err := coldRepo2.GetJoinedCommunities(ctx, user)
	if err != nil {
		t.Fatalf("GetJoinedCommunities after leave failed: %v", err)
	}
	if len(afterLeave) != 0 {
		t.Errorf("expected 0 communities after leave, got %d", len(afterLeave))
	}
}
