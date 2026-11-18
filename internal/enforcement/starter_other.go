//go:build !windows

package enforcement

// EnsureDesktopAgentRunning is a stub on non-Windows platforms.
func EnsureDesktopAgentRunning() error {
	return nil
}
