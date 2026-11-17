package queue_test

import (
	"context"
	"strings"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/queue"
	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

func TestSQSQueue_ClientInitializationAndURLResolution(t *testing.T) {
	sqsClient := shared.NewSQSClient()
	if sqsClient == nil {
		t.Skip("AWS SQS client could not be created from environment. Skipping.")
	}

	queueMap := map[string]string{
		"proof-events":    "https://sqs.eu-north-1.amazonaws.com/123456789012/proof-events",
		"approval-events": "https://sqs.eu-north-1.amazonaws.com/123456789012/approval-events",
	}

	sqsQ := queue.NewSQSQueue(sqsClient, queueMap)

	ctx := context.Background()

	// 1. Unmapped queue name should fail fast with descriptive error
	err := sqsQ.Publish(ctx, "unconfigured-queue", []byte(`{}`))
	if err == nil {
		t.Fatalf("expected error for unconfigured queue URL, got nil")
	}
	if !strings.Contains(err.Error(), "no SQS queue URL configured") {
		t.Errorf("expected 'no SQS queue URL configured' error, got %v", err)
	}

	_, err = sqsQ.Receive(ctx, "unconfigured-queue", 10)
	if err == nil {
		t.Fatalf("expected error for unconfigured queue URL, got nil")
	}

	err = sqsQ.Delete(ctx, "unconfigured-queue", "receipt-123")
	if err == nil {
		t.Fatalf("expected error for unconfigured queue URL, got nil")
	}

	// 2. Direct HTTPS URL resolution
	// Even if not in map, direct HTTPS URLs are passed to the AWS SQS SDK
	// This will attempt network call and return an AWS error (e.g. NonExistentQueue),
	// proving that the SQS client is correctly dispatched.
	err = sqsQ.Publish(ctx, "https://sqs.eu-north-1.amazonaws.com/631690098190/nonexistent-test-queue", []byte(`{"test":true}`))
	if err == nil {
		t.Fatalf("expected AWS SQS error for nonexistent queue URL, got nil")
	}
	// AWS error returned should be NonExistentQueue or AccessDenied, not local config error
	if strings.Contains(err.Error(), "no SQS queue URL configured") {
		t.Errorf("direct URL was not resolved, got local error: %v", err)
	}
}
