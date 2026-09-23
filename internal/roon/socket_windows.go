package roon

import (
	"context"
	"golang.org/x/sys/windows"
	"net"
	"syscall"
)

func listenUDPReuse(network, address string) (*net.UDPConn, error) {
	lc := net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
		var optErr error
		err := raw.Control(func(fd uintptr) {
			optErr = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, 1)
		})
		if err != nil {
			return err
		}
		return optErr
	}}
	pc, err := lc.ListenPacket(context.Background(), network, address)
	if err != nil {
		return nil, err
	}
	return pc.(*net.UDPConn), nil
}
func setSocketOption(conn *net.UDPConn, level, opt, value int) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var optErr error
	err = raw.Control(func(fd uintptr) { optErr = windows.SetsockoptInt(windows.Handle(fd), level, opt, value) })
	if err != nil {
		return err
	}
	return optErr
}
func enableBroadcast(conn *net.UDPConn) error {
	return setSocketOption(conn, windows.SOL_SOCKET, windows.SO_BROADCAST, 1)
}
func setIPv4MulticastTTL(conn *net.UDPConn, ttl int) error {
	return setSocketOption(conn, windows.IPPROTO_IP, windows.IP_MULTICAST_TTL, ttl)
}
