package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileCredentialStore struct {
	tokenPath string
}

func NewFileCredentialStore(tokenPath string) *FileCredentialStore {
	return &FileCredentialStore{tokenPath: tokenPath}
}

func (s *FileCredentialStore) SaveToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("cannot save empty token")
	}

	if err := os.MkdirAll(filepath.Dir(s.tokenPath), 0700); err != nil {
		return fmt.Errorf("failed to create credentials dir: %w", err)
	}

	// Restrictive file permissions (0600: read/write only by owner)
	return os.WriteFile(s.tokenPath, []byte(token), 0600)
}

func (s *FileCredentialStore) GetToken() (string, error) {
	data, err := os.ReadFile(s.tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (s *FileCredentialStore) ClearToken() error {
	err := os.Remove(s.tokenPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

var (
	ErrTokenExpired = errors.New("token expired")
)

// ParseTokenUserID extracts the "sub" claim from JWT without signature verification (local introspection)
func ParseTokenUserID(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid jwt format")
	}

	segment := parts[1]
	// Pad base64
	if rem := len(segment) % 4; rem > 0 {
		segment += strings.Repeat("=", 4-rem)
	}

	data, err := base64.URLEncoding.DecodeString(segment)
	if err != nil {
		return "", fmt.Errorf("failed to decode jwt segment: %w", err)
	}

	var claims struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(data, &claims); err != nil {
		return "", fmt.Errorf("failed to parse jwt claims: %w", err)
	}

	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return claims.Sub, ErrTokenExpired
	}

	return claims.Sub, nil
}
