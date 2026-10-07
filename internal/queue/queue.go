package queue

import (
	"context"
)

// Message represents a generic queue message.
type Message struct {
	ID            string
	Body          []byte
	ReceiptHandle string
}

// Queue defines the contract for an asynchronous message queue.
type Queue interface {
	Publish(ctx context.Context, queueName string, payload []byte) error
	Receive(ctx context.Context, queueName string, maxMessages int) ([]Message, error)
	Delete(ctx context.Context, queueName string, receiptHandle string) error
}
