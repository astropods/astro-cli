package colorextract

import (
	"image"
	"image/color"
	"math"
	"os"
	"testing"
)

func TestRGBToHSL(t *testing.T) {
	tests := []struct {
		name      string
		r, g, b   int
		wantH     float64
		wantS     float64
		wantL     float64
		tolerance float64
	}{
		{"pure red", 255, 0, 0, 0, 1.0, 0.5, 0.01},
		{"pure green", 0, 255, 0, 120, 1.0, 0.5, 0.01},
		{"pure blue", 0, 0, 255, 240, 1.0, 0.5, 0.01},
		{"white", 255, 255, 255, 0, 0, 1.0, 0.01},
		{"black", 0, 0, 0, 0, 0, 0, 0.01},
		{"mid gray", 128, 128, 128, 0, 0, 0.502, 0.01},
		{"teal-ish", 20, 184, 166, 173.4, 0.804, 0.4, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, s, l := rgbToHSL(tt.r, tt.g, tt.b)
			if math.Abs(h-tt.wantH) > tt.tolerance*360 {
				t.Errorf("hue: got %f, want %f", h, tt.wantH)
			}
			if math.Abs(s-tt.wantS) > tt.tolerance {
				t.Errorf("saturation: got %f, want %f", s, tt.wantS)
			}
			if math.Abs(l-tt.wantL) > tt.tolerance {
				t.Errorf("lightness: got %f, want %f", l, tt.wantL)
			}
		})
	}
}

func TestHSLToHex(t *testing.T) {
	tests := []struct {
		name    string
		h, s, l float64
		want    string
	}{
		{"pure red", 0, 1.0, 0.5, "#ff0000"},
		{"pure green", 120, 1.0, 0.5, "#00ff00"},
		{"pure blue", 240, 1.0, 0.5, "#0000ff"},
		{"white", 0, 0, 1.0, "#ffffff"},
		{"black", 0, 0, 0, "#000000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hslToHex(tt.h, tt.s, tt.l)
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestHSLRoundtrip(t *testing.T) {
	// Converting RGB → HSL → hex should produce the original color.
	colors := [][3]int{
		{255, 0, 0}, {0, 255, 0}, {0, 0, 255},
		{128, 64, 32}, {200, 100, 50}, {50, 150, 200},
	}
	for _, c := range colors {
		h, s, l := rgbToHSL(c[0], c[1], c[2])
		hex := hslToHex(h, s, l)
		got := parseHex(hex)
		if got == nil {
			t.Fatalf("parseHex(%s) returned nil", hex)
		}
		// Allow ±1 for float rounding.
		if abs(got.r-c[0]) > 1 || abs(got.g-c[1]) > 1 || abs(got.b-c[2]) > 1 {
			t.Errorf("roundtrip (%d,%d,%d) → HSL(%f,%f,%f) → %s → (%d,%d,%d)",
				c[0], c[1], c[2], h, s, l, hex, got.r, got.g, got.b)
		}
	}
}

func TestParseHex(t *testing.T) {
	tests := []struct {
		input string
		want  *rgb
	}{
		{"#ff0000", &rgb{255, 0, 0}},
		{"#00ff00", &rgb{0, 255, 0}},
		{"#0000ff", &rgb{0, 0, 255}},
		{"#a4c2f4", &rgb{164, 194, 244}},
		{"invalid", nil},
		{"#fff", nil},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseHex(tt.input)
			if tt.want == nil && got != nil {
				t.Errorf("expected nil, got %+v", got)
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("expected %+v, got nil", tt.want)
				}
				if *got != *tt.want {
					t.Errorf("got %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestExtractPalette_SingleColor(t *testing.T) {
	// Create a 4x4 image of solid red pixels (RGBA).
	data := make([]byte, 4*4*4)
	for i := 0; i < len(data); i += 4 {
		data[i] = 200   // R
		data[i+1] = 50  // G
		data[i+2] = 50  // B
		data[i+3] = 255 // A
	}

	palette := extractPalette(data, 8, 1)
	if len(palette) == 0 {
		t.Fatal("expected non-empty palette")
	}
	// With a single color, all swatches should converge.
	if palette[0].hex != "#c83232" {
		t.Errorf("expected #c83232, got %s", palette[0].hex)
	}
}

func TestExtractPalette_SkipsTransparent(t *testing.T) {
	// All transparent pixels → empty palette.
	data := make([]byte, 4*4*4)
	for i := 0; i < len(data); i += 4 {
		data[i] = 200
		data[i+1] = 50
		data[i+2] = 50
		data[i+3] = 0 // transparent
	}

	palette := extractPalette(data, 8, 1)
	if len(palette) != 0 {
		t.Errorf("expected empty palette for transparent pixels, got %d swatches", len(palette))
	}
}

func TestExtractFromRGBA(t *testing.T) {
	// Create a 64x64 image with a red-ish color.
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{R: 180, G: 60, B: 40, A: 255})
		}
	}

	colors, err := ExtractFromRGBA(img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if colors.Accent == "" {
		t.Error("accent should not be empty")
	}
	if colors.Base == "" {
		t.Error("base should not be empty")
	}
	if colors.Vibrant == "" {
		t.Error("vibrant should not be empty")
	}
	if colors.VibrantLight == "" {
		t.Error("vibrant_light should not be empty")
	}

	// Verify accent is a valid hex color.
	if parseHex(colors.Accent) == nil {
		t.Errorf("accent is not valid hex: %s", colors.Accent)
	}
}

func TestPickCardColors(t *testing.T) {
	swatches := []swatch{
		{r: 200, g: 50, b: 50, population: 100, hex: "#c83232"},
		{r: 50, g: 50, b: 200, population: 80, hex: "#3232c8"},
		{r: 128, g: 128, b: 128, population: 200, hex: "#808080"},
	}

	colors := pickCardColors(swatches)
	if colors == nil {
		t.Fatal("expected non-nil colors")
	}
	// The gray swatch has highest population but lowest saturation.
	// The red and blue swatches should score higher due to saturation.
	if colors.accent == "#808080" {
		t.Error("gray should not be selected as accent — saturation scoring should prefer chromatic colors")
	}
}

func TestSaturation(t *testing.T) {
	if s := saturation(255, 0, 0); s != 1.0 {
		t.Errorf("pure red saturation: got %f, want 1.0", s)
	}
	if s := saturation(128, 128, 128); s != 0.0 {
		t.Errorf("gray saturation: got %f, want 0.0", s)
	}
}

// TestCrossValidation_TypeScriptParity verifies that the Go color extraction
// produces the same results as the TypeScript implementation for the same input.
//
// The reference values below were generated by running the TypeScript pipeline
// (extractPalette → pickCardColors → BlueprintCard HSL derivations) on
// testdata/identity_test.jpg, which is a deterministic identity avatar for
// seed "acme/customer-insight-engine".
//
// To regenerate: go run ./internal/colorextract/testdata/gen_test_image.go
// To re-extract TS reference: bun run apps/astro-server/internal/colorextract/testdata/extract_ts_colors.ts
func TestCrossValidation_TypeScriptParity(t *testing.T) {
	jpegBytes, err := os.ReadFile("testdata/identity_test.jpg")
	if err != nil {
		t.Fatalf("read test image: %v", err)
	}

	got, err := ExtractFromJPEG(jpegBytes)
	if err != nil {
		t.Fatalf("ExtractFromJPEG: %v", err)
	}

	// Reference values from the TypeScript implementation.
	want := AvatarColors{
		Base:         "#291916",
		Vibrant:      "#863d2d",
		VibrantLight: "#e09685",
		Accent:       "#32130c",
		AccentLight:  "#e6bc99", // TS reference; Go may produce #e6bb99 (±1 green channel)
		Background:   "#22100b",
		Foreground:   "#f6f4f4",
		Glow:         "#ebb8ad",
	}

	fields := []struct {
		name      string
		got, want string
	}{
		{"base", got.Base, want.Base},
		{"vibrant", got.Vibrant, want.Vibrant},
		{"vibrant_light", got.VibrantLight, want.VibrantLight},
		{"accent", got.Accent, want.Accent},
		{"accent_light", got.AccentLight, want.AccentLight},
		{"background", got.Background, want.Background},
		{"foreground", got.Foreground, want.Foreground},
		{"glow", got.Glow, want.Glow},
	}

	for _, f := range fields {
		gotRGB := parseHex(f.got)
		wantRGB := parseHex(f.want)
		if gotRGB == nil || wantRGB == nil {
			t.Errorf("%s: invalid hex (got=%s, want=%s)", f.name, f.got, f.want)
			continue
		}
		// Allow ±1 per channel for float rounding differences between Go and TS.
		if abs(gotRGB.r-wantRGB.r) > 1 || abs(gotRGB.g-wantRGB.g) > 1 || abs(gotRGB.b-wantRGB.b) > 1 {
			t.Errorf("%s: got %s, want %s (diff: r=%d, g=%d, b=%d)",
				f.name, f.got, f.want,
				gotRGB.r-wantRGB.r, gotRGB.g-wantRGB.g, gotRGB.b-wantRGB.b)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
