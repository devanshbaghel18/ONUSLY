package identity

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileDeviceIdentityStore struct {
	filePath string
}

func NewFileDeviceIdentityStore(filePath string) *FileDeviceIdentityStore {
	return &FileDeviceIdentityStore{filePath: filePath}
}

func (s *FileDeviceIdentityStore) GetDeviceID() (string, error) {
	if data, err := os.ReadFile(s.filePath); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	}

	newID, err := generateUUIDv4()
	if err != nil {
		return "", fmt.Errorf("failed to generate device id: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0700); err != nil {
		return "", fmt.Errorf("failed to create identity directory: %w", err)
	}

	if err := os.WriteFile(s.filePath, []byte(newID+"\n"), 0600); err != nil {
		return "", fmt.Errorf("failed to persist device id: %w", err)
	}

	return newID, nil
}

func generateUUIDv4() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 1

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
