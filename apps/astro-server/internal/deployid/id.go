package deployid

import (
	crypto_rand "crypto/rand"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// New generates a deployment ID in the format xxx-xxx-xxx using
// cryptographically random bytes. The ID space is 36^9 ≈ 101.6 trillion
// combinations, making collisions astronomically unlikely.
func New() string {
	var buf [11]byte // xxx-xxx-xxx
	b := make([]byte, 9)
	if _, err := crypto_rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	i := 0
	for seg := 0; seg < 3; seg++ {
		for j := 0; j < 3; j++ {
			buf[i] = alphabet[b[seg*3+j]%36]
			i++
		}
		if seg < 2 {
			buf[i] = '-'
			i++
		}
	}
	return string(buf[:])
}

// Compact returns the ID with hyphens stripped (9 chars), suitable for
// use in K8s namespace derivation.
func Compact(id string) string {
	var buf [9]byte
	j := 0
	for i := 0; i < len(id); i++ {
		if id[i] != '-' {
			buf[j] = id[i]
			j++
		}
	}
	return string(buf[:j])
}

// Expand reverses Compact, inserting hyphens to restore the xxx-xxx-xxx format.
// Returns empty string if the input is not exactly 9 characters.
func Expand(compact string) string {
	if len(compact) != 9 {
		return ""
	}
	return compact[:3] + "-" + compact[3:6] + "-" + compact[6:]
}

// FromNamespace extracts a deployment ID from a K8s namespace name.
// If the namespace matches the current format (astro-{9chars}-0), it returns
// the expanded xxx-xxx-xxx ID. Otherwise returns empty string.
func FromNamespace(namespace string) string {
	// Current format: astro-{compact}-0
	const prefix = "astro-"
	const suffix = "-0"
	if len(namespace) != len(prefix)+9+len(suffix) {
		return ""
	}
	if namespace[:len(prefix)] != prefix || namespace[len(namespace)-len(suffix):] != suffix {
		return ""
	}
	return Expand(namespace[len(prefix) : len(namespace)-len(suffix)])
}
