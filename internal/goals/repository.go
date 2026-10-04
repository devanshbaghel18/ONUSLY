package goals

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
	input UpdateGoalInput,
) (*Goal, error) {

	var setParts []string
	names := map[string]string{}
	values := map[string]types.AttributeValue{}

	updatedAt := input.UpdatedAt
	if updatedAt == "" {
		updatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	setParts = append(setParts, "#updatedAt = :updatedAt")
	names["#updatedAt"] = "UpdatedAt"
	values[":updatedAt"] = &types.AttributeValueMemberS{Value: updatedAt}

	if input.Title != nil {
		setParts = append(setParts, "#title = :title")
		names["#title"] = "Title"
		values[":title"] = &types.AttributeValueMemberS{Value: *input.Title}
	}

	if input.Description != nil {
		setParts = append(setParts, "#description = :description")
		names["#description"] = "Description"
		values[":description"] = &types.AttributeValueMemberS{Value: *input.Description}
	}

	if input.ApprovalType != nil {
		setParts = append(setParts, "#approvalType = :approvalType")
		names["#approvalType"] = "ApprovalType"
		values[":approvalType"] = &types.AttributeValueMemberS{Value: *input.ApprovalType}
	}

	if input.ApproverEmail != nil {
		setParts = append(setParts, "#approverEmail = :approverEmail")
		names["#approverEmail"] = "ApproverEmail"
		values[":approverEmail"] = &types.AttributeValueMemberS{Value: *input.ApproverEmail}
	}

	if input.ApproverID != nil {
		setParts = append(setParts, "#approverId = :approverId")
		names["#approverId"] = "ApproverID"
		values[":approverId"] = &types.AttributeValueMemberS{Value: *input.ApproverID}
	}

	updateExpression := "SET " + strings.Join(setParts, ", ")

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
		UpdateExpression:          aws.String(updateExpression),
		ConditionExpression:       aws.String("attribute_exists(PK)"),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
		ReturnValues:              types.ReturnValueAllNew,
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
