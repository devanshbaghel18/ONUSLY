package chat

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
)

// DynamoDBAPI defines the DynamoDB operations required by the chat repository.
type DynamoDBAPI interface {
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

type Repository struct {
	db        DynamoDBAPI
	tableName string
}

func NewRepository(db DynamoDBAPI, tableName string) *Repository {
	return &Repository{
		db:        db,
		tableName: tableName,
	}
}

func normalizeStorageKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if strings.Contains(trimmed, "@") {
		return strings.ToLower(trimmed)
	}
	return trimmed
}

// SaveMessage stores two copies of the message (sender copy and recipient copy) atomically
// using immutable authenticated user IDs for partition and sort keys.
func (r *Repository) SaveMessage(ctx context.Context, msg realtime.StoredMessage, isRecipientOnline bool) error {
	senderKey := strings.TrimSpace(msg.SenderID)
	if senderKey == "" {
		senderKey = strings.TrimSpace(strings.ToLower(msg.SenderEmail))
		if senderKey == "" && msg.SenderHandle != "" {
			senderKey = "@" + strings.TrimPrefix(strings.TrimSpace(strings.ToLower(msg.SenderHandle)), "@")
		}
	}

	recipientKey := strings.TrimSpace(msg.RecipientID)
	if recipientKey == "" {
		recipientKey = strings.TrimSpace(strings.ToLower(msg.RecipientEmail))
		if recipientKey == "" && msg.RecipientHandle != "" {
			recipientKey = "@" + strings.TrimPrefix(strings.TrimSpace(strings.ToLower(msg.RecipientHandle)), "@")
		}
	}

	if senderKey == "" || recipientKey == "" {
		return fmt.Errorf("sender and recipient identifiers are required")
	}

	senderItem := Message{
		PK:              "USER#" + senderKey,
		SK:              fmt.Sprintf("CHAT#%s#%s#%s", recipientKey, msg.Time, msg.ID),
		ID:              msg.ID,
		SenderEmail:     msg.SenderEmail,
		SenderHandle:    msg.SenderHandle,
		SenderID:        msg.SenderID,
		RecipientEmail:  msg.RecipientEmail,
		RecipientHandle: msg.RecipientHandle,
		RecipientID:     msg.RecipientID,
		Text:            msg.Text,
		IsProof:         msg.IsProof,
		GoalID:          msg.GoalID,
		GoalTitle:       msg.GoalTitle,
		OwnerID:         msg.OwnerID,
		Images:          msg.Images,
		ExternalLink:    msg.ExternalLink,
		Approved:        msg.Approved,
		Time:            msg.Time,
		Delivered:       true,
	}

	recipientItem := Message{
		PK:              "USER#" + recipientKey,
		SK:              fmt.Sprintf("CHAT#%s#%s#%s", senderKey, msg.Time, msg.ID),
		ID:              msg.ID,
		SenderEmail:     msg.SenderEmail,
		SenderHandle:    msg.SenderHandle,
		SenderID:        msg.SenderID,
		RecipientEmail:  msg.RecipientEmail,
		RecipientHandle: msg.RecipientHandle,
		RecipientID:     msg.RecipientID,
		Text:            msg.Text,
		IsProof:         msg.IsProof,
		GoalID:          msg.GoalID,
		GoalTitle:       msg.GoalTitle,
		OwnerID:         msg.OwnerID,
		Images:          msg.Images,
		ExternalLink:    msg.ExternalLink,
		Approved:        msg.Approved,
		Time:            msg.Time,
		Delivered:       isRecipientOnline,
	}

	sMap, err := attributevalue.MarshalMap(senderItem)
	if err != nil {
		return fmt.Errorf("marshal sender message: %w", err)
	}

	var transactItems []types.TransactWriteItem
	transactItems = append(transactItems, types.TransactWriteItem{
		Put: &types.Put{TableName: aws.String(r.tableName), Item: sMap},
	})

	if senderKey != recipientKey {
		rMap, err := attributevalue.MarshalMap(recipientItem)
		if err != nil {
			return fmt.Errorf("marshal recipient message: %w", err)
		}
		transactItems = append(transactItems, types.TransactWriteItem{
			Put: &types.Put{TableName: aws.String(r.tableName), Item: rMap},
		})
	}

	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		return fmt.Errorf("transact write chat message: %w", err)
	}

	return nil
}

// GetPendingMessages fetches all undelivered messages for an offline recipient.
// Security: Never queries mutable handle partitions (@handle) to prevent data leaks upon handle recycling.
func (r *Repository) GetPendingMessages(ctx context.Context, recipientKey string) ([]realtime.StoredMessage, error) {
	norm := normalizeStorageKey(recipientKey)
	if norm == "" || strings.HasPrefix(norm, "@") {
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
			ID:              m.ID,
			SenderEmail:     m.SenderEmail,
			SenderHandle:    m.SenderHandle,
			SenderID:        m.SenderID,
			RecipientEmail:  m.RecipientEmail,
			RecipientHandle: m.RecipientHandle,
			RecipientID:     m.RecipientID,
			Text:            m.Text,
			IsProof:         m.IsProof,
			GoalID:          m.GoalID,
			GoalTitle:       m.GoalTitle,
			OwnerID:         m.OwnerID,
			Images:          m.Images,
			ExternalLink:    m.ExternalLink,
			Approved:        m.Approved,
			Time:            m.Time,
			Delivered:       m.Delivered,
		}
	}

	return out, nil
}

// MarkMessagesDelivered marks a recipient's pending messages as delivered.
func (r *Repository) MarkMessagesDelivered(ctx context.Context, recipientKey string, msgIDs []string) error {
	norm := normalizeStorageKey(recipientKey)
	if norm == "" || strings.HasPrefix(norm, "@") || len(msgIDs) == 0 {
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

// GetConversationHistory returns messages between userKey and peerKey in chronological order.
// Supports deterministic pagination with composite before cursor (<timestamp>#<id> or <timestamp>).
func (r *Repository) GetConversationHistory(ctx context.Context, userKey, peerKey string, limit int32, before ...string) ([]Message, error) {
	normUser := normalizeStorageKey(userKey)
	normPeer := normalizeStorageKey(peerKey)
	if normUser == "" || normPeer == "" {
		return nil, nil
	}

	if limit <= 0 {
		limit = 100
	}

	beforeCursor := ""
	if len(before) > 0 && strings.TrimSpace(before[0]) != "" {
		beforeCursor = strings.TrimSpace(before[0])
	}

	var queryInput *dynamodb.QueryInput
	var cursorSuffix string
	if beforeCursor != "" {
		cursorSuffix = beforeCursor
		if strings.HasPrefix(cursorSuffix, fmt.Sprintf("CHAT#%s#", normPeer)) {
			cursorSuffix = strings.TrimPrefix(cursorSuffix, fmt.Sprintf("CHAT#%s#", normPeer))
		}
		skMax := fmt.Sprintf("CHAT#%s#%s", normPeer, cursorSuffix)
		queryInput = &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("PK = :pk AND SK BETWEEN :skMin AND :skMax"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":    &types.AttributeValueMemberS{Value: "USER#" + normUser},
				":skMin": &types.AttributeValueMemberS{Value: fmt.Sprintf("CHAT#%s#", normPeer)},
				":skMax": &types.AttributeValueMemberS{Value: skMax},
			},
			ScanIndexForward: aws.Bool(false), // Descending: newest before cursor
			Limit:            aws.Int32(limit + 1),
		}
	} else {
		queryInput = &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: "USER#" + normUser},
				":sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("CHAT#%s#", normPeer)},
			},
			ScanIndexForward: aws.Bool(false), // Descending: newest messages up to limit
			Limit:            aws.Int32(limit),
		}
	}

	result, err := r.db.Query(ctx, queryInput)
	if err != nil {
		return nil, err
	}

	var rawMsgs []Message
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &rawMsgs); err != nil {
		return nil, err
	}

	var filtered []Message
	for _, m := range rawMsgs {
		if cursorSuffix != "" {
			itemSuffix := fmt.Sprintf("%s#%s", m.Time, m.ID)
			// Filter out the cursor boundary item itself when matching composite cursor or timestamp
			if cursorSuffix == itemSuffix || cursorSuffix == m.Time {
				continue
			}
		}
		filtered = append(filtered, m)
		if len(filtered) == int(limit) {
			break
		}
	}

	// Sort chronologically (ascending) with deterministic ID tie-breaking
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Time == filtered[j].Time {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].Time < filtered[j].Time
	})

	return filtered, nil
}

// ClearConversation deletes all message records for userKey with peerKey from DynamoDB.
func (r *Repository) ClearConversation(ctx context.Context, userKey, peerKey string) error {
	normUser := normalizeStorageKey(userKey)
	normPeer := normalizeStorageKey(peerKey)
	if normUser == "" || normPeer == "" {
		return nil
	}

	result, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USER#" + normUser},
			":sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("CHAT#%s#", normPeer)},
		},
	})
	if err != nil {
		return err
	}

	for _, item := range result.Items {
		pk := item["PK"]
		sk := item["SK"]
		if pk != nil && sk != nil {
			_, err := r.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: aws.String(r.tableName),
				Key: map[string]types.AttributeValue{
					"PK": pk,
					"SK": sk,
				},
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// DeleteMessage deletes a message by ID if and only if the caller is the verified sender.
// It queries exclusively within the caller's immutable partition (and provable email partition),
// eliminating full-table scans, verifies authorship strictly by UserID (or verified email for legacy data),
// and deletes both the sender and recipient copies by exact primary keys.
func (r *Repository) DeleteMessage(ctx context.Context, callerEmail, callerHandle, callerID, msgID string) error {
	normID := strings.TrimSpace(msgID)
	if normID == "" {
		return errors.New("missing message id")
	}

	normEmail := strings.TrimSpace(strings.ToLower(callerEmail))
	normIDKey := strings.TrimSpace(callerID)

	var callerPartitions []string
	if normIDKey != "" {
		callerPartitions = append(callerPartitions, "USER#"+normIDKey)
	}
	if normEmail != "" && normEmail != normIDKey {
		callerPartitions = append(callerPartitions, "USER#"+normEmail)
	}

	if len(callerPartitions) == 0 {
		return errors.New("unauthorized: caller identity required")
	}

	// Query caller partition(s) directly without full-table scan
	var foundItem *Message
	for _, pk := range callerPartitions {
		out, err := r.db.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
			FilterExpression:       aws.String("ID = :id"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":       &types.AttributeValueMemberS{Value: pk},
				":skPrefix": &types.AttributeValueMemberS{Value: "CHAT#"},
				":id":       &types.AttributeValueMemberS{Value: normID},
			},
			Limit: aws.Int32(1),
		})
		if err != nil {
			return fmt.Errorf("query caller partition: %w", err)
		}
		if len(out.Items) > 0 {
			var m Message
			if err := attributevalue.UnmarshalMap(out.Items[0], &m); err == nil {
				foundItem = &m
				break
			}
		}
	}

	if foundItem == nil {
		return errors.New("message not found or unauthorized")
	}

	// Verify authorship: caller must be the authentic sender.
	// Security: Matching handle alone is NEVER sufficient proof of authorship
	// because handles are mutable and can be recycled.
	isSender := false
	if normIDKey != "" && (foundItem.SenderID == normIDKey || foundItem.OwnerID == normIDKey) {
		isSender = true
	} else if foundItem.SenderID == "" && normEmail != "" && strings.EqualFold(foundItem.SenderEmail, normEmail) {
		// Legacy message fallback when SenderID was not recorded
		isSender = true
	}

	if !isSender {
		return errors.New("forbidden: only the sender may delete this message")
	}

	senderPK := foundItem.PK
	senderSK := foundItem.SK
	transactItems := []types.TransactWriteItem{
		{
			Delete: &types.Delete{
				TableName: aws.String(r.tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: senderPK},
					"SK": &types.AttributeValueMemberS{Value: senderSK},
				},
			},
		},
	}

	recipientPK := ""
	recipientSK := ""

	if foundItem.RecipientID != "" && foundItem.SenderID != "" {
		recipientPK = "USER#" + foundItem.RecipientID
		recipientSK = fmt.Sprintf("CHAT#%s#%s#%s", foundItem.SenderID, foundItem.Time, foundItem.ID)
	} else {
		// Legacy fallback when RecipientID was not recorded
		recipientKey := strings.TrimSpace(strings.ToLower(foundItem.RecipientEmail))
		if recipientKey == "" && foundItem.RecipientHandle != "" {
			recipientKey = "@" + strings.TrimPrefix(strings.TrimSpace(strings.ToLower(foundItem.RecipientHandle)), "@")
		}

		senderKey := strings.TrimSpace(strings.ToLower(foundItem.SenderEmail))
		if senderKey == "" && foundItem.SenderHandle != "" {
			senderKey = "@" + strings.TrimPrefix(strings.TrimSpace(strings.ToLower(foundItem.SenderHandle)), "@")
		}

		if recipientKey != "" && senderKey != "" {
			recipientPK = "USER#" + recipientKey
			recipientSK = fmt.Sprintf("CHAT#%s#%s#%s", senderKey, foundItem.Time, foundItem.ID)
		}
	}

	if recipientPK != "" && (recipientPK != senderPK || recipientSK != senderSK) {
		transactItems = append(transactItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(r.tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: recipientPK},
					"SK": &types.AttributeValueMemberS{Value: recipientSK},
				},
			},
		})
	}

	_, err := r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		return fmt.Errorf("transact delete chat message: %w", err)
	}

	return nil
}

// SaveCommunityMessage stores a message in the community partition.
func (r *Repository) SaveCommunityMessage(ctx context.Context, communityID string, msg realtime.StoredMessage) error {
	cleanID := strings.TrimSpace(communityID)
	if cleanID == "" {
		return errors.New("empty community id")
	}

	now := strings.TrimSpace(msg.Time)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}

	sk := fmt.Sprintf("MSG#%s#%s", now, msg.ID)
	item := Message{
		PK:           "COMMUNITY#" + cleanID,
		SK:           sk,
		ID:           msg.ID,
		SenderID:     msg.SenderID,
		SenderEmail:  msg.SenderEmail,
		SenderHandle: msg.SenderHandle,
		RecipientID:  cleanID,
		Text:         msg.Text,
		IsProof:      msg.IsProof,
		GoalID:       msg.GoalID,
		GoalTitle:    msg.GoalTitle,
		OwnerID:      msg.OwnerID,
		Images:       msg.Images,
		ExternalLink: msg.ExternalLink,
		Approved:     msg.Approved,
		Time:         now,
		Delivered:    true,
	}

	mMap, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshal community message: %w", err)
	}

	_, err = r.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName: aws.String(r.tableName),
					Item:      mMap,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("transact write community message: %w", err)
	}
	return nil
}

// GetCommunityHistory returns messages for a community in chronological order.
func (r *Repository) GetCommunityHistory(ctx context.Context, communityID string, limit int32, before ...string) ([]Message, error) {
	cleanID := strings.TrimSpace(communityID)
	if cleanID == "" {
		return []Message{}, nil
	}
	if limit <= 0 {
		limit = 100
	}

	beforeCursor := ""
	if len(before) > 0 && strings.TrimSpace(before[0]) != "" {
		beforeCursor = strings.TrimSpace(before[0])
	}

	var queryInput *dynamodb.QueryInput
	if beforeCursor != "" {
		cursorSuffix := beforeCursor
		if strings.HasPrefix(cursorSuffix, "MSG#") {
			cursorSuffix = strings.TrimPrefix(cursorSuffix, "MSG#")
		}
		skMax := fmt.Sprintf("MSG#%s", cursorSuffix)
		queryInput = &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("PK = :pk AND SK BETWEEN :skMin AND :skMax"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":    &types.AttributeValueMemberS{Value: "COMMUNITY#" + cleanID},
				":skMin": &types.AttributeValueMemberS{Value: "MSG#"},
				":skMax": &types.AttributeValueMemberS{Value: skMax},
			},
			ScanIndexForward: aws.Bool(false),
			Limit:            aws.Int32(limit + 1),
		}
	} else {
		queryInput = &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: "COMMUNITY#" + cleanID},
				":sk": &types.AttributeValueMemberS{Value: "MSG#"},
			},
			ScanIndexForward: aws.Bool(false),
			Limit:            aws.Int32(limit),
		}
	}

	result, err := r.db.Query(ctx, queryInput)
	if err != nil {
		return nil, err
	}

	var msgs []Message
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &msgs); err != nil {
		return nil, err
	}

	sort.Slice(msgs, func(i, j int) bool {
		if msgs[i].Time == msgs[j].Time {
			return msgs[i].ID < msgs[j].ID
		}
		return msgs[i].Time < msgs[j].Time
	})

	if msgs == nil {
		msgs = make([]Message, 0)
	}
	return msgs, nil
}
