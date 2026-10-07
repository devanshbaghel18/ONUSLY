package notifications

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/events"
	"github.com/devanshbaghel18/ONUSLY/internal/queue"
)

// Worker consumes proof and approval event queues and creates notifications idempotently.
type Worker struct {
	q                 queue.Queue
	service           *Service
	proofQueueName    string
	approvalQueueName string
	pollInterval      time.Duration

	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
	doneChan chan struct{}
}

// NewWorker initializes a new background notification worker.
func NewWorker(
	q queue.Queue,
	service *Service,
	proofQueueName string,
	approvalQueueName string,
	pollInterval time.Duration,
) *Worker {
	if pollInterval <= 0 {
		pollInterval = 250 * time.Millisecond
	}
	return &Worker{
		q:                 q,
		service:           service,
		proofQueueName:    proofQueueName,
		approvalQueueName: approvalQueueName,
		pollInterval:      pollInterval,
	}
}

// Start begins background queue polling in a dedicated goroutine.
func (w *Worker) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stopChan = make(chan struct{})
	w.doneChan = make(chan struct{})
	w.mu.Unlock()

	go w.run(ctx)
}

// Stop gracefully shuts down the worker, waiting for in-flight messages to complete.
func (w *Worker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopChan)
	w.mu.Unlock()

	<-w.doneChan
}

// IsRunning reports whether the worker background goroutine is active.
func (w *Worker) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.doneChan)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopChan:
			return
		case <-ticker.C:
			_ = w.ProcessOnce(ctx)
		}
	}
}

// ProcessOnce polls each queue once and processes any pending messages.
// Returns the total number of processed messages.
func (w *Worker) ProcessOnce(ctx context.Context) int {
	total := 0

	// 1. Process proof-events queue
	if w.proofQueueName != "" {
		msgs, err := w.q.Receive(ctx, w.proofQueueName, 10)
		if err == nil {
			for _, msg := range msgs {
				if w.processProofMessage(ctx, msg) {
					total++
				}
			}
		}
	}

	// 2. Process approval-events queue
	if w.approvalQueueName != "" {
		msgs, err := w.q.Receive(ctx, w.approvalQueueName, 10)
		if err == nil {
			for _, msg := range msgs {
				if w.processApprovalMessage(ctx, msg) {
					total++
				}
			}
		}
	}

	return total
}

func (w *Worker) processProofMessage(ctx context.Context, msg queue.Message) bool {
	var ev events.ProofSubmittedEvent
	if err := json.Unmarshal(msg.Body, &ev); err != nil {
		log.Printf("[Worker] Dropping malformed proof event message %s: %v", msg.ID, err)
		// Poison pill: delete to prevent endless retry loop
		_ = w.q.Delete(ctx, w.proofQueueName, msg.ReceiptHandle)
		return false
	}

	if err := w.service.CreateFromProofSubmitted(ctx, ev); err != nil {
		log.Printf("[Worker] Failed to persist proof notification for event %s: %v (will retry)", ev.EventID, err)
		// Do NOT delete: message stays in queue for redelivery
		return false
	}

	// Persisted (or already existed): delete from queue
	_ = w.q.Delete(ctx, w.proofQueueName, msg.ReceiptHandle)
	return true
}

func (w *Worker) processApprovalMessage(ctx context.Context, msg queue.Message) bool {
	var ev events.ApprovalDecidedEvent
	if err := json.Unmarshal(msg.Body, &ev); err != nil {
		log.Printf("[Worker] Dropping malformed approval event message %s: %v", msg.ID, err)
		// Poison pill: delete to prevent endless retry loop
		_ = w.q.Delete(ctx, w.approvalQueueName, msg.ReceiptHandle)
		return false
	}

	if err := w.service.CreateFromApprovalDecided(ctx, ev); err != nil {
		log.Printf("[Worker] Failed to persist approval notification for event %s: %v (will retry)", ev.EventID, err)
		// Do NOT delete: message stays in queue for redelivery
		return false
	}

	// Persisted (or already existed): delete from queue
	_ = w.q.Delete(ctx, w.approvalQueueName, msg.ReceiptHandle)
	return true
}
