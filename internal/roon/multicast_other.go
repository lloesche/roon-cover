//go:build !darwin && !linux && !windows

package roon

import "net"

func setIPv4MulticastTTL(conn *net.UDPConn, ttl int) error {
	_, _ = conn, ttl
	return nil
}
