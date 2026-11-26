package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

const defaultTableName = "Onusly"

type DynamoRepository struct {
	client     *dynamodb.Client
	tableName  string
	mu         sync.RWMutex
	memUsers   map[string]User   // Key: User.ID
	memEmails  map[string]string // Key: lowercase email -> User.ID
	memHandles map[string]string // Key: lowercase handle -> User.ID
}

func NewRepository() Repository {
	tbl := os.Getenv("DYNAMODB_TABLE")
	if tbl == "" {
		tbl = defaultTableName
	}
	return NewRepositoryWithTable(shared.NewDynamoClient(), tbl)
}

func NewRepositoryWithTable(client *dynamodb.Client, tbl string) Repository {
	if tbl == "" {
		tbl = defaultTableName
	}
	return &DynamoRepository{
		client:     client,
		tableName:  tbl,
		memUsers:   make(map[string]User),
		memEmails:  make(map[string]string),
		memHandles: make(map[string]string),
	}
}

func isConflictError(err error) bool {
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
	return strings.Contains(errStr, "ConditionalCheckFailed") ||
		strings.Contains(errStr, "TransactionConflict") ||
		strings.Contains(errStr, "TransactionCanceledException")
}

func (r *DynamoRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	if cleanEmail == "" {
		return nil, nil
	}

	if r.client == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if uid, ok := r.memEmails[cleanEmail]; ok {
			if u, exists := r.memUsers[uid]; exists {
				return &u, nil
			}
		}
		return nil, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	out, err := r.client.Query(queryCtx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{
				Value: "EMAIL#" + cleanEmail,
			},
		},
		Limit: aws.Int32(1),
	})

	if err != nil {
		return nil, err
	}

	if len(out.Items) == 0 {
		return nil, nil
	}

	var user User
	if err := attributevalue.UnmarshalMap(out.Items[0], &user); err != nil {
		return nil, err
	}
	if user.TokenVersion <= 0 {
		user.TokenVersion = 1
	}

	r.mu.Lock()
	r.memUsers[user.ID] = user
	r.memEmails[cleanEmail] = user.ID
	if user.Handle != "" {
		r.memHandles[strings.ToLower(user.Handle)] = user.ID
	}
	r.mu.Unlock()

	return &user, nil
}

func (r *DynamoRepository) GetUserByID(ctx context.Context, userID string) (*User, error) {
	if userID == "" {
		return nil, nil
	}

	if r.client == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if u, exists := r.memUsers[userID]; exists {
			return &u, nil
		}
		return nil, nil
	}

	getCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	out, err := r.client.GetItem(getCtx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	var user User
	if err := attributevalue.UnmarshalMap(out.Item, &user); err != nil {
		return nil, err
	}
	if user.TokenVersion <= 0 {
		user.TokenVersion = 1
	}

	r.mu.Lock()
	r.memUsers[user.ID] = user
	if user.Email != "" {
		r.memEmails[strings.ToLower(user.Email)] = user.ID
	}
	if user.Handle != "" {
		r.memHandles[strings.ToLower(user.Handle)] = user.ID
	}
	r.mu.Unlock()

	return &user, nil
}

func (r *DynamoRepository) GetUserByHandle(ctx context.Context, handle string) (*User, error) {
	cleaned := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(handle)), "@")
	if cleaned == "" {
		return nil, nil
	}

	if r.client == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if uid, ok := r.memHandles[cleaned]; ok {
			if u, exists := r.memUsers[uid]; exists {
				return &u, nil
			}
		}
		return nil, nil
	}

	// 1. Direct O(1) lookup on HANDLE#<clean> reservation item
	getCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resOut, err := r.client.GetItem(getCtx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "HANDLE#" + cleaned},
			"SK": &types.AttributeValueMemberS{Value: "RESERVATION"},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resOut.Item) == 0 {
		return nil, nil
	}

	var reservation HandleReservation
	if err := attributevalue.UnmarshalMap(resOut.Item, &reservation); err != nil {
		return nil, err
	}
	if reservation.UserID == "" {
		return nil, nil
	}

	return r.GetUserByID(ctx, reservation.UserID)
}

func (r *DynamoRepository) CreateUser(ctx context.Context, user *User) error {
	user.PK = "USER#" + user.ID
	user.SK = "PROFILE"
	user.GSI1PK = "EMAIL#" + strings.ToLower(strings.TrimSpace(user.Email))
	user.GSI1SK = "PROFILE"
	if user.TokenVersion <= 0 {
		user.TokenVersion = 1
	}

	cleanHandle := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(user.Handle)), "@")

	cacheInMem := func() {
		r.mu.Lock()
		r.memUsers[user.ID] = *user
		if user.Email != "" {
			r.memEmails[strings.ToLower(user.Email)] = user.ID
		}
		if cleanHandle != "" {
			r.memHandles[cleanHandle] = user.ID
		}
		r.mu.Unlock()
	}

	if r.client == nil {
		cacheInMem()
		return nil
	}

	item, err := attributevalue.MarshalMap(user)
	if err != nil {
		return err
	}

	putCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if cleanHandle != "" {
		handleItem := map[string]types.AttributeValue{
			"PK":        &types.AttributeValueMemberS{Value: "HANDLE#" + cleanHandle},
			"SK":        &types.AttributeValueMemberS{Value: "RESERVATION"},
			"UserID":    &types.AttributeValueMemberS{Value: user.ID},
			"Handle":    &types.AttributeValueMemberS{Value: cleanHandle},
			"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		}

		_, err = r.client.TransactWriteItems(putCtx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []types.TransactWriteItem{
				{
					Put: &types.Put{
						TableName:           aws.String(r.tableName),
						Item:                item,
						ConditionExpression: aws.String("attribute_not_exists(PK)"),
					},
				},
				{
					Put: &types.Put{
						TableName:           aws.String(r.tableName),
						Item:                handleItem,
						ConditionExpression: aws.String("attribute_not_exists(PK)"),
					},
				},
			},
		})
		if err != nil {
			if isConflictError(err) {
				return errors.New("this handle is already taken, please choose another")
			}
			return err
		}
		cacheInMem()
		return nil
	}

	_, err = r.client.PutItem(putCtx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		return err
	}

	cacheInMem()
	return nil
}

func (r *DynamoRepository) UpdateUserHandle(ctx context.Context, userID, oldHandle, newHandle string) error {
	cleanNew := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(newHandle)), "@")
	if cleanNew == "" {
		return fmt.Errorf("handle cannot be empty")
	}

	cleanOld := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(oldHandle)), "@")
	if cleanOld == cleanNew {
		// Idempotent: caller already holds this handle
		return nil
	}

	if r.client == nil {
		r.mu.Lock()
		if existing, exists := r.memHandles[cleanNew]; exists && existing != userID {
			r.mu.Unlock()
			return errors.New("this handle is already taken, please choose another")
		}
		if u, exists := r.memUsers[userID]; exists {
			u.Handle = cleanNew
			r.memUsers[userID] = u
		}
		if cleanOld != "" {
			delete(r.memHandles, cleanOld)
		}
		r.memHandles[cleanNew] = userID
		r.mu.Unlock()
		return nil
	}

	handleItem := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: "HANDLE#" + cleanNew},
		"SK":        &types.AttributeValueMemberS{Value: "RESERVATION"},
		"UserID":    &types.AttributeValueMemberS{Value: userID},
		"Handle":    &types.AttributeValueMemberS{Value: cleanNew},
		"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
	}

	transactItems := []types.TransactWriteItem{
		{
			Put: &types.Put{
				TableName:           aws.String(r.tableName),
				Item:                handleItem,
				ConditionExpression: aws.String("attribute_not_exists(PK) OR UserID = :uid"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":uid": &types.AttributeValueMemberS{Value: userID},
				},
			},
		},
		{
			Update: &types.Update{
				TableName: aws.String(r.tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
					"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
				},
				UpdateExpression: aws.String("SET #handle = :handle"),
				ExpressionAttributeNames: map[string]string{
					"#handle": "Handle",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":handle": &types.AttributeValueMemberS{Value: cleanNew},
				},
			},
		},
	}

	if cleanOld != "" && cleanOld != cleanNew {
		transactItems = append(transactItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(r.tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "HANDLE#" + cleanOld},
					"SK": &types.AttributeValueMemberS{Value: "RESERVATION"},
				},
				ConditionExpression: aws.String("attribute_not_exists(PK) OR UserID = :uid"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":uid": &types.AttributeValueMemberS{Value: userID},
				},
			},
		})
	}

	txCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := r.client.TransactWriteItems(txCtx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		if isConflictError(err) {
			return errors.New("this handle is already taken, please choose another")
		}
		return err
	}

	r.mu.Lock()
	if u, exists := r.memUsers[userID]; exists {
		u.Handle = cleanNew
		r.memUsers[userID] = u
	}
	if cleanOld != "" {
		delete(r.memHandles, cleanOld)
	}
	r.memHandles[cleanNew] = userID
	r.mu.Unlock()

	return nil
}

// IncrementTokenVersion increments the user's token version in DynamoDB and memory,
// invalidating all prior active sessions and cookies across all devices.
func (r *DynamoRepository) IncrementTokenVersion(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, errors.New("userID is required")
	}

	if r.client == nil {
		r.mu.Lock()
		newVer := 2
		if u, exists := r.memUsers[userID]; exists {
			if u.TokenVersion <= 0 {
				u.TokenVersion = 1
			}
			u.TokenVersion++
			newVer = u.TokenVersion
			r.memUsers[userID] = u
		} else {
			r.memUsers[userID] = User{ID: userID, TokenVersion: newVer}
		}
		r.mu.Unlock()
		return newVer, nil
	}

	txCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	out, err := r.client.UpdateItem(txCtx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression: aws.String("SET TokenVersion = if_not_exists(TokenVersion, :one) + :inc"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one": &types.AttributeValueMemberN{Value: "1"},
			":inc": &types.AttributeValueMemberN{Value: "1"},
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		return 0, err
	}

	var updated User
	if err := attributevalue.UnmarshalMap(out.Attributes, &updated); err != nil {
		return 0, err
	}
	newVer := updated.TokenVersion
	if newVer <= 0 {
		newVer = 2
	}

	r.mu.Lock()
	if existing, exists := r.memUsers[userID]; exists {
		existing.TokenVersion = newVer
		r.memUsers[userID] = existing
	} else if updated.Email != "" || updated.Handle != "" {
		updated.ID = userID
		r.memUsers[userID] = updated
	}
	r.mu.Unlock()

	return newVer, nil
}

// GetTokenVersion retrieves the current session version for a user (fast in-memory path with DynamoDB fallback).
func (r *DynamoRepository) GetTokenVersion(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 1, nil
	}

	r.mu.RLock()
	if u, exists := r.memUsers[userID]; exists && u.TokenVersion > 0 {
		v := u.TokenVersion
		r.mu.RUnlock()
		return v, nil
	}
	r.mu.RUnlock()

	user, err := r.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return 1, nil
	}

	if user.TokenVersion <= 0 {
		return 1, nil
	}
	return user.TokenVersion, nil
}
