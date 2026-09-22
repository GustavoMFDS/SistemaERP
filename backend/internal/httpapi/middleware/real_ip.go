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
				if forwarded := firstForwardedIP(r.Header.Get("X-Forwarded-For")); forwarded != nil {
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

func firstForwardedIP(value string) net.IP {
	if value == "" {
		return nil
	}
	first := strings.TrimSpace(strings.Split(value, ",")[0])
	return net.ParseIP(first)
}

func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
