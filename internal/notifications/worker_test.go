package notifications_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/events"
	"github.com/devanshbaghel18/ONUSLY/internal/notifications"
	"github.com/devanshbaghel18/ONUSLY/internal/queue"
)

func TestWorker_OutageRecoveryAndDeduplication(t *testing.T) {
	db, tableName := getTestDynamo(t)
	repo := notifications.NewRepository(db, tableName)
	service := notifications.NewService(repo)
	memQueue := queue.NewMemoryQueue()

	proofQ := "proof-events"
	appQ := "approval-events"

	worker := notifications.NewWorker(memQueue, service, proofQ, appQ, 50*time.Millisecond)

	approverID := "approver-" + uuid.NewString()
	ownerID := "owner-" + uuid.NewString()
	proofID := uuid.NewString()
	eventID := "proof-" + proofID

	proofEv := events.ProofSubmittedEvent{
		EventID:     eventID,
		GoalID:      "goal-test",
		ProofID:     proofID,
		OwnerID:     ownerID,
		ApproverID:  approverID,
		Title:       "Outage Goal",
		SubmittedAt: time.Now().UTC(),
	}
	evBytes, _ := json.Marshal(proofEv)

	// -------------------------------------------------------------
	// 1. OUTAGE TEST: Worker is STOPPED. Publish message to queue.
	// -------------------------------------------------------------
	err := memQueue.Publish(context.Background(), proofQ, evBytes)
	if err != nil {
		t.Fatalf("failed to publish to queue: %v", err)
	}

	// Confirm message sits in queue
	if memQueue.Length(proofQ) != 1 {
		t.Fatalf("expected 1 message in queue, got %d", memQueue.Length(proofQ))
	}

	// Confirm DynamoDB does NOT yet have the notification
	res, err := service.List(context.Background(), approverID, 10)
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if len(res.Notifications) != 0 {
		t.Fatalf("expected 0 notifications before worker runs, got %d", len(res.Notifications))
	}

	// -------------------------------------------------------------
	// 2. RESUME WORKER: Worker drains backlog and persists notification
	// -------------------------------------------------------------
	processed := worker.ProcessOnce(context.Background())
	if processed != 1 {
		t.Fatalf("expected worker to process 1 message, got %d", processed)
	}

	// Queue must now be empty
	if memQueue.Length(proofQ) != 0 {
		t.Fatalf("expected queue to be empty after processing, got %d", memQueue.Length(proofQ))
	}

	// DynamoDB must now have exactly 1 notification
	res, err = service.List(context.Background(), approverID, 10)
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if len(res.Notifications) != 1 {
		t.Fatalf("expected 1 notification in DynamoDB, got %d", len(res.Notifications))
	}
	if res.Notifications[0].ID != eventID {
		t.Errorf("expected notification ID %q, got %q", eventID, res.Notifications[0].ID)
	}

	// -------------------------------------------------------------
	// 3. DEDUPLICATION TEST: Replay same message multiple times
	// -------------------------------------------------------------
	_ = memQueue.Publish(context.Background(), proofQ, evBytes)
	_ = memQueue.Publish(context.Background(), proofQ, evBytes)

	dupProcessed := worker.ProcessOnce(context.Background())
	if dupProcessed != 2 {
		t.Fatalf("expected worker to process 2 duplicate messages, got %d", dupProcessed)
	}

	// Queue drained
	if memQueue.Length(proofQ) != 0 {
		t.Fatalf("expected queue to be empty, got %d", memQueue.Length(proofQ))
	}

	// DynamoDB STILL has strictly 1 notification (no duplicates!)
	res, _ = service.List(context.Background(), approverID, 10)
	if len(res.Notifications) != 1 {
		t.Fatalf("expected strictly 1 notification after duplicates, got %d", len(res.Notifications))
	}
}

func TestWorker_PoisonPillSafety(t *testing.T) {
	db, tableName := getTestDynamo(t)
	repo := notifications.NewRepository(db, tableName)
	service := notifications.NewService(repo)
	memQueue := queue.NewMemoryQueue()

	proofQ := "proof-events"
	appQ := "approval-events"

	worker := notifications.NewWorker(memQueue, service, proofQ, appQ, 50*time.Millisecond)

	// Publish malformed / invalid JSON
	_ = memQueue.Publish(context.Background(), proofQ, []byte("{not-valid-json!!"))

	// Worker should safely drop the poison pill and not crash
	processed := worker.ProcessOnce(context.Background())
	if processed != 0 {
		t.Errorf("expected 0 valid processed, got %d", processed)
	}

	// Queue must be deleted/cleared of poison pill
	if memQueue.Length(proofQ) != 0 {
		t.Errorf("expected poison pill to be deleted from queue, length is %d", memQueue.Length(proofQ))
	}
}

func TestWorker_GracefulShutdown(t *testing.T) {
	db, tableName := getTestDynamo(t)
	repo := notifications.NewRepository(db, tableName)
	service := notifications.NewService(repo)
	memQueue := queue.NewMemoryQueue()

	worker := notifications.NewWorker(memQueue, service, "proofQ", "appQ", 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker.Start(ctx)
	if !worker.IsRunning() {
		t.Fatalf("expected worker to be running")
	}

	time.Sleep(50 * time.Millisecond)
	worker.Stop()

	if worker.IsRunning() {
		t.Fatalf("expected worker to be stopped")
	}
}
