package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func handleNativeFrame(ipcPath string, msgBuf []byte) core.IPCResponse {
	var req core.IPCRequest
	if err := json.Unmarshal(msgBuf, &req); err != nil {
		return core.IPCResponse{
			Success: false,
			Error:   "invalid json payload: " + err.Error(),
		}
	}

	if req.Action == "" {
		req.Action = "get-blocklist" // Default to fetching blocklist
	}

	// Strict Action Allowlist: Native messaging host only permits safe, read-only/status queries.
	// Privileged modifications, pairing changes, or arbitrary commands are strictly rejected.
	switch req.Action {
	case "get-blocklist", "sync", "status", "version", "heartbeat":
		// Permitted actions
	default:
		return core.IPCResponse{
			Success: false,
			Error:   "unauthorized native host action: " + req.Action,
		}
	}

	return queryAgent(ipcPath, req)
}

func main() {
	ipcPath := os.Getenv("ONUSLY_IPC_PATH")
	if ipcPath == "" {
		ipcPath = os.Getenv("ONUSLY_IPC_SOCKET")
	}
	if ipcPath == "" {
		ipcPath = getDefaultIPCPath()
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

		resp := handleNativeFrame(ipcPath, msgBuf)
		sendNativeResponse(resp)
	}
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
