//go:build linux

package ipc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
)

// checkPeerCredentials verifies that the peer connected to the Unix domain socket
// is authorized.
// - In user mode: peer UID must match the process's running UID, or be root (UID 0).
// - In system root mode: peer UID must match an authorized paired UID or be root (UID 0).
func checkPeerCredentials(conn net.Conn, allowedUIDs []uint32) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("not a unix socket connection")
	}

	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("failed to get syscall connection: %w", err)
	}

	var cred *syscall.Ucred
	var credErr error
	err = rawConn.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return fmt.Errorf("control error on socket fd: %w", err)
	}
	if credErr != nil {
		return fmt.Errorf("failed to get SO_PEERCRED: %w", credErr)
	}

	currentUID := uint32(os.Geteuid())

	// Peer matching current running UID is authorized (user mode or root-to-root)
	if cred.Uid == currentUID {
		return nil
	}

	// Root (UID 0) is always authorized to query the agent (administration / status check)
	if cred.Uid == 0 {
		return nil
	}

	// In system daemon mode (running as root): check against authorized paired user UIDs
	for _, uid := range allowedUIDs {
		if cred.Uid == uid {
			return nil
		}
	}

	return fmt.Errorf("unauthorized peer UID %d (server running as UID %d, allowed UIDs: %v)", cred.Uid, currentUID, allowedUIDs)
}
