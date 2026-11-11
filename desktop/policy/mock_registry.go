package policy

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

type mockValue struct {
	valType string // "string" or "dword"
	strVal  string
	dwVal   uint32
}

type mockKey struct {
	path   string
	values map[string]mockValue
}

// MockRegistry implements RegistryAccessor for in-memory testing.
type MockRegistry struct {
	mu   sync.RWMutex
	keys map[string]*mockKey
}

func NewMockRegistry() *MockRegistry {
	return &MockRegistry{
		keys: make(map[string]*mockKey),
	}
}

func (m *MockRegistry) normalize(path string) string {
	return strings.ToLower(strings.Trim(path, `\`))
}

func (m *MockRegistry) KeyExists(keyPath string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.keys[m.normalize(keyPath)]
	return exists
}

func (m *MockRegistry) OpenKey(keyPath string, write bool) (RegistryKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	norm := m.normalize(keyPath)
	k, exists := m.keys[norm]
	if !exists {
		return nil, fmt.Errorf("registry key not found: %s", keyPath)
	}

	return &mockKeyHandle{
		reg:     m,
		keyPath: norm,
		key:     k,
	}, nil
}

func (m *MockRegistry) CreateKey(keyPath string) (RegistryKey, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	norm := m.normalize(keyPath)
	k, exists := m.keys[norm]
	if exists {
		return &mockKeyHandle{
			reg:     m,
			keyPath: norm,
			key:     k,
		}, true, nil
	}

	newKey := &mockKey{
		path:   norm,
		values: make(map[string]mockValue),
	}
	m.keys[norm] = newKey

	return &mockKeyHandle{
		reg:     m,
		keyPath: norm,
		key:     newKey,
	}, false, nil
}

func (m *MockRegistry) DeleteKey(keyPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.keys, m.normalize(keyPath))
	return nil
}

func (m *MockRegistry) DeleteValue(keyPath, valueName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	k, exists := m.keys[m.normalize(keyPath)]
	if !exists {
		return nil
	}
	delete(k.values, valueName)
	return nil
}

type mockKeyHandle struct {
	reg     *MockRegistry
	keyPath string
	key     *mockKey
}

func (h *mockKeyHandle) GetStringValue(name string) (string, error) {
	h.reg.mu.RLock()
	defer h.reg.mu.RUnlock()

	val, exists := h.key.values[name]
	if !exists || val.valType != "string" {
		return "", errors.New("value not found or not string")
	}
	return val.strVal, nil
}

func (h *mockKeyHandle) GetDWordValue(name string) (uint32, error) {
	h.reg.mu.RLock()
	defer h.reg.mu.RUnlock()

	val, exists := h.key.values[name]
	if !exists || val.valType != "dword" {
		return 0, errors.New("value not found or not dword")
	}
	return val.dwVal, nil
}

func (h *mockKeyHandle) GetValueNames() ([]string, error) {
	h.reg.mu.RLock()
	defer h.reg.mu.RUnlock()

	var names []string
	for n := range h.key.values {
		names = append(names, n)
	}
	return names, nil
}

func (h *mockKeyHandle) SetStringValue(name, value string) error {
	h.reg.mu.Lock()
	defer h.reg.mu.Unlock()

	h.key.values[name] = mockValue{
		valType: "string",
		strVal:  value,
	}
	return nil
}

func (h *mockKeyHandle) SetDWordValue(name string, value uint32) error {
	h.reg.mu.Lock()
	defer h.reg.mu.Unlock()

	h.key.values[name] = mockValue{
		valType: "dword",
		dwVal:   value,
	}
	return nil
}

func (h *mockKeyHandle) DeleteValue(name string) error {
	h.reg.mu.Lock()
	defer h.reg.mu.Unlock()

	delete(h.key.values, name)
	return nil
}

func (h *mockKeyHandle) Close() error {
	return nil
}

