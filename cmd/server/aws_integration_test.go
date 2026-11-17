package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/approval"
	"github.com/devanshbaghel18/ONUSLY/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/internal/chat"
	"github.com/devanshbaghel18/ONUSLY/internal/enforcement"
	"github.com/devanshbaghel18/ONUSLY/internal/friends"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/notifications"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

// TestAWS_IdentityAndCredentials verifies that AWS credentials are valid, active,
// and correctly resolve caller identity.
func TestAWS_IdentityAndCredentials(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("eu-north-1"))
	if err != nil {
		t.Fatalf("Failed to load AWS configuration: %v", err)
	}

	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		t.Skipf("AWS STS GetCallerIdentity failed: %v. Skipping live AWS test.", err)
	}

	if identity.Account == nil || *identity.Account == "" {
		t.Errorf("expected non-empty AWS account ID")
	}
	if identity.Arn == nil || *identity.Arn == "" {
		t.Errorf("expected non-empty AWS caller ARN")
	}

	t.Logf("[AWS Identity Verified] Account: %s, ARN: %s, UserId: %s",
		*identity.Account, *identity.Arn, *identity.UserId)
}

// TestAWS_DynamoDB_TableAndIndexes verifies that the Onusly DynamoDB table exists,
// is ACTIVE, and possesses the required Primary Key and Global Secondary Indexes (GSI1, GSI2).
func TestAWS_DynamoDB_TableAndIndexes(t *testing.T) {
	db := shared.NewDynamoClient()
	tableName := "Onusly"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	desc, err := db.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		t.Skipf("DynamoDB table %q is not accessible (%v). Skipping live AWS test.", tableName, err)
	}

	if desc.Table.TableStatus != types.TableStatusActive {
		t.Fatalf("expected table status ACTIVE, got %v", desc.Table.TableStatus)
	}

	// Verify Primary Key Schema
	pkFound, skFound := false, false
	for _, k := range desc.Table.KeySchema {
		if *k.AttributeName == "PK" && k.KeyType == types.KeyTypeHash {
			pkFound = true
		}
		if *k.AttributeName == "SK" && k.KeyType == types.KeyTypeRange {
			skFound = true
		}
	}
	if !pkFound || !skFound {
		t.Errorf("expected PK (HASH) and SK (RANGE), got %+v", desc.Table.KeySchema)
	}

	// Verify GSI1 (Email index) and GSI2 (Approver index)
	gsi1Found, gsi2Found := false, false
	for _, gsi := range desc.Table.GlobalSecondaryIndexes {
		if *gsi.IndexName == "GSI1" {
			if gsi.IndexStatus == types.IndexStatusActive {
				gsi1Found = true
			}
		}
		if *gsi.IndexName == "GSI2" {
			if gsi.IndexStatus == types.IndexStatusActive {
				gsi2Found = true
			}
		}
	}

	if !gsi1Found {
		t.Errorf("GSI1 index not found or not ACTIVE")
	}
	if !gsi2Found {
		t.Errorf("GSI2 index not found or not ACTIVE")
	}

	t.Logf("[DynamoDB Table Verified] %s (Status: %s, ItemCount: %d, GSI1: ACTIVE, GSI2: ACTIVE)",
		tableName, desc.Table.TableStatus, desc.Table.ItemCount)
}

// TestAWS_DynamoDB_CompleteSystemLifecycle exercises every single AWS entity lifecycle:
// Auth -> Friends -> Goals -> Proof -> Approval -> Notifications -> Enforcement -> Chat.
func TestAWS_DynamoDB_CompleteSystemLifecycle(t *testing.T) {
	db := shared.NewDynamoClient()
	tableName := "Onusly"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := db.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}); err != nil {
		t.Skipf("DynamoDB table %q is not accessible (%v). Skipping live AWS test.", tableName, err)
	}

	// 1. Setup unique test participants
	ownerID := "e2e-owner-" + uuid.NewString()
	ownerEmail := "owner-" + uuid.NewString() + "@example.com"
	ownerHandle := "own" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]

	approverID := "e2e-approver-" + uuid.NewString()
	approverEmail := "approver-" + uuid.NewString() + "@example.com"
	approverHandle := "appr" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]

	goalID := uuid.NewString()
	proofID := uuid.NewString()
	approvalID := uuid.NewString()
	deviceID := uuid.NewString()
	pairCode := "PAIR" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
	commID := "comm-e2e-" + uuid.NewString()

	// Register comprehensive cleanup for every created item
	t.Cleanup(func() {
		cleanupCtx, cCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cCancel()

		// Delete Goal, Proof, Approval
		_, _ = db.DeleteItem(cleanupCtx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
				"SK": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
			},
		})
		_, _ = db.DeleteItem(cleanupCtx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
				"SK": &types.AttributeValueMemberS{Value: "PROOF#" + goalID + "#" + proofID},
			},
		})
		_, _ = db.DeleteItem(cleanupCtx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
				"SK": &types.AttributeValueMemberS{Value: "APPROVAL#" + goalID + "#" + approvalID},
			},
		})

		// Delete Owner and Approver user partitions
		for _, uid := range []string{ownerID, approverID} {
			out, err := db.Query(cleanupCtx, &dynamodb.QueryInput{
				TableName:              aws.String(tableName),
				KeyConditionExpression: aws.String("PK = :pk"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":pk": &types.AttributeValueMemberS{Value: "USER#" + uid},
				},
			})
			if err == nil && out != nil {
				for _, it := range out.Items {
					if sk, ok := it["SK"].(*types.AttributeValueMemberS); ok {
						_, _ = db.DeleteItem(cleanupCtx, &dynamodb.DeleteItemInput{
							TableName: aws.String(tableName),
							Key: map[string]types.AttributeValue{
								"PK": &types.AttributeValueMemberS{Value: "USER#" + uid},
								"SK": sk,
							},
						})
					}
				}
			}
		}

		// Delete Handles
		for _, h := range []string{ownerHandle, approverHandle} {
			_, _ = db.DeleteItem(cleanupCtx, &dynamodb.DeleteItemInput{
				TableName: aws.String(tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: "HANDLE#" + h},
					"SK": &types.AttributeValueMemberS{Value: "RESERVATION"},
				},
			})
		}

		// Delete Pairing Session
		_, _ = db.DeleteItem(cleanupCtx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "PAIR#" + pairCode},
				"SK": &types.AttributeValueMemberS{Value: "SESSION"},
			},
		})
	})

	// ==========================================
	// 2. AUTH: Create Users & Handle Reservations
	// ==========================================
	authRepo := auth.NewRepositoryWithTable(db, tableName)

	err := authRepo.CreateUser(ctx, &auth.User{
		ID:        ownerID,
		Email:     ownerEmail,
		Handle:    ownerHandle,
		Name:      "E2E Owner",
		Picture:   "https://example.com/owner.png",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("[AUTH] CreateUser for owner failed: %v", err)
	}

	err = authRepo.CreateUser(ctx, &auth.User{
		ID:        approverID,
		Email:     approverEmail,
		Handle:    approverHandle,
		Name:      "E2E Approver",
		Picture:   "https://example.com/approver.png",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("[AUTH] CreateUser for approver failed: %v", err)
	}

	// Verify GSI1 index lookup by Email
	coldAuth := auth.NewRepositoryWithTable(db, tableName)
	fetchedOwner, err := coldAuth.GetUserByEmail(ctx, ownerEmail)
	if err != nil || fetchedOwner == nil || fetchedOwner.ID != ownerID {
		t.Fatalf("[AUTH GSI1] GetUserByEmail failed to find owner: %v", err)
	}

	// ==========================================
	// 3. FRIENDS: Send & Accept Friend Request
	// ==========================================
	friendsRepo := friends.NewRepository(db, tableName)
	fReqID := uuid.NewString()

	err = friendsRepo.SaveFriendRequest(ctx, friends.FriendRequest{
		ID:              fReqID,
		SenderID:        ownerID,
		SenderEmail:     ownerEmail,
		SenderHandle:    ownerHandle,
		SenderName:      "E2E Owner",
		RecipientID:     approverID,
		RecipientEmail:  approverEmail,
		RecipientHandle: approverHandle,
		RecipientName:   "E2E Approver",
		Status:          "pending",
		CreatedAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("[FRIENDS] SaveFriendRequest failed: %v", err)
	}

	coldFriends := friends.NewRepository(db, tableName)
	reqs, err := coldFriends.GetIncomingRequests(ctx, approverID)
	if err != nil || len(reqs) == 0 {
		t.Fatalf("[FRIENDS] GetIncomingRequests failed: %v", err)
	}

	err = friendsRepo.AcceptFriendRequest(ctx, reqs[0])
	if err != nil {
		t.Fatalf("[FRIENDS] AcceptFriendRequest failed: %v", err)
	}

	// Verify mutual friendship
	fList, err := coldFriends.GetFriends(ctx, ownerID)
	if err != nil || len(fList) == 0 || fList[0].FriendID != approverID {
		t.Fatalf("[FRIENDS] Expected approver in owner's friends list, got %+v", fList)
	}

	// ==========================================
	// 4. GOALS: Create with GSI2 Approver Index
	// ==========================================
	goalRepo := goals.NewRepository(db, tableName)

	testGoal := goals.Goal{
		ID:             goalID,
		OwnerID:        ownerID,
		Title:          "Read AWS Architecture Docs",
		Description:    "Complete AWS Well-Architected Framework review",
		Status:         "active",
		ApprovalType:   "friend",
		ApproverID:     approverID,
		ApproverEmail:  approverEmail,
		BlockedApps:    []string{"com.instagram.android"},
		BlockedDomains: []string{"instagram.com"},
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	if err := goalRepo.Create(ctx, testGoal); err != nil {
		t.Fatalf("[GOALS] Create goal failed: %v", err)
	}

	// Verify GSI2 query for Approver Inbox
	coldGoal := goals.NewRepository(db, tableName)
	approverGoals, err := coldGoal.ListByApprover(ctx, approverID, approverEmail, approverHandle, "active")
	if err != nil {
		t.Fatalf("[GOALS GSI2] ListByApprover failed: %v", err)
	}
	gFound := false
	for _, g := range approverGoals {
		if g.ID == goalID {
			gFound = true
			break
		}
	}
	if !gFound {
		t.Fatalf("[GOALS GSI2] Created goal was not found in approver inbox query")
	}

	// ==========================================
	// 5. PROOF: Submit Proof & Transition Status
	// ==========================================
	proofRepo := proof.NewRepository(db, tableName)

	testProof := proof.Proof{
		ID:              proofID,
		GoalID:          goalID,
		OwnerID:         ownerID,
		ProofType:       "text",
		TextExplanation: "Finished chapters 1 through 5 on AWS Security & DynamoDB modeling.",
		SubmittedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	if err := proofRepo.Create(ctx, testProof); err != nil {
		t.Fatalf("[PROOF] Create proof failed: %v", err)
	}

	// Transition goal status to proof_submitted
	_, err = goalRepo.UpdateStatus(ctx, ownerID, goalID, "proof_submitted")
	if err != nil {
		t.Fatalf("[GOALS] UpdateStatus to proof_submitted failed: %v", err)
	}

	// ==========================================
	// 6. APPROVAL: Approve Proof & Complete Goal
	// ==========================================
	approvalRepo := approval.NewRepository(db, tableName)

	testApproval := approval.Approval{
		ID:         approvalID,
		GoalID:     goalID,
		ProofID:    proofID,
		OwnerID:    ownerID,
		ApproverID: approverID,
		Status:     "approved",
		Comment:    "Well done on the AWS DynamoDB modeling!",
		DecidedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	if err := approvalRepo.Create(ctx, testApproval); err != nil {
		t.Fatalf("[APPROVAL] Create approval failed: %v", err)
	}

	_, err = goalRepo.UpdateStatus(ctx, ownerID, goalID, "completed")
	if err != nil {
		t.Fatalf("[GOALS] UpdateStatus to completed failed: %v", err)
	}

	// Verify goal is completed
	completedGoal, err := coldGoal.GetByID(ctx, ownerID, goalID)
	if err != nil || completedGoal == nil || completedGoal.Status != "completed" {
		t.Fatalf("[GOALS] Expected goal status completed, got %+v", completedGoal)
	}

	// ==========================================
	// 7. NOTIFICATIONS: Dispatch & Mark As Read
	// ==========================================
	notifRepo := notifications.NewRepository(db, tableName)
	notifID := uuid.NewString()

	err = notifRepo.Create(ctx, &notifications.Notification{
		ID:          notifID,
		RecipientID: ownerID,
		Type:        "approval_decided",
		Title:       "Goal Approved!",
		Message:     "Your accountability partner approved your proof.",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Read:        false,
	})
	if err != nil {
		t.Fatalf("[NOTIFICATIONS] Create notification failed: %v", err)
	}

	notifs, err := notifRepo.List(ctx, ownerID, 10)
	if err != nil || len(notifs) == 0 {
		t.Fatalf("[NOTIFICATIONS] List notifications failed: %v", err)
	}

	if err := notifRepo.MarkAsRead(ctx, ownerID, notifID); err != nil {
		t.Fatalf("[NOTIFICATIONS] MarkAsRead failed: %v", err)
	}

	// ==========================================
	// 8. ENFORCEMENT: Device Registration & Revocation
	// ==========================================
	enforcementRepo := enforcement.NewDynamoRepository(db, tableName)

	// Pairing session
	err = enforcementRepo.SavePairingSession(ctx, enforcement.PairingSession{
		Code:      pairCode,
		Platform:  "linux",
		ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("[ENFORCEMENT] SavePairingSession failed: %v", err)
	}

	// Device
	nowStr := time.Now().UTC().Format(time.RFC3339)
	err = enforcementRepo.UpsertDevice(ctx, enforcement.Device{
		DeviceID:          deviceID,
		UserID:            ownerID,
		Platform:          "linux",
		ClientType:        "desktop_agent",
		Status:            enforcement.DeviceStatusPaired,
		Enforcing:         true,
		LastHeartbeatAt:   nowStr,
		CreatedAt:         nowStr,
		UpdatedAt:         nowStr,
		CredentialVersion: 1,
	})
	if err != nil {
		t.Fatalf("[ENFORCEMENT] UpsertDevice failed: %v", err)
	}

	coldEnf := enforcement.NewDynamoRepository(db, tableName)
	dList, err := coldEnf.ListDevices(ctx, ownerID)
	if err != nil || len(dList) == 0 {
		t.Fatalf("[ENFORCEMENT] ListDevices failed: %v", err)
	}

	// ==========================================
	// 9. CHAT: 2-Party Atomic Messaging
	// ==========================================
	chatRepo := chat.NewRepository(db, tableName)
	msgID := uuid.NewString()

	err = chatRepo.SaveMessage(ctx, realtime.StoredMessage{
		ID:             msgID,
		SenderID:       ownerID,
		SenderEmail:    ownerEmail,
		RecipientID:    approverID,
		RecipientEmail: approverEmail,
		Text:           "Thank you for approving my proof!",
		Time:           time.Now().UTC().Format(time.RFC3339),
	}, true)
	if err != nil {
		t.Fatalf("[CHAT] SaveMessage failed: %v", err)
	}

	coldChat := chat.NewRepository(db, tableName)
	history, err := coldChat.GetConversationHistory(ctx, ownerID, approverID, 10)
	if err != nil || len(history) == 0 {
		t.Fatalf("[CHAT] GetConversationHistory failed: %v", err)
	}

	t.Logf("=== ALL AWS DYNAMODB WORKFLOWS VERIFIED SUCCESSFULLY ===")
	_ = commID
}
