package shared_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

func TestExecuteWithRetry_SuccessFirstAttempt(t *testing.T) {
	var calls int32
	err := shared.ExecuteWithRetry(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}, shared.DefaultRetryConfig())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestExecuteWithRetry_SuccessAfterTransientFailures(t *testing.T) {
	var calls int32
	cfg := shared.RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
		BackoffFactor:  2.0,
		Jitter:         false,
	}

	err := shared.ExecuteWithRetry(context.Background(), func(ctx context.Context) error {
		c := atomic.AddInt32(&calls, 1)
		if c < 3 {
			return &types.ProvisionedThroughputExceededException{
				Message: aws.String("Rate exceeded"),
			}
		}
		return nil
	}, cfg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestExecuteWithRetry_NonRetryableFailsFast(t *testing.T) {
	var calls int32
	cfg := shared.DefaultRetryConfig()

	condErr := &types.ConditionalCheckFailedException{
		Message: aws.String("Condition check failed"),
	}

	err := shared.ExecuteWithRetry(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return condErr
	}, cfg)

	if !errors.Is(err, condErr) {
		t.Fatalf("expected conditional check error, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call (no retries for non-retryable error), got %d", calls)
	}
}

func TestExecuteWithRetry_ExhaustionReturnsError(t *testing.T) {
	var calls int32
	cfg := shared.RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
		Jitter:         false,
	}

	transientErr := &types.InternalServerError{
		Message: aws.String("Internal server error"),
	}

	err := shared.ExecuteWithRetry(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return transientErr
	}, cfg)

	if !errors.Is(err, shared.ErrRetryExhausted) {
		t.Fatalf("expected ErrRetryExhausted, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestExecuteWithRetry_ContextCancellationAborts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls int32
	cfg := shared.RetryConfig{
		MaxAttempts:    5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     500 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	transientErr := &types.InternalServerError{
		Message: aws.String("Server error"),
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := shared.ExecuteWithRetry(ctx, func(c context.Context) error {
		atomic.AddInt32(&calls, 1)
		return transientErr
	}, cfg)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if calls > 2 {
		t.Fatalf("expected loop to terminate promptly on context cancellation, got %d calls", calls)
	}
}

func TestWithBoundedTimeout(t *testing.T) {
	// Parent without deadline gets bounded timeout
	ctx, cancel := shared.WithBoundedTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected context to have a deadline")
	}
	if time.Until(deadline) > 60*time.Millisecond {
		t.Fatalf("expected deadline <= 60ms, got %v", time.Until(deadline))
	}

	// Parent with tighter deadline preserves tighter deadline
	tightParent, tightCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer tightCancel()

	ctx2, cancel2 := shared.WithBoundedTimeout(tightParent, 100*time.Millisecond)
	defer cancel2()

	deadline2, _ := ctx2.Deadline()
	if time.Until(deadline2) > 30*time.Millisecond {
		t.Fatalf("expected tight deadline preserved, got %v", time.Until(deadline2))
	}
}
