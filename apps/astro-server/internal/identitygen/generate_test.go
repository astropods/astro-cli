package identitygen

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureChoices struct {
	Size          int     `json:"size"`
	BGPalette     string  `json:"bgPalette"`
	BGShade       int     `json:"bgShade"`
	FGPalette     string  `json:"fgPalette"`
	FGShade       int     `json:"fgShade"`
	EyePalette    string  `json:"eyePalette"`
	EyeShade      int     `json:"eyeShade"`
	Sides         int     `json:"sides"`
	EdgeStyle     string  `json:"edgeStyle"`
	Rotation      float64 `json:"rotation"`
	Radius        float64 `json:"radius"`
	SpikeDepth    float64 `json:"spikeDepth"`
	CurveAmount   float64 `json:"curveAmount"`
	LeftEyeStyle  string  `json:"leftEyeStyle"`
	RightEyeStyle string  `json:"rightEyeStyle"`
	EyeSpacing    float64 `json:"eyeSpacing"`
	EyeSize       float64 `json:"eyeSize"`
}

type choicesRow struct {
	Seed    string         `json:"seed"`
	Size    int            `json:"size"`
	Choices fixtureChoices `json:"choices"`
}

func loadChoices(t *testing.T) []choicesRow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "choices.json"))
	if err != nil {
		t.Fatalf("read choices.json: %v", err)
	}
	var rows []choicesRow
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatalf("parse choices.json: %v", err)
	}
	return rows
}

// floatEpsilon is the tolerance for choice-level float comparisons. The
// reference TS uses JS doubles + libm; Go uses libm. In practice values match
// to well under 1e-12, but 1e-9 gives us headroom.
const floatEpsilon = 1e-9

func TestChoicesParity(t *testing.T) {
	for _, row := range loadChoices(t) {
		name := fmt.Sprintf("%s_%d", quoteSeed(row.Seed), row.Size)
		t.Run(name, func(t *testing.T) {
			_, got := generateIdentityWithChoices(IdentityOptions{
				Seed: row.Seed,
				Size: row.Size,
			})
			w := row.Choices

			// Discrete fields must match exactly.
			mustEqual(t, "size", got.Size, w.Size)
			mustEqual(t, "bgPalette", got.BGPalette, w.BGPalette)
			mustEqual(t, "bgShade", got.BGShade, w.BGShade)
			mustEqual(t, "fgPalette", got.FGPalette, w.FGPalette)
			mustEqual(t, "fgShade", got.FGShade, w.FGShade)
			mustEqual(t, "eyePalette", got.EyePalette, w.EyePalette)
			mustEqual(t, "eyeShade", got.EyeShade, w.EyeShade)
			mustEqual(t, "sides", got.Sides, w.Sides)
			mustEqual(t, "edgeStyle", string(got.EdgeStyle), w.EdgeStyle)
			mustEqual(t, "leftEyeStyle", string(got.LeftEyeStyle), w.LeftEyeStyle)
			mustEqual(t, "rightEyeStyle", string(got.RightEyeStyle), w.RightEyeStyle)

			// Continuous fields within epsilon.
			mustClose(t, "rotation", got.Rotation, w.Rotation)
			mustClose(t, "radius", got.Radius, w.Radius)
			mustClose(t, "spikeDepth", got.SpikeDepth, w.SpikeDepth)
			mustClose(t, "curveAmount", got.CurveAmount, w.CurveAmount)
			mustClose(t, "eyeSpacing", got.EyeSpacing, w.EyeSpacing)
			mustClose(t, "eyeSize", got.EyeSize, w.EyeSize)
		})
	}
}

func mustEqual[T comparable](t *testing.T, field string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", field, got, want)
	}
}

func mustClose(t *testing.T, field string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > floatEpsilon {
		t.Errorf("%s: got %.17g, want %.17g (diff %.17g > %.17g)",
			field, got, want, math.Abs(got-want), floatEpsilon)
	}
}

// TestGenerateIdentitySVGStructure walks a few seeds and verifies the SVG is
// well-formed, has the right dimensions, the background color of the chosen
// palette, and two eye elements.
func TestGenerateIdentitySVGStructure(t *testing.T) {
	seeds := []string{"", "alice", "account/agent-1", "日本語", "😀-unicode"}
	for _, seed := range seeds {
		t.Run(quoteSeed(seed), func(t *testing.T) {
			svg, choices := generateIdentityWithChoices(IdentityOptions{Seed: seed, Size: 128})

			// Parse as XML — catches malformed attributes, quotes, etc.
			if err := xml.Unmarshal([]byte(svg), new(struct{ XMLName xml.Name })); err != nil {
				t.Fatalf("svg did not parse: %v\n%s", err, svg)
			}

			// Dimensions reflect size.
			if !strings.Contains(svg, `width="128"`) || !strings.Contains(svg, `height="128"`) {
				t.Errorf("expected width/height=128, got:\n%s", svg)
			}

			// Background fill matches chosen palette/shade.
			wantBG := palettesHex[choices.BGPalette][choices.BGShade]
			if !strings.Contains(svg, fmt.Sprintf(`<rect width="128" height="128" fill="%s"`, wantBG)) {
				t.Errorf("expected bg fill %q in svg", wantBG)
			}

			// Two eye elements.
			if n := strings.Count(svg, `class="dp-eye"`); n != 2 {
				t.Errorf("expected 2 eyes, got %d", n)
			}

			// Polygon path with the chosen fg color is present.
			wantFG := palettesHex[choices.FGPalette][choices.FGShade]
			if !strings.Contains(svg, fmt.Sprintf(`fill="%s"`, wantFG)) {
				t.Errorf("expected polygon fill %q in svg", wantFG)
			}
		})
	}
}

func TestGenerateIdentityDeterminism(t *testing.T) {
	first := GenerateIdentity(IdentityOptions{Seed: "stable"})
	for i := range 100 {
		if got := GenerateIdentity(IdentityOptions{Seed: "stable"}); got != first {
			t.Fatalf("determinism broken at iteration %d", i)
		}
	}
}

func TestGenerateIdentityDefaultSize(t *testing.T) {
	svg := GenerateIdentity(IdentityOptions{Seed: "default-size"})
	if !strings.Contains(svg, `width="128"`) || !strings.Contains(svg, `height="128"`) {
		t.Errorf("default size should be 128:\n%s", svg)
	}
}

func TestGenerateIdentitySizeRespected(t *testing.T) {
	for _, size := range []int{64, 128, 256} {
		svg := GenerateIdentity(IdentityOptions{Seed: "sz", Size: size})
		if !strings.Contains(svg, fmt.Sprintf(`width="%d"`, size)) {
			t.Errorf("size %d not reflected in width", size)
		}
	}
}

func TestGenerateCustomIdentityMatrix(t *testing.T) {
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
				if err := xml.Unmarshal([]byte(svg), new(struct{ XMLName xml.Name })); err != nil {
					t.Fatalf("svg did not parse: %v\n%s", err, svg)
				}
				if !strings.Contains(svg, `class="dp-eye"`) {
					t.Errorf("no dp-eye class present")
				}
			})
		}
	}
}
