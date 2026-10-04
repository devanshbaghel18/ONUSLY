package proof

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
	return &Repository{db: db, tableName: tableName}
}

func (r *Repository) Create(ctx context.Context, p Proof) error {
	item, err := attributevalue.MarshalMap(p)
	if err != nil {
		return fmt.Errorf("marshal proof: %w", err)
	}

	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("put proof: %w", err)
	}

	return nil
}

// CreateWithGoalTransition atomically puts the proof and updates the goal status
// using a DynamoDB TransactWriteItems request conditioned on the goal currently
// having the expected status (e.g. "active").
func (r *Repository) CreateWithGoalTransition(
	ctx context.Context,
	p Proof,
	expectedGoalStatus string,
	newGoalStatus string,
) error {
	item, err := attributevalue.MarshalMap(p)
	if err != nil {
		return fmt.Errorf("marshal proof: %w", err)
	}

	now := p.SubmittedAt
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
						Value: "USER#" + p.OwnerID,
					},
					"SK": &types.AttributeValueMemberS{
						Value: "GOAL#" + p.GoalID,
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
			return ErrProofAlreadySubmitted
		}
		return fmt.Errorf("transact proof submission: %w", err)
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

func (r *Repository) GetByID(ctx context.Context, ownerID, goalID, proofID string) (*Proof, error) {
	pk := "USER#" + ownerID
	sk := "PROOF#" + goalID + "#" + proofID

	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &r.tableName,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get proof: %w", err)
	}
	if out.Item == nil {
		return nil, nil
	}

	var p Proof
	if err := attributevalue.UnmarshalMap(out.Item, &p); err != nil {
		return nil, fmt.Errorf("unmarshal proof: %w", err)
	}

	return &p, nil
}

func (r *Repository) ListByGoal(ctx context.Context, ownerID, goalID string) ([]Proof, error) {
	pk := "USER#" + ownerID
	skPrefix := "PROOF#" + goalID + "#"

	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              &r.tableName,
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
			":sk": &types.AttributeValueMemberS{Value: skPrefix},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query proofs: %w", err)
	}

	var proofs []Proof
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &proofs); err != nil {
		return nil, fmt.Errorf("unmarshal proofs: %w", err)
	}

	return proofs, nil
}
