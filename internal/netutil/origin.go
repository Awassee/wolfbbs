package netutil

import (
	"net"
	"strings"
)

// RemoteHost normalizes a remote endpoint string and returns the host/IP portion.
func RemoteHost(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return strings.TrimSpace(strings.Trim(host, "[]"))
	}
	return strings.TrimSpace(strings.Trim(remoteAddr, "[]"))
}

// RemoteOrigin returns a coarse caller origin class for classic node/caller displays.
func RemoteOrigin(remoteAddr string) string {
	host := RemoteHost(remoteAddr)
	if host == "" {
		return "unknown"
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "host"
	}
	if ip.IsLoopback() {
		return "loopback"
	}
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || isCarrierGradeNAT(ip) {
		return "lan"
	}
	return "wan"
}

func isCarrierGradeNAT(ip net.IP) bool {
	// RFC 6598 shared address space: 100.64.0.0/10
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
}
