package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type memoryItem struct {
	id            string
	body          []byte
	receiptHandle string
}

// MemoryQueue is a thread-safe, in-memory implementation of Queue for local dev and deterministic outage testing.
type MemoryQueue struct {
	mu       sync.RWMutex
	queues   map[string][]*memoryItem
	paused   map[string]bool
	counter  uint64
}

// NewMemoryQueue initializes an empty MemoryQueue.
func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		queues: make(map[string][]*memoryItem),
		paused: make(map[string]bool),
	}
}

// Publish enqueues a payload onto the specified queue.
func (m *MemoryQueue) Publish(ctx context.Context, queueName string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("msg-%d", atomic.AddUint64(&m.counter, 1))
	receipt := fmt.Sprintf("rcpt-%s", id)

	item := &memoryItem{
		id:            id,
		body:          payload,
		receiptHandle: receipt,
	}

	m.queues[queueName] = append(m.queues[queueName], item)
	return nil
}

// Receive dequeues up to maxMessages from the queue, unless paused.
func (m *MemoryQueue) Receive(ctx context.Context, queueName string, maxMessages int) ([]Message, error) {
	m.mu.RLock()
	isPaused := m.paused[queueName]
	m.mu.RUnlock()

	if isPaused {
		return []Message{}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	items, exists := m.queues[queueName]
	if !exists || len(items) == 0 {
		return []Message{}, nil
	}

	if maxMessages <= 0 || maxMessages > len(items) {
		maxMessages = len(items)
	}

	result := make([]Message, 0, maxMessages)
	for i := 0; i < maxMessages; i++ {
		result = append(result, Message{
			ID:            items[i].id,
			Body:          items[i].body,
			ReceiptHandle: items[i].receiptHandle,
		})
	}

	return result, nil
}

// Delete removes the message matching receiptHandle from the queue.
func (m *MemoryQueue) Delete(ctx context.Context, queueName string, receiptHandle string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	items, exists := m.queues[queueName]
	if !exists {
		return nil
	}

	for i, item := range items {
		if item.receiptHandle == receiptHandle {
			m.queues[queueName] = append(items[:i], items[i+1:]...)
			return nil
		}
	}

	return nil
}

// Pause stops Receive from returning messages for the specified queue, simulating a consumer outage.
func (m *MemoryQueue) Pause(queueName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paused[queueName] = true
}

// Resume reenables Receive for the specified queue, allowing accumulated messages to drain.
func (m *MemoryQueue) Resume(queueName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paused[queueName] = false
}

// IsPaused reports whether consumption on the specified queue is paused.
func (m *MemoryQueue) IsPaused(queueName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.paused[queueName]
}

// Length returns the number of messages currently pending in the queue.
func (m *MemoryQueue) Length(queueName string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.queues[queueName])
}

var _ Queue = (*MemoryQueue)(nil)
