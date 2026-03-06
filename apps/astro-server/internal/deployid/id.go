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
