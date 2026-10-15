package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	ctx := context.TODO()

	// 1. Connect to Stockholm (eu-north-1)
	cfgStockholm, err := config.LoadDefaultConfig(ctx, config.WithRegion("eu-north-1"))
	if err != nil {
		log.Fatalf("Failed to load eu-north-1 config: %v", err)
	}
	ddbStockholm := dynamodb.NewFromConfig(cfgStockholm)

	fmt.Println("=== STEP 1: Scanning Stockholm (eu-north-1) Onusly Table ===")
	var allItems []map[string]types.AttributeValue
	var lastKey map[string]types.AttributeValue

	for {
		scanInput := &dynamodb.ScanInput{
			TableName:         aws.String("Onusly"),
			ExclusiveStartKey: lastKey,
		}
		res, err := ddbStockholm.Scan(ctx, scanInput)
		if err != nil {
			log.Fatalf("Error scanning Stockholm: %v", err)
		}
		allItems = append(allItems, res.Items...)
		if res.LastEvaluatedKey == nil {
			break
		}
		lastKey = res.LastEvaluatedKey
	}

	fmt.Printf("Total items scanned from Stockholm: %d\n", len(allItems))

	// 2. Backup items to JSON file
	var rawItems []map[string]interface{}
	for _, item := range allItems {
		var m map[string]interface{}
		_ = attributevalue.UnmarshalMap(item, &m)
		rawItems = append(rawItems, m)
	}

	backupData, err := json.MarshalIndent(rawItems, "", "  ")
	if err != nil {
		log.Fatalf("Error encoding backup JSON: %v", err)
	}

	backupPath := "deploy/dynamodb/stockholm_items.json"
	err = os.WriteFile(backupPath, backupData, 0644)
	if err != nil {
		log.Fatalf("Error saving backup file: %v", err)
	}
	fmt.Printf("Saved full backup to %s (%d bytes)\n", backupPath, len(backupData))

	// 3. Check Mumbai (ap-south-1)
	fmt.Println("\n=== STEP 2: Checking Mumbai (ap-south-1) Onusly Table ===")
	cfgMumbai, err := config.LoadDefaultConfig(ctx, config.WithRegion("ap-south-1"))
	if err != nil {
		log.Fatalf("Failed to load ap-south-1 config: %v", err)
	}
	ddbMumbai := dynamodb.NewFromConfig(cfgMumbai)

	desc, err := ddbMumbai.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String("Onusly"),
	})
	if err != nil {
		fmt.Printf("Notice: Mumbai table not yet accessible or not created (%v)\n", err)
		fmt.Println("To create the table in Mumbai, run the create_table script once permissions are granted.")
		return
	}

	fmt.Printf("Mumbai table status: %s\n", desc.Table.TableStatus)
	if desc.Table.TableStatus != types.TableStatusActive {
		fmt.Println("Table is not active yet. Please wait until ACTIVE.")
		return
	}

	// 4. Batch write to Mumbai
	fmt.Println("\n=== STEP 3: Migrating items to Mumbai ===")
	migrated := 0
	batchSize := 25

	for i := 0; i < len(allItems); i += batchSize {
		end := i + batchSize
		if end > len(allItems) {
			end = len(allItems)
		}
		chunk := allItems[i:end]

		writeReqs := make([]types.WriteRequest, len(chunk))
		for j, it := range chunk {
			writeReqs[j] = types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: it,
				},
			}
		}

		batchInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				"Onusly": writeReqs,
			},
		}

		_, err := ddbMumbai.BatchWriteItem(ctx, batchInput)
		if err != nil {
			log.Fatalf("BatchWriteItem failed at offset %d: %v", i, err)
		}
		migrated += len(chunk)
		fmt.Printf("Migrated %d / %d items...\n", migrated, len(allItems))
	}

	fmt.Printf("\nSUCCESS! All %d items migrated to Mumbai (ap-south-1)!\n", migrated)
}
