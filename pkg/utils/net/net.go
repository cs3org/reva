package net

import "net"

func AddressEqual(a net.Addr, network, address string) bool {
	if a.Network() != network {
		return false
	}

	switch network {
	case "tcp":
		t, err := net.ResolveTCPAddr(network, address)
		if err != nil {
			return false
		}
		return tcpAddressEqual(a.(*net.TCPAddr), t)
	case "unix":
		t, err := net.ResolveUnixAddr(network, address)
		if err != nil {
			return false
		}
		return unixAddressEqual(a.(*net.UnixAddr), t)
	}
	return false
}

func tcpAddressEqual(a1, a2 *net.TCPAddr) bool {
	return a1.Port == a2.Port
}

func unixAddressEqual(a1, a2 *net.UnixAddr) bool {
	return a1.Name == a2.Name && a1.Net == a2.Net
}
