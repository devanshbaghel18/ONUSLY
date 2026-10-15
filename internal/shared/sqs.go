package shared

import (
	"context"
	"log"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// NewSQSClient creates an AWS SQS client configured from AWS_REGION (defaults to eu-north-1).
func NewSQSClient() *sqs.Client {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "eu-north-1"
	}

	cfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		log.Printf("[AWS SQS] Failed to load AWS config: %v", err)
		return nil
	}

	return sqs.NewFromConfig(cfg)
}

