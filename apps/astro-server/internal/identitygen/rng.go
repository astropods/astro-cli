package identitygen

import "unicode/utf16"

// hash returns a djb2 hash of s, matching the TypeScript reference implementation.
// The seed is iterated as UTF-16 code units (mirroring JS `String.prototype.charCodeAt`),
// so ASCII inputs behave byte-wise and multi-byte code points are split at surrogate
// boundaries the same way JS does.
func hash(s string) uint32 {
	var h int32 = 5381
	for _, u := range utf16.Encode([]rune(s)) {
		// ((h << 5) + h + code) | 0  — all arithmetic wraps as int32 in JS.
		h = (h<<5 + h) + int32(u)
	}
	return uint32(h)
}

// createRng returns a Mulberry32 PRNG producing float64 values in [0, 1),
// matching the TypeScript reference implementation bit-for-bit.
func createRng(seed uint32) func() float64 {
	s := int32(seed) //nolint:gosec // intentional wrap — mirrors JS signed 32-bit arithmetic
	return func() float64 {
		// s = (s + 0x6d2b79f5) | 0
		s = s + 0x6d2b79f5

		// let t = Math.imul(s ^ (s >>> 15), 1 | s)
		t := imul(s^int32(uint32(s)>>15), 1|s) //nolint:gosec // intentional unsigned→signed cast for JS >>> emulation

		// t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
		t = (t + imul(t^int32(uint32(t)>>7), 61|t)) ^ t //nolint:gosec // intentional unsigned→signed cast for JS >>> emulation

		// return ((t ^ (t >>> 14)) >>> 0) / 4294967296
		return float64(uint32(t)^(uint32(t)>>14)) / 4294967296 //nolint:gosec // intentional signed→unsigned cast for JS >>> 0
	}
}

// imul replicates JavaScript's Math.imul: signed 32-bit integer multiplication
// with wrap-around semantics.
func imul(a, b int32) int32 {
	return int32(uint32(a) * uint32(b))
}
