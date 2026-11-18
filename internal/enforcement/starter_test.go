//go:build windows

package enforcement

import (
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func TestEnsureDesktopAgentRunning(t *testing.T) {
	err := EnsureDesktopAgentRunning()
	if err != nil {
		t.Logf("EnsureDesktopAgentRunning notice: %v", err)
	}

	// Verify ping
	resp, errPing := queryLocalAgent(core.IPCRequest{Action: "ping"}, 2*time.Second)
	if errPing != nil || resp == nil || !resp.Success {
		t.Fatalf("Agent not responding to ping after EnsureDesktopAgentRunning: %v (resp: %+v)", errPing, resp)
	}
	t.Logf("Agent successfully responded to ping! resp: %+v", resp)
}
