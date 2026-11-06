package ipc_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/ipc"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type mockSyncer struct {
	blocklist *core.BlocklistData
}

func (m *mockSyncer) Sync(ctx context.Context) (*core.BlocklistData, error) {
	return m.blocklist, nil
}

func (m *mockSyncer) GetCachedBlocklist() (*core.BlocklistData, error) {
	return m.blocklist, nil
}

type mockIdentityStore struct {
	deviceID string
}

func (m *mockIdentityStore) GetDeviceID() (string, error) {
	return m.deviceID, nil
}

type mockCredStore struct {
	token string
}

func (m *mockCredStore) SaveToken(t string) error  { m.token = t; return nil }
func (m *mockCredStore) GetToken() (string, error) { return m.token, nil }
func (m *mockCredStore) ClearToken() error         { m.token = ""; return nil }

func TestIPC_ServerDispatchAndSecurity(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-ipc-test-*")
	defer os.RemoveAll(tmpDir)

	sockPath := filepath.Join(tmpDir, "test_agent.sock")

	syncer := &mockSyncer{
		blocklist: &core.BlocklistData{
			UserID:  "user-desktop-owner",
			Version: "v1-ipc987",
			Targets: []core.TargetItem{
				{
					GoalID:    "g-reddit",
					GoalTitle: "Block Reddit",
					Status:    "active",
					Domains:   []string{"reddit.com"},
				},
			},
		},
	}
	credStore := &mockCredStore{token: "mock-jwt"}
	idStore := &mockIdentityStore{deviceID: "dddddddd-4444-4444-4444-444444444444"}

	server := ipc.NewServer(sockPath, syncer, credStore, idStore)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("failed to start IPC server: %v", err)
	}
	defer server.Stop()

	// Wait briefly for socket to become ready
	time.Sleep(30 * time.Millisecond)

	// 1. Connect client and request "get-blocklist"
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to connect to IPC socket: %v", err)
	}
	defer conn.Close()

	req := core.IPCRequest{Action: "get-blocklist"}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("failed to write IPC request: %v", err)
	}

	var resp core.IPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("failed to read IPC response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success: true, got error: %s", resp.Error)
	}
	if resp.Blocklist == nil || len(resp.Blocklist.Targets) != 1 {
		t.Fatalf("expected 1 target in blocklist, got %+v", resp.Blocklist)
	}
	if resp.Blocklist.Targets[0].Domains[0] != "reddit.com" {
		t.Errorf("expected domain reddit.com, got %s", resp.Blocklist.Targets[0].Domains[0])
	}

	// 2. Request "get-status"
	reqStatus := core.IPCRequest{Action: "get-status"}
	if err := json.NewEncoder(conn).Encode(reqStatus); err != nil {
		t.Fatalf("failed to send get-status: %v", err)
	}

	var respStatus core.IPCResponse
	if err := json.NewDecoder(conn).Decode(&respStatus); err != nil {
		t.Fatalf("failed to read status response: %v", err)
	}
	if !respStatus.Success || respStatus.Status == nil {
		t.Fatalf("expected successful status response, got %+v", respStatus)
	}
	if respStatus.Status.DeviceID != "dddddddd-4444-4444-4444-444444444444" {
		t.Errorf("expected deviceId in status, got %s", respStatus.Status.DeviceID)
	}

	// 3. Request unauthorized / arbitrary command -> strictly rejected
	reqHax := core.IPCRequest{Action: "execute-shell"}
	if err := json.NewEncoder(conn).Encode(reqHax); err != nil {
		t.Fatalf("failed to send exploit action: %v", err)
	}

	var respHax core.IPCResponse
	_ = json.NewDecoder(conn).Decode(&respHax)
	if respHax.Success {
		t.Fatalf("SECURITY VIOLATION: arbitrary action must be rejected, got success: true")
	}
}
