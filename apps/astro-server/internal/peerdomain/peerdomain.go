// Package peerdomain reduces the peer hostnames Beyla reports to the domain
// clients should group them under. Shared by the per-deployment network
// endpoints and the fleet-wide admin aggregation so both agree on what
// counts as a domain.
package peerdomain

import (
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Registrable returns the eTLD+1 for an address peer, so clients can group a
// vendor's hosts without shipping a public suffix list of their own. Empty for
// anything that is not a registrable domain — bare IPs, single-label internal
// names — which callers treat as "stands alone".
func Registrable(peer string) string {
	host := peer
	if h, _, err := net.SplitHostPort(peer); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" || net.ParseIP(host) != nil {
		return ""
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return ""
	}
	return domain
}
