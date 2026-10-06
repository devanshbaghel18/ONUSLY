package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
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

// SaveMessage stores two copies of the message (sender copy and recipient copy) atomically.
func (r *Repository) SaveMessage(ctx context.Context, msg realtime.StoredMessage, isRecipientOnline bool) error {
	senderEmail := strings.TrimSpace(strings.ToLower(msg.SenderEmail))
	recipientEmail := strings.TrimSpace(strings.ToLower(msg.RecipientEmail))
	if senderEmail == "" || recipientEmail == "" {
		return fmt.Errorf("sender and recipient emails are required")
	}

	senderItem := Message{
		PK:             "USER#" + senderEmail,
		SK:             fmt.Sprintf("CHAT#%s#%s#%s", recipientEmail, msg.Time, msg.ID),
		ID:             msg.ID,
		SenderEmail:    senderEmail,
		SenderID:       msg.SenderID,
		RecipientEmail: recipientEmail,
		Text:           msg.Text,
		Time:           msg.Time,
		Delivered:      true,
	}

	recipientItem := Message{
		PK:             "USER#" + recipientEmail,
		SK:             fmt.Sprintf("CHAT#%s#%s#%s", senderEmail, msg.Time, msg.ID),
		ID:             msg.ID,
		SenderEmail:    senderEmail,
		SenderID:       msg.SenderID,
		RecipientEmail: recipientEmail,
		Text:           msg.Text,
		Time:           msg.Time,
		Delivered:      isRecipientOnline,
	}

	sMap, err := attributevalue.MarshalMap(senderItem)
	if err != nil {
		return fmt.Errorf("marshal sender message: %w", err)
	}

	rMap, err := attributevalue.MarshalMap(recipientItem)
	if err != nil {
		return fmt.Errorf("marshal recipient message: %w", err)
	}

	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{TableName: aws.String(r.tableName), Item: sMap}},
			{Put: &types.Put{TableName: aws.String(r.tableName), Item: rMap}},
		},
	})
	if err != nil {
		return fmt.Errorf("transact write chat message: %w", err)
	}

	return nil
}

// GetPendingMessages fetches all undelivered messages for an offline recipient.
func (r *Repository) GetPendingMessages(ctx context.Context, recipientEmail string) ([]realtime.StoredMessage, error) {
	norm := strings.TrimSpace(strings.ToLower(recipientEmail))
	if norm == "" {
		return nil, nil
	}

	result, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		FilterExpression:       aws.String("#delivered = :false"),
		ExpressionAttributeNames: map[string]string{
			"#delivered": "Delivered",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: "USER#" + norm},
			":sk":    &types.AttributeValueMemberS{Value: "CHAT#"},
			":false": &types.AttributeValueMemberBOOL{Value: false},
		},
	})
	if err != nil {
		return nil, err
	}

	var msgs []Message
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &msgs); err != nil {
		return nil, err
	}

	out := make([]realtime.StoredMessage, len(msgs))
	for i, m := range msgs {
		out[i] = realtime.StoredMessage{
			ID:             m.ID,
			SenderEmail:    m.SenderEmail,
			SenderID:       m.SenderID,
			RecipientEmail: m.RecipientEmail,
			Text:           m.Text,
			Time:           m.Time,
			Delivered:      m.Delivered,
		}
	}

	return out, nil
}

// MarkMessagesDelivered marks a recipient's pending messages as delivered.
func (r *Repository) MarkMessagesDelivered(ctx context.Context, recipientEmail string, msgIDs []string) error {
	norm := strings.TrimSpace(strings.ToLower(recipientEmail))
	if norm == "" || len(msgIDs) == 0 {
		return nil
	}

	idSet := make(map[string]bool)
	for _, id := range msgIDs {
		idSet[id] = true
	}

	// Query recipient messages to match IDs with exact SKs
	result, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		FilterExpression:       aws.String("#delivered = :false"),
		ExpressionAttributeNames: map[string]string{
			"#delivered": "Delivered",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: "USER#" + norm},
			":sk":    &types.AttributeValueMemberS{Value: "CHAT#"},
			":false": &types.AttributeValueMemberBOOL{Value: false},
		},
	})
	if err != nil {
		return err
	}

	for _, item := range result.Items {
		var m Message
		if err := attributevalue.UnmarshalMap(item, &m); err != nil {
			continue
		}
		if idSet[m.ID] {
			_, _ = r.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName: aws.String(r.tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: m.PK},
					"SK": &types.AttributeValueMemberS{Value: m.SK},
				},
				UpdateExpression: aws.String("SET #delivered = :true"),
				ExpressionAttributeNames: map[string]string{
					"#delivered": "Delivered",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":true": &types.AttributeValueMemberBOOL{Value: true},
				},
			})
		}
	}

	return nil
}

// GetConversationHistory returns messages between userEmail and peerEmail in chronological order.
func (r *Repository) GetConversationHistory(ctx context.Context, userEmail, peerEmail string, limit int32) ([]Message, error) {
	normUser := strings.TrimSpace(strings.ToLower(userEmail))
	normPeer := strings.TrimSpace(strings.ToLower(peerEmail))
	if normUser == "" || normPeer == "" {
		return nil, nil
	}

	if limit <= 0 {
		limit = 100
	}

	result, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USER#" + normUser},
			":sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("CHAT#%s#", normPeer)},
		},
		ScanIndexForward: aws.Bool(true), // Ascending order by timestamp
		Limit:            aws.Int32(limit),
	})
	if err != nil {
		return nil, err
	}

	var msgs []Message
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &msgs); err != nil {
		return nil, err
	}

	return msgs, nil
}
