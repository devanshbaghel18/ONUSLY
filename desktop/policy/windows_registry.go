//go:build windows

package policy

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// RealWindowsRegistry connects RegistryAccessor to the actual Windows registry under HKEY_LOCAL_MACHINE.
type RealWindowsRegistry struct{}

func NewRealWindowsRegistry() *RealWindowsRegistry {
	return &RealWindowsRegistry{}
}

func (r *RealWindowsRegistry) KeyExists(keyPath string) bool {
	keyPath = strings.Trim(keyPath, `\`)
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	_ = k.Close()
	return true
}

func (r *RealWindowsRegistry) OpenKey(keyPath string, write bool) (RegistryKey, error) {
	keyPath = strings.Trim(keyPath, `\`)
	access := uint32(registry.QUERY_VALUE)
	if write {
		access |= registry.SET_VALUE
	}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, access)
	if err != nil {
		return nil, err
	}
	return &realRegistryKey{k: k}, nil
}

func (r *RealWindowsRegistry) CreateKey(keyPath string) (RegistryKey, bool, error) {
	keyPath = strings.Trim(keyPath, `\`)
	k, openedExisting, err := registry.CreateKey(registry.LOCAL_MACHINE, keyPath, registry.ALL_ACCESS)
	if err != nil {
		return nil, false, err
	}
	return &realRegistryKey{k: k}, openedExisting, nil
}

func (r *RealWindowsRegistry) DeleteKey(keyPath string) error {
	keyPath = strings.Trim(keyPath, `\`)
	err := registry.DeleteKey(registry.LOCAL_MACHINE, keyPath)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func (r *RealWindowsRegistry) DeleteValue(keyPath, valueName string) error {
	keyPath = strings.Trim(keyPath, `\`)
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	defer k.Close()

	errVal := k.DeleteValue(valueName)
	if errors.Is(errVal, registry.ErrNotExist) {
		return nil
	}
	return errVal
}

type realRegistryKey struct {
	k registry.Key
}

func (k *realRegistryKey) GetStringValue(name string) (string, error) {
	val, _, err := k.k.GetStringValue(name)
	return val, err
}

func (k *realRegistryKey) GetDWordValue(name string) (uint32, error) {
	val, _, err := k.k.GetIntegerValue(name)
	return uint32(val), err
}

func (k *realRegistryKey) GetValueNames() ([]string, error) {
	ki, err := k.k.Stat()
	if err != nil {
		return nil, err
	}
	return k.k.ReadValueNames(int(ki.ValueCount))
}

func (k *realRegistryKey) SetStringValue(name, value string) error {
	return k.k.SetStringValue(name, value)
}

func (k *realRegistryKey) SetDWordValue(name string, value uint32) error {
	return k.k.SetDWordValue(name, value)
}

func (k *realRegistryKey) DeleteValue(name string) error {
	return k.k.DeleteValue(name)
}

func (k *realRegistryKey) Close() error {
	return k.k.Close()
}
