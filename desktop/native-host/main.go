package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

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

		var req core.IPCRequest
		_ = json.Unmarshal(msgBuf, &req)
		if req.Action == "" {
			req.Action = "get-blocklist" // Default to fetching blocklist
		}

		resp := queryAgent(ipcPath, req)
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
