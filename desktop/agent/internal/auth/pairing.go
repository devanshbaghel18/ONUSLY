package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type PairingClient struct {
	httpClient *http.Client
}

func NewPairingClient(httpClient *http.Client) *PairingClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &PairingClient{httpClient: httpClient}
}

type pairRequestPayload struct {
	ClientType string `json:"clientType"`
	Platform   string `json:"platform"`
	DeviceID   string `json:"deviceId"`
}

type pairResponsePayload struct {
	Code      string `json:"code"`
	ExpiresAt int64  `json:"expiresAt"`
}

type exchangePayload struct {
	Code string `json:"code"`
}

type exchangeResponse struct {
	Token string `json:"token"`
}

func (c *PairingClient) RequestCode(ctx context.Context, apiBaseURL, clientType, platform, deviceID string) (string, int64, error) {
	url := fmt.Sprintf("%s/enforcement/pair/request", strings.TrimRight(apiBaseURL, "/"))
	reqBody, _ := json.Marshal(pairRequestPayload{
		ClientType: clientType,
		Platform:   platform,
		DeviceID:   deviceID,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("pairing request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("pairing request returned status %d", resp.StatusCode)
	}

	var res pairResponsePayload
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", 0, fmt.Errorf("failed to decode pairing response: %w", err)
	}

	return res.Code, res.ExpiresAt, nil
}

func (c *PairingClient) PollForToken(ctx context.Context, apiBaseURL, code string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("%s/enforcement/pair/exchange", strings.TrimRight(apiBaseURL, "/"))
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case now := <-ticker.C:
			if now.After(deadline) {
				return "", errors.New("pairing timed out waiting for user confirmation")
			}

			reqBody, _ := json.Marshal(exchangePayload{Code: code})
			req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := c.httpClient.Do(req)
			if err != nil {
				continue
			}

			if resp.StatusCode == http.StatusOK {
				var res exchangeResponse
				if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.Token != "" {
					resp.Body.Close()
					return res.Token, nil
				}
			}
			resp.Body.Close()
		}
	}
}
