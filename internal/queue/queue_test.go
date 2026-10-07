package queue_test

import (
	"context"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/queue"
)

func TestMemoryQueue_BasicPublishReceiveDelete(t *testing.T) {
	q := queue.NewMemoryQueue()
	ctx := context.Background()
	qName := "test-queue"

	// Initially empty
	msgs, err := q.Receive(ctx, qName, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}

	// Publish message
	payload := []byte(`{"hello":"world"}`)
	err = q.Publish(ctx, qName, payload)
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	if q.Length(qName) != 1 {
		t.Fatalf("expected queue length 1, got %d", q.Length(qName))
	}

	// Receive message
	msgs, err = q.Receive(ctx, qName, 10)
	if err != nil {
		t.Fatalf("failed to receive: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if string(msgs[0].Body) != string(payload) {
		t.Fatalf("payload mismatch: %s", string(msgs[0].Body))
	}

	// Delete message
	err = q.Delete(ctx, qName, msgs[0].ReceiptHandle)
	if err != nil {
		t.Fatalf("failed to delete message: %v", err)
	}
	if q.Length(qName) != 0 {
		t.Fatalf("expected queue length 0 after delete, got %d", q.Length(qName))
	}
}

func TestMemoryQueue_PauseAndResumeOutage(t *testing.T) {
	q := queue.NewMemoryQueue()
	ctx := context.Background()
	qName := "outage-queue"

	// 1. Simulate worker outage by pausing the queue
	q.Pause(qName)
	if !q.IsPaused(qName) {
		t.Fatalf("expected queue to be paused")
	}

	// 2. Publish during outage
	_ = q.Publish(ctx, qName, []byte("event-1"))
	_ = q.Publish(ctx, qName, []byte("event-2"))

	if q.Length(qName) != 2 {
		t.Fatalf("expected 2 messages accumulated in queue, got %d", q.Length(qName))
	}

	// 3. Receive while paused returns empty (simulating worker unreachable / down)
	msgs, err := q.Receive(ctx, qName, 10)
	if err != nil {
		t.Fatalf("unexpected receive error: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages returned while paused, got %d", len(msgs))
	}

	// 4. Resume queue (worker recovers)
	q.Resume(qName)
	if q.IsPaused(qName) {
		t.Fatalf("expected queue to be unpaused")
	}

	// 5. Backlog drains in FIFO order
	msgs, err = q.Receive(ctx, qName, 10)
	if err != nil {
		t.Fatalf("unexpected receive error after resume: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages drained, got %d", len(msgs))
	}
	if string(msgs[0].Body) != "event-1" || string(msgs[1].Body) != "event-2" {
		t.Fatalf("unexpected message order: %s, %s", string(msgs[0].Body), string(msgs[1].Body))
	}
}
