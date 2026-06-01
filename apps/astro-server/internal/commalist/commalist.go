// Package commalist parses comma-separated configuration strings stored on
// cluster rows (VPCE IPs, pod subnet CIDRs).
package commalist

import "strings"

// Parse splits on commas, trims each token, and drops empty entries.
// Whitespace-only input returns nil.
func Parse(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
