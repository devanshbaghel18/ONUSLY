package apps

import "context"

// AppEnforcer abstracts platform-specific process restriction and termination.
type AppEnforcer interface {
	Enforce(ctx context.Context, proc ProcessInfo, target TargetApp) (bool, error)
	CapabilityTier() string
}

