package identitygen

import (
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type hashRngRow struct {
	Seed   string   `json:"seed"`
	Hash   uint32   `json:"hash"`
	Rng100 []string `json:"rng100"` // IEEE 754 big-endian hex
}

func loadHashRng(t *testing.T) []hashRngRow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "hash_rng.json"))
	if err != nil {
		t.Fatalf("read hash_rng.json: %v", err)
	}
	var rows []hashRngRow
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatalf("parse hash_rng.json: %v", err)
	}
	return rows
}

// bitsToFloat decodes a big-endian hex string produced by the TS fixture into
// its IEEE 754 float64 value. We compare bit patterns rather than float values
// so we catch any drift, including in NaN/±0 corner cases.
func bitsToFloat(t *testing.T, h string) float64 {
	t.Helper()
	b, err := hex.DecodeString(h)
	if err != nil {
		t.Fatalf("decode hex %q: %v", h, err)
	}
	if len(b) != 8 {
		t.Fatalf("expected 8 bytes, got %d for %q", len(b), h)
	}
	var u uint64
	for _, x := range b {
		u = (u << 8) | uint64(x)
	}
	return math.Float64frombits(u)
}

func TestHashParity(t *testing.T) {
	for _, row := range loadHashRng(t) {
		t.Run(quoteSeed(row.Seed), func(t *testing.T) {
			got := hash(row.Seed)
			if got != row.Hash {
				t.Fatalf("hash(%q) = %d, want %d", row.Seed, got, row.Hash)
			}
		})
	}
}

func TestRngParity(t *testing.T) {
	for _, row := range loadHashRng(t) {
		t.Run(quoteSeed(row.Seed), func(t *testing.T) {
			rng := createRng(row.Hash)
			for i, wantHex := range row.Rng100 {
				got := rng()
				want := bitsToFloat(t, wantHex)
				if math.Float64bits(got) != math.Float64bits(want) {
					t.Fatalf("seed=%q rng[%d]: got bits=%016x want bits=%016x (got=%v want=%v)",
						row.Seed, i, math.Float64bits(got), math.Float64bits(want), got, want)
				}
			}
		})
	}
}

func TestRngRangeBounds(t *testing.T) {
	// Regardless of parity, output must be in [0, 1) across a broad sample.
	rng := createRng(123)
	for i := range 10000 {
		v := rng()
		if v < 0 || v >= 1 {
			t.Fatalf("rng()[%d] = %v, out of [0, 1)", i, v)
		}
	}
}

// quoteSeed renders a seed suitable as a subtest name — avoids slashes / unicode
// collapsing names.
func quoteSeed(s string) string {
	if s == "" {
		return "empty"
	}
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r == '/' || r == ' ':
			out = append(out, '_')
		case r >= 0x20 && r < 0x7f:
			out = append(out, byte(r))
		default:
			// Replace non-ASCII with a placeholder so subtest names stay stable.
			out = append(out, '.')
		}
	}
	return string(out)
}
