//go:build !windows

package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

// Query connects to the Unix domain socket, sends an IPCRequest, and returns the decoded IPCResponse.
func Query(socketPath string, req core.IPCRequest, timeout time.Duration) (*core.IPCResponse, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, fmt.Errorf("agent socket unreachable at %s: %w", socketPath, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	var resp core.IPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response JSON: %w", err)
	}

	return &resp, nil
}

