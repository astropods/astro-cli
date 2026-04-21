package identitygen

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"strconv"
	"testing"
)

func TestGenerateIdentityJPEGBasics(t *testing.T) {
	seeds := []string{"", "alice", "account/agent-1", "stable", "日本語"}
	for _, seed := range seeds {
		t.Run(quoteSeed(seed), func(t *testing.T) {
			data, err := GenerateIdentityJPEG(IdentityOptions{Seed: seed, Size: 128})
			if err != nil {
				t.Fatalf("GenerateIdentityJPEG: %v", err)
			}
			img, err := jpeg.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("decode jpeg: %v", err)
			}
			b := img.Bounds()
			if b.Dx() != rasterSize || b.Dy() != rasterSize {
				t.Fatalf("expected %dx%d, got %dx%d", rasterSize, rasterSize, b.Dx(), b.Dy())
			}

			// The background is the first element drawn and fills the whole canvas,
			// so (0,0) must be within a small delta of the bg palette color.
			_, choices := generateIdentityWithChoices(IdentityOptions{Seed: seed, Size: 128})
			want := palettesHex[choices.BGPalette][choices.BGShade]
			r, g, bl, _ := img.At(0, 0).RGBA()
			wantR, wantG, wantB := parseHex(t, want)
			// JPEG is lossy; colors can shift by several units per channel.
			const tol = 10
			if diff(int(r>>8), wantR) > tol || diff(int(g>>8), wantG) > tol || diff(int(bl>>8), wantB) > tol {
				t.Fatalf("bg pixel (%d,%d,%d) not within %d of expected %s (%d,%d,%d)",
					r>>8, g>>8, bl>>8, tol, want, wantR, wantG, wantB)
			}
		})
	}
}

// TestRasterizeAllStyles sanity-checks every edge × eye combination through the
// rasterizer so any SVG feature oksvg mis-handles trips a test.
func TestRasterizeAllStyles(t *testing.T) {
	for _, edge := range edgeStyles {
		for _, eye := range eyeStyles {
			t.Run(fmt.Sprintf("%s_%s", edge, eye), func(t *testing.T) {
				svg := GenerateCustomIdentity(CustomIdentityOptions{
					Size:          128,
					BGPalette:     "teal",
					BGShade:       100,
					FGPalette:     "indigo",
					FGShade:       700,
					EyePalette:    "neutral",
					EyeShade:      950,
					Sides:         5,
					EdgeStyle:     edge,
					Rotation:      0.5,
					Radius:        0.8,
					SpikeDepth:    0.5,
					CurveAmount:   0.3,
					LeftEyeStyle:  eye,
					RightEyeStyle: eye,
					EyeSpacing:    0.25,
					EyeSize:       0.08,
				})
				data, err := rasterizeSVGToJPEG(svg)
				if err != nil {
					t.Fatalf("rasterize %s+%s: %v", edge, eye, err)
				}
				if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
					t.Fatalf("decode jpeg: %v", err)
				}
			})
		}
	}
}

func parseHex(t *testing.T, h string) (int, int, int) {
	t.Helper()
	if len(h) != 7 || h[0] != '#' {
		t.Fatalf("unexpected hex %q", h)
	}
	r, _ := strconv.ParseInt(h[1:3], 16, 0)
	g, _ := strconv.ParseInt(h[3:5], 16, 0)
	b, _ := strconv.ParseInt(h[5:7], 16, 0)
	return int(r), int(g), int(b)
}

func diff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}
