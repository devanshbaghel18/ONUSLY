package dns

import (
	"sync/atomic"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

// BlockedRule holds the normalized domain and metadata for a blocked target.
type BlockedRule struct {
	TargetDomain string
	GoalID       string
	GoalTitle    string
	GoalStatus   string
}

// PolicySnapshot represents an immutable, atomic snapshot of active DNS domain restrictions.
// Consumed by concurrent DNS worker routines without lock contention.
type PolicySnapshot struct {
	Version     string
	GeneratedAt time.Time
	UpdatedAt   time.Time
	Rules       []BlockedRule
	DomainsMap  map[string]BlockedRule // Exact match fast-path
}

// PolicyManager holds the atomic reference to the current PolicySnapshot
// and manages atomic updates from the authoritative blocklist syncer/cache.
type PolicyManager struct {
	current atomic.Pointer[PolicySnapshot]
	cache   *MemoryDNSCache
}

func NewPolicyManager(dnsCache *MemoryDNSCache) *PolicyManager {
	pm := &PolicyManager{
		cache: dnsCache,
	}
	// Initialize with empty snapshot
	pm.current.Store(&PolicySnapshot{
		Version:     "init-0",
		GeneratedAt: time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		Rules:       []BlockedRule{},
		DomainsMap:  make(map[string]BlockedRule),
	})
	return pm
}

// GetSnapshot returns the current immutable policy snapshot.
func (pm *PolicyManager) GetSnapshot() *PolicySnapshot {
	return pm.current.Load()
}

// UpdateFromBlocklist extracts and compiles active enforcement targets into a new immutable PolicySnapshot.
// Strictly enforces D4 goal-status rules:
// - active: Block
// - proof_submitted: Block
// - rejected: Block
// - completed: Unblock / Do not block
//
// Automatically invalidates matching allowed DNS cache entries upon a successful policy update.
func (pm *PolicyManager) UpdateFromBlocklist(data *core.BlocklistData) (*PolicySnapshot, error) {
	if data == nil {
		// Do not wipe valid policy if incoming data is nil
		return pm.GetSnapshot(), nil
	}

	var rules []BlockedRule
	domainsMap := make(map[string]BlockedRule)
	var newlyBlockedDomains []string

	for _, target := range data.Targets {
		status := target.Status

		// Only enforce active, proof_submitted, and rejected targets
		// Completed goals are explicitly unblocked
		isEnforcedStatus := status == "active" || status == "proof_submitted" || status == "rejected"
		if !isEnforcedStatus {
			continue
		}

		for _, d := range target.Domains {
			cleanDomain := NormalizeDomain(d)
			if cleanDomain == "" || IsProtectedDomain(cleanDomain) {
				continue
			}

			rule := BlockedRule{
				TargetDomain: cleanDomain,
				GoalID:       target.GoalID,
				GoalTitle:    target.GoalTitle,
				GoalStatus:   status,
			}
			rules = append(rules, rule)
			domainsMap[cleanDomain] = rule
			newlyBlockedDomains = append(newlyBlockedDomains, cleanDomain)
		}
	}

	genTime, err := time.Parse(time.RFC3339, data.GeneratedAt)
	if err != nil {
		genTime = time.Now().UTC()
	}

	snapshot := &PolicySnapshot{
		Version:     data.Version,
		GeneratedAt: genTime,
		UpdatedAt:   time.Now().UTC(),
		Rules:       rules,
		DomainsMap:  domainsMap,
	}

	// Atomic pointer swap: concurrent DNS queries transition seamlessly to the new snapshot
	pm.current.Store(snapshot)

	// Invalidate any previously cached allowed DNS queries for the newly blocked domains
	if pm.cache != nil && len(newlyBlockedDomains) > 0 {
		pm.cache.Invalidate(newlyBlockedDomains)
	}

	return snapshot, nil
}

// CheckBlocked evaluates whether a candidate query hostname is restricted by the active snapshot.
func (pm *PolicyManager) CheckBlocked(hostname string) (blocked bool, rule BlockedRule) {
	cleanHost := NormalizeDomain(hostname)
	if cleanHost == "" || IsProtectedDomain(cleanHost) {
		return false, BlockedRule{}
	}

	snap := pm.GetSnapshot()
	if snap == nil || len(snap.Rules) == 0 {
		return false, BlockedRule{}
	}

	// Fast-path: exact match in map
	if r, found := snap.DomainsMap[cleanHost]; found {
		return true, r
	}

	// Subdomain search across rules
	for _, r := range snap.Rules {
		if IsDomainMatch(cleanHost, r.TargetDomain) {
			return true, r
		}
	}

	return false, BlockedRule{}
}

