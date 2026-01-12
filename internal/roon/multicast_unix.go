//go:build darwin || linux

package roon

import (
	"net"

	"golang.org/x/sys/unix"
)

func setIPv4MulticastTTL(conn *net.UDPConn, ttl int) error {
	rc, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var sockErr error
	if err := rc.Control(func(fd uintptr) {
		sockErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_MULTICAST_TTL, ttl)
	}); err != nil {
		return err
	}
	return sockErr
}
