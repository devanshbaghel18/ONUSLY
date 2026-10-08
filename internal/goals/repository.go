package goals

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

func (r *Repository) FindByID(ctx context.Context, goalID string) (*Goal, error) {
	var startKey map[string]types.AttributeValue
	for {
		scanInput := &dynamodb.ScanInput{
			TableName:        aws.String(r.tableName),
			FilterExpression: aws.String("ID = :id AND begins_with(SK, :sk)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":id": &types.AttributeValueMemberS{Value: goalID},
				":sk": &types.AttributeValueMemberS{Value: "GOAL#"},
			},
			ExclusiveStartKey: startKey,
		}

		result, err := r.db.Scan(ctx, scanInput)
		if err != nil {
			return nil, err
		}

		if len(result.Items) > 0 {
			var goal Goal
			if err := attributevalue.UnmarshalMap(result.Items[0], &goal); err != nil {
				return nil, err
			}
			return &goal, nil
		}

		if len(result.LastEvaluatedKey) == 0 {
			break
		}
		startKey = result.LastEvaluatedKey
	}

	return nil, fmt.Errorf("goal not found")
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
		ConditionExpression: aws.String(
			"attribute_not_exists(PK) OR #status = :completedStatus OR " +
				"((attribute_not_exists(#blockedApps) OR size(#blockedApps) = :zero) AND " +
				"(attribute_not_exists(#blockedDomains) OR size(#blockedDomains) = :zero))",
		),
		ExpressionAttributeNames: map[string]string{
			"#status":         "Status",
			"#blockedApps":    "BlockedApps",
			"#blockedDomains": "BlockedDomains",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":completedStatus": &types.AttributeValueMemberS{Value: "completed"},
			":zero":            &types.AttributeValueMemberN{Value: "0"},
		},
	})
	if err != nil {
		var condFailed *types.ConditionalCheckFailedException
		if errors.As(err, &condFailed) || strings.Contains(err.Error(), "ConditionalCheckFailed") {
			return ErrGoalLocked
		}
		return err
	}

	return nil
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

	if input.BlockedApps != nil {
		setParts = append(setParts, "#blockedApps = :blockedApps")
		names["#blockedApps"] = "BlockedApps"
		avApps, err := attributevalue.Marshal(*input.BlockedApps)
		if err != nil {
			return nil, err
		}
		values[":blockedApps"] = avApps
	}

	if input.BlockedDomains != nil {
		setParts = append(setParts, "#blockedDomains = :blockedDomains")
		names["#blockedDomains"] = "BlockedDomains"
		avDomains, err := attributevalue.Marshal(*input.BlockedDomains)
		if err != nil {
			return nil, err
		}
		values[":blockedDomains"] = avDomains
	}

	conditionParts := []string{"attribute_exists(PK)"}

	if input.DisallowNoneIfLocked {
		conditionParts = append(conditionParts, "(#status = :completedStatus OR ((attribute_not_exists(#blockedApps) OR size(#blockedApps) = :zero) AND (attribute_not_exists(#blockedDomains) OR size(#blockedDomains) = :zero)))")
		names["#status"] = "Status"
		names["#blockedApps"] = "BlockedApps"
		names["#blockedDomains"] = "BlockedDomains"
		values[":completedStatus"] = &types.AttributeValueMemberS{Value: "completed"}
		values[":zero"] = &types.AttributeValueMemberN{Value: "0"}
	}

	if input.CheckTargetsUnchanged {
		if len(input.ExpectedBlockedApps) > 0 {
			conditionParts = append(conditionParts, "#blockedApps = :expectedBlockedApps")
			names["#blockedApps"] = "BlockedApps"
			expApps, err := attributevalue.Marshal(input.ExpectedBlockedApps)
			if err != nil {
				return nil, err
			}
			values[":expectedBlockedApps"] = expApps
		} else {
			conditionParts = append(conditionParts, "(attribute_not_exists(#blockedApps) OR size(#blockedApps) = :zero)")
			names["#blockedApps"] = "BlockedApps"
			values[":zero"] = &types.AttributeValueMemberN{Value: "0"}
		}

		if len(input.ExpectedBlockedDomains) > 0 {
			conditionParts = append(conditionParts, "#blockedDomains = :expectedBlockedDomains")
			names["#blockedDomains"] = "BlockedDomains"
			expDomains, err := attributevalue.Marshal(input.ExpectedBlockedDomains)
			if err != nil {
				return nil, err
			}
			values[":expectedBlockedDomains"] = expDomains
		} else {
			conditionParts = append(conditionParts, "(attribute_not_exists(#blockedDomains) OR size(#blockedDomains) = :zero)")
			names["#blockedDomains"] = "BlockedDomains"
			values[":zero"] = &types.AttributeValueMemberN{Value: "0"}
		}
	}

	updateExpression := "SET " + strings.Join(setParts, ", ")
	conditionExpression := strings.Join(conditionParts, " AND ")

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
		ConditionExpression:       aws.String(conditionExpression),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
		ReturnValues:              types.ReturnValueAllNew,
	})

	if err != nil {
		var condFailed *types.ConditionalCheckFailedException
		if errors.As(err, &condFailed) || strings.Contains(err.Error(), "ConditionalCheckFailed") {
			if input.DisallowNoneIfLocked {
				return nil, ErrAccountabilityLocked
			}
			if input.CheckTargetsUnchanged {
				return nil, ErrTargetsLocked
			}
			return nil, ErrGoalLocked
		}
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
