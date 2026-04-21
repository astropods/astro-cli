package identitygen

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestBuildEyeAllStylesProduceValidXML(t *testing.T) {
	for _, style := range eyeStyles {
		t.Run(string(style), func(t *testing.T) {
			frag := buildEye(style, 64, 64, 8, "oklch(50% 0.1 200)")
			if frag == "" {
				t.Fatal("empty eye fragment")
			}
			if !strings.Contains(frag, `class="dp-eye"`) {
				t.Fatalf("missing dp-eye class in %q", frag)
			}
			svg := `<svg xmlns="http://www.w3.org/2000/svg">` + frag + `</svg>`
			if err := xml.Unmarshal([]byte(svg), new(struct{ XMLName xml.Name })); err != nil {
				t.Fatalf("eye fragment did not parse: %v (%q)", err, frag)
			}
		})
	}
}

func TestBuildEyesRendersBoth(t *testing.T) {
	out := buildEyes(EyeParams{
		LeftStyle:  EyeDots,
		RightStyle: EyeRings,
		Spacing:    0.2,
		EyeSize:    0.08,
	}, 128, "oklch(50% 0.1 200)")
	// Two eyes means two elements with dp-eye class.
	if n := strings.Count(out, `class="dp-eye"`); n != 2 {
		t.Fatalf("expected 2 eye elements, got %d", n)
	}
	// Left eye is a circle with fill (dots), right is a circle with stroke (rings).
	if !strings.Contains(out, `fill="oklch(50% 0.1 200)"`) {
		t.Fatalf("expected left-eye fill, got: %s", out)
	}
	if !strings.Contains(out, `stroke="oklch(50% 0.1 200)"`) {
		t.Fatalf("expected right-eye stroke, got: %s", out)
	}
}
