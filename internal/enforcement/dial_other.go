//go:build !windows

package enforcement

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func queryLocalAgent(req core.IPCRequest, timeout time.Duration) (*core.IPCResponse, error) {
	socketPath := os.Getenv("ONUSLY_IPC_PATH")
	if socketPath == "" {
		socketPath = "/var/run/onusly/agent.sock"
	}

	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, fmt.Errorf("desktop agent unreachable on %s: %w", socketPath, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	var resp core.IPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to parse agent response: %w", err)
	}

	return &resp, nil
}
