package shared

import (
	"context"
	"log"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func NewDynamoClient() *dynamodb.Client {
	cfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion("eu-north-1"),
	)
	if err != nil {
		log.Fatal(err)
	}

	return dynamodb.NewFromConfig(cfg)
}