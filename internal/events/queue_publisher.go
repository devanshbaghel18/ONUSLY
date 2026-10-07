package events

import (
	"context"
	"encoding/json"

	"github.com/devanshbaghel18/ONUSLY/internal/queue"
)

// QueuePublisher implements Publisher by publishing serialized events to a Queue (SQS or MemoryQueue).
type QueuePublisher struct {
	q                  queue.Queue
	proofQueueName     string
	approvalQueueName  string
}

// NewQueuePublisher creates a new QueuePublisher.
func NewQueuePublisher(q queue.Queue, proofQueueName, approvalQueueName string) *QueuePublisher {
	return &QueuePublisher{
		q:                 q,
		proofQueueName:    proofQueueName,
		approvalQueueName: approvalQueueName,
	}
}

// PublishProofSubmitted serializes and enqueues a ProofSubmittedEvent.
func (p *QueuePublisher) PublishProofSubmitted(ctx context.Context, event ProofSubmittedEvent) error {
	if p.q == nil || p.proofQueueName == "" {
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return p.q.Publish(ctx, p.proofQueueName, data)
}

// PublishApprovalDecided serializes and enqueues an ApprovalDecidedEvent.
func (p *QueuePublisher) PublishApprovalDecided(ctx context.Context, event ApprovalDecidedEvent) error {
	if p.q == nil || p.approvalQueueName == "" {
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return p.q.Publish(ctx, p.approvalQueueName, data)
}

var _ Publisher = (*QueuePublisher)(nil)
