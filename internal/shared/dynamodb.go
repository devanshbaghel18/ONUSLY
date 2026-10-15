package shared

import (
	"context"
	"log"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func NewDynamoClient() *dynamodb.Client {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "eu-north-1"
	}

	cfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		log.Fatal(err)
	}

	return dynamodb.NewFromConfig(cfg)
}