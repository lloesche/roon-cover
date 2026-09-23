//go:build !darwin && !linux && !windows

package roon

import "net"

func listenUDPReuse(network, address string) (*net.UDPConn, error) {
	return net.ListenUDP(network, &net.UDPAddr{Port: soodPort})
}
