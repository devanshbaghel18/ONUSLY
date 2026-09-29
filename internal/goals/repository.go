package goals

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Repository struct {
	db        *dynamodb.Client
	tableName string
}

func NewRepository(db *dynamodb.Client, tableName string) *Repository {
	return &Repository{
		db:        db,
		tableName: tableName,
	}
}

func (r *Repository) Create(ctx context.Context, goal Goal) error {
	item, err := attributevalue.MarshalMap(goal)
	if err != nil {
		return err
	}

	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})

	return err
}

func (r *Repository) GetByID(ctx context.Context, ownerID, goalID string) (*Goal, error) {
	result, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
			"SK": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
		},
	})

	if err != nil {
		return nil, err
	}

	if len(result.Item) == 0 {
		return nil, fmt.Errorf("goal not found")
	}

	var goal Goal

	if err := attributevalue.UnmarshalMap(result.Item, &goal); err != nil {
		return nil, err
	}

	return &goal, nil
}

func (r *Repository) List(ctx context.Context, ownerID string) ([]Goal, error) {
	result, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{
				Value: "USER#" + ownerID,
			},
			":sk": &types.AttributeValueMemberS{
				Value: "GOAL#",
			},
		},
	})

	if err != nil {
		return nil, err
	}

	var goals []Goal

	if err := attributevalue.UnmarshalListOfMaps(result.Items, &goals); err != nil {
		return nil, err
	}

	return goals, nil
}

func (r *Repository) Delete(ctx context.Context, ownerID, goalID string) error {
	_, err := r.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
			"SK": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
		},
	})

	return err
}

func (r *Repository) Update(
	ctx context.Context,
	ownerID string,
	goalID string,
	title string,
	description string,
) (*Goal, error) {

	result, err := r.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{
				Value: "USER#" + ownerID,
			},
			"SK": &types.AttributeValueMemberS{
				Value: "GOAL#" + goalID,
			},
		},
		UpdateExpression: aws.String(
			"SET #title = :title, #description = :description",
		),
		ExpressionAttributeNames: map[string]string{
			"#title":       "Title",
			"#description": "Description",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":title": &types.AttributeValueMemberS{
				Value: title,
			},
			":description": &types.AttributeValueMemberS{
				Value: description,
			},
		},
		ReturnValues: types.ReturnValueAllNew,
	})

	if err != nil {
		return nil, err
	}

	var goal Goal

	if err := attributevalue.UnmarshalMap(result.Attributes, &goal); err != nil {
		return nil, err
	}

	return &goal, nil
}
func (r *Repository) UpdateStatus(
	ctx context.Context,
	ownerID string,
	goalID string,
	status string,
) (*Goal, error) {

	result, err := r.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{
				Value: "USER#" + ownerID,
			},
			"SK": &types.AttributeValueMemberS{
				Value: "GOAL#" + goalID,
			},
		},
		UpdateExpression: aws.String(
			"SET #status = :status",
		),
		ExpressionAttributeNames: map[string]string{
			"#status": "Status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{
				Value: status,
			},
		},
		ReturnValues: types.ReturnValueAllNew,
	})

	if err != nil {
		return nil, err
	}

	var goal Goal

	if err := attributevalue.UnmarshalMap(result.Attributes, &goal); err != nil {
		return nil, err
	}

	return &goal, nil
}
