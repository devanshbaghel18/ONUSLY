package enforcement

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

func getTestEnforcementDynamoClient(t *testing.T) (*dynamodb.Client, string) {
	t.Helper()
	tableName := "Onusly"
	db := shared.NewDynamoClient()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		t.Skipf("DynamoDB table %q is not accessible (%v). Skipping live test.", tableName, err)
	}

	return db, tableName
}

func cleanupEnforcementItems(t *testing.T, db *dynamodb.Client, tableName, userID string, deviceIDs []string, pairingCodes []string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for _, devID := range deviceIDs {
			_, _ = db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: aws.String(tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
					"SK": &types.AttributeValueMemberS{Value: "DEVICE#" + devID},
				},
			})
		}

		for _, code := range pairingCodes {
			_, _ = db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: aws.String(tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "PAIR#" + code},
					"SK": &types.AttributeValueMemberS{Value: "SESSION"},
				},
			})
		}
	})
}

// TestEnforcement_Live_DeviceLifecycle tests UpsertDevice, GetDevice, ListDevices, UpdateHeartbeat,
// and RevokeDevice directly on real AWS DynamoDB.
func TestEnforcement_Live_DeviceLifecycle(t *testing.T) {
	db, tableName := getTestEnforcementDynamoClient(t)

	repo := NewDynamoRepository(db, tableName)

	userID := "test-user-" + uuid.NewString()
	deviceID := uuid.NewString()

	cleanupEnforcementItems(t, db, tableName, userID, []string{deviceID}, nil)

	ctx := context.Background()

	// 1. UpsertDevice
	now := time.Now().UTC().Format(time.RFC3339)
	dev := Device{
		DeviceID:          deviceID,
		UserID:            userID,
		Platform:          "linux",
		ClientType:        "desktop_agent",
		ClientVersion:     "1.0.0",
		Status:            DeviceStatusPaired,
		Enforcing:         true,
		LastHeartbeatAt:   now,
		CreatedAt:         now,
		UpdatedAt:         now,
		CredentialVersion: 1,
	}

	if err := repo.UpsertDevice(ctx, dev); err != nil {
		t.Fatalf("UpsertDevice failed on live DynamoDB: %v", err)
	}

	// Read on cold instance to bypass in-memory map
	coldRepo := NewDynamoRepository(db, tableName)

	// 2. GetDevice
	fetched, err := coldRepo.GetDevice(ctx, userID, deviceID)
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	if fetched == nil || fetched.DeviceID != deviceID {
		t.Fatalf("expected device %s, got %+v", deviceID, fetched)
	}
	if fetched.Platform != "linux" || fetched.Status != DeviceStatusPaired {
		t.Errorf("unexpected device data: %+v", fetched)
	}

	// 3. ListDevices
	devices, err := coldRepo.ListDevices(ctx, userID)
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) != 1 || devices[0].DeviceID != deviceID {
		t.Fatalf("expected 1 device with ID %s, got %d devices", deviceID, len(devices))
	}

	// 4. UpdateHeartbeat
	newHeartbeat := time.Now().UTC().Format(time.RFC3339)
	dev.LastHeartbeatAt = newHeartbeat
	dev.BlocklistVersion = "v2"
	if err := repo.UpdateHeartbeat(ctx, dev); err != nil {
		t.Fatalf("UpdateHeartbeat failed: %v", err)
	}

	coldRepo2 := NewDynamoRepository(db, tableName)
	fetchedAfterHB, err := coldRepo2.GetDevice(ctx, userID, deviceID)
	if err != nil || fetchedAfterHB == nil {
		t.Fatalf("GetDevice after heartbeat failed: %v", err)
	}
	if fetchedAfterHB.BlocklistVersion != "v2" {
		t.Errorf("expected blocklist version v2, got %s", fetchedAfterHB.BlocklistVersion)
	}

	// 5. RevokeDevice
	if err := repo.RevokeDevice(ctx, userID, deviceID, "admin_action"); err != nil {
		t.Fatalf("RevokeDevice failed: %v", err)
	}

	coldRepo3 := NewDynamoRepository(db, tableName)
	fetchedRevoked, err := coldRepo3.GetDevice(ctx, userID, deviceID)
	if err != nil || fetchedRevoked == nil {
		t.Fatalf("GetDevice after revoke failed: %v", err)
	}
	if fetchedRevoked.Status != DeviceStatusRevoked {
		t.Errorf("expected status revoked, got %s", fetchedRevoked.Status)
	}

	// 6. Conditional Heartbeat rejection on revoked device
	err = repo.UpdateHeartbeat(ctx, dev)
	if err == nil {
		t.Errorf("expected UpdateHeartbeat to be rejected for revoked device, got nil")
	}
}

// TestEnforcement_Live_PairingSessionLifecycle tests SavePairingSession, GetPairingSession,
// ConfirmPairingSession, and ConsumePairingSession on live DynamoDB.
func TestEnforcement_Live_PairingSessionLifecycle(t *testing.T) {
	db, tableName := getTestEnforcementDynamoClient(t)

	repo := NewDynamoRepository(db, tableName)

	code := "PAIR" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
	userID := "test-user-" + uuid.NewString()
	token := "jwt-pair-token-" + uuid.NewString()

	cleanupEnforcementItems(t, db, tableName, userID, nil, []string{code})

	ctx := context.Background()

	// 1. SavePairingSession
	sess := PairingSession{
		Code:      code,
		Platform:  "linux",
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err := repo.SavePairingSession(ctx, sess); err != nil {
		t.Fatalf("SavePairingSession failed: %v", err)
	}

	// 2. GetPairingSession on cold instance
	coldRepo := NewDynamoRepository(db, tableName)
	fetchedSess, err := coldRepo.GetPairingSession(ctx, code)
	if err != nil {
		t.Fatalf("GetPairingSession failed: %v", err)
	}
	if fetchedSess.Code != code || fetchedSess.Platform != "linux" {
		t.Errorf("unexpected session data: %+v", fetchedSess)
	}

	// 3. ConfirmPairingSession
	if err := repo.ConfirmPairingSession(ctx, code, userID, token); err != nil {
		t.Fatalf("ConfirmPairingSession failed: %v", err)
	}

	// 4. ConsumePairingSession
	coldRepo2 := NewDynamoRepository(db, tableName)
	consumed, err := coldRepo2.ConsumePairingSession(ctx, code)
	if err != nil {
		t.Fatalf("ConsumePairingSession failed: %v", err)
	}
	if consumed == nil || consumed.ConfirmedBy != userID || consumed.Token != token {
		t.Errorf("unexpected consumed session: %+v", consumed)
	}

	// 5. Consuming again must fail
	_, err = coldRepo2.ConsumePairingSession(ctx, code)
	if err == nil {
		t.Errorf("expected second ConsumePairingSession to fail, got nil")
	}
}
