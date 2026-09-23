//go:build !darwin && !linux && !windows

package roon

import "net"

func enableBroadcast(conn *net.UDPConn) error {
	_ = conn
	return nil
}
