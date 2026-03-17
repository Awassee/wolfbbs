package netutil

import (
	"net"
	"net/http"
	"strings"
)

type ProxyResolver struct {
	trusted []*net.IPNet
}

func NewProxyResolver(cidrs []string) (*ProxyResolver, error) {
	r := &ProxyResolver{trusted: []*net.IPNet{}}
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			// Allow single IP entries.
			if strings.Contains(raw, ":") {
				raw = raw + "/128"
			} else {
				raw = raw + "/32"
			}
		}
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, err
		}
		r.trusted = append(r.trusted, network)
	}
	return r, nil
}

func (r *ProxyResolver) Resolve(remoteAddr string, headers http.Header) string {
	remoteIP := parseRemoteIP(remoteAddr)
	if remoteIP == nil {
		return ""
	}
	if !r.isTrusted(remoteIP) {
		return remoteIP.String()
	}
	for _, ip := range parseForwardedIPs(headers.Get("X-Forwarded-For")) {
		return ip.String()
	}
	if ip := net.ParseIP(strings.TrimSpace(headers.Get("X-Real-IP"))); ip != nil {
		return ip.String()
	}
	return remoteIP.String()
}

func (r *ProxyResolver) isTrusted(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, network := range r.trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func parseForwardedIPs(raw string) []net.IP {
	parts := strings.Split(raw, ",")
	out := make([]net.IP, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if ip := net.ParseIP(part); ip != nil {
			out = append(out, ip)
		}
	}
	return out
}

func parseRemoteIP(remoteAddr string) net.IP {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return net.ParseIP(strings.TrimSpace(host))
	}
	return net.ParseIP(remoteAddr)
}
