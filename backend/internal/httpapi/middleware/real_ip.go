package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/config"
)

func TrustedRealIP(cfg config.Config) func(http.Handler) http.Handler {
	trusted := make([]*net.IPNet, 0, len(cfg.TrustedProxyCIDRs))
	for _, raw := range cfg.TrustedProxyCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err == nil {
			trusted = append(trusted, network)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			remoteIP := parseRemoteIP(r.RemoteAddr)
			if remoteIP != nil && ipInNetworks(remoteIP, trusted) {
				if forwarded := forwardedClientIP(r.Header.Get("X-Forwarded-For"), trusted); forwarded != nil {
					r.RemoteAddr = forwarded.String()
				} else if realIP := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); realIP != nil {
					r.RemoteAddr = realIP.String()
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseRemoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(strings.TrimSpace(remoteAddr))
}

func forwardedClientIP(value string, trusted []*net.IPNet) net.IP {
	parts := strings.Split(value, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(parts[i]))
		if ip == nil {
			continue
		}
		if !ipInNetworks(ip, trusted) {
			return ip
		}
	}
	return nil
}

func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
