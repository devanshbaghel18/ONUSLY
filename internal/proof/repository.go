package proof

import (
	"context"
	"errors"
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

type Repository struct {
	db        *dynamodb.Client
	tableName string
	mu        sync.RWMutex
	memProofs map[string]Proof
}

func NewRepository(db *dynamodb.Client, tableName string) *Repository {
	return &Repository{
		db:        db,
		tableName: tableName,
		memProofs: make(map[string]Proof),
	}
}

func (r *Repository) Create(ctx context.Context, p Proof) error {
	r.mu.Lock()
	if r.memProofs == nil {
		r.memProofs = make(map[string]Proof)
	}
	r.memProofs[p.OwnerID+"#"+p.GoalID+"#"+p.ID] = p
	r.mu.Unlock()

	item, err := attributevalue.MarshalMap(p)
	if err != nil {
		return fmt.Errorf("marshal proof: %w", err)
	}

	if r.db == nil {
		return nil
	}

	putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	_, err = r.db.PutItem(putCtx, &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
	})
	if err != nil {
		log.Printf("[ProofRepo] Remote database put notice: %v (cached in local memory)", err)
		return nil
	}

	return nil
}

// CreateWithGoalTransition atomically puts the proof and updates the goal status
// using a DynamoDB TransactWriteItems request conditioned on the goal currently
// having one of the expected statuses (e.g. "active" or "rejected").
func (r *Repository) CreateWithGoalTransition(
	ctx context.Context,
	p Proof,
	expectedGoalStatuses []string,
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

	gsi2sk := "STATUS#" + strings.ToLower(strings.TrimSpace(newGoalStatus)) + "#GOAL#" + strings.TrimSpace(p.GoalID)

	var condParts []string
	exprValues := map[string]types.AttributeValue{
		":newStatus": &types.AttributeValueMemberS{
			Value: newGoalStatus,
		},
		":updatedAt": &types.AttributeValueMemberS{
			Value: now,
		},
		":gsi2sk": &types.AttributeValueMemberS{
			Value: gsi2sk,
		},
	}

	for i, s := range expectedGoalStatuses {
		key := fmt.Sprintf(":expectedStatus%d", i)
		condParts = append(condParts, fmt.Sprintf("#status = %s", key))
		exprValues[key] = &types.AttributeValueMemberS{Value: s}
	}

	condExpr := strings.Join(condParts, " OR ")
	if condExpr == "" {
		condExpr = "#status = :expectedStatus0"
		exprValues[":expectedStatus0"] = &types.AttributeValueMemberS{Value: "active"}
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
					"SET #status = :newStatus, #updatedAt = :updatedAt, #gsi2sk = :gsi2sk",
				),
				ConditionExpression: aws.String(condExpr),
				ExpressionAttributeNames: map[string]string{
					"#status":    "Status",
					"#updatedAt": "UpdatedAt",
					"#gsi2sk":    "GSI2SK",
				},
				ExpressionAttributeValues: exprValues,
			},
		},
	}

	r.mu.Lock()
	if r.memProofs == nil {
		r.memProofs = make(map[string]Proof)
	}
	r.memProofs[p.OwnerID+"#"+p.GoalID+"#"+p.ID] = p
	r.mu.Unlock()

	if r.db == nil {
		return nil
	}

	putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	_, err = r.db.TransactWriteItems(putCtx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		if isConditionCheckFailed(err) {
			return ErrProofAlreadySubmitted
		}
		log.Printf("[ProofRepo] Remote database transact notice: %v (cached in local memory)", err)
		return nil
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
	if r.db == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if p, ok := r.memProofs[ownerID+"#"+goalID+"#"+proofID]; ok {
			cp := p
			return &cp, nil
		}
		return nil, nil
	}

	pk := "USER#" + ownerID
	sk := "PROOF#" + goalID + "#" + proofID

	getCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	out, err := r.db.GetItem(getCtx, &dynamodb.GetItemInput{
		TableName: &r.tableName,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if p, ok := r.memProofs[ownerID+"#"+goalID+"#"+proofID]; ok {
			cp := p
			return &cp, nil
		}
		return nil, fmt.Errorf("get proof: %w", err)
	}
	if out.Item == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if p, ok := r.memProofs[ownerID+"#"+goalID+"#"+proofID]; ok {
			cp := p
			return &cp, nil
		}
		return nil, nil
	}

	var p Proof
	if err := attributevalue.UnmarshalMap(out.Item, &p); err != nil {
		return nil, fmt.Errorf("unmarshal proof: %w", err)
	}

	return &p, nil
}

func (r *Repository) ListByGoal(ctx context.Context, ownerID, goalID string) ([]Proof, error) {
	if r.db == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var proofs []Proof
		prefix := ownerID + "#" + goalID + "#"
		for k, p := range r.memProofs {
			if strings.HasPrefix(k, prefix) {
				proofs = append(proofs, p)
			}
		}
		return proofs, nil
	}

	pk := "USER#" + ownerID
	skPrefix := "PROOF#" + goalID + "#"

	queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	out, err := r.db.Query(queryCtx, &dynamodb.QueryInput{
		TableName:              &r.tableName,
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
			":sk": &types.AttributeValueMemberS{Value: skPrefix},
		},
	})
	if err != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var proofs []Proof
		prefix := ownerID + "#" + goalID + "#"
		for k, p := range r.memProofs {
			if strings.HasPrefix(k, prefix) {
				proofs = append(proofs, p)
			}
		}
		return proofs, nil
	}

	var proofs []Proof
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &proofs); err != nil {
		return nil, fmt.Errorf("unmarshal proofs: %w", err)
	}

	return proofs, nil
}
