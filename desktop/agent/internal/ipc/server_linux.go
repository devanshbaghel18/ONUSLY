//go:build linux

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("ipc server already running")
	}

	isRoot := s.isRootFn()
	grpLookup := s.groupLookup
	if grpLookup == nil {
		grpLookup = s.defaultGroupLookup()
	}
	chown := s.chownFn
	if chown == nil {
		chown = os.Chown
	}
	chmod := s.chmodFn
	if chmod == nil {
		chmod = os.Chmod
	}

	sockDir := filepath.Dir(s.socketPath)

	var onuslyGID int
	if isRoot {
		// System mode requires the dedicated 'onusly' group to exist before startup.
		// If the group is missing, system installation/startup fails closed. Never fall back to 0666.
		grp, err := grpLookup("onusly")
		if err != nil {
			s.mu.Unlock()
			return fmt.Errorf("system mode requires dedicated 'onusly' group: %w", err)
		}
		gid, err := strconv.Atoi(grp.Gid)
		if err != nil {
			s.mu.Unlock()
			return fmt.Errorf("invalid GID for 'onusly' group (%s): %w", grp.Gid, err)
		}
		onuslyGID = gid

		// Ensure socket directory exists with mode 0750 owned by root:onusly.
		// Group members (paired browser user) have r-x (traversal & socket access), but cannot modify directory.
		// Others have 000 (no entry).
		if err := os.MkdirAll(sockDir, 0750); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to create socket dir %s: %w", sockDir, err)
		}
		if err := chown(sockDir, 0, onuslyGID); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to chown socket dir %s to root:onusly: %w", sockDir, err)
		}
		if err := chmod(sockDir, 0750); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to chmod socket dir %s to 0750: %w", sockDir, err)
		}

		// Verify directory is not world-writable
		if di, err := os.Stat(sockDir); err == nil && di.Mode().Perm()&0002 != 0 {
			s.mu.Unlock()
			return fmt.Errorf("socket directory %s is world-writable, refusing to start", sockDir)
		}
	} else {
		// User mode: socket directory is strictly user-private 0700
		if err := os.MkdirAll(sockDir, 0700); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to create socket dir %s: %w", sockDir, err)
		}
		if err := chmod(sockDir, 0700); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to chmod user socket dir %s to 0700: %w", sockDir, err)
		}
	}

	// Race-safe stale-socket cleanup: verify it is not a directory or dangerous entity
	if fi, err := os.Lstat(s.socketPath); err == nil {
		if fi.IsDir() {
			s.mu.Unlock()
			return fmt.Errorf("socket path %s is a directory, refusing to overwrite", s.socketPath)
		}
		if err := os.Remove(s.socketPath); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to remove stale socket %s: %w", s.socketPath, err)
		}
	}

	// Race-safe socket creation: set restrictive umask 0077 so net.Listen creates the
	// socket with owner-only access before we explicitly chown and chmod.
	oldUmask := syscall.Umask(0077)
	l, err := net.Listen("unix", s.socketPath)
	syscall.Umask(oldUmask)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to listen on unix socket %s: %w", s.socketPath, err)
	}

	// Enforce strict ownership and permissions on the created socket
	if isRoot {
		// System socket must be owned by root:onusly with mode 0660
		if err := chown(s.socketPath, 0, onuslyGID); err != nil {
			_ = l.Close()
			_ = os.Remove(s.socketPath)
			s.mu.Unlock()
			return fmt.Errorf("failed to chown socket to root:onusly: %w", err)
		}
		if err := chmod(s.socketPath, 0660); err != nil {
			_ = l.Close()
			_ = os.Remove(s.socketPath)
			s.mu.Unlock()
			return fmt.Errorf("failed to chmod socket to 0660: %w", err)
		}

		// Verify socket ownership and mode directly from filesystem
		si, err := os.Stat(s.socketPath)
		if err != nil {
			_ = l.Close()
			_ = os.Remove(s.socketPath)
			s.mu.Unlock()
			return fmt.Errorf("failed to stat created socket: %w", err)
		}
		if perm := si.Mode().Perm(); perm != 0660 {
			_ = l.Close()
			_ = os.Remove(s.socketPath)
			s.mu.Unlock()
			return fmt.Errorf("incorrect socket permissions: got %04o, expected 0660", perm)
		}
	} else {
		// User socket must be 0600
		if err := chmod(s.socketPath, 0600); err != nil {
			_ = l.Close()
			_ = os.Remove(s.socketPath)
			s.mu.Unlock()
			return fmt.Errorf("failed to chmod user socket to 0600: %w", err)
		}
		si, err := os.Stat(s.socketPath)
		if err != nil {
			_ = l.Close()
			_ = os.Remove(s.socketPath)
			s.mu.Unlock()
			return fmt.Errorf("failed to stat created user socket: %w", err)
		}
		if perm := si.Mode().Perm(); perm != 0600 {
			_ = l.Close()
			_ = os.Remove(s.socketPath)
			s.mu.Unlock()
			return fmt.Errorf("incorrect user socket permissions: got %04o, expected 0600", perm)
		}
	}

	s.listener = l
	s.running = true
	s.mu.Unlock()

	go s.acceptLoop(ctx)
	return nil
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}
	s.running = false
	close(s.stopChan)

	var err error
	if s.listener != nil {
		err = s.listener.Close()
		_ = os.Remove(s.socketPath)
	}
	return err
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			default:
				log.Printf("[IPC] Accept error: %v", err)
				continue
			}
		}

		go s.handleConnection(ctx, conn)
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	checker := s.peerCredChecker
	if checker == nil {
		checker = checkPeerCredentials
	}
	if err := checker(conn, s.allowedUIDs); err != nil {
		log.Printf("[IPC] Security rejection: %v", err)
		_ = json.NewEncoder(conn).Encode(core.IPCResponse{
			Success: false,
			Error:   "unauthorized: peer credentials rejected",
		})
		return
	}

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	limitedReader := io.LimitReader(conn, 64*1024)
	decoder := json.NewDecoder(limitedReader)
	encoder := json.NewEncoder(conn)

	for {
		var req core.IPCRequest
		if err := decoder.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			_ = encoder.Encode(core.IPCResponse{
				Success: false,
				Error:   "invalid json request",
			})
			return
		}

		resp := s.dispatch(ctx, req, "")
		if err := encoder.Encode(resp); err != nil {
			return
		}
	}
}

