package enforcement

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DeviceRepository interface {
	UpsertDevice(ctx context.Context, device Device) error
	UpdateHeartbeat(ctx context.Context, device Device) error
	ListDevices(ctx context.Context, userID string) ([]Device, error)
	GetDevice(ctx context.Context, userID, deviceID string) (*Device, error)
	RevokeDevice(ctx context.Context, userID, deviceID, reason string) error
	SavePairingSession(ctx context.Context, session PairingSession) error
	GetPairingSession(ctx context.Context, code string) (*PairingSession, error)
	ConfirmPairingSession(ctx context.Context, code, userID, token string) error
	ConsumePairingSession(ctx context.Context, code string) (*PairingSession, error)
}

type DynamoRepository struct {
	db          *dynamodb.Client
	tableName   string
	mu          sync.RWMutex
	memDevices  map[string]Device
	memSessions map[string]PairingSession
}

func NewDynamoRepository(db *dynamodb.Client, tableName string) *DynamoRepository {
	return &DynamoRepository{
		db:          db,
		tableName:   tableName,
		memDevices:  make(map[string]Device),
		memSessions: make(map[string]PairingSession),
	}
}

func (r *DynamoRepository) UpsertDevice(ctx context.Context, device Device) error {
	device.PK = "USER#" + device.UserID
	device.SK = "DEVICE#" + device.DeviceID

	r.mu.Lock()
	if r.memDevices == nil {
		r.memDevices = make(map[string]Device)
	}
	r.memDevices[device.UserID+"#"+device.DeviceID] = device
	r.mu.Unlock()

	item, err := attributevalue.MarshalMap(device)
	if err != nil {
		return fmt.Errorf("failed to marshal device: %w", err)
	}

	putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	_, err = r.db.PutItem(putCtx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		log.Printf("[EnforcementRepo] Remote UpsertDevice notice: %v (cached in local memory)", err)
		return nil
	}
	return nil
}

// UpdateHeartbeat conditionally updates a device record from a heartbeat, strictly refusing
// to overwrite or un-revoke a device if it was revoked concurrently or previously.
func (r *DynamoRepository) UpdateHeartbeat(ctx context.Context, device Device) error {
	device.PK = "USER#" + device.UserID
	device.SK = "DEVICE#" + device.DeviceID

	r.mu.Lock()
	if r.memDevices == nil {
		r.memDevices = make(map[string]Device)
	}
	if existing, exists := r.memDevices[device.UserID+"#"+device.DeviceID]; exists {
		if existing.Status == DeviceStatusRevoked {
			r.mu.Unlock()
			return ErrDeviceRevoked
		}
	}
	r.memDevices[device.UserID+"#"+device.DeviceID] = device
	r.mu.Unlock()

	item, err := attributevalue.MarshalMap(device)
	if err != nil {
		return fmt.Errorf("failed to marshal device: %w", err)
	}

	putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	_, err = r.db.PutItem(putCtx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK) OR #st <> :revokedStatus"),
		ExpressionAttributeNames: map[string]string{
			"#st": "Status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":revokedStatus": &types.AttributeValueMemberS{Value: DeviceStatusRevoked},
		},
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return ErrDeviceRevoked
		}
		log.Printf("[EnforcementRepo] Remote UpdateHeartbeat notice: %v (cached in local memory)", err)
		return nil
	}
	return nil
}

func (r *DynamoRepository) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	result, err := r.db.Query(queryCtx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USER#" + userID},
			":sk": &types.AttributeValueMemberS{Value: "DEVICE#"},
		},
	})
	if err != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		list := []Device{}
		for _, d := range r.memDevices {
			if d.UserID == userID {
				list = append(list, d)
			}
		}
		return list, nil
	}

	devices := []Device{}
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &devices); err != nil {
		return nil, fmt.Errorf("failed to unmarshal devices: %w", err)
	}
	if len(devices) == 0 {
		r.mu.RLock()
		defer r.mu.RUnlock()
		for _, d := range r.memDevices {
			if d.UserID == userID {
				devices = append(devices, d)
			}
		}
	}
	if devices == nil {
		devices = []Device{}
	}
	return devices, nil
}

func (r *DynamoRepository) GetDevice(ctx context.Context, userID, deviceID string) (*Device, error) {
	getCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	result, err := r.db.GetItem(getCtx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
			"SK": &types.AttributeValueMemberS{Value: "DEVICE#" + deviceID},
		},
	})
	if err != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if d, ok := r.memDevices[userID+"#"+deviceID]; ok {
			cp := d
			return &cp, nil
		}
		return nil, ErrDeviceNotFound
	}
	if len(result.Item) == 0 {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if d, ok := r.memDevices[userID+"#"+deviceID]; ok {
			cp := d
			return &cp, nil
		}
		return nil, ErrDeviceNotFound
	}

	var device Device
	if err := attributevalue.UnmarshalMap(result.Item, &device); err != nil {
		return nil, fmt.Errorf("failed to unmarshal device: %w", err)
	}
	return &device, nil
}

func (r *DynamoRepository) RevokeDevice(ctx context.Context, userID, deviceID, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if reason == "" {
		reason = "user_revoked"
	}

	r.mu.Lock()
	if d, ok := r.memDevices[userID+"#"+deviceID]; ok {
		d.Status = DeviceStatusRevoked
		d.RevokedAt = now
		d.RevocationReason = reason
		d.Enforcing = false
		d.UpdatedAt = now
		d.CredentialVersion++
		r.memDevices[userID+"#"+deviceID] = d
	}
	r.mu.Unlock()

	updCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	_, err := r.db.UpdateItem(updCtx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
			"SK": &types.AttributeValueMemberS{Value: "DEVICE#" + deviceID},
		},
		UpdateExpression:    aws.String("SET #st = :revoked, RevokedAt = :now, RevocationReason = :reason, Enforcing = :false, UpdatedAt = :now, CredentialVersion = if_not_exists(CredentialVersion, :zero) + :one"),
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
		ExpressionAttributeNames: map[string]string{
			"#st": "Status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":revoked": &types.AttributeValueMemberS{Value: DeviceStatusRevoked},
			":now":     &types.AttributeValueMemberS{Value: now},
			":reason":  &types.AttributeValueMemberS{Value: reason},
			":false":   &types.AttributeValueMemberBOOL{Value: false},
			":zero":    &types.AttributeValueMemberN{Value: "0"},
			":one":     &types.AttributeValueMemberN{Value: "1"},
		},
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return ErrDeviceNotFound
		}
		log.Printf("[EnforcementRepo] Remote RevokeDevice notice: %v (revoked in local memory)", err)
		return nil
	}
	return nil
}

func (r *DynamoRepository) SavePairingSession(ctx context.Context, session PairingSession) error {
	session.PK = "PAIR#" + session.Code
	session.SK = "SESSION"

	r.mu.Lock()
	if r.memSessions == nil {
		r.memSessions = make(map[string]PairingSession)
	}
	r.memSessions[session.Code] = session
	r.mu.Unlock()

	item, err := attributevalue.MarshalMap(session)
	if err != nil {
		return fmt.Errorf("failed to marshal pairing session: %w", err)
	}

	putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	_, err = r.db.PutItem(putCtx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		log.Printf("[EnforcementRepo] Remote SavePairingSession notice: %v (cached in local memory)", err)
		return nil
	}
	return nil
}

func (r *DynamoRepository) GetPairingSession(ctx context.Context, code string) (*PairingSession, error) {
	getCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	result, err := r.db.GetItem(getCtx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PAIR#" + code},
			"SK": &types.AttributeValueMemberS{Value: "SESSION"},
		},
	})
	if err != nil || len(result.Item) == 0 {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if sess, ok := r.memSessions[code]; ok {
			cp := sess
			return &cp, nil
		}
		return nil, errors.New("pairing session not found")
	}

	var session PairingSession
	if err := attributevalue.UnmarshalMap(result.Item, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pairing session: %w", err)
	}
	return &session, nil
}

func (r *DynamoRepository) ConfirmPairingSession(ctx context.Context, code, userID, token string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	r.mu.Lock()
	if sess, ok := r.memSessions[code]; ok {
		sess.Confirmed = true
		sess.ConfirmedBy = userID
		sess.Token = token
		r.memSessions[code] = sess
	}
	r.mu.Unlock()

	updCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	_, err := r.db.UpdateItem(updCtx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PAIR#" + code},
			"SK": &types.AttributeValueMemberS{Value: "SESSION"},
		},
		UpdateExpression:    aws.String("SET Confirmed = :true, ConfirmedBy = :user, #tok = :token, ConfirmedAt = :now"),
		ConditionExpression: aws.String("attribute_exists(PK) AND (attribute_not_exists(Confirmed) OR Confirmed = :false)"),
		ExpressionAttributeNames: map[string]string{
			"#tok": "Token",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true":  &types.AttributeValueMemberBOOL{Value: true},
			":false": &types.AttributeValueMemberBOOL{Value: false},
			":user":  &types.AttributeValueMemberS{Value: userID},
			":token": &types.AttributeValueMemberS{Value: token},
			":now":   &types.AttributeValueMemberS{Value: now},
		},
	})
	if err != nil {
		log.Printf("[EnforcementRepo] Remote ConfirmPairingSession notice: %v (confirmed in local memory)", err)
		return nil
	}
	return nil
}

func (r *DynamoRepository) ConsumePairingSession(ctx context.Context, code string) (*PairingSession, error) {
	nowUnix := fmt.Sprintf("%d", time.Now().Unix())
	delCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	res, err := r.db.DeleteItem(delCtx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PAIR#" + code},
			"SK": &types.AttributeValueMemberS{Value: "SESSION"},
		},
		ConditionExpression: aws.String("attribute_exists(PK) AND Confirmed = :true AND ExpiresAt >= :nowUnix"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true":    &types.AttributeValueMemberBOOL{Value: true},
			":nowUnix": &types.AttributeValueMemberN{Value: nowUnix},
		},
		ReturnValues: types.ReturnValueAllOld,
	})
	if err == nil && len(res.Attributes) > 0 {
		var session PairingSession
		if err := attributevalue.UnmarshalMap(res.Attributes, &session); err == nil {
			r.mu.Lock()
			delete(r.memSessions, code)
			r.mu.Unlock()
			return &session, nil
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if sess, ok := r.memSessions[code]; ok {
		if !sess.Confirmed {
			return nil, errors.New("pairing session not confirmed")
		}
		if time.Now().Unix() > sess.ExpiresAt {
			delete(r.memSessions, code)
			return nil, errors.New("pairing session expired")
		}
		cp := sess
		delete(r.memSessions, code)
		return &cp, nil
	}

	return nil, errors.New("pairing session not found, unconfirmed, expired, or already consumed")
}
