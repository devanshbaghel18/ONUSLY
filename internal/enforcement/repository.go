package enforcement

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DeviceRepository interface {
	UpsertDevice(ctx context.Context, device Device) error
	ListDevices(ctx context.Context, userID string) ([]Device, error)
	GetDevice(ctx context.Context, userID, deviceID string) (*Device, error)
	SavePairingSession(ctx context.Context, session PairingSession) error
	GetPairingSession(ctx context.Context, code string) (*PairingSession, error)
	ConfirmPairingSession(ctx context.Context, code, userID, token string) error
	ConsumePairingSession(ctx context.Context, code string) (*PairingSession, error)
}

type DynamoRepository struct {
	db        *dynamodb.Client
	tableName string
}

func NewDynamoRepository(db *dynamodb.Client, tableName string) *DynamoRepository {
	return &DynamoRepository{
		db:        db,
		tableName: tableName,
	}
}

func (r *DynamoRepository) UpsertDevice(ctx context.Context, device Device) error {
	device.PK = "USER#" + device.UserID
	device.SK = "DEVICE#" + device.DeviceID

	item, err := attributevalue.MarshalMap(device)
	if err != nil {
		return fmt.Errorf("failed to marshal device: %w", err)
	}

	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to put device item: %w", err)
	}
	return nil
}

func (r *DynamoRepository) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	result, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USER#" + userID},
			":sk": &types.AttributeValueMemberS{Value: "DEVICE#"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query user devices: %w", err)
	}

	var devices []Device
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &devices); err != nil {
		return nil, fmt.Errorf("failed to unmarshal devices: %w", err)
	}
	return devices, nil
}

func (r *DynamoRepository) GetDevice(ctx context.Context, userID, deviceID string) (*Device, error) {
	result, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
			"SK": &types.AttributeValueMemberS{Value: "DEVICE#" + deviceID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}
	if len(result.Item) == 0 {
		return nil, ErrDeviceNotFound
	}

	var device Device
	if err := attributevalue.UnmarshalMap(result.Item, &device); err != nil {
		return nil, fmt.Errorf("failed to unmarshal device: %w", err)
	}
	return &device, nil
}

func (r *DynamoRepository) SavePairingSession(ctx context.Context, session PairingSession) error {
	session.PK = "PAIR#" + session.Code
	session.SK = "SESSION"

	item, err := attributevalue.MarshalMap(session)
	if err != nil {
		return fmt.Errorf("failed to marshal pairing session: %w", err)
	}

	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	return err
}

func (r *DynamoRepository) GetPairingSession(ctx context.Context, code string) (*PairingSession, error) {
	result, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PAIR#" + code},
			"SK": &types.AttributeValueMemberS{Value: "SESSION"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get pairing session: %w", err)
	}
	if len(result.Item) == 0 {
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
	_, err := r.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
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
	return err
}

func (r *DynamoRepository) ConsumePairingSession(ctx context.Context, code string) (*PairingSession, error) {
	session, err := r.GetPairingSession(ctx, code)
	if err != nil {
		return nil, err
	}
	if !session.Confirmed {
		return nil, errors.New("pairing session not confirmed")
	}
	if time.Now().Unix() > session.ExpiresAt {
		return nil, errors.New("pairing session expired")
	}

	// Delete after retrieval to ensure single-use
	_, _ = r.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PAIR#" + code},
			"SK": &types.AttributeValueMemberS{Value: "SESSION"},
		},
	})

	return session, nil
}
