//go:build !windows

package ipc_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/ipc"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func TestIPC_AdversarialSecurity(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-adversarial-test-*")
	defer os.RemoveAll(tmpDir)

	sockPath := filepath.Join(tmpDir, "adversarial_agent.sock")
	syncer := &mockSyncer{
		blocklist: &core.BlocklistData{
			UserID:  "test-user",
			Version: "v1-security",
			Targets: []core.TargetItem{
				{
					GoalID:    "g1",
					GoalTitle: "Focus Goal",
					Status:    "active",
					Domains:   []string{"example.com"},
				},
			},
		},
	}
	credStore := &mockCredStore{token: "mock-token"}
	idStore := &mockIdentityStore{deviceID: "11111111-2222-3333-4444-555555555555"}

	server := ipc.NewServer(sockPath, syncer, credStore, idStore)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()
	time.Sleep(30 * time.Millisecond)

	// Subtest 1: Malformed JSON input
	t.Run("Malformed_JSON_Rejected", func(t *testing.T) {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		_, _ = conn.Write([]byte("{invalid-json-payload\n"))
		var resp core.IPCResponse
		_ = json.NewDecoder(conn).Decode(&resp)
		if resp.Success {
			t.Errorf("expected failure on malformed json, got success")
		}
	})

	// Subtest 2: Oversized payload (DDoS / Memory Exhaustion attempt)
	t.Run("Oversized_Payload_Blocked", func(t *testing.T) {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		oversized := `{"action": "ping", "junk": "` + strings.Repeat("A", 128*1024) + `"}`
		_, _ = conn.Write([]byte(oversized))
		var resp core.IPCResponse
		_ = json.NewDecoder(conn).Decode(&resp)
		if resp.Success {
			t.Errorf("expected failure on oversized payload exceeding 64KB, got success")
		}
	})

	// Subtest 3: Null bytes and binary junk
	t.Run("Binary_Garbage_Rejected", func(t *testing.T) {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		_, _ = conn.Write([]byte("\x00\xff\xfe\x00\x01\x02\x03\n"))
		var resp core.IPCResponse
		_ = json.NewDecoder(conn).Decode(&resp)
		if resp.Success {
			t.Errorf("expected failure on binary garbage, got success")
		}
	})

	// Subtest 4: Unapproved action names
	t.Run("Unapproved_Actions_Rejected", func(t *testing.T) {
		dangerousActions := []string{"rm -rf", "eval", "exec", "set-token", "dump-db", "shutdown"}
		for _, action := range dangerousActions {
			conn, err := net.Dial("unix", sockPath)
			if err != nil {
				t.Fatalf("failed to connect: %v", err)
			}
			req := core.IPCRequest{Action: action}
			_ = json.NewEncoder(conn).Encode(req)

			var resp core.IPCResponse
			_ = json.NewDecoder(conn).Decode(&resp)
			conn.Close()

			if resp.Success {
				t.Errorf("expected rejection for unapproved action %q, got success", action)
			}
		}
	})

	// Subtest 5: On-demand sync action
	t.Run("Sync_Action_Supported", func(t *testing.T) {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		req := core.IPCRequest{Action: "sync"}
		if err := json.NewEncoder(conn).Encode(req); err != nil {
			t.Fatalf("failed to encode: %v", err)
		}

		var resp core.IPCResponse
		if err := json.NewDecoder(conn).Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if !resp.Success || resp.Blocklist == nil {
			t.Fatalf("expected successful sync response, got %+v", resp)
		}
		if resp.Blocklist.Version != "v1-security" {
			t.Errorf("expected blocklist version v1-security, got %s", resp.Blocklist.Version)
		}
	})
}

func TestIPC_PeerCredentialAuthentication(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-peercred-test-*")
	defer os.RemoveAll(tmpDir)

	sockPath := filepath.Join(tmpDir, "system_agent.sock")
	syncer := &mockSyncer{
		blocklist: &core.BlocklistData{
			UserID:  "paired-user-1000",
			Version: "v1-system",
			Targets: []core.TargetItem{
				{
					GoalID:    "g-system",
					GoalTitle: "System Enforcement",
					Status:    "active",
					Domains:   []string{"restricted-site.internal"},
				},
			},
		},
	}
	credStore := &mockCredStore{token: "system-token"}
	idStore := &mockIdentityStore{deviceID: "22222222-3333-4444-5555-666666666666"}

	server := ipc.NewServer(sockPath, syncer, credStore, idStore)
	server.SetAllowedUIDs([]uint32{1000}) // Authorized desktop browser user UID

	// Simulated peer credential validator for cross-UID testing
	simulatedPeerUID := uint32(1000)
	server.SetPeerCredChecker(func(conn net.Conn, allowedUIDs []uint32) error {
		if simulatedPeerUID == 0 {
			return nil // root is always authorized
		}
		for _, uid := range allowedUIDs {
			if simulatedPeerUID == uid {
				return nil
			}
		}
		return fmt.Errorf("unauthorized peer UID %d", simulatedPeerUID)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()
	time.Sleep(30 * time.Millisecond)

	// Subtest 1: Authorized browser-user connection (UID 1000)
	t.Run("Authorized_Browser_User_Connected", func(t *testing.T) {
		simulatedPeerUID = 1000
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		req := core.IPCRequest{Action: "get-blocklist"}
		_ = json.NewEncoder(conn).Encode(req)

		var resp core.IPCResponse
		if err := json.NewDecoder(conn).Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if !resp.Success || resp.Blocklist == nil {
			t.Fatalf("expected successful blocklist response, got %+v", resp)
		}
		if resp.Blocklist.UserID != "paired-user-1000" {
			t.Errorf("expected blocklist for paired-user-1000, got %s", resp.Blocklist.UserID)
		}
	})

	// Subtest 2: Unauthorized different-user connection (UID 1001)
	t.Run("Unauthorized_Different_User_Rejected", func(t *testing.T) {
		simulatedPeerUID = 1001
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		req := core.IPCRequest{Action: "get-blocklist"}
		_ = json.NewEncoder(conn).Encode(req)

		var resp core.IPCResponse
		_ = json.NewDecoder(conn).Decode(&resp)
		if resp.Success {
			t.Errorf("expected security rejection for unauthorized UID 1001, but got success")
		}
		if !strings.Contains(resp.Error, "unauthorized") {
			t.Errorf("expected 'unauthorized' in error message, got %s", resp.Error)
		}
	})

	// Subtest 3: Root connection (UID 0) to system agent
	t.Run("Root_Connection_Allowed", func(t *testing.T) {
		simulatedPeerUID = 0
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		req := core.IPCRequest{Action: "get-status"}
		_ = json.NewEncoder(conn).Encode(req)

		var resp core.IPCResponse
		if err := json.NewDecoder(conn).Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if !resp.Success || resp.Status == nil {
			t.Fatalf("expected successful status response for root, got %+v", resp)
		}
		if resp.Status.DeviceID != "22222222-3333-4444-5555-666666666666" {
			t.Errorf("unexpected device ID: %s", resp.Status.DeviceID)
		}
	})

	// Subtest 4: Account Isolation (Different local user cannot query another user's blocklist)
	t.Run("Account_Isolation_Enforced", func(t *testing.T) {
		simulatedPeerUID = 5000 // Untrusted other local UID
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		req := core.IPCRequest{Action: "get-blocklist"}
		_ = json.NewEncoder(conn).Encode(req)

		var resp core.IPCResponse
		_ = json.NewDecoder(conn).Decode(&resp)
		if resp.Success || resp.Blocklist != nil {
			t.Fatalf("SECURITY VIOLATION: Unauthorized local UID 5000 received blocklist: %+v", resp)
		}
	})

	// Subtest 6: Real SO_PEERCRED Kernel Verification on Linux
	t.Run("Real_Kernel_SO_PEERCRED_Verification", func(t *testing.T) {
		// Create a separate server using the real un-mocked checkPeerCredentials
		realSockPath := filepath.Join(tmpDir, "real_peercred.sock")
		realServer := ipc.NewServer(realSockPath, syncer, credStore, idStore)
		realServer.SetAllowedUIDs([]uint32{uint32(os.Geteuid())})

		if err := realServer.Start(ctx); err != nil {
			t.Fatalf("failed to start real server: %v", err)
		}
		defer realServer.Stop()
		time.Sleep(20 * time.Millisecond)

		conn, err := net.Dial("unix", realSockPath)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		req := core.IPCRequest{Action: "ping"}
		_ = json.NewEncoder(conn).Encode(req)

		var resp core.IPCResponse
		if err := json.NewDecoder(conn).Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if !resp.Success {
			t.Errorf("expected real SO_PEERCRED check to succeed for current process UID %d, got %+v", os.Geteuid(), resp)
		}
	})
}

func TestIPC_SystemModePermissionsAndSecurity(t *testing.T) {
	syncer := &mockSyncer{
		blocklist: &core.BlocklistData{
			UserID:  "test-user",
			Version: "v1",
		},
	}
	credStore := &mockCredStore{token: "mock-token"}
	idStore := &mockIdentityStore{deviceID: "test-device"}

	t.Run("Missing_Onusly_Group_Fails_Closed", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "ipc-missing-group-*")
		defer os.RemoveAll(tmpDir)
		sockPath := filepath.Join(tmpDir, "agent.sock")

		server := ipc.NewServer(sockPath, syncer, credStore, idStore)
		server.SetIsRoot(func() bool { return true })
		server.SetGroupLookup(func(name string) (*user.Group, error) {
			return nil, errors.New("group: unknown group onusly")
		})

		ctx := context.Background()
		err := server.Start(ctx)
		if err == nil {
			server.Stop()
			t.Fatalf("expected server.Start to fail closed when 'onusly' group is missing, but it succeeded")
		}
		if !strings.Contains(err.Error(), "requires dedicated 'onusly' group") {
			t.Errorf("expected missing group error, got: %v", err)
		}

		// Ensure socket was NOT left behind
		if _, statErr := os.Stat(sockPath); !os.IsNotExist(statErr) {
			t.Errorf("socket file was created despite missing group: %v", statErr)
		}
	})

	t.Run("Failed_Permission_Assignment_Fails_Closed", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "ipc-failed-perm-*")
		defer os.RemoveAll(tmpDir)
		sockPath := filepath.Join(tmpDir, "agent.sock")

		server := ipc.NewServer(sockPath, syncer, credStore, idStore)
		server.SetIsRoot(func() bool { return true })
		server.SetGroupLookup(func(name string) (*user.Group, error) {
			return &user.Group{Gid: "999", Name: "onusly"}, nil
		})
		// Simulate chown failure
		server.SetFileOps(func(name string, uid, gid int) error {
			if strings.HasSuffix(name, "agent.sock") {
				return errors.New("operation not permitted (simulated chown failure)")
			}
			return nil
		}, nil)

		ctx := context.Background()
		err := server.Start(ctx)
		if err == nil {
			server.Stop()
			t.Fatalf("expected server.Start to fail closed when chown fails, but it succeeded")
		}
		if !strings.Contains(err.Error(), "failed to chown socket") {
			t.Errorf("expected chown error, got: %v", err)
		}

		// Ensure socket was cleaned up
		if _, statErr := os.Stat(sockPath); !os.IsNotExist(statErr) {
			t.Errorf("stale socket was not cleaned up after chown failure: %v", statErr)
		}
	})

	t.Run("Incorrect_Socket_Mode_Detected_And_Rejected", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "ipc-bad-mode-*")
		defer os.RemoveAll(tmpDir)
		sockPath := filepath.Join(tmpDir, "agent.sock")

		server := ipc.NewServer(sockPath, syncer, credStore, idStore)
		server.SetIsRoot(func() bool { return true })
		server.SetGroupLookup(func(name string) (*user.Group, error) {
			return &user.Group{Gid: "999", Name: "onusly"}, nil
		})
		// Simulate chmod setting incorrect mode (e.g. 0666 fallback violation)
		server.SetFileOps(func(name string, uid, gid int) error {
			return nil
		}, func(name string, mode os.FileMode) error {
			if strings.HasSuffix(name, "agent.sock") {
				// Deliberately set mode 0666 on the underlying file to test verification rejection
				return os.Chmod(name, 0666)
			}
			return nil
		})

		ctx := context.Background()
		err := server.Start(ctx)
		if err == nil {
			server.Stop()
			t.Fatalf("expected server.Start to fail when socket mode is incorrect, but it succeeded")
		}
		if !strings.Contains(err.Error(), "incorrect socket permissions") {
			t.Errorf("expected incorrect permissions error, got: %v", err)
		}
	})

	t.Run("User_Mode_Creates_Private_0600_Socket", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "ipc-user-mode-*")
		defer os.RemoveAll(tmpDir)
		sockPath := filepath.Join(tmpDir, "user_agent.sock")

		server := ipc.NewServer(sockPath, syncer, credStore, idStore)
		server.SetIsRoot(func() bool { return false }) // User mode

		ctx := context.Background()
		if err := server.Start(ctx); err != nil {
			t.Fatalf("expected user-mode start to succeed, got: %v", err)
		}
		defer server.Stop()

		fi, err := os.Stat(sockPath)
		if err != nil {
			t.Fatalf("failed to stat user socket: %v", err)
		}
		if fi.Mode().Perm() != 0600 {
			t.Errorf("expected user socket mode 0600, got %04o", fi.Mode().Perm())
		}

		dirInfo, err := os.Stat(tmpDir)
		if err != nil {
			t.Fatalf("failed to stat socket dir: %v", err)
		}
		if dirInfo.Mode().Perm() != 0700 {
			t.Errorf("expected user socket dir mode 0700, got %04o", dirInfo.Mode().Perm())
		}
	})

	t.Run("Stale_Directory_Collision_Refused", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "ipc-dir-collision-*")
		defer os.RemoveAll(tmpDir)
		sockPath := filepath.Join(tmpDir, "agent.sock")

		// Create a directory where the socket is supposed to be
		if err := os.MkdirAll(sockPath, 0755); err != nil {
			t.Fatalf("failed to create directory collision: %v", err)
		}

		server := ipc.NewServer(sockPath, syncer, credStore, idStore)
		server.SetIsRoot(func() bool { return false })

		ctx := context.Background()
		err := server.Start(ctx)
		if err == nil {
			server.Stop()
			t.Fatalf("expected start to fail when socket path is an existing directory, got success")
		}
		if !strings.Contains(err.Error(), "is a directory") {
			t.Errorf("expected directory collision error, got: %v", err)
		}
	})
}
