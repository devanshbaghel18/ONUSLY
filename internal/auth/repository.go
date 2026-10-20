package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

const tableName = "Onusly"

type DynamoRepository struct {
	client *dynamodb.Client
}

func NewRepository() Repository {
	return &DynamoRepository{
		client: shared.NewDynamoClient(),
	}
}

func isConflictError(err error) bool {
	if err == nil {
		return false
	}
	var tce *types.TransactionCanceledException
	if errors.As(err, &tce) {
		for _, reason := range tce.CancellationReasons {
			if reason.Code != nil {
				code := *reason.Code
				if code == "ConditionalCheckFailed" || code == "TransactionConflict" {
					return true
				}
			}
		}
	}
	var tcfe *types.TransactionConflictException
	if errors.As(err, &tcfe) {
		return true
	}
	var ccfe *types.ConditionalCheckFailedException
	if errors.As(err, &ccfe) {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "ConditionalCheckFailed") ||
		strings.Contains(errStr, "TransactionConflict") ||
		strings.Contains(errStr, "TransactionCanceledException")
}

func (r *DynamoRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{
				Value: "EMAIL#" + strings.ToLower(strings.TrimSpace(email)),
			},
		},
		Limit: aws.Int32(1),
	})

	if err != nil {
		return nil, err
	}

	if len(out.Items) == 0 {
		return nil, nil
	}

	var user User
	if err := attributevalue.UnmarshalMap(out.Items[0], &user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *DynamoRepository) GetUserByID(ctx context.Context, userID string) (*User, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	var user User
	if err := attributevalue.UnmarshalMap(out.Item, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *DynamoRepository) GetUserByHandle(ctx context.Context, handle string) (*User, error) {
	cleaned := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(handle)), "@")
	if cleaned == "" {
		return nil, nil
	}

	// 1. Direct O(1) lookup on HANDLE#<clean> reservation item
	resOut, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "HANDLE#" + cleaned},
			"SK": &types.AttributeValueMemberS{Value: "RESERVATION"},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resOut.Item) == 0 {
		return nil, nil
	}

	var reservation HandleReservation
	if err := attributevalue.UnmarshalMap(resOut.Item, &reservation); err != nil {
		return nil, err
	}
	if reservation.UserID == "" {
		return nil, nil
	}

	return r.GetUserByID(ctx, reservation.UserID)
}

func (r *DynamoRepository) CreateUser(ctx context.Context, user *User) error {
	user.PK = "USER#" + user.ID
	user.SK = "PROFILE"
	user.GSI1PK = "EMAIL#" + strings.ToLower(strings.TrimSpace(user.Email))
	user.GSI1SK = "PROFILE"

	item, err := attributevalue.MarshalMap(user)
	if err != nil {
		return err
	}

	cleanHandle := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(user.Handle)), "@")
	if cleanHandle != "" {
		handleItem := map[string]types.AttributeValue{
			"PK":        &types.AttributeValueMemberS{Value: "HANDLE#" + cleanHandle},
			"SK":        &types.AttributeValueMemberS{Value: "RESERVATION"},
			"UserID":    &types.AttributeValueMemberS{Value: user.ID},
			"Handle":    &types.AttributeValueMemberS{Value: cleanHandle},
			"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		}

		_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []types.TransactWriteItem{
				{
					Put: &types.Put{
						TableName:           aws.String(tableName),
						Item:                item,
						ConditionExpression: aws.String("attribute_not_exists(PK)"),
					},
				},
				{
					Put: &types.Put{
						TableName:           aws.String(tableName),
						Item:                handleItem,
						ConditionExpression: aws.String("attribute_not_exists(PK)"),
					},
				},
			},
		})
		if err != nil {
			if isConflictError(err) {
				return errors.New("this handle is already taken, please choose another")
			}
			return fmt.Errorf("failed creating user: %w", err)
		}
		return nil
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed creating user: %w", err)
	}

	return nil
}

func (r *DynamoRepository) UpdateUserHandle(ctx context.Context, userID, oldHandle, newHandle string) error {
	cleanNew := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(newHandle)), "@")
	if cleanNew == "" {
		return fmt.Errorf("handle cannot be empty")
	}

	cleanOld := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(oldHandle)), "@")
	if cleanOld == cleanNew {
		// Idempotent: caller already holds this handle
		return nil
	}

	handleItem := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: "HANDLE#" + cleanNew},
		"SK":        &types.AttributeValueMemberS{Value: "RESERVATION"},
		"UserID":    &types.AttributeValueMemberS{Value: userID},
		"Handle":    &types.AttributeValueMemberS{Value: cleanNew},
		"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
	}

	transactItems := []types.TransactWriteItem{
		{
			Put: &types.Put{
				TableName:           aws.String(tableName),
				Item:                handleItem,
				ConditionExpression: aws.String("attribute_not_exists(PK) OR UserID = :uid"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":uid": &types.AttributeValueMemberS{Value: userID},
				},
			},
		},
		{
			Update: &types.Update{
				TableName: aws.String(tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
					"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
				},
				UpdateExpression: aws.String("SET #handle = :handle"),
				ExpressionAttributeNames: map[string]string{
					"#handle": "Handle",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":handle": &types.AttributeValueMemberS{Value: cleanNew},
				},
			},
		},
	}

	if cleanOld != "" && cleanOld != cleanNew {
		transactItems = append(transactItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "HANDLE#" + cleanOld},
					"SK": &types.AttributeValueMemberS{Value: "RESERVATION"},
				},
				ConditionExpression: aws.String("attribute_not_exists(PK) OR UserID = :uid"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":uid": &types.AttributeValueMemberS{Value: userID},
				},
			},
		})
	}

	_, err := r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		if isConflictError(err) {
			return errors.New("this handle is already taken, please choose another")
		}
		return fmt.Errorf("failed updating handle: %w", err)
	}

	return nil
}
