package queue

import (
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// SQSQueue implements the Queue interface backed by AWS SQS.
type SQSQueue struct {
	client    *sqs.Client
	queueURLs map[string]string
}

// NewSQSQueue creates an SQSQueue adapter with an AWS SQS client and optional name-to-URL mappings.
func NewSQSQueue(client *sqs.Client, queueURLs map[string]string) *SQSQueue {
	if queueURLs == nil {
		queueURLs = make(map[string]string)
	}
	return &SQSQueue{
		client:    client,
		queueURLs: queueURLs,
	}
}

func (s *SQSQueue) resolveQueueURL(queueName string) (string, error) {
	if url, ok := s.queueURLs[queueName]; ok && url != "" {
		return url, nil
	}
	if strings.HasPrefix(queueName, "http://") || strings.HasPrefix(queueName, "https://") {
		return queueName, nil
	}
	return "", errors.New("no SQS queue URL configured for " + queueName)
}

// Publish sends a message to AWS SQS.
func (s *SQSQueue) Publish(ctx context.Context, queueName string, payload []byte) error {
	queueURL, err := s.resolveQueueURL(queueName)
	if err != nil {
		return err
	}

	_, err = s.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(payload)),
	})
	return err
}

// Receive polls up to maxMessages from AWS SQS.
func (s *SQSQueue) Receive(ctx context.Context, queueName string, maxMessages int) ([]Message, error) {
	queueURL, err := s.resolveQueueURL(queueName)
	if err != nil {
		return nil, err
	}

	if maxMessages <= 0 {
		maxMessages = 10
	}
	if maxMessages > 10 {
		maxMessages = 10
	}

	resp, err := s.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: int32(maxMessages),
		WaitTimeSeconds:     1, // Short wait for responsive polling
	})
	if err != nil {
		return nil, err
	}

	result := make([]Message, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		var id, receipt, body string
		if m.MessageId != nil {
			id = *m.MessageId
		}
		if m.ReceiptHandle != nil {
			receipt = *m.ReceiptHandle
		}
		if m.Body != nil {
			body = *m.Body
		}

		result = append(result, Message{
			ID:            id,
			Body:          []byte(body),
			ReceiptHandle: receipt,
		})
	}

	return result, nil
}

// Delete removes a message from AWS SQS using its receipt handle.
func (s *SQSQueue) Delete(ctx context.Context, queueName string, receiptHandle string) error {
	queueURL, err := s.resolveQueueURL(queueName)
	if err != nil {
		return err
	}

	_, err = s.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	return err
}

// Check types
var _ types.Message
var _ Queue = (*SQSQueue)(nil)
