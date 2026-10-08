package enforcement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

type GoalLister interface {
	List(ctx context.Context, ownerID string) ([]goals.Goal, error)
}

type Service struct {
	goalLister GoalLister
}

func NewService(goalLister GoalLister) *Service {
	return &Service{
		goalLister: goalLister,
	}
}

// canonicalTarget has fixed JSON field order for deterministic canonical JSON serialization.
type canonicalTarget struct {
	GoalID    string   `json:"goalId"`
	GoalTitle string   `json:"goalTitle"`
	Status    string   `json:"status"`
	Apps      []string `json:"apps"`
	Domains   []string `json:"domains"`
}

// ComputeVersion computes v1- + first 16 hex characters of SHA-256 over the canonical JSON of targets.
// Canonical JSON: fixed field order, sorted arrays, no whitespace, exclude userId and generatedAt.
func ComputeVersion(targets []TargetItem) string {
	list := make([]canonicalTarget, len(targets))
	for i, t := range targets {
		apps := t.Apps
		if apps == nil {
			apps = []string{}
		}
		domains := t.Domains
		if domains == nil {
			domains = []string{}
		}
		list[i] = canonicalTarget{
			GoalID:    t.GoalID,
			GoalTitle: t.GoalTitle,
			Status:    t.Status,
			Apps:      apps,
			Domains:   domains,
		}
	}

	data, _ := json.Marshal(list)
	h := sha256.Sum256(data)
	hexHash := hex.EncodeToString(h[:])
	return "v1-" + hexHash[:16]
}

func (s *Service) GetBlocklist(ctx context.Context, userID string) (*BlocklistResponse, error) {
	goalsList, err := s.goalLister.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	var targets []TargetItem
	for _, g := range goalsList {
		// Include goals only when: status == active OR proof_submitted AND at least one target exists
		if g.Status != "active" && g.Status != "proof_submitted" {
			continue
		}
		if len(g.BlockedApps) == 0 && len(g.BlockedDomains) == 0 {
			continue
		}

		apps := append([]string(nil), g.BlockedApps...)
		domains := append([]string(nil), g.BlockedDomains...)
		sort.Strings(apps)
		sort.Strings(domains)
		if apps == nil {
			apps = []string{}
		}
		if domains == nil {
			domains = []string{}
		}

		targets = append(targets, TargetItem{
			GoalID:    g.ID,
			GoalTitle: g.Title,
			Status:    g.Status,
			Apps:      apps,
			Domains:   domains,
			createdAt: g.CreatedAt,
		})
	}

	// Ordering: targets by creation time, then goal ID
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].createdAt == targets[j].createdAt {
			return targets[i].GoalID < targets[j].GoalID
		}
		return targets[i].createdAt < targets[j].createdAt
	})

	version := ComputeVersion(targets)
	now := time.Now().UTC().Format(time.RFC3339)

	return &BlocklistResponse{
		UserID:      userID,
		Version:     version,
		GeneratedAt: now,
		Targets:     targets,
	}, nil
}
