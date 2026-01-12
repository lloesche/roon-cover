//go:build darwin || linux

package roon

import (
	"context"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func listenUDPReuse(network, address string) (*net.UDPConn, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			if controlErr := c.Control(func(fd uintptr) {
				// Best-effort reuse, like node-roon-api's reuseAddr: true.
				_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				// On some platforms, REUSEPORT helps multiple listeners; ignore failures.
				_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			}); controlErr != nil {
				return controlErr
			}
			return err
		},
	}

	pc, err := lc.ListenPacket(context.Background(), network, address)
	if err != nil {
		return nil, err
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, syscall.EINVAL
	}
	return conn, nil
}
