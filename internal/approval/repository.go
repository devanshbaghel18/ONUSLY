package approval

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

func (r *Repository) Create(ctx context.Context, approval Approval) error {
	item, err := attributevalue.MarshalMap(approval)
	if err != nil {
		return fmt.Errorf("marshal approval: %w", err)
	}

	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("put approval: %w", err)
	}

	return nil
}

// CreateDecision atomically puts the approval record and updates the goal status
// using a DynamoDB TransactWriteItems request conditioned on the goal currently
// having the expected status (e.g. "proof_submitted").
func (r *Repository) CreateDecision(
	ctx context.Context,
	approval Approval,
	expectedGoalStatus string,
	newGoalStatus string,
) error {
	item, err := attributevalue.MarshalMap(approval)
	if err != nil {
		return fmt.Errorf("marshal approval: %w", err)
	}

	now := approval.DecidedAt
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}

	transactItems := []types.TransactWriteItem{
		{
			Put: &types.Put{
				TableName:           aws.String(r.tableName),
				Item:                item,
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			},
		},
		{
			Update: &types.Update{
				TableName: aws.String(r.tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{
						Value: "USER#" + approval.OwnerID,
					},
					"SK": &types.AttributeValueMemberS{
						Value: "GOAL#" + approval.GoalID,
					},
				},
				UpdateExpression: aws.String(
					"SET #status = :newStatus, #updatedAt = :updatedAt",
				),
				ConditionExpression: aws.String(
					"#status = :expectedStatus",
				),
				ExpressionAttributeNames: map[string]string{
					"#status":    "Status",
					"#updatedAt": "UpdatedAt",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":newStatus": &types.AttributeValueMemberS{
						Value: newGoalStatus,
					},
					":expectedStatus": &types.AttributeValueMemberS{
						Value: expectedGoalStatus,
					},
					":updatedAt": &types.AttributeValueMemberS{
						Value: now,
					},
				},
			},
		},
	}

	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		if isConditionCheckFailed(err) {
			return ErrAlreadyDecided
		}
		return fmt.Errorf("transact approval decision: %w", err)
	}

	return nil
}

func isConditionCheckFailed(err error) bool {
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
	if strings.Contains(errStr, "ConditionalCheckFailed") ||
		strings.Contains(errStr, "TransactionConflict") ||
		strings.Contains(errStr, "TransactionCanceledException") {
		return true
	}

	return false
}

func (r *Repository) GetByID(
	ctx context.Context,
	ownerID string,
	goalID string,
	approvalID string,
) (*Approval, error) {

	pk := "USER#" + ownerID
	sk := "APPROVAL#" + goalID + "#" + approvalID

	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get approval: %w", err)
	}

	if len(out.Item) == 0 {
		return nil, nil
	}

	var approval Approval

	if err := attributevalue.UnmarshalMap(out.Item, &approval); err != nil {
		return nil, fmt.Errorf("unmarshal approval: %w", err)
	}

	return &approval, nil
}

func (r *Repository) ListByGoal(
	ctx context.Context,
	ownerID string,
	goalID string,
) ([]Approval, error) {

	pk := "USER#" + ownerID
	skPrefix := "APPROVAL#" + goalID + "#"

	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName: aws.String(r.tableName),
		KeyConditionExpression: aws.String(
			"PK = :pk AND begins_with(SK, :sk)",
		),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{
				Value: pk,
			},
			":sk": &types.AttributeValueMemberS{
				Value: skPrefix,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query approvals: %w", err)
	}

	var approvals []Approval

	if err := attributevalue.UnmarshalListOfMaps(out.Items, &approvals); err != nil {
		return nil, fmt.Errorf("unmarshal approvals: %w", err)
	}

	return approvals, nil
}
