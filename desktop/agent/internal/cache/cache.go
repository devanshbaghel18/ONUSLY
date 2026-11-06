package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type FileBlocklistCache struct {
	cachePath string
	mutex     sync.RWMutex
}

func NewFileBlocklistCache(cachePath string) *FileBlocklistCache {
	return &FileBlocklistCache{cachePath: cachePath}
}

func (c *FileBlocklistCache) Get(expectedUserID string) (*core.BlocklistData, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read blocklist cache: %w", err)
	}

	var cached core.BlocklistData
	if err := json.Unmarshal(data, &cached); err != nil {
		// Corruption recovery: return nil rather than crashing
		return nil, nil
	}

	// Multi-user safety check: prevent cache leakage across accounts
	if expectedUserID != "" && cached.UserID != "" && cached.UserID != expectedUserID {
		return nil, core.ErrUserMismatch
	}

	return &cached, nil
}

func (c *FileBlocklistCache) Save(data *core.BlocklistData) error {
	if data == nil {
		return errors.New("cannot save nil blocklist data")
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err := os.MkdirAll(filepath.Dir(c.cachePath), 0700); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	bytesData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	// Atomic write using a temp file in the same directory
	tmpPath := fmt.Sprintf("%s.tmp.%d", c.cachePath, os.Getpid())
	if err := os.WriteFile(tmpPath, bytesData, 0600); err != nil {
		return fmt.Errorf("failed to write temp cache: %w", err)
	}

	if err := os.Rename(tmpPath, c.cachePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to commit cache rename: %w", err)
	}

	return nil
}

func (c *FileBlocklistCache) Clear() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	err := os.Remove(c.cachePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
