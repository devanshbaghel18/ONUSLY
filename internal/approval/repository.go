package approval

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
