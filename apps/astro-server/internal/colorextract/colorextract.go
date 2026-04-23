// Package colorextract extracts dominant colors from avatar images.
//
// It implements the same algorithm as the browser-side extraction in
// packages/astro-trading-card (MMCQ palette extraction + card color derivation)
// so that results are consistent between server and client.
package colorextract

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"math"
	"sort"

	"golang.org/x/image/draw"
)

// AvatarColors is the JSON-serializable color scheme extracted from an avatar.
type AvatarColors struct {
	Base         string `json:"base"`
	Vibrant      string `json:"vibrant"`
	VibrantLight string `json:"vibrant_light"`
	Accent       string `json:"accent"`
	AccentLight  string `json:"accent_light"`
	Background   string `json:"background"`
	Foreground   string `json:"foreground"`
	Glow         string `json:"glow"`
}

// ExtractFromJPEG decodes JPEG bytes and extracts avatar colors.
func ExtractFromJPEG(jpegBytes []byte) (*AvatarColors, error) {
	img, _, err := image.Decode(bytes.NewReader(jpegBytes))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	// Convert to RGBA if needed.
	rgba, ok := img.(*image.RGBA)
	if !ok {
		bounds := img.Bounds()
		rgba = image.NewRGBA(bounds)
		draw.Copy(rgba, image.Point{}, img, bounds, draw.Src, nil)
	}

	return ExtractFromRGBA(rgba)
}

// ExtractFromRGBA extracts avatar colors from an image.RGBA.
// The image is downsampled to 64x64 before palette extraction.
func ExtractFromRGBA(img *image.RGBA) (*AvatarColors, error) {
	// Downsample to 64x64 for palette extraction (matches frontend canvas size).
	const sampleSize = 64
	dst := image.NewRGBA(image.Rect(0, 0, sampleSize, sampleSize))
	draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	palette := extractPalette(dst.Pix, 8, 5)
	colors := pickCardColors(palette)
	if colors == nil {
		return nil, fmt.Errorf("no dominant colors found")
	}

	rgb := parseHex(colors.accent)
	if rgb == nil {
		return &AvatarColors{
			Base: colors.accent, Vibrant: colors.accent, VibrantLight: colors.accent,
			Accent: colors.accent, AccentLight: colors.accentLight,
			Background: colors.background, Foreground: colors.foreground, Glow: colors.glow,
		}, nil
	}

	h, s, l := rgbToHSL(rgb.r, rgb.g, rgb.b)

	return &AvatarColors{
		Base:         hslToHex(h, s*0.5, l),
		Vibrant:      hslToHex(h, math.Min(s, 0.5), 0.35),
		VibrantLight: hslToHex(h, math.Min(s, 0.6), 0.7),
		Accent:       colors.accent,
		AccentLight:  colors.accentLight,
		Background:   colors.background,
		Foreground:   colors.foreground,
		Glow:         colors.glow,
	}, nil
}

// --- MMCQ (Modified Median Cut Quantization) ---

type rgb struct {
	r, g, b int
}

type swatch struct {
	r, g, b    int
	population int
	hex        string
}

type colorBox struct {
	pixels     []rgb
	rMin, rMax int
	gMin, gMax int
	bMin, bMax int
}

func makeBox(pixels []rgb) colorBox {
	rMin, rMax := 255, 0
	gMin, gMax := 255, 0
	bMin, bMax := 255, 0
	for _, p := range pixels {
		if p.r < rMin {
			rMin = p.r
		}
		if p.r > rMax {
			rMax = p.r
		}
		if p.g < gMin {
			gMin = p.g
		}
		if p.g > gMax {
			gMax = p.g
		}
		if p.b < bMin {
			bMin = p.b
		}
		if p.b > bMax {
			bMax = p.b
		}
	}
	return colorBox{pixels: pixels, rMin: rMin, rMax: rMax, gMin: gMin, gMax: gMax, bMin: bMin, bMax: bMax}
}

func splitBox(box colorBox) (colorBox, colorBox) {
	rRange := box.rMax - box.rMin
	gRange := box.gMax - box.gMin
	bRange := box.bMax - box.bMin

	pixels := box.pixels

	if rRange >= gRange && rRange >= bRange {
		sort.Slice(pixels, func(i, j int) bool { return pixels[i].r < pixels[j].r })
	} else if gRange >= rRange && gRange >= bRange {
		sort.Slice(pixels, func(i, j int) bool { return pixels[i].g < pixels[j].g })
	} else {
		sort.Slice(pixels, func(i, j int) bool { return pixels[i].b < pixels[j].b })
	}

	mid := len(pixels) / 2
	return makeBox(pixels[:mid]), makeBox(pixels[mid:])
}

func averageBox(box colorBox) swatch {
	var rSum, gSum, bSum int
	for _, p := range box.pixels {
		rSum += p.r
		gSum += p.g
		bSum += p.b
	}
	n := len(box.pixels)
	r := int(math.Round(float64(rSum) / float64(n)))
	g := int(math.Round(float64(gSum) / float64(n)))
	b := int(math.Round(float64(bSum) / float64(n)))
	return swatch{r: r, g: g, b: b, population: n, hex: rgbToHexStr(r, g, b)}
}

func boxVolume(box colorBox) int {
	return (box.rMax - box.rMin + 1) * (box.gMax - box.gMin + 1) * (box.bMax - box.bMin + 1)
}

// samplePixels extracts RGB values from raw RGBA pixel data, skipping
// transparent, near-white, and near-black pixels.
func samplePixels(data []byte, quality int) []rgb {
	if quality < 1 {
		quality = 1
	}
	var pixels []rgb
	step := 4 * quality
	for i := 0; i+3 < len(data); i += step {
		a := data[i+3]
		if a < 128 {
			continue
		}
		r, g, b := int(data[i]), int(data[i+1]), int(data[i+2])
		if r > 240 && g > 240 && b > 240 {
			continue
		}
		if r < 15 && g < 15 && b < 15 {
			continue
		}
		pixels = append(pixels, rgb{r, g, b})
	}
	return pixels
}

// extractPalette extracts a color palette from raw RGBA pixel data using MMCQ.
func extractPalette(data []byte, paletteSize, quality int) []swatch {
	pixels := samplePixels(data, quality)
	if len(pixels) == 0 {
		return nil
	}

	boxes := []colorBox{makeBox(pixels)}

	for len(boxes) < paletteSize {
		maxVol := -1
		maxIdx := 0
		for i, box := range boxes {
			if len(box.pixels) < 2 {
				continue
			}
			vol := boxVolume(box)
			if vol > maxVol {
				maxVol = vol
				maxIdx = i
			}
		}
		if maxVol <= 0 {
			break
		}

		a, b := splitBox(boxes[maxIdx])
		boxes = append(boxes[:maxIdx], append([]colorBox{a, b}, boxes[maxIdx+1:]...)...)
	}

	var swatches []swatch
	for _, box := range boxes {
		if len(box.pixels) > 0 {
			swatches = append(swatches, averageBox(box))
		}
	}
	sort.Slice(swatches, func(i, j int) bool {
		return swatches[i].population > swatches[j].population
	})
	return swatches
}

// --- Target-based Swatch Selection (Android Palette algorithm) ---

type cardColors struct {
	accent      string
	accentLight string
	background  string
	foreground  string
	glow        string
}

type swatchTarget struct {
	targetSaturation, minSaturation, maxSaturation float64
	targetLightness, minLightness, maxLightness    float64
}

var (
	targetVibrant = swatchTarget{
		targetSaturation: 1.0, minSaturation: 0.35, maxSaturation: 1.0,
		targetLightness: 0.5, minLightness: 0.3, maxLightness: 0.7,
	}
	targetLightVibrant = swatchTarget{
		targetSaturation: 1.0, minSaturation: 0.35, maxSaturation: 1.0,
		targetLightness: 0.74, minLightness: 0.55, maxLightness: 0.9,
	}
	targetDarkVibrant = swatchTarget{
		targetSaturation: 1.0, minSaturation: 0.35, maxSaturation: 1.0,
		targetLightness: 0.26, minLightness: 0.1, maxLightness: 0.45,
	}
	targetMuted = swatchTarget{
		targetSaturation: 0.3, minSaturation: 0.0, maxSaturation: 0.4,
		targetLightness: 0.5, minLightness: 0.3, maxLightness: 0.7,
	}
)

const (
	weightSaturation = 0.24
	weightLightness  = 0.52
	weightPopulation = 0.24
)

// scoreForTarget returns 0-1 indicating how well a swatch matches a target.
func scoreForTarget(s swatch, t swatchTarget, maxPop int) float64 {
	_, sat, lum := rgbToHSL(s.r, s.g, s.b)
	if sat < t.minSaturation || sat > t.maxSaturation {
		return 0
	}
	if lum < t.minLightness || lum > t.maxLightness {
		return 0
	}
	satScore := 1 - math.Abs(sat-t.targetSaturation)
	lumScore := 1 - math.Abs(lum-t.targetLightness)
	popScore := 0.0
	if maxPop > 0 {
		popScore = float64(s.population) / float64(maxPop)
	}
	return satScore*weightSaturation + lumScore*weightLightness + popScore*weightPopulation
}

// pickForTarget picks the best swatch for a target, skipping indices in used.
func pickForTarget(swatches []swatch, t swatchTarget, maxPop int, used map[int]bool) (swatch, int, bool) {
	bestIdx := -1
	bestScore := 0.0
	for i, s := range swatches {
		if used[i] {
			continue
		}
		score := scoreForTarget(s, t, maxPop)
		if score > 0 && score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return swatch{}, -1, false
	}
	return swatches[bestIdx], bestIdx, true
}

// pickCardColors uses target-based selection to pick the best swatch for each
// role from the MMCQ palette, then derives a full color scheme.
func pickCardColors(swatches []swatch) *cardColors {
	if len(swatches) == 0 {
		return nil
	}

	maxPop := 0
	for _, s := range swatches {
		if s.population > maxPop {
			maxPop = s.population
		}
	}

	used := map[int]bool{}

	vibrant, vibrantIdx, hasVibrant := pickForTarget(swatches, targetVibrant, maxPop, used)
	if hasVibrant {
		used[vibrantIdx] = true
	}

	lightVibrant, lvIdx, hasLV := pickForTarget(swatches, targetLightVibrant, maxPop, used)
	if hasLV {
		used[lvIdx] = true
	}

	_, dvIdx, hasDV := pickForTarget(swatches, targetDarkVibrant, maxPop, used)
	if hasDV {
		used[dvIdx] = true
	}

	_, _, _ = pickForTarget(swatches, targetMuted, maxPop, used)

	// Accent is the vibrant swatch, falling back to the most populated
	accent := swatches[0] // swatches are sorted by population
	if hasVibrant {
		accent = vibrant
	}
	accentH, accentS, _ := rgbToHSL(accent.r, accent.g, accent.b)

	// AccentLight from light vibrant or derived
	var accentLightHex string
	if hasLV {
		lvH, lvS, _ := rgbToHSL(lightVibrant.r, lightVibrant.g, lightVibrant.b)
		accentLightHex = hslToHex(lvH, math.Min(lvS, 0.6), 0.75)
	} else {
		accentLightHex = hslToHex(accentH, math.Min(accentS, 0.4), 0.75)
	}

	return &cardColors{
		accent:      accent.hex,
		accentLight: accentLightHex,
		background:  hslToHex(accentH, math.Min(accentS, 0.5), 0.09),
		foreground:  hslToHex(accentH, math.Min(accentS, 0.1), 0.96),
		glow:        hslToHex(accentH, math.Min(accentS, 0.9), 0.8),
	}
}

// --- Color Conversion Utilities ---

// rgbToHSL converts RGB (0-255) to HSL (h: 0-360, s: 0-1, l: 0-1).
func rgbToHSL(ri, gi, bi int) (h, s, l float64) {
	r := float64(ri) / 255
	g := float64(gi) / 255
	b := float64(bi) / 255
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	l = (max + min) / 2

	if max == min {
		return 0, 0, l
	}

	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}

	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
		h /= 6
	case g:
		h = ((b-r)/d + 2) / 6
	default:
		h = ((r-g)/d + 4) / 6
	}

	h *= 360
	return h, s, l
}

func hue2rgb(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2 {
		return q
	}
	if t < 2.0/3 {
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}

// hslToHex converts HSL (h: 0-360, s: 0-1, l: 0-1) to a hex color string.
func hslToHex(h, s, l float64) string {
	h /= 360
	var r, g, b float64
	if s == 0 {
		r, g, b = l, l, l
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q
		r = hue2rgb(p, q, h+1.0/3)
		g = hue2rgb(p, q, h)
		b = hue2rgb(p, q, h-1.0/3)
	}
	return rgbToHexStr(
		int(math.Round(r*255)),
		int(math.Round(g*255)),
		int(math.Round(b*255)),
	)
}

// rgbToHexStr formats RGB (0-255) as a "#rrggbb" hex string.
func rgbToHexStr(r, g, b int) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// parseHex parses a "#rrggbb" hex color to RGB. Returns nil if invalid.
func parseHex(hex string) *rgb {
	if len(hex) != 7 || hex[0] != '#' {
		return nil
	}
	var r, g, b int
	n, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	if err != nil || n != 3 {
		return nil
	}
	return &rgb{r, g, b}
}

// Ensure jpeg decoder is registered.
func init() {
	_ = jpeg.Decode
}
