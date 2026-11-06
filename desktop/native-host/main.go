package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func getDefaultSocketPath() string {
	// If system-wide socket exists, use it
	systemSock := "/run/onusly/agent.sock"
	if _, err := os.Stat(systemSock); err == nil {
		return systemSock
	}

	// Fallback to user-level socket
	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, ".config", "onusly", "agent.sock")
	}

	return systemSock
}

func main() {
	socketPath := os.Getenv("ONUSLY_IPC_SOCKET")
	if socketPath == "" {
		socketPath = getDefaultSocketPath()
	}

	for {
		// Read 4-byte little-endian message length from Stdin
		var msgLen uint32
		if err := binary.Read(os.Stdin, binary.LittleEndian, &msgLen); err != nil {
			if err == io.EOF {
				return // Browser closed pipe cleanly
			}
			return
		}

		if msgLen == 0 || msgLen > 1024*1024 { // 1MB sanity check
			return
		}

		msgBuf := make([]byte, msgLen)
		if _, err := io.ReadFull(os.Stdin, msgBuf); err != nil {
			return
		}

		var req core.IPCRequest
		_ = json.Unmarshal(msgBuf, &req)
		if req.Action == "" {
			req.Action = "get-blocklist" // Default to fetching blocklist
		}

		resp := queryAgent(socketPath, req)
		sendNativeResponse(resp)
	}
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

func sendNativeResponse(resp core.IPCResponse) {
	respBytes, err := json.Marshal(resp)
	if err != nil {
		return
	}

	msgLen := uint32(len(respBytes))
	_ = binary.Write(os.Stdout, binary.LittleEndian, msgLen)
	_, _ = os.Stdout.Write(respBytes)
}
