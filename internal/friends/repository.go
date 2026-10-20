package friends

import (
	"context"
	"fmt"
	"strings"
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
	db        DynamoDBAPI
	tableName string
}

func NewRepository(db DynamoDBAPI, tableName string) *Repository {
	return &Repository{
		db:        db,
		tableName: tableName,
	}
}

// dbItem wraps fields with DynamoDB PK and SK.
type dbItem struct {
	PK string `dynamodbav:"PK"`
	SK string `dynamodbav:"SK"`
}

// SaveFriendRequest writes both incoming (for recipient) and outgoing (for sender) friend request records.
func (r *Repository) SaveFriendRequest(ctx context.Context, req FriendRequest) error {
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

	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
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
	return err
}

// GetIncomingRequests returns all pending incoming friend requests for a user.
func (r *Repository) GetIncomingRequests(ctx context.Context, userID string) ([]FriendRequest, error) {
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":       &types.AttributeValueMemberS{Value: "USER#" + userID},
			":skPrefix": &types.AttributeValueMemberS{Value: "FRIEND_REQ_IN#"},
		},
	})
	if err != nil {
		return nil, err
	}

	requests := make([]FriendRequest, 0)
	for _, item := range out.Items {
		var req FriendRequest
		if err := attributevalue.UnmarshalMap(item, &req); err == nil {
			requests = append(requests, req)
		}
	}
	return requests, nil
}

// GetOutgoingRequests returns all pending outgoing friend requests sent by a user.
func (r *Repository) GetOutgoingRequests(ctx context.Context, userID string) ([]FriendRequest, error) {
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":       &types.AttributeValueMemberS{Value: "USER#" + userID},
			":skPrefix": &types.AttributeValueMemberS{Value: "FRIEND_REQ_OUT#"},
		},
	})
	if err != nil {
		return nil, err
	}

	requests := make([]FriendRequest, 0)
	for _, item := range out.Items {
		var req FriendRequest
		if err := attributevalue.UnmarshalMap(item, &req); err == nil {
			requests = append(requests, req)
		}
	}
	return requests, nil
}

// GetIncomingRequestByID fetches a specific incoming friend request.
func (r *Repository) GetIncomingRequestByID(ctx context.Context, userID, reqID string) (*FriendRequest, error) {
	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
			"SK": &types.AttributeValueMemberS{Value: "FRIEND_REQ_IN#" + reqID},
		},
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, nil
	}

	var req FriendRequest
	if err := attributevalue.UnmarshalMap(out.Item, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// AcceptFriendRequest atomically creates mutual friend records and removes both request items.
func (r *Repository) AcceptFriendRequest(ctx context.Context, req FriendRequest) error {
	now := time.Now().UTC()

	// Recipient's view of Sender
	recipientFriend := struct {
		dbItem
		Friend
	}{
		dbItem: dbItem{
			PK: "USER#" + req.RecipientID,
			SK: "FRIEND#" + req.SenderID,
		},
		Friend: Friend{
			UserID:       req.RecipientID,
			FriendID:     req.SenderID,
			FriendHandle: req.SenderHandle,
			FriendEmail:  req.SenderEmail,
			FriendName:   req.SenderName,
			CreatedAt:    now,
		},
	}

	// Sender's view of Recipient
	senderFriend := struct {
		dbItem
		Friend
	}{
		dbItem: dbItem{
			PK: "USER#" + req.SenderID,
			SK: "FRIEND#" + req.RecipientID,
		},
		Friend: Friend{
			UserID:       req.SenderID,
			FriendID:     req.RecipientID,
			FriendHandle: req.RecipientHandle,
			FriendEmail:  req.RecipientEmail,
			FriendName:   req.RecipientName,
			CreatedAt:    now,
		},
	}

	rfAv, err := attributevalue.MarshalMap(recipientFriend)
	if err != nil {
		return fmt.Errorf("marshal recipient friend: %w", err)
	}

	sfAv, err := attributevalue.MarshalMap(senderFriend)
	if err != nil {
		return fmt.Errorf("marshal sender friend: %w", err)
	}

	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
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
	return err
}

// RejectFriendRequest removes the incoming and outgoing friend request items.
func (r *Repository) RejectFriendRequest(ctx context.Context, req FriendRequest) error {
	_, err := r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
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
	return err
}

// GetFriends returns all accepted friends of the specified user.
func (r *Repository) GetFriends(ctx context.Context, userID string) ([]Friend, error) {
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":       &types.AttributeValueMemberS{Value: "USER#" + userID},
			":skPrefix": &types.AttributeValueMemberS{Value: "FRIEND#"},
		},
	})
	if err != nil {
		return nil, err
	}

	friendsList := make([]Friend, 0)
	for _, item := range out.Items {
		var f Friend
		if err := attributevalue.UnmarshalMap(item, &f); err == nil {
			friendsList = append(friendsList, f)
		}
	}
	return friendsList, nil
}

// DeleteFriend removes mutual friendship between two users.
func (r *Repository) DeleteFriend(ctx context.Context, userID, friendID string) error {
	_, err := r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
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
	return err
}

// JoinCommunity records that a user joined a community.
func (r *Repository) JoinCommunity(ctx context.Context, userID, communityID string) error {
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: "USER#" + userID},
		"SK":        &types.AttributeValueMemberS{Value: "COMMUNITY#" + communityID},
		"JoinedAt":  &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		"Community": &types.AttributeValueMemberS{Value: communityID},
	}
	_, err := r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	return err
}

// LeaveCommunity removes a user from a community.
func (r *Repository) LeaveCommunity(ctx context.Context, userID, communityID string) error {
	_, err := r.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
			"SK": &types.AttributeValueMemberS{Value: "COMMUNITY#" + communityID},
		},
	})
	return err
}

// GetJoinedCommunities returns the IDs of all communities the user has joined.
func (r *Repository) GetJoinedCommunities(ctx context.Context, userID string) ([]string, error) {
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":       &types.AttributeValueMemberS{Value: "USER#" + userID},
			":skPrefix": &types.AttributeValueMemberS{Value: "COMMUNITY#"},
		},
	})
	if err != nil {
		return nil, err
	}

	commIDs := make([]string, 0)
	for _, item := range out.Items {
		if skVal, ok := item["SK"].(*types.AttributeValueMemberS); ok {
			commIDs = append(commIDs, strings.TrimPrefix(skVal.Value, "COMMUNITY#"))
		}
	}
	return commIDs, nil
}
