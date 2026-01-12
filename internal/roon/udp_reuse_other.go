//go:build !darwin && !linux

package roon

import "net"

func listenUDPReuse(network, address string) (*net.UDPConn, error) {
	return net.ListenUDP(network, &net.UDPAddr{Port: soodPort})
}
