//go:build !darwin && !linux

package roon

import "net"

func enableBroadcast(conn *net.UDPConn) error {
	_ = conn
	return nil
}
