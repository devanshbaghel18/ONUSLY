//go:build !linux

package ipc

import "net"

func checkPeerCredentials(conn net.Conn, allowedUIDs []uint32) error {
	return nil
}
