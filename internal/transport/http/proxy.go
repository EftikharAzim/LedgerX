package httptransport

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// TrustedProxyMiddleware accepts client addresses only from configured ingress
// peers. Walking from the right stops a client-supplied leftmost spoof from
// bypassing rate limits. Ingress must sanitize/append forwarding headers.
func TrustedProxyMiddleware(config string) (func(http.Handler) http.Handler, error) {
	var ranges []netip.Prefix
	for _, value := range strings.Split(config, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Bits() == 0 {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS must contain explicit non-zero CIDR ranges")
		}
		ranges = append(ranges, prefix.Masked())
	}
	trusted := func(ip netip.Addr) bool {
		for _, prefix := range ranges {
			if prefix.Contains(ip.Unmap()) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			peer, err := netip.ParseAddr(host)
			forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
			if err != nil || !trusted(peer) || forwarded == "" || len(forwarded) > 4096 {
				next.ServeHTTP(w, r)
				return
			}
			hops := strings.Split(forwarded, ",")
			if len(hops) > 32 {
				next.ServeHTTP(w, r)
				return
			}
			client := peer
			for i := len(hops) - 1; i >= 0 && trusted(client); i-- {
				candidate, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
				if err != nil {
					next.ServeHTTP(w, r)
					return
				}
				client = candidate.Unmap()
			}
			clone := r.Clone(r.Context())
			clone.RemoteAddr = net.JoinHostPort(client.String(), "0")
			next.ServeHTTP(w, clone)
		})
	}, nil
}
