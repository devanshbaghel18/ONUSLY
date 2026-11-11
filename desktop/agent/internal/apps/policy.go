package apps

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

// AppPolicySnapshot represents an immutable point-in-time view of authoritative application restrictions.
type AppPolicySnapshot struct {
	Version     string      `json:"version"`
	GeneratedAt time.Time   `json:"generatedAt"`
	Targets     []TargetApp `json:"targets"`
}

// AppPolicyManager evaluates running processes against authoritative blocklist application targets.
type AppPolicyManager struct {
	snapshot atomic.Pointer[AppPolicySnapshot]
}

// NewAppPolicyManager creates a policy manager initialized with an empty snapshot.
func NewAppPolicyManager() *AppPolicyManager {
	mgr := &AppPolicyManager{}
	mgr.snapshot.Store(&AppPolicySnapshot{
		Version:     "init-0",
		GeneratedAt: time.Now().UTC(),
		Targets:     []TargetApp{},
	})
	return mgr
}

// UpdateFromBlocklist extracts application targets and updates the atomic policy snapshot.
func (m *AppPolicyManager) UpdateFromBlocklist(data *core.BlocklistData) {
	if data == nil {
		return
	}

	var targets []TargetApp

	for _, item := range data.Targets {
		if len(item.Apps) == 0 && len(item.ApplicationTargets) == 0 {
			continue
		}

		// Contract: active, proof_submitted, rejected -> Block (Restricted=true)
		// completed -> Allow (Restricted=false)
		// D9 Version Compatibility Invariant: Unrecognized security statuses must fail-closed (block), not allow.
		isRestricted := item.Status != "" && item.Status != "completed"

		if len(item.ApplicationTargets) > 0 {
			// D6.1: Strict Target Typing
			// Desktop agent consumes ONLY targets of kind "desktop_app".
			// Android packages ("android_package") are strictly ignored on desktop to prevent cross-platform collisions!
			for _, at := range item.ApplicationTargets {
				if at.Kind != "desktop_app" {
					continue
				}
				cleanID := NormalizeAppID(at.Identifier)
				if cleanID == "" {
					continue
				}

				targets = append(targets, TargetApp{
					GoalID:     item.GoalID,
					GoalTitle:  item.GoalTitle,
					Status:     item.Status,
					AppID:      cleanID,
					Restricted: isRestricted,
				})
			}
		} else {
			// Backward-compatibility fallback for legacy blocklists lacking ApplicationTargets:
			// Consume only identifiers without dots (canonical desktop IDs).
			// Identifiers with dots (e.g. com.whatsapp, com.spotify.music) are assumed to be Android packages and skipped.
			for _, appID := range item.Apps {
				if strings.Contains(appID, ".") {
					continue
				}
				cleanID := NormalizeAppID(appID)
				if cleanID == "" {
					continue
				}

				targets = append(targets, TargetApp{
					GoalID:     item.GoalID,
					GoalTitle:  item.GoalTitle,
					Status:     item.Status,
					AppID:      cleanID,
					Restricted: isRestricted,
				})
			}
		}
	}

	newSnapshot := &AppPolicySnapshot{
		Version:     data.Version,
		GeneratedAt: time.Now().UTC(),
		Targets:     targets,
	}

	m.snapshot.Store(newSnapshot)
}

// GetSnapshot returns the current atomic policy snapshot.
func (m *AppPolicyManager) GetSnapshot() *AppPolicySnapshot {
	return m.snapshot.Load()
}

// EvaluateProcess determines the enforcement disposition of an observed process.
func (m *AppPolicyManager) EvaluateProcess(proc ProcessInfo) (TargetApp, EnforcementState, MatchConfidence, string) {
	snap := m.snapshot.Load()
	if snap == nil || len(snap.Targets) == 0 {
		return TargetApp{}, StateAllowed, ConfidenceNone, "no active application targets"
	}

	target, conf, reason, matched := MatchProcess(proc, snap.Targets)
	if !matched {
		return TargetApp{}, StateAllowed, ConfidenceNone, "no policy target match"
	}

	if target.Restricted {
		return target, StateRestricted, conf, reason
	}

	return target, StateAllowed, conf, "target goal completed"
}

