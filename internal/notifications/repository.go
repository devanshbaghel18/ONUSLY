package notifications

import (
	"context"
	"errors"
	"sort"
	"strings"

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

// Create inserts a new notification record with a conditional check to ensure idempotency.
// If the notification already exists (by PK and SK), returns ErrAlreadyExists.
func (r *Repository) Create(ctx context.Context, notif *Notification) error {
	if notif == nil || strings.TrimSpace(notif.RecipientID) == "" || strings.TrimSpace(notif.ID) == "" {
		return errors.New("invalid notification payload")
	}

	notif.PK = "USER#" + notif.RecipientID
	notif.SK = "NOTIF#" + notif.ID

	item, err := attributevalue.MarshalMap(notif)
	if err != nil {
		return err
	}

	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) || strings.Contains(err.Error(), "ConditionalCheckFailed") {
			return ErrAlreadyExists
		}
		return err
	}

	return nil
}

// List returns the notifications for a recipient, sorted with the newest first.
func (r *Repository) List(ctx context.Context, recipientID string, limit int) ([]Notification, error) {
	if strings.TrimSpace(recipientID) == "" {
		return []Notification{}, nil
	}

	pk := "USER#" + recipientID
	prefix := "NOTIF#"

	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName: aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: pk},
			":prefix": &types.AttributeValueMemberS{Value: prefix},
		},
	})
	if err != nil {
		return nil, err
	}

	var notifs []Notification
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &notifs); err != nil {
		return nil, err
	}

	// Sort descending by CreatedAt (newest first)
	sort.Slice(notifs, func(i, j int) bool {
		return notifs[i].CreatedAt > notifs[j].CreatedAt
	})

	if limit > 0 && len(notifs) > limit {
		notifs = notifs[:limit]
	}

	return notifs, nil
}

// GetByID retrieves a single notification by recipient and ID.
func (r *Repository) GetByID(ctx context.Context, recipientID, notifID string) (*Notification, error) {
	pk := "USER#" + strings.TrimSpace(recipientID)
	sk := "NOTIF#" + strings.TrimSpace(notifID)

	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		return nil, err
	}

	if out.Item == nil {
		return nil, ErrNotificationNotFound
	}

	var notif Notification
	if err := attributevalue.UnmarshalMap(out.Item, &notif); err != nil {
		return nil, err
	}

	return &notif, nil
}

// MarkAsRead marks a specific notification as read for the authenticated recipient.
func (r *Repository) MarkAsRead(ctx context.Context, recipientID, notifID string) error {
	pk := "USER#" + strings.TrimSpace(recipientID)
	sk := "NOTIF#" + strings.TrimSpace(notifID)

	_, err := r.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression: aws.String("SET #r = :true"),
		ExpressionAttributeNames: map[string]string{
			"#r": "read",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true": &types.AttributeValueMemberBOOL{Value: true},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) || strings.Contains(err.Error(), "ConditionalCheckFailed") {
			return ErrNotificationNotFound
		}
		return err
	}

	return nil
}

// MarkAllAsRead marks all unread notifications as read for a given recipient.
func (r *Repository) MarkAllAsRead(ctx context.Context, recipientID string) error {
	notifs, err := r.List(ctx, recipientID, 100)
	if err != nil {
		return err
	}

	for _, n := range notifs {
		if !n.Read {
			_ = r.MarkAsRead(ctx, recipientID, n.ID)
		}
	}

	return nil
}
