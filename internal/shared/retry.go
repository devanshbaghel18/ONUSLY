package shared

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
	// ErrRetryExhausted is returned when all retry attempts fail.
	ErrRetryExhausted = errors.New("maximum retry attempts exhausted")
	// ErrNonRetryable is returned or wrapped when an error is strictly not retryable.
	ErrNonRetryable = errors.New("non-retryable operation failure")
)

// RetryConfig defines the parameters for exponential backoff with full jitter.
type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
	Jitter         bool
}

// DefaultRetryConfig returns production-tested retry settings for transient failures.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		BackoffFactor:  2.0,
		Jitter:         true,
	}
}

// IsRetryable determines whether an error indicates a transient failure that can be safely retried.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// 1. Context cancellation is NOT retryable
	if errors.Is(err, context.Canceled) {
		return false
	}

	// 2. DynamoDB conditional check failures mean state changed; MUST NOT retry
	var ccfe *types.ConditionalCheckFailedException
	if errors.As(err, &ccfe) {
		return false
	}
	var rnfe *types.ResourceNotFoundException
	if errors.As(err, &rnfe) {
		return false
	}

	// 3. DynamoDB transient throughput / throttling / conflict errors ARE retryable
	var ptee *types.ProvisionedThroughputExceededException
	if errors.As(err, &ptee) {
		return true
	}
	var rle *types.RequestLimitExceeded
	if errors.As(err, &rle) {
		return true
	}
	var ise *types.InternalServerError
	if errors.As(err, &ise) {
		return true
	}
	var tce *types.TransactionConflictException
	if errors.As(err, &tce) {
		return true
	}

	// 4. Network and transient I/O errors
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || strings.Contains(err.Error(), "connection reset")) {
		return true
	}

	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "throughput") ||
		strings.Contains(errStr, "throttling") ||
		strings.Contains(errStr, "transient") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "temporary") {
		return true
	}

	return false
}

// ExecuteWithRetry executes the provided operation with exponential backoff and jitter.
func ExecuteWithRetry(ctx context.Context, op func(ctx context.Context) error, cfg RetryConfig) error {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 25 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 1 * time.Second
	}
	if cfg.BackoffFactor <= 1.0 {
		cfg.BackoffFactor = 2.0
	}

	var lastErr error
	backoff := cfg.InitialBackoff

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := op(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Fail immediately if error is non-retryable
		if !IsRetryable(err) {
			return err
		}

		// If this was our final attempt, don't sleep
		if attempt == cfg.MaxAttempts {
			break
		}

		// Calculate sleep duration with full jitter
		sleepDuration := backoff
		if cfg.Jitter {
			jitterMax := int64(backoff)
			if jitterMax > 0 {
				sleepDuration = time.Duration(rand.Int63n(jitterMax))
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepDuration):
		}

		// Exponential increase capped at MaxBackoff
		nextBackoff := time.Duration(float64(backoff) * cfg.BackoffFactor)
		if nextBackoff > cfg.MaxBackoff {
			backoff = cfg.MaxBackoff
		} else {
			backoff = nextBackoff
		}
	}

	return fmt.Errorf("%w: %v", ErrRetryExhausted, lastErr)
}

// WithBoundedTimeout derives a context with a bounded timeout, respecting any tighter existing deadline.
func WithBoundedTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok {
		if time.Until(deadline) < timeout {
			return context.WithCancel(parent)
		}
	}
	return context.WithTimeout(parent, timeout)
}

