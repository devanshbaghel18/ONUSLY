package shared

import (
	"context"
	"log"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// NewSQSClient creates an AWS SQS client configured for eu-north-1.
func NewSQSClient() *sqs.Client {
	cfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion("eu-north-1"),
	)
	if err != nil {
		log.Printf("[AWS SQS] Failed to load AWS config: %v", err)
		return nil
	}

	return sqs.NewFromConfig(cfg)
}
