//go:build !windows

package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func getDefaultIPCPath() string {
	systemSock := "/run/onusly/agent.sock"
	if _, err := os.Stat(systemSock); err == nil {
		return systemSock
	}

	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, ".config", "onusly", "agent.sock")
	}

	return systemSock
}

func queryAgent(socketPath string, req core.IPCRequest) core.IPCResponse {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return core.IPCResponse{
			Success: false,
			Error:   "agent unreachable: " + err.Error(),
		}
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return core.IPCResponse{
			Success: false,
			Error:   "failed to send request to agent: " + err.Error(),
		}
	}

	var resp core.IPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return core.IPCResponse{
			Success: false,
			Error:   "failed to decode agent response: " + err.Error(),
		}
	}

	return resp
}
