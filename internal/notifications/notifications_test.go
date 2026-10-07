package notifications_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/events"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/notifications"
	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

func getTestDynamo(t *testing.T) (*dynamodb.Client, string) {
	t.Helper()
	tableName := "Onusly"
	db := shared.NewDynamoClient()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		t.Skipf("DynamoDB table %q not accessible: %v. Skipping.", tableName, err)
	}

	return db, tableName
}

func TestNotifications_SingleTableAndDeduplication(t *testing.T) {
	db, tableName := getTestDynamo(t)
	repo := notifications.NewRepository(db, tableName)
	service := notifications.NewService(repo)
	ctx := context.Background()

	recipientID := "user-notif-" + uuid.NewString()
	eventID := "proof-" + uuid.NewString()

	ev := events.ProofSubmittedEvent{
		EventID:     eventID,
		GoalID:      "goal-xyz",
		ProofID:     "proof-xyz",
		OwnerID:     "owner-abc",
		ApproverID:  recipientID,
		Title:       "Ship Milestone 1",
		SubmittedAt: time.Now().UTC(),
	}

	// 1. First processing -> successfully creates notification
	err := service.CreateFromProofSubmitted(ctx, ev)
	if err != nil {
		t.Fatalf("unexpected error creating notification: %v", err)
	}

	// 2. Query list -> exactly 1 notification
	res, err := service.List(ctx, recipientID, 10)
	if err != nil {
		t.Fatalf("failed to list notifications: %v", err)
	}
	if len(res.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(res.Notifications))
	}
	if res.UnreadCount != 1 {
		t.Errorf("expected 1 unread notification, got %d", res.UnreadCount)
	}
	if res.Notifications[0].ID != eventID {
		t.Errorf("expected ID %q, got %q", eventID, res.Notifications[0].ID)
	}

	// 3. Repeated delivery of same event (idempotency check) -> treated as success
	err = service.CreateFromProofSubmitted(ctx, ev)
	if err != nil {
		t.Fatalf("expected duplicate delivery to succeed idempotently, got %v", err)
	}

	// Confirm list still contains exactly 1 notification
	res, err = service.List(ctx, recipientID, 10)
	if err != nil {
		t.Fatalf("failed to list notifications: %v", err)
	}
	if len(res.Notifications) != 1 {
		t.Fatalf("expected strictly 1 notification after duplicate, got %d", len(res.Notifications))
	}
}

func TestNotifications_MarkAsReadAndCrossUserIsolation(t *testing.T) {
	db, tableName := getTestDynamo(t)
	repo := notifications.NewRepository(db, tableName)
	service := notifications.NewService(repo)
	handler := notifications.NewHandler(service)
	ctx := context.Background()

	userA := "user-A-" + uuid.NewString()
	userB := "user-B-" + uuid.NewString()
	eventID_A := "approval-" + uuid.NewString()

	// Create notification for User A
	_ = service.CreateFromApprovalDecided(ctx, events.ApprovalDecidedEvent{
		EventID:    eventID_A,
		GoalID:     "goal-a",
		ProofID:    "proof-a",
		OwnerID:    userA,
		ApproverID: "approver-x",
		Status:     "approved",
		Title:      "Goal A",
		DecidedAt:  time.Now().UTC(),
	})

	// User B lists notifications -> must see 0 notifications (Cross-user isolation)
	resB, err := service.List(ctx, userB, 10)
	if err != nil {
		t.Fatalf("unexpected error listing for user B: %v", err)
	}
	if len(resB.Notifications) != 0 {
		t.Errorf("user B must not see user A's notifications, found %d", len(resB.Notifications))
	}

	// User B attempts to mark User A's notification as read via HTTP endpoint -> 404 Not Found
	req := httptest.NewRequest("PATCH", "/notifications/"+eventID_A+"/read", nil)
	req.SetPathValue("id", eventID_A)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userB))
	rec := httptest.NewRecorder()

	handler.MarkAsRead(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when user B tries to mark user A's notification, got %d", rec.Code)
	}

	// User A marks their own notification as read -> 200 OK
	reqA := httptest.NewRequest("PATCH", "/notifications/"+eventID_A+"/read", nil)
	reqA.SetPathValue("id", eventID_A)
	reqA = reqA.WithContext(context.WithValue(reqA.Context(), middleware.UserIDKey, userA))
	recA := httptest.NewRecorder()

	handler.MarkAsRead(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Fatalf("expected 200 for user A, got %d", recA.Code)
	}

	// Verify User A unread count is now 0
	resA, _ := service.List(ctx, userA, 10)
	if resA.UnreadCount != 0 {
		t.Errorf("expected 0 unread for user A, got %d", resA.UnreadCount)
	}
	if !resA.Notifications[0].Read {
		t.Errorf("expected notification to be marked as read")
	}
}
