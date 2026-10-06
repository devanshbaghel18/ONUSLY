package auth

import (
	"context"
	"fmt"
	"strings"

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

	withAt := "@" + cleaned

	var startKey map[string]types.AttributeValue
	for {
		out, err := r.client.Scan(ctx, &dynamodb.ScanInput{
			TableName:        aws.String(tableName),
			FilterExpression: aws.String("SK = :sk AND (#handle = :h1 OR #handle = :h2)"),
			ExpressionAttributeNames: map[string]string{
				"#handle": "Handle",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":sk": &types.AttributeValueMemberS{Value: "PROFILE"},
				":h1": &types.AttributeValueMemberS{Value: cleaned},
				":h2": &types.AttributeValueMemberS{Value: withAt},
			},
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, err
		}

		if len(out.Items) > 0 {
			var user User
			if err := attributevalue.UnmarshalMap(out.Items[0], &user); err != nil {
				return nil, err
			}
			return &user, nil
		}

		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		startKey = out.LastEvaluatedKey
	}

	return nil, nil
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

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed creating user: %w", err)
	}

	return nil
}

func (r *DynamoRepository) UpdateUserHandle(ctx context.Context, userID, handle string) error {
	clean := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(handle)), "@")
	if clean == "" {
		return fmt.Errorf("handle cannot be empty")
	}

	_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
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
			":handle": &types.AttributeValueMemberS{Value: clean},
		},
	})
	return err
}