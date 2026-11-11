package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func TestNativeHost_FramingProtocol(t *testing.T) {
	// 1. Simulate extension writing a request to native host stdin
	req := core.IPCRequest{Action: "get-blocklist"}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	var stdinBuf bytes.Buffer
	msgLen := uint32(len(reqBytes))
	if err := binary.Write(&stdinBuf, binary.LittleEndian, msgLen); err != nil {
		t.Fatalf("Failed to write length: %v", err)
	}
	stdinBuf.Write(reqBytes)

	// Read and verify framing
	var readLen uint32
	if err := binary.Read(&stdinBuf, binary.LittleEndian, &readLen); err != nil {
		t.Fatalf("Failed to read framed length: %v", err)
	}
	if readLen != msgLen {
		t.Fatalf("Length mismatch: got %d, expected %d", readLen, msgLen)
	}

	readMsg := make([]byte, readLen)
	if _, err := io.ReadFull(&stdinBuf, readMsg); err != nil {
		t.Fatalf("Failed to read payload: %v", err)
	}

	var decodedReq core.IPCRequest
	if err := json.Unmarshal(readMsg, &decodedReq); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}
	if decodedReq.Action != "get-blocklist" {
		t.Fatalf("Action mismatch: got %s, expected get-blocklist", decodedReq.Action)
	}
}

func TestNativeHost_ResponseFraming(t *testing.T) {
	resp := core.IPCResponse{
		Success: true,
		Blocklist: &core.BlocklistData{
			UserID:  "test-user-123",
			Version: "v1",
		},
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	var stdoutBuf bytes.Buffer
	msgLen := uint32(len(respBytes))
	_ = binary.Write(&stdoutBuf, binary.LittleEndian, msgLen)
	stdoutBuf.Write(respBytes)

	// Decode back
	var outLen uint32
	_ = binary.Read(&stdoutBuf, binary.LittleEndian, &outLen)
	if outLen != msgLen {
		t.Fatalf("Output length mismatch: got %d, expected %d", outLen, msgLen)
	}

	outPayload := make([]byte, outLen)
	_, _ = io.ReadFull(&stdoutBuf, outPayload)

	var decodedResp core.IPCResponse
	_ = json.Unmarshal(outPayload, &decodedResp)
	if !decodedResp.Success || decodedResp.Blocklist.UserID != "test-user-123" {
		t.Fatalf("Decoded response mismatch: %+v", decodedResp)
	}
}

func TestNativeHost_UnreachableAgent(t *testing.T) {
	// Querying non-existent socket or pipe must fail gracefully with structured IPCResponse
	resp := queryAgent("non_existent_target_socket_or_pipe_xyz", core.IPCRequest{Action: "ping"})
	if resp.Success {
		t.Fatalf("Expected failure for non-existent agent, got success")
	}
	if resp.Error == "" {
		t.Fatalf("Expected non-empty error message for unreachable agent")
	}
}

