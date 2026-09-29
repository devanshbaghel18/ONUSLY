package proof

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
