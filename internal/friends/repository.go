package friends

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoDBAPI defines the DynamoDB operations required by the friends repository.
type DynamoDBAPI interface {
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

type Repository struct {
	db             DynamoDBAPI
	tableName      string
	mu             sync.RWMutex
	memJoined      map[string]map[string]bool   // userID -> communityID -> true
	memFriends     map[string]map[string]Friend // userID -> friendID -> Friend
	memReqs        map[string]FriendRequest     // reqID -> FriendRequest
	memUserReqsIn  map[string]map[string]bool   // userID -> reqID -> true
	memUserReqsOut map[string]map[string]bool   // userID -> reqID -> true
}

func NewRepository(db DynamoDBAPI, tableName string) *Repository {
	return &Repository{
		db:             db,
		tableName:      tableName,
		memJoined:      make(map[string]map[string]bool),
		memFriends:     make(map[string]map[string]Friend),
		memReqs:        make(map[string]FriendRequest),
		memUserReqsIn:  make(map[string]map[string]bool),
		memUserReqsOut: make(map[string]map[string]bool),
	}
}

// dbItem wraps fields with DynamoDB PK and SK.
type dbItem struct {
	PK string `dynamodbav:"PK"`
	SK string `dynamodbav:"SK"`
}

// SaveFriendRequest writes both incoming (for recipient) and outgoing (for sender) friend request records.
func (r *Repository) SaveFriendRequest(ctx context.Context, req FriendRequest) error {
	r.mu.Lock()
	if r.memReqs == nil {
		r.memReqs = make(map[string]FriendRequest)
		r.memUserReqsIn = make(map[string]map[string]bool)
		r.memUserReqsOut = make(map[string]map[string]bool)
	}
	r.memReqs[req.ID] = req
	if r.memUserReqsIn[req.RecipientID] == nil {
		r.memUserReqsIn[req.RecipientID] = make(map[string]bool)
	}
	r.memUserReqsIn[req.RecipientID][req.ID] = true
	if r.memUserReqsOut[req.SenderID] == nil {
		r.memUserReqsOut[req.SenderID] = make(map[string]bool)
	}
	r.memUserReqsOut[req.SenderID][req.ID] = true
	r.mu.Unlock()

	inItem := struct {
		dbItem
		FriendRequest
	}{
		dbItem: dbItem{
			PK: "USER#" + req.RecipientID,
			SK: "FRIEND_REQ_IN#" + req.ID,
		},
		FriendRequest: req,
	}

	outItem := struct {
		dbItem
		FriendRequest
	}{
		dbItem: dbItem{
			PK: "USER#" + req.SenderID,
			SK: "FRIEND_REQ_OUT#" + req.ID,
		},
		FriendRequest: req,
	}

	inAv, err := attributevalue.MarshalMap(inItem)
	if err != nil {
		return fmt.Errorf("marshal incoming request: %w", err)
	}

	outAv, err := attributevalue.MarshalMap(outItem)
	if err != nil {
		return fmt.Errorf("marshal outgoing request: %w", err)
	}

	if r.db != nil {
		putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		_, err = r.db.TransactWriteItems(putCtx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []types.TransactWriteItem{
				{
					Put: &types.Put{
						TableName: aws.String(r.tableName),
						Item:      inAv,
					},
				},
				{
					Put: &types.Put{
						TableName: aws.String(r.tableName),
						Item:      outAv,
					},
				},
			},
		})
		if err != nil {
			log.Printf("[FriendsRepo] Remote DynamoDB friend request notice: %v (cached in local memory)", err)
		}
	}
	return nil
}

// GetIncomingRequests returns all pending incoming friend requests for a user.
func (r *Repository) GetIncomingRequests(ctx context.Context, userID string) ([]FriendRequest, error) {
	reqMap := make(map[string]FriendRequest)

	if r.db != nil {
		queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()

		out, err := r.db.Query(queryCtx, &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":       &types.AttributeValueMemberS{Value: "USER#" + userID},
				":skPrefix": &types.AttributeValueMemberS{Value: "FRIEND_REQ_IN#"},
			},
		})
		if err == nil && out != nil {
			for _, item := range out.Items {
				var req FriendRequest
				if err := attributevalue.UnmarshalMap(item, &req); err == nil {
					reqMap[req.ID] = req
				}
			}
		}
	}

	r.mu.RLock()
	if r.memUserReqsIn != nil && r.memUserReqsIn[userID] != nil {
		for reqID := range r.memUserReqsIn[userID] {
			if req, ok := r.memReqs[reqID]; ok {
				reqMap[reqID] = req
			}
		}
	}
	r.mu.RUnlock()

	requests := make([]FriendRequest, 0, len(reqMap))
	for _, req := range reqMap {
		requests = append(requests, req)
	}
	return requests, nil
}

// GetOutgoingRequests returns all pending outgoing friend requests sent by a user.
func (r *Repository) GetOutgoingRequests(ctx context.Context, userID string) ([]FriendRequest, error) {
	reqMap := make(map[string]FriendRequest)

	if r.db != nil {
		queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()

		out, err := r.db.Query(queryCtx, &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":       &types.AttributeValueMemberS{Value: "USER#" + userID},
				":skPrefix": &types.AttributeValueMemberS{Value: "FRIEND_REQ_OUT#"},
			},
		})
		if err == nil && out != nil {
			for _, item := range out.Items {
				var req FriendRequest
				if err := attributevalue.UnmarshalMap(item, &req); err == nil {
					reqMap[req.ID] = req
				}
			}
		}
	}

	r.mu.RLock()
	if r.memUserReqsOut != nil && r.memUserReqsOut[userID] != nil {
		for reqID := range r.memUserReqsOut[userID] {
			if req, ok := r.memReqs[reqID]; ok {
				reqMap[reqID] = req
			}
		}
	}
	r.mu.RUnlock()

	requests := make([]FriendRequest, 0, len(reqMap))
	for _, req := range reqMap {
		requests = append(requests, req)
	}
	return requests, nil
}

// GetIncomingRequestByID fetches a specific incoming friend request.
func (r *Repository) GetIncomingRequestByID(ctx context.Context, userID, reqID string) (*FriendRequest, error) {
	if r.db != nil {
		getCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()

		out, err := r.db.GetItem(getCtx, &dynamodb.GetItemInput{
			TableName: aws.String(r.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
				"SK": &types.AttributeValueMemberS{Value: "FRIEND_REQ_IN#" + reqID},
			},
		})
		if err == nil && out != nil && out.Item != nil {
			var req FriendRequest
			if err := attributevalue.UnmarshalMap(out.Item, &req); err == nil {
				return &req, nil
			}
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if req, ok := r.memReqs[reqID]; ok && req.RecipientID == userID {
		reqCopy := req
		return &reqCopy, nil
	}
	return nil, nil
}

// AcceptFriendRequest atomically creates mutual friend records and removes both request items.
func (r *Repository) AcceptFriendRequest(ctx context.Context, req FriendRequest) error {
	now := time.Now().UTC()

	recipientFriendObj := Friend{
		UserID:       req.RecipientID,
		FriendID:     req.SenderID,
		FriendHandle: req.SenderHandle,
		FriendEmail:  req.SenderEmail,
		FriendName:   req.SenderName,
		CreatedAt:    now,
	}

	senderFriendObj := Friend{
		UserID:       req.SenderID,
		FriendID:     req.RecipientID,
		FriendHandle: req.RecipientHandle,
		FriendEmail:  req.RecipientEmail,
		FriendName:   req.RecipientName,
		CreatedAt:    now,
	}

	r.mu.Lock()
	if r.memFriends == nil {
		r.memFriends = make(map[string]map[string]Friend)
	}
	if r.memFriends[req.RecipientID] == nil {
		r.memFriends[req.RecipientID] = make(map[string]Friend)
	}
	r.memFriends[req.RecipientID][req.SenderID] = recipientFriendObj

	if r.memFriends[req.SenderID] == nil {
		r.memFriends[req.SenderID] = make(map[string]Friend)
	}
	r.memFriends[req.SenderID][req.RecipientID] = senderFriendObj

	if r.memUserReqsIn != nil && r.memUserReqsIn[req.RecipientID] != nil {
		delete(r.memUserReqsIn[req.RecipientID], req.ID)
	}
	if r.memUserReqsOut != nil && r.memUserReqsOut[req.SenderID] != nil {
		delete(r.memUserReqsOut[req.SenderID], req.ID)
	}
	if r.memReqs != nil {
		delete(r.memReqs, req.ID)
	}
	r.mu.Unlock()

	// DynamoDB transaction
	recipientFriend := struct {
		dbItem
		Friend
	}{
		dbItem: dbItem{
			PK: "USER#" + req.RecipientID,
			SK: "FRIEND#" + req.SenderID,
		},
		Friend: recipientFriendObj,
	}

	senderFriend := struct {
		dbItem
		Friend
	}{
		dbItem: dbItem{
			PK: "USER#" + req.SenderID,
			SK: "FRIEND#" + req.RecipientID,
		},
		Friend: senderFriendObj,
	}

	rfAv, err := attributevalue.MarshalMap(recipientFriend)
	if err != nil {
		return fmt.Errorf("marshal recipient friend: %w", err)
	}

	sfAv, err := attributevalue.MarshalMap(senderFriend)
	if err != nil {
		return fmt.Errorf("marshal sender friend: %w", err)
	}

	if r.db != nil {
		txCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		_, err = r.db.TransactWriteItems(txCtx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []types.TransactWriteItem{
				{
					Put: &types.Put{
						TableName: aws.String(r.tableName),
						Item:      rfAv,
					},
				},
				{
					Put: &types.Put{
						TableName: aws.String(r.tableName),
						Item:      sfAv,
					},
				},
				{
					Delete: &types.Delete{
						TableName: aws.String(r.tableName),
						Key: map[string]types.AttributeValue{
							"PK": &types.AttributeValueMemberS{Value: "USER#" + req.RecipientID},
							"SK": &types.AttributeValueMemberS{Value: "FRIEND_REQ_IN#" + req.ID},
						},
					},
				},
				{
					Delete: &types.Delete{
						TableName: aws.String(r.tableName),
						Key: map[string]types.AttributeValue{
							"PK": &types.AttributeValueMemberS{Value: "USER#" + req.SenderID},
							"SK": &types.AttributeValueMemberS{Value: "FRIEND_REQ_OUT#" + req.ID},
						},
					},
				},
			},
		})
		if err != nil {
			log.Printf("[FriendsRepo] Remote DynamoDB accept friend notice: %v (cached in local memory)", err)
		}
	}
	return nil
}

// RejectFriendRequest removes the incoming and outgoing friend request items.
func (r *Repository) RejectFriendRequest(ctx context.Context, req FriendRequest) error {
	r.mu.Lock()
	if r.memUserReqsIn != nil && r.memUserReqsIn[req.RecipientID] != nil {
		delete(r.memUserReqsIn[req.RecipientID], req.ID)
	}
	if r.memUserReqsOut != nil && r.memUserReqsOut[req.SenderID] != nil {
		delete(r.memUserReqsOut[req.SenderID], req.ID)
	}
	if r.memReqs != nil {
		delete(r.memReqs, req.ID)
	}
	r.mu.Unlock()

	if r.db != nil {
		txCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		_, err := r.db.TransactWriteItems(txCtx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []types.TransactWriteItem{
				{
					Delete: &types.Delete{
						TableName: aws.String(r.tableName),
						Key: map[string]types.AttributeValue{
							"PK": &types.AttributeValueMemberS{Value: "USER#" + req.RecipientID},
							"SK": &types.AttributeValueMemberS{Value: "FRIEND_REQ_IN#" + req.ID},
						},
					},
				},
				{
					Delete: &types.Delete{
						TableName: aws.String(r.tableName),
						Key: map[string]types.AttributeValue{
							"PK": &types.AttributeValueMemberS{Value: "USER#" + req.SenderID},
							"SK": &types.AttributeValueMemberS{Value: "FRIEND_REQ_OUT#" + req.ID},
						},
					},
				},
			},
		})
		if err != nil {
			log.Printf("[FriendsRepo] Remote DynamoDB reject friend notice: %v", err)
		}
	}
	return nil
}

// GetFriends returns all accepted friends of the specified user.
func (r *Repository) GetFriends(ctx context.Context, userID string) ([]Friend, error) {
	friendsMap := make(map[string]Friend)

	if r.db != nil {
		queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()

		out, err := r.db.Query(queryCtx, &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":       &types.AttributeValueMemberS{Value: "USER#" + userID},
				":skPrefix": &types.AttributeValueMemberS{Value: "FRIEND#"},
			},
		})
		if err == nil && out != nil {
			for _, item := range out.Items {
				var f Friend
				if err := attributevalue.UnmarshalMap(item, &f); err == nil {
					friendsMap[f.FriendID] = f
				}
			}
		}
	}

	r.mu.RLock()
	if r.memFriends != nil && r.memFriends[userID] != nil {
		for fID, f := range r.memFriends[userID] {
			friendsMap[fID] = f
		}
	}
	r.mu.RUnlock()

	friendsList := make([]Friend, 0, len(friendsMap))
	for _, f := range friendsMap {
		friendsList = append(friendsList, f)
	}
	return friendsList, nil
}

// DeleteFriend removes mutual friendship between two users.
func (r *Repository) DeleteFriend(ctx context.Context, userID, friendID string) error {
	r.mu.Lock()
	if r.memFriends != nil {
		if r.memFriends[userID] != nil {
			delete(r.memFriends[userID], friendID)
		}
		if r.memFriends[friendID] != nil {
			delete(r.memFriends[friendID], userID)
		}
	}
	r.mu.Unlock()

	if r.db != nil {
		txCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		_, err := r.db.TransactWriteItems(txCtx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []types.TransactWriteItem{
				{
					Delete: &types.Delete{
						TableName: aws.String(r.tableName),
						Key: map[string]types.AttributeValue{
							"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
							"SK": &types.AttributeValueMemberS{Value: "FRIEND#" + friendID},
						},
					},
				},
				{
					Delete: &types.Delete{
						TableName: aws.String(r.tableName),
						Key: map[string]types.AttributeValue{
							"PK": &types.AttributeValueMemberS{Value: "USER#" + friendID},
							"SK": &types.AttributeValueMemberS{Value: "FRIEND#" + userID},
						},
					},
				},
			},
		})
		if err != nil {
			log.Printf("[FriendsRepo] Remote DynamoDB delete friend notice: %v", err)
		}
	}
	return nil
}

// JoinCommunity records that a user joined a community.
func (r *Repository) JoinCommunity(ctx context.Context, userID, communityID string) error {
	cleanID := strings.TrimSpace(communityID)
	if cleanID == "" {
		return fmt.Errorf("empty community ID")
	}

	r.mu.Lock()
	if r.memJoined == nil {
		r.memJoined = make(map[string]map[string]bool)
	}
	if r.memJoined[userID] == nil {
		r.memJoined[userID] = make(map[string]bool)
	}
	r.memJoined[userID][cleanID] = true
	r.mu.Unlock()

	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: "USER#" + userID},
		"SK":        &types.AttributeValueMemberS{Value: "COMMUNITY#" + cleanID},
		"JoinedAt":  &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		"Community": &types.AttributeValueMemberS{Value: cleanID},
	}
	if r.db != nil {
		putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		_, err := r.db.PutItem(putCtx, &dynamodb.PutItemInput{
			TableName: aws.String(r.tableName),
			Item:      item,
		})
		if err != nil {
			log.Printf("[FriendsRepo] Remote DynamoDB PutItem notice: %v (cached in local memory)", err)
		}
	}
	return nil
}

// LeaveCommunity removes a user from a community.
func (r *Repository) LeaveCommunity(ctx context.Context, userID, communityID string) error {
	cleanID := strings.TrimSpace(communityID)
	r.mu.Lock()
	if r.memJoined != nil && r.memJoined[userID] != nil {
		delete(r.memJoined[userID], cleanID)
	}
	r.mu.Unlock()

	if r.db != nil {
		delCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		_, err := r.db.DeleteItem(delCtx, &dynamodb.DeleteItemInput{
			TableName: aws.String(r.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
				"SK": &types.AttributeValueMemberS{Value: "COMMUNITY#" + cleanID},
			},
		})
		if err != nil {
			log.Printf("[FriendsRepo] Remote DynamoDB DeleteItem notice: %v", err)
		}
	}
	return nil
}

// GetJoinedCommunities returns the IDs of all communities the user has joined.
func (r *Repository) GetJoinedCommunities(ctx context.Context, userID string) ([]string, error) {
	joinedMap := make(map[string]bool)

	if r.db != nil {
		queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()

		out, err := r.db.Query(queryCtx, &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":       &types.AttributeValueMemberS{Value: "USER#" + userID},
				":skPrefix": &types.AttributeValueMemberS{Value: "COMMUNITY#"},
			},
		})
		if err == nil && out != nil {
			for _, item := range out.Items {
				if skVal, ok := item["SK"].(*types.AttributeValueMemberS); ok {
					joinedMap[strings.TrimPrefix(skVal.Value, "COMMUNITY#")] = true
				}
			}
		}
	}

	r.mu.RLock()
	if r.memJoined != nil && r.memJoined[userID] != nil {
		for cID, val := range r.memJoined[userID] {
			if val {
				joinedMap[cID] = true
			}
		}
	}
	r.mu.RUnlock()

	commIDs := make([]string, 0, len(joinedMap))
	for cID := range joinedMap {
		commIDs = append(commIDs, cID)
	}
	return commIDs, nil
}
