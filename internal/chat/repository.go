package chat

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
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
	db                   DynamoDBAPI
	tableName            string
	mu                   sync.RWMutex
	memMessages          map[string][]Message // key: normUser + "#" + normPeer -> []Message
	memCommunityMessages map[string][]Message // key: communityID -> []Message
	memAllMessages       map[string]Message   // key: msgID -> Message
}

func NewRepository(db DynamoDBAPI, tableName string) *Repository {
	return &Repository{
		db:                   db,
		tableName:            tableName,
		memMessages:          make(map[string][]Message),
		memCommunityMessages: make(map[string][]Message),
		memAllMessages:       make(map[string]Message),
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

	r.mu.Lock()
	if r.memMessages == nil {
		r.memMessages = make(map[string][]Message)
		r.memAllMessages = make(map[string]Message)
	}
	sNorm := normalizeStorageKey(senderKey)
	rNorm := normalizeStorageKey(recipientKey)
	sConvKey := fmt.Sprintf("%s#%s", sNorm, rNorm)
	rConvKey := fmt.Sprintf("%s#%s", rNorm, sNorm)
	r.memMessages[sConvKey] = append(r.memMessages[sConvKey], senderItem)
	if sConvKey != rConvKey {
		r.memMessages[rConvKey] = append(r.memMessages[rConvKey], recipientItem)
	}
	r.memAllMessages[msg.ID] = senderItem
	r.mu.Unlock()

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

	if r.db != nil {
		putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		_, err = r.db.TransactWriteItems(putCtx, &dynamodb.TransactWriteItemsInput{
			TransactItems: transactItems,
		})
		if err != nil {
			log.Printf("[ChatRepo] Remote DynamoDB direct message notice: %v (cached in local memory)", err)
		}
	}

	return nil
}

// GetPendingMessages fetches all undelivered messages for an offline recipient.
func (r *Repository) GetPendingMessages(ctx context.Context, recipientKey string) ([]realtime.StoredMessage, error) {
	norm := normalizeStorageKey(recipientKey)
	if norm == "" || strings.HasPrefix(norm, "@") {
		return nil, nil
	}

	var msgs []Message

	if r.db != nil {
		queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		result, err := r.db.Query(queryCtx, &dynamodb.QueryInput{
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
		if err == nil && result != nil {
			_ = attributevalue.UnmarshalListOfMaps(result.Items, &msgs)
		}
	}

	r.mu.RLock()
	if r.memMessages != nil {
		prefix := norm + "#"
		for k, mList := range r.memMessages {
			if strings.HasPrefix(k, prefix) {
				for _, m := range mList {
					if !m.Delivered {
						msgs = append(msgs, m)
					}
				}
			}
		}
	}
	r.mu.RUnlock()

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

	r.mu.Lock()
	if r.memMessages != nil {
		prefix := norm + "#"
		for k, mList := range r.memMessages {
			if strings.HasPrefix(k, prefix) {
				for i := range mList {
					if idSet[mList[i].ID] {
						mList[i].Delivered = true
					}
				}
			}
		}
	}
	r.mu.Unlock()

	if r.db != nil {
		queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		result, err := r.db.Query(queryCtx, &dynamodb.QueryInput{
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
		if err == nil && result != nil {
			for _, item := range result.Items {
				var m Message
				if err := attributevalue.UnmarshalMap(item, &m); err == nil && idSet[m.ID] {
					updCtx, cancelUpd := context.WithTimeout(ctx, 800*time.Millisecond)
					_, _ = r.db.UpdateItem(updCtx, &dynamodb.UpdateItemInput{
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
					cancelUpd()
				}
			}
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

	msgMap := make(map[string]Message)

	if r.db != nil {
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
				ScanIndexForward: aws.Bool(false),
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
				ScanIndexForward: aws.Bool(false),
				Limit:            aws.Int32(limit),
			}
		}

		queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		result, err := r.db.Query(queryCtx, queryInput)
		if err == nil && result != nil {
			var rawMsgs []Message
			if err := attributevalue.UnmarshalListOfMaps(result.Items, &rawMsgs); err == nil {
				for _, m := range rawMsgs {
					msgMap[m.ID] = m
				}
			}
		}
	}

	r.mu.RLock()
	convKey := fmt.Sprintf("%s#%s", normUser, normPeer)
	if r.memMessages != nil && r.memMessages[convKey] != nil {
		for _, m := range r.memMessages[convKey] {
			msgMap[m.ID] = m
		}
	}
	r.mu.RUnlock()

	var filtered []Message
	for _, m := range msgMap {
		if beforeCursor != "" {
			cursorSuffix := beforeCursor
			if strings.HasPrefix(cursorSuffix, fmt.Sprintf("CHAT#%s#", normPeer)) {
				cursorSuffix = strings.TrimPrefix(cursorSuffix, fmt.Sprintf("CHAT#%s#", normPeer))
			}
			itemSuffix := fmt.Sprintf("%s#%s", m.Time, m.ID)
			if cursorSuffix == itemSuffix || cursorSuffix == m.Time || m.Time >= cursorSuffix {
				continue
			}
		}
		filtered = append(filtered, m)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Time == filtered[j].Time {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].Time < filtered[j].Time
	})

	if len(filtered) > int(limit) {
		filtered = filtered[len(filtered)-int(limit):]
	}

	return filtered, nil
}

// ClearConversation deletes all message records for userKey with peerKey.
func (r *Repository) ClearConversation(ctx context.Context, userKey, peerKey string) error {
	normUser := normalizeStorageKey(userKey)
	normPeer := normalizeStorageKey(peerKey)
	if normUser == "" || normPeer == "" {
		return nil
	}

	convKey := fmt.Sprintf("%s#%s", normUser, normPeer)
	convKeyRev := fmt.Sprintf("%s#%s", normPeer, normUser)
	r.mu.Lock()
	if r.memMessages != nil {
		delete(r.memMessages, convKey)
		delete(r.memMessages, convKeyRev)
	}
	r.mu.Unlock()

	if r.db != nil {
		queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		result, err := r.db.Query(queryCtx, &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: "USER#" + normUser},
				":sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("CHAT#%s#", normPeer)},
			},
		})
		if err == nil && result != nil {
			for _, item := range result.Items {
				pk := item["PK"]
				sk := item["SK"]
				if pk != nil && sk != nil {
					delCtx, cancelDel := context.WithTimeout(ctx, 800*time.Millisecond)
					_, _ = r.db.DeleteItem(delCtx, &dynamodb.DeleteItemInput{
						TableName: aws.String(r.tableName),
						Key: map[string]types.AttributeValue{
							"PK": pk,
							"SK": sk,
						},
					})
					cancelDel()
				}
			}
		}
	}

	return nil
}

// DeleteMessage deletes a message by ID if and only if the caller is the verified sender.
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

	var foundItem *Message

	r.mu.RLock()
	if m, ok := r.memAllMessages[normID]; ok {
		mCopy := m
		foundItem = &mCopy
	}
	r.mu.RUnlock()

	if foundItem == nil && r.db != nil {
		for _, pk := range callerPartitions {
			queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
			out, err := r.db.Query(queryCtx, &dynamodb.QueryInput{
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
			cancel()
			if err == nil && len(out.Items) > 0 {
				var m Message
				if err := attributevalue.UnmarshalMap(out.Items[0], &m); err == nil {
					foundItem = &m
					break
				}
			}
		}
	}

	if foundItem == nil {
		return errors.New("message not found or unauthorized")
	}

	isSender := false
	if normIDKey != "" && (foundItem.SenderID == normIDKey || foundItem.OwnerID == normIDKey) {
		isSender = true
	} else if foundItem.SenderID == "" && normEmail != "" && strings.EqualFold(foundItem.SenderEmail, normEmail) {
		isSender = true
	}

	if !isSender {
		return errors.New("forbidden: only the sender may delete this message")
	}

	r.mu.Lock()
	delete(r.memAllMessages, normID)
	for cid, mList := range r.memCommunityMessages {
		var filtered []Message
		for _, m := range mList {
			if m.ID != normID {
				filtered = append(filtered, m)
			}
		}
		r.memCommunityMessages[cid] = filtered
	}
	for cKey, mList := range r.memMessages {
		var filtered []Message
		for _, m := range mList {
			if m.ID != normID {
				filtered = append(filtered, m)
			}
		}
		r.memMessages[cKey] = filtered
	}
	r.mu.Unlock()

	if r.db != nil {
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

		txCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		_, err := r.db.TransactWriteItems(txCtx, &dynamodb.TransactWriteItemsInput{
			TransactItems: transactItems,
		})
		if err != nil {
			return fmt.Errorf("transact delete chat message: %w", err)
		}
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

	r.mu.Lock()
	if r.memCommunityMessages == nil {
		r.memCommunityMessages = make(map[string][]Message)
		r.memAllMessages = make(map[string]Message)
	}
	r.memCommunityMessages[cleanID] = append(r.memCommunityMessages[cleanID], item)
	r.memAllMessages[msg.ID] = item
	r.mu.Unlock()

	if r.db != nil {
		mMap, err := attributevalue.MarshalMap(item)
		if err == nil {
			putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
			defer cancel()
			_, err = r.db.TransactWriteItems(putCtx, &dynamodb.TransactWriteItemsInput{
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
				log.Printf("[ChatRepo] Remote DynamoDB community message notice: %v (cached in local memory)", err)
			}
		}
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

	msgMap := make(map[string]Message)

	if r.db != nil {
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

		queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()
		result, err := r.db.Query(queryCtx, queryInput)
		if err == nil && result != nil {
			var remoteMsgs []Message
			if err := attributevalue.UnmarshalListOfMaps(result.Items, &remoteMsgs); err == nil {
				for _, m := range remoteMsgs {
					msgMap[m.ID] = m
				}
			}
		}
	}

	r.mu.RLock()
	if r.memCommunityMessages != nil && r.memCommunityMessages[cleanID] != nil {
		for _, m := range r.memCommunityMessages[cleanID] {
			msgMap[m.ID] = m
		}
	}
	r.mu.RUnlock()

	msgs := make([]Message, 0, len(msgMap))
	for _, m := range msgMap {
		if beforeCursor != "" {
			cursorSuffix := beforeCursor
			if strings.HasPrefix(cursorSuffix, "MSG#") {
				cursorSuffix = strings.TrimPrefix(cursorSuffix, "MSG#")
			}
			itemSuffix := fmt.Sprintf("%s#%s", m.Time, m.ID)
			if cursorSuffix == itemSuffix || cursorSuffix == m.Time || m.Time >= cursorSuffix {
				continue
			}
		}
		msgs = append(msgs, m)
	}

	sort.Slice(msgs, func(i, j int) bool {
		if msgs[i].Time == msgs[j].Time {
			return msgs[i].ID < msgs[j].ID
		}
		return msgs[i].Time < msgs[j].Time
	})

	if len(msgs) > int(limit) {
		msgs = msgs[len(msgs)-int(limit):]
	}

	if msgs == nil {
		msgs = make([]Message, 0)
	}
	return msgs, nil
}
