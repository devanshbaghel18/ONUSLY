package approval_test

import (
	"context"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/approval"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

func TestDecisionListenerFunc(t *testing.T) {
	called := false
	var receivedApp *approval.Approval
	var receivedGoal *goals.Goal

	listener := approval.DecisionListenerFunc(func(ctx context.Context, app *approval.Approval, goal *goals.Goal) {
		called = true
		receivedApp = app
		receivedGoal = goal
	})

	testApp := &approval.Approval{
		ID:      "app-1",
		GoalID:  "goal-1",
		OwnerID: "owner-1",
		Status:  "approved",
	}
	testGoal := &goals.Goal{
		ID:    "goal-1",
		Title: "Test Goal",
	}

	listener.OnDecision(context.Background(), testApp, testGoal)

	if !called {
		t.Fatalf("expected listener to be called")
	}
	if receivedApp == nil || receivedApp.ID != "app-1" {
		t.Fatalf("expected receivedApp with ID app-1, got %v", receivedApp)
	}
	if receivedGoal == nil || receivedGoal.Title != "Test Goal" {
		t.Fatalf("expected receivedGoal with title 'Test Goal', got %v", receivedGoal)
	}
}
