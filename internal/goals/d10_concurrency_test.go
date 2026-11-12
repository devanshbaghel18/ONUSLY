package goals_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type d10MockGoalRepo struct {
	mu    sync.Mutex
	goals map[string]goals.Goal
}

func newD10MockGoalRepo() *d10MockGoalRepo {
	return &d10MockGoalRepo{
		goals: make(map[string]goals.Goal),
	}
}

func (r *d10MockGoalRepo) Create(ctx context.Context, goal goals.Goal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if goal.Version == 0 {
		goal.Version = 1
	}
	r.goals[goal.OwnerID+"#"+goal.ID] = goal
	return nil
}

func (r *d10MockGoalRepo) GetByID(ctx context.Context, ownerID, goalID string) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.goals[ownerID+"#"+goalID]
	if !ok {
		return nil, errors.New("goal not found")
	}
	return &g, nil
}

func (r *d10MockGoalRepo) FindByID(ctx context.Context, goalID string) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.goals {
		if g.ID == goalID {
			return &g, nil
		}
	}
	return nil, errors.New("goal not found")
}

func (r *d10MockGoalRepo) List(ctx context.Context, ownerID string) ([]goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []goals.Goal
	for _, g := range r.goals {
		if g.OwnerID == ownerID {
			list = append(list, g)
		}
	}
	return list, nil
}

func (r *d10MockGoalRepo) ListForApprover(ctx context.Context, approverID, approverEmail string) ([]goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []goals.Goal
	for _, g := range r.goals {
		if (approverID != "" && g.ApproverID == approverID) || (approverEmail != "" && g.ApproverEmail == approverEmail) {
			list = append(list, g)
		}
	}
	return list, nil
}

func (r *d10MockGoalRepo) Delete(ctx context.Context, ownerID, goalID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerID + "#" + goalID
	g, ok := r.goals[key]
	if !ok {
		return errors.New("goal not found")
	}
	if g.Status == "active" && (len(g.BlockedApps) > 0 || len(g.BlockedDomains) > 0) {
		return goals.ErrGoalLocked
	}
	delete(r.goals, key)
	return nil
}

func (r *d10MockGoalRepo) Update(ctx context.Context, ownerID, goalID string, input goals.UpdateGoalInput) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerID + "#" + goalID
	g, ok := r.goals[key]
	if !ok {
		return nil, errors.New("goal not found")
	}

	// Optimistic concurrency check
	if input.ExpectedVersion != nil && g.Version != *input.ExpectedVersion {
		return nil, goals.ErrGoalConflict
	}
	if input.ExpectedUpdatedAt != nil && *input.ExpectedUpdatedAt != "" && g.UpdatedAt != *input.ExpectedUpdatedAt {
		return nil, goals.ErrGoalConflict
	}

	if input.CheckTargetsUnchanged && g.Status == "active" {
		if input.BlockedApps != nil || input.BlockedDomains != nil {
			return nil, goals.ErrTargetsLocked
		}
	}

	if input.Title != nil {
		g.Title = *input.Title
	}
	if input.Description != nil {
		g.Description = *input.Description
	}
	if input.BlockedApps != nil {
		g.BlockedApps = *input.BlockedApps
	}
	if input.BlockedDomains != nil {
		g.BlockedDomains = *input.BlockedDomains
	}

	g.Version++
	g.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	r.goals[key] = g
	return &g, nil
}

func (r *d10MockGoalRepo) UpdateStatus(ctx context.Context, ownerID, goalID, status string) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerID + "#" + goalID
	g, ok := r.goals[key]
	if !ok {
		return nil, errors.New("goal not found")
	}
	g.Status = status
	g.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	r.goals[key] = g
	return &g, nil
}

type d10MockAuthRepo struct{}

func (a *d10MockAuthRepo) GetUserByEmail(ctx context.Context, email string) (interface{}, error) {
	return nil, nil
}

// 1. Optimistic Concurrency Control on Goal Updates
func TestD10_GoalUpdate_OptimisticConcurrency(t *testing.T) {
	repo := newD10MockGoalRepo()
	svc := goals.NewService(repo, nil)
	handler := goals.NewHandler(svc)

	ownerID := "user-concurrency-owner"
	goalID := "goal-race-test"
	initialUpdatedAt := "2026-11-11T12:00:00Z"

	_ = repo.Create(context.Background(), goals.Goal{
		ID:          goalID,
		OwnerID:     ownerID,
		Title:       "Original Title",
		Description: "Initial description",
		Status:      "active",
		Version:     1,
		CreatedAt:   initialUpdatedAt,
		UpdatedAt:   initialUpdatedAt,
	})

	// Client A reads goal at version 1 and submits update with If-Match: initialUpdatedAt
	reqA := httptest.NewRequest("PATCH", "/goals/"+goalID, bytes.NewBufferString(`{"title":"Client A Updated Title"}`))
	reqA.SetPathValue("id", goalID)
	reqA.Header.Set("If-Match", initialUpdatedAt)
	ctxA := context.WithValue(reqA.Context(), middleware.UserIDKey, ownerID)
	recA := httptest.NewRecorder()
	handler.Update(recA, reqA.WithContext(ctxA))

	if recA.Code != http.StatusOK {
		t.Fatalf("Client A update expected 200 OK, got %d (body: %s)", recA.Code, recA.Body.String())
	}

	// Client B submits update with STALE If-Match: initialUpdatedAt
	reqB := httptest.NewRequest("PATCH", "/goals/"+goalID, bytes.NewBufferString(`{"title":"Client B Stale Overwrite"}`))
	reqB.SetPathValue("id", goalID)
	reqB.Header.Set("If-Match", initialUpdatedAt) // Stale timestamp!
	ctxB := context.WithValue(reqB.Context(), middleware.UserIDKey, ownerID)
	recB := httptest.NewRecorder()
	handler.Update(recB, reqB.WithContext(ctxB))

	// Invariant: Stale concurrent request must be rejected with HTTP 409 Conflict
	if recB.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for stale concurrent update, got %d (body: %s)", recB.Code, recB.Body.String())
	}

	// Verify database preserved Client A's winning title
	g, _ := repo.GetByID(context.Background(), ownerID, goalID)
	if g.Title != "Client A Updated Title" {
		t.Fatalf("database title corrupted: expected 'Client A Updated Title', got %q", g.Title)
	}
	if g.Version != 2 {
		t.Fatalf("expected version 2, got %d", g.Version)
	}
}

// 2. Cross-Account Authorization & Approver Privilege Boundaries
func TestD10_CrossAccount_GoalProtection(t *testing.T) {
	repo := newD10MockGoalRepo()
	svc := goals.NewService(repo, nil)
	handler := goals.NewHandler(svc)

	ownerID := "legitimate-owner"
	approverID := "assigned-approver"
	attackerID := "malicious-stranger"
	goalID := "protected-goal-1"

	_ = repo.Create(context.Background(), goals.Goal{
		ID:         goalID,
		OwnerID:    ownerID,
		ApproverID: approverID,
		Title:      "Secret Goal",
		Status:     "active",
	})

	// Stranger attempts to view goal -> strictly rejected with 403 Forbidden
	reqView := httptest.NewRequest("GET", "/goals/"+goalID, nil)
	reqView.SetPathValue("id", goalID)
	ctxAttacker := context.WithValue(reqView.Context(), middleware.UserIDKey, attackerID)
	recView := httptest.NewRecorder()
	handler.GetByID(recView, reqView.WithContext(ctxAttacker))

	if recView.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unauthorized viewer, got %d", recView.Code)
	}

	// Approver attempts to delete owner's goal -> rejected
	reqDel := httptest.NewRequest("DELETE", "/goals/"+goalID, nil)
	reqDel.SetPathValue("id", goalID)
	ctxApprover := context.WithValue(reqDel.Context(), middleware.UserIDKey, approverID)
	recDel := httptest.NewRecorder()
	handler.Delete(recDel, reqDel.WithContext(ctxApprover))

	if recDel.Code != http.StatusNotFound && recDel.Code != http.StatusForbidden {
		t.Fatalf("approver must not be able to delete another user's goal, got %d", recDel.Code)
	}
}

// 3. Goal Active Target Locking Contract
func TestD10_ActiveGoal_TargetLocking(t *testing.T) {
	repo := newD10MockGoalRepo()
	svc := goals.NewService(repo, nil)
	handler := goals.NewHandler(svc)

	ownerID := "user-locking-owner"
	goalID := "goal-locked-targets"

	_ = repo.Create(context.Background(), goals.Goal{
		ID:             goalID,
		OwnerID:        ownerID,
		Title:          "Locked Accountability",
		Status:         "active",
		BlockedApps:    []string{"discord"},
		BlockedDomains: []string{"youtube.com"},
	})

	// Attempt to delete active goal with active restrictions -> strictly rejected
	reqDel := httptest.NewRequest("DELETE", "/goals/"+goalID, nil)
	reqDel.SetPathValue("id", goalID)
	ctx := context.WithValue(reqDel.Context(), middleware.UserIDKey, ownerID)
	recDel := httptest.NewRecorder()
	handler.Delete(recDel, reqDel.WithContext(ctx))

	if recDel.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when deleting active locked goal, got %d", recDel.Code)
	}
}
