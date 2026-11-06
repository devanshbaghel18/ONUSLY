package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type PairedUserStore interface {
	GetAllowedUIDs() ([]uint32, error)
	SaveAllowedUIDs(uids []uint32) error
	AddAllowedUID(uid uint32) error
}

type FilePairedUserStore struct {
	filePath string
	mu       sync.RWMutex
}

func NewFilePairedUserStore(filePath string) *FilePairedUserStore {
	return &FilePairedUserStore{filePath: filePath}
}

func (s *FilePairedUserStore) GetAllowedUIDs() ([]uint32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []uint32{}, nil
		}
		return nil, fmt.Errorf("failed to read paired UIDs: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var uids []uint32
	seen := make(map[uint32]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parsed, err := strconv.ParseUint(trimmed, 10, 32)
		if err == nil {
			uid := uint32(parsed)
			if !seen[uid] {
				seen[uid] = true
				uids = append(uids, uid)
			}
		}
	}

	return uids, nil
}

func (s *FilePairedUserStore) SaveAllowedUIDs(uids []uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0700); err != nil {
		return fmt.Errorf("failed to create config directory for paired UIDs: %w", err)
	}

	var lines []string
	for _, uid := range uids {
		lines = append(lines, fmt.Sprintf("%d", uid))
	}
	content := strings.Join(lines, "\n") + "\n"

	return os.WriteFile(s.filePath, []byte(content), 0600)
}

func (s *FilePairedUserStore) AddAllowedUID(uid uint32) error {
	existing, err := s.GetAllowedUIDs()
	if err != nil {
		return err
	}

	for _, u := range existing {
		if u == uid {
			return nil // Already authorized
		}
	}

	existing = append(existing, uid)
	return s.SaveAllowedUIDs(existing)
}
