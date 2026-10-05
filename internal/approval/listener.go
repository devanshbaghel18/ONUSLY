package approval

import (
	"context"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

// DecisionListener is invoked when an approval decision has successfully committed to the database.
type DecisionListener interface {
	OnDecision(ctx context.Context, approval *Approval, goal *goals.Goal)
}

// DecisionListenerFunc allows an ordinary function to be used as a DecisionListener.
type DecisionListenerFunc func(ctx context.Context, approval *Approval, goal *goals.Goal)

// OnDecision calls f(ctx, approval, goal).
func (f DecisionListenerFunc) OnDecision(ctx context.Context, approval *Approval, goal *goals.Goal) {
	f(ctx, approval, goal)
}
