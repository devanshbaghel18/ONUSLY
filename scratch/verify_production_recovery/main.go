package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
)

type VerificationReport struct {
	Timestamp               string   `json:"timestamp"`
	BaseURL                 string   `json:"baseUrl"`
	HealthStatus            int      `json:"healthStatus"`
	HealthBody              string   `json:"healthBody"`
	HealthPassed            bool     `json:"healthPassed"`
	UnauthBlocklistStatus   int      `json:"unauthBlocklistStatus"`
	UnauthBlocklistPassed   bool     `json:"unauthBlocklistPassed"`
	AuthBlocklistStatus     int      `json:"authBlocklistStatus"`
	AuthBlocklistPassed     bool     `json:"authBlocklistPassed"`
	UpdatesXMLStatus        int      `json:"updatesXmlStatus"`
	UpdatesXMLContentType   string   `json:"updatesXmlContentType"`
	UpdatesXMLPassed        bool     `json:"updatesXmlPassed"`
	CrxStatus               int      `json:"crxStatus"`
	CrxContentType          string   `json:"crxContentType"`
	CrxMagicHeader          string   `json:"crxMagicHeader"`
	CrxPassed               bool     `json:"crxPassed"`
	HeartbeatStatus         int      `json:"heartbeatStatus"`
	HeartbeatBody           string   `json:"heartbeatBody"`
	HeartbeatPassed         bool     `json:"heartbeatPassed"`
	DdbWriteVerified        bool     `json:"ddbWriteVerified"`
	DdbCleanupSuccess       bool     `json:"ddbCleanupSuccess"`
	CloudflareOriginDown521 bool     `json:"cloudflareOriginDown521"`
	Errors                  []string `json:"errors"`
}

func main() {
	_ = godotenv.Load()

	baseURL := os.Getenv("VERIFY_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.onusly.com"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "sre-test-secret-min-16-characters-length"
	}

	report := VerificationReport{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		BaseURL:   baseURL,
		Errors:    []string{},
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	fmt.Printf("==============================================================================\n")
	fmt.Printf(">>> ONUSLY Verification Probe executing against: %s\n", baseURL)
	fmt.Printf("==============================================================================\n\n")

	// 1. GET /health
	fmt.Print("[1/5] Testing GET /health ... ")
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("Health request failed: %v", err))
		fmt.Printf("FAILED (%v)\n", err)
	} else {
		report.HealthStatus = resp.StatusCode
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		report.HealthBody = string(bodyBytes)
		if resp.StatusCode == 521 {
			report.CloudflareOriginDown521 = true
			fmt.Printf("FAILED (Cloudflare HTTP 521 - Origin Server Down)\n")
		} else if resp.StatusCode == 200 && strings.Contains(report.HealthBody, `"ok"`) {
			report.HealthPassed = true
			fmt.Printf("PASS (HTTP 200, body: %s)\n", strings.TrimSpace(report.HealthBody))
		} else {
			fmt.Printf("FAILED (Status %d, body: %s)\n", resp.StatusCode, strings.TrimSpace(report.HealthBody))
		}
	}

	// 2. GET /enforcement/blocklist (unauthenticated)
	fmt.Print("[2/5] Testing GET /enforcement/blocklist (unauthenticated) ... ")
	resp, err = client.Get(baseURL + "/enforcement/blocklist")
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("Unauth blocklist failed: %v", err))
		fmt.Printf("FAILED (%v)\n", err)
	} else {
		report.UnauthBlocklistStatus = resp.StatusCode
		resp.Body.Close()
		if resp.StatusCode == 401 {
			report.UnauthBlocklistPassed = true
			fmt.Printf("PASS (HTTP 401 Unauthorized as expected)\n")
		} else {
			fmt.Printf("FAILED (Expected 401, got %d)\n", resp.StatusCode)
		}
	}

	// 3. GET /enforcement/blocklist (authenticated)
	testOwnerID := "test-owner-234a1dd3-3539-46f9-9961-80921d89fd72"
	claims := jwt.MapClaims{
		"sub":   testOwnerID,
		"email": "test-owner@example.com",
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokStr, _ := tok.SignedString([]byte(jwtSecret))

	fmt.Print("[3/5] Testing GET /enforcement/blocklist (authenticated) ... ")
	req, _ := http.NewRequest("GET", baseURL+"/enforcement/blocklist", nil)
	req.Header.Set("Authorization", "Bearer "+tokStr)
	resp, err = client.Do(req)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("Auth blocklist failed: %v", err))
		fmt.Printf("FAILED (%v)\n", err)
	} else {
		report.AuthBlocklistStatus = resp.StatusCode
		bBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			report.AuthBlocklistPassed = true
			fmt.Printf("PASS (HTTP 200, targets returned)\n")
		} else {
			fmt.Printf("FAILED (Status %d, body: %s)\n", resp.StatusCode, string(bBytes))
		}
	}

	// 4. Legacy Downloads routes: updates.xml and onusly.crx
	fmt.Print("[4/5] Testing Legacy Downloads routes ...\n")
	resp, err = client.Get(baseURL + "/downloads/updates.xml")
	if err == nil {
		report.UpdatesXMLStatus = resp.StatusCode
		report.UpdatesXMLContentType = resp.Header.Get("Content-Type")
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 && strings.Contains(report.UpdatesXMLContentType, "xml") && strings.Contains(string(bodyBytes), "updatecheck") {
			report.UpdatesXMLPassed = true
			fmt.Printf("   -> updates.xml: PASS (HTTP 200, %s)\n", report.UpdatesXMLContentType)
		} else {
			fmt.Printf("   -> updates.xml: FAILED (Status %d, Content-Type: %s)\n", resp.StatusCode, report.UpdatesXMLContentType)
		}
	}

	resp, err = client.Get(baseURL + "/downloads/onusly.crx")
	if err == nil {
		report.CrxStatus = resp.StatusCode
		report.CrxContentType = resp.Header.Get("Content-Type")
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		magic := ""
		if len(bodyBytes) >= 4 {
			magic = string(bodyBytes[:4])
		}
		report.CrxMagicHeader = magic
		if resp.StatusCode == 200 && (magic == "Cr24" || strings.Contains(report.CrxContentType, "chrome-extension")) {
			report.CrxPassed = true
			fmt.Printf("   -> onusly.crx: PASS (HTTP 200, magic: %s, size: %d bytes)\n", magic, len(bodyBytes))
		} else {
			fmt.Printf("   -> onusly.crx: FAILED (Status %d, Content-Type: %s)\n", resp.StatusCode, report.CrxContentType)
		}
	}

	// 5. Heartbeat & DynamoDB State Verification + Cleanup
	testDeviceID := "a1c71cf9-8a02-47fd-a655-d93f5db9b205"
	nowUTC := time.Now().UTC().Format(time.RFC3339)
	hbPayload := fmt.Sprintf(`{"deviceId":"%s","clientType":"desktop_agent","clientVersion":"1.0.0","platform":"linux","sentAt":"%s"}`, testDeviceID, nowUTC)

	fmt.Print("[5/5] Testing Heartbeat POST & DynamoDB write consistency ... ")
	req, _ = http.NewRequest("POST", baseURL+"/enforcement/heartbeat", strings.NewReader(hbPayload))
	req.Header.Set("Authorization", "Bearer "+tokStr)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("Heartbeat request failed: %v", err))
		fmt.Printf("FAILED (%v)\n", err)
	} else {
		report.HeartbeatStatus = resp.StatusCode
		bBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		report.HeartbeatBody = string(bBytes)
		if resp.StatusCode == 200 {
			report.HeartbeatPassed = true
			fmt.Printf("PASS (HTTP 200 received)\n")

			// Check DynamoDB directly in eu-north-1
			ctx := context.TODO()
			cfgDDB, errDDB := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("eu-north-1"))
			if errDDB == nil {
				ddb := dynamodb.NewFromConfig(cfgDDB)
				pk := "USER#" + testOwnerID
				sk := "DEVICE#" + testDeviceID
				getRes, errGet := ddb.GetItem(ctx, &dynamodb.GetItemInput{
					TableName: aws.String("Onusly"),
					Key: map[string]ddbtypes.AttributeValue{
						"PK": &ddbtypes.AttributeValueMemberS{Value: pk},
						"SK": &ddbtypes.AttributeValueMemberS{Value: sk},
					},
				})
				if errGet == nil && getRes.Item != nil {
					report.DdbWriteVerified = true
					fmt.Printf("   -> DynamoDB persistence verified: Item found (PK=%s, SK=%s)\n", pk, sk)

					// Clean up the test device record
					_, errDel := ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
						TableName: aws.String("Onusly"),
						Key: map[string]ddbtypes.AttributeValue{
							"PK": &ddbtypes.AttributeValueMemberS{Value: pk},
							"SK": &ddbtypes.AttributeValueMemberS{Value: sk},
						},
					})
					if errDel == nil {
						report.DdbCleanupSuccess = true
						fmt.Printf("   -> Test device cleanup successful (deleted PK=%s, SK=%s)\n", pk, sk)
					}
				}
			}
		} else {
			fmt.Printf("FAILED (Status %d, body: %s)\n", resp.StatusCode, string(bBytes))
		}
	}

	fmt.Printf("\n==============================================================================\n")
	fmt.Printf(">>> Verification Summary: Health=%v, Unauth=%v, Auth=%v, UpdatesXML=%v, CRX=%v, Heartbeat=%v\n",
		report.HealthPassed, report.UnauthBlocklistPassed, report.AuthBlocklistPassed,
		report.UpdatesXMLPassed, report.CrxPassed, report.HeartbeatPassed)
	if report.CloudflareOriginDown521 {
		fmt.Println(">>> Origin State: Cloudflare HTTP 521. Origin host must be started and DNS pointed to Elastic IP.")
	}
	fmt.Printf("==============================================================================\n")
}
