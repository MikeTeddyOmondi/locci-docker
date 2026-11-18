package server

import (
	"fmt"
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// DefaultProxies are trusted when LOCCI_TRUSTED_PROXIES is unset: loopback and
// private networks, where a reverse proxy in front of locci normally sits
// (the same host, a Docker network, a LAN).
const DefaultProxies = "127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7"

// ParseProxies parses LOCCI_TRUSTED_PROXIES: comma separated IPs or CIDRs,
// "*" for any address, "none" for no proxy at all. Empty means DefaultProxies.
func ParseProxies(v string) ([]string, error) {
	v = strings.TrimSpace(v)
	switch v {
	case "":
		v = DefaultProxies
	case "none":
		return []string{}, nil
	case "*":
		return []string{"0.0.0.0/0", "::/0"}, nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			ip := net.ParseIP(p)
			if ip == nil {
				return nil, fmt.Errorf("LOCCI_TRUSTED_PROXIES: %q is not an IP or CIDR", p)
			}
			if ip.To4() != nil {
				p += "/32"
			} else {
				p += "/128"
			}
		}
		if _, _, err := net.ParseCIDR(p); err != nil {
			return nil, fmt.Errorf("LOCCI_TRUSTED_PROXIES: %q is not an IP or CIDR", p)
		}
		out = append(out, p)
	}
	return out, nil
}

// proxies returns the trusted networks (DefaultProxies when none were set).
func (s *Server) proxies() []*net.IPNet {
	s.proxyOnce.Do(func() {
		list := s.TrustedProxies
		if list == nil {
			list, _ = ParseProxies("")
		}
		for _, p := range list {
			if _, n, err := net.ParseCIDR(p); err == nil {
				s.proxyNets = append(s.proxyNets, n)
			}
		}
	})
	return s.proxyNets
}

// fromTrustedProxy reports whether the request came straight from a trusted
// proxy, so its X-Forwarded-* headers may be believed.
func (s *Server) fromTrustedProxy(c *gin.Context) bool {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		host = c.Request.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range s.proxies() {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
