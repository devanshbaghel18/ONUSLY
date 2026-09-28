package auth

import (
	"context"
	"fmt"

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
		TableName:              &[]string{tableName}[0],
		IndexName:              &[]string{"GSI1"}[0],
		KeyConditionExpression: &[]string{"GSI1PK = :pk"}[0],
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{
				Value: "EMAIL#" + email,
			},
		},
		Limit: &[]int32{1}[0],
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

func (r *DynamoRepository) CreateUser(ctx context.Context, user *User) error {

	user.PK = "USER#" + user.ID
	user.SK = "PROFILE"
	user.GSI1PK = "EMAIL#" + user.Email
	user.GSI1SK = "PROFILE"

	item, err := attributevalue.MarshalMap(user)
	if err != nil {
		return err
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &[]string{tableName}[0],
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("failed creating user: %w", err)
	}

	return nil
}