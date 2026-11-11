package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type PairingStatus string

const (
	StatusUninstalled          PairingStatus = "UNINSTALLED"
	StatusInstalledUnpaired    PairingStatus = "INSTALLED_UNPAIRED"
	StatusPairingPending       PairingStatus = "PAIRING_PENDING"
	StatusPairingCommitPending PairingStatus = "PAIRING_COMMIT_PENDING"
	StatusPaired               PairingStatus = "PAIRED"
	StatusAuthRequired         PairingStatus = "AUTHENTICATION_REQUIRED"
	StatusRepairRequired       PairingStatus = "REPAIR_REQUIRED"
)

var (
	ErrInvalidStateTransition = errors.New("invalid pairing state transition")
	ErrOwnerSIDMismatch       = errors.New("owner SID mismatch: cannot alter owner SID during repair")
	ErrPrematurePairedState   = errors.New("cannot transition to PAIRED without verified credential")
)

type PairingState struct {
	OwnerSID        string        `json:"ownerSid"`
	Status          PairingStatus `json:"status"`
	Generation      int           `json:"generation"`
	PairedAt        string        `json:"pairedAt,omitempty"`
	LastValidatedAt string        `json:"lastValidatedAt,omitempty"`
}

type PairingStateManager struct {
	stateFilePath string
	mu            sync.RWMutex
}

func NewPairingStateManager(stateFilePath string) *PairingStateManager {
	return &PairingStateManager{
		stateFilePath: stateFilePath,
	}
}

// GetState reads and parses the current pairing state from disk.
// Returns an UNINSTALLED state if the file does not exist.
func (m *PairingStateManager) GetState() (*PairingState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getStateLocked()
}

func (m *PairingStateManager) getStateLocked() (*PairingState, error) {
	data, err := os.ReadFile(m.stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &PairingState{
				Status:     StatusUninstalled,
				Generation: 0,
			}, nil
		}
		return nil, fmt.Errorf("failed to read pairing state: %w", err)
	}

	var state PairingState
	if err := json.Unmarshal(data, &state); err != nil {
		// Corrupted state file: return REPAIR_REQUIRED so the agent can safely recover
		return &PairingState{
			Status:     StatusRepairRequired,
			Generation: 1,
		}, nil
	}

	return &state, nil
}

// SaveState atomically writes the pairing state using a temporary file.
func (m *PairingStateManager) SaveState(state *PairingState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.saveStateLocked(state)
}

func (m *PairingStateManager) saveStateLocked(state *PairingState) error {
	if state == nil {
		return errors.New("cannot save nil pairing state")
	}

	dir := filepath.Dir(m.stateFilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create pairing state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal pairing state: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", m.stateFilePath, os.Getpid())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp pairing state: %w", err)
	}

	if err := os.Rename(tmpFile, m.stateFilePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to commit pairing state: %w", err)
	}

	return nil
}

// Transition validates and executes a state transition, updating generation and timestamps.
func (m *PairingStateManager) Transition(newStatus PairingStatus, ownerSID string) (*PairingState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	curr, err := m.getStateLocked()
	if err != nil {
		return nil, err
	}

	// Invariant: Do not silently change owner SID during repair or normal lifecycle
	if curr.OwnerSID != "" && ownerSID != "" && curr.OwnerSID != ownerSID {
		return nil, fmt.Errorf("%w: existing %s != requested %s", ErrOwnerSIDMismatch, curr.OwnerSID, ownerSID)
	}

	// Validate permitted state transitions
	if !isValidTransition(curr.Status, newStatus) {
		return nil, fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidStateTransition, curr.Status, newStatus)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	targetOwner := curr.OwnerSID
	if targetOwner == "" {
		targetOwner = ownerSID
	}

	next := &PairingState{
		OwnerSID:        targetOwner,
		Status:          newStatus,
		Generation:      curr.Generation + 1,
		PairedAt:        curr.PairedAt,
		LastValidatedAt: curr.LastValidatedAt,
	}

	if newStatus == StatusPaired {
		if next.PairedAt == "" {
			next.PairedAt = nowStr
		}
		next.LastValidatedAt = nowStr
	}

	if err := m.saveStateLocked(next); err != nil {
		return nil, err
	}

	return next, nil
}

func isValidTransition(from, to PairingStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusUninstalled:
		return to == StatusInstalledUnpaired || to == StatusPairingPending
	case StatusInstalledUnpaired:
		return to == StatusPairingPending || to == StatusUninstalled
	case StatusPairingPending:
		return to == StatusPairingCommitPending || to == StatusRepairRequired || to == StatusInstalledUnpaired || to == StatusUninstalled
	case StatusPairingCommitPending:
		return to == StatusPaired || to == StatusRepairRequired || to == StatusUninstalled
	case StatusPaired:
		return to == StatusAuthRequired || to == StatusRepairRequired || to == StatusUninstalled
	case StatusAuthRequired:
		return to == StatusPairingPending || to == StatusPaired || to == StatusRepairRequired || to == StatusUninstalled
	case StatusRepairRequired:
		return to == StatusPairingPending || to == StatusPairingCommitPending || to == StatusInstalledUnpaired || to == StatusUninstalled
	default:
		return to == StatusUninstalled
	}
}

// Recover handles startup reconciliation for interrupted two-phase commit transactions.
func (m *PairingStateManager) Recover(ctx context.Context, credStore core.CredentialStore, syncer core.BlocklistSyncer) (*PairingState, error) {
	state, err := m.GetState()
	if err != nil {
		return nil, err
	}

	switch state.Status {
	case StatusPairingCommitPending:
		// Phase 1 completed, Phase 2 interrupted:
		// Check if token exists and validates against backend GET /enforcement/blocklist
		tok, errTok := credStore.GetToken()
		if errTok == nil && tok != "" {
			// Validate credential
			blocklist, errSync := syncer.Sync(ctx)
			if errSync == nil && blocklist != nil {
				// HTTP 200 validation confirmed: promote safely to PAIRED
				return m.Transition(StatusPaired, state.OwnerSID)
			}
			// Backend is temporarily unavailable (e.g. HTTP 521 or offline):
			// Keep PAIRING_COMMIT_PENDING, retain offline cache, retry later.
			return state, nil
		}
		// Token is missing: transition to REPAIR_REQUIRED
		return m.Transition(StatusRepairRequired, state.OwnerSID)

	case StatusPaired:
		// Verify token file existence
		tok, errTok := credStore.GetToken()
		if errTok != nil || tok == "" {
			// Token missing or corrupt: fail closed to REPAIR_REQUIRED
			return m.Transition(StatusRepairRequired, state.OwnerSID)
		}
		// Check if token is expired
		if _, errExp := ParseTokenUserID(tok); errExp != nil && (errors.Is(errExp, ErrTokenExpired) || strings.Contains(errExp.Error(), "token expired")) {
			return m.Transition(StatusAuthRequired, state.OwnerSID)
		}
		return state, nil

	default:
		return state, nil
	}
}
