package identitygen

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// IdentityOptions controls seeded identity generation.
type IdentityOptions struct {
	Seed string
	Size int // 0 means default 128
}

// CustomIdentityOptions specifies every identity trait explicitly, bypassing RNG.
type CustomIdentityOptions struct {
	Size          int
	BGPalette     string
	BGShade       int
	FGPalette     string
	FGShade       int
	EyePalette    string
	EyeShade      int
	Sides         int
	EdgeStyle     EdgeStyle
	Rotation      float64
	Radius        float64
	SpikeDepth    float64
	CurveAmount   float64
	LeftEyeStyle  EyeStyle
	RightEyeStyle EyeStyle
	EyeSpacing    float64
	EyeSize       float64
}

// identityChoices is the structured record of every decision generateIdentity
// makes for a given seed. Used by parity tests.
type identityChoices struct {
	Size          int
	BGPalette     string
	BGShade       int
	FGPalette     string
	FGShade       int
	EyePalette    string
	EyeShade      int
	Sides         int
	EdgeStyle     EdgeStyle
	Rotation      float64
	Radius        float64
	SpikeDepth    float64
	CurveAmount   float64
	LeftEyeStyle  EyeStyle
	RightEyeStyle EyeStyle
	EyeSpacing    float64
	EyeSize       float64
}

const minShadeDistance = 3

// GenerateIdentity returns a deterministic SVG identity for the given seed.
func GenerateIdentity(opts IdentityOptions) string {
	svg, _ := generateIdentityWithChoices(opts)
	return svg
}

// GenerateCustomIdentity returns an SVG built from explicit traits (no RNG).
func GenerateCustomIdentity(opts CustomIdentityOptions) string {
	size := opts.Size
	if size == 0 {
		size = 128
	}

	bgColor := palettes[opts.BGPalette][opts.BGShade]
	fgColor := palettes[opts.FGPalette][opts.FGShade]
	eyeColor := palettes[opts.EyePalette][opts.EyeShade]

	path := buildPolygonPath(PolygonParams{
		Sides:       opts.Sides,
		EdgeStyle:   opts.EdgeStyle,
		Rotation:    opts.Rotation,
		Radius:      opts.Radius,
		SpikeDepth:  opts.SpikeDepth,
		CurveAmount: opts.CurveAmount,
	}, size)

	eyes := buildEyes(EyeParams{
		LeftStyle:  opts.LeftEyeStyle,
		RightStyle: opts.RightEyeStyle,
		Spacing:    opts.EyeSpacing,
		EyeSize:    opts.EyeSize,
	}, size, eyeColor)

	return assembleSVG(size, bgColor, fgColor, path, eyes)
}

// generateIdentityWithChoices is the internal implementation shared by
// GenerateIdentity and tests. Every rng() call must occur in the same sequence
// as the TypeScript reference — that sequence is the parity contract.
func generateIdentityWithChoices(opts IdentityOptions) (string, identityChoices) {
	size := opts.Size
	if size == 0 {
		size = 128
	}

	rng := createRng(hash(opts.Seed))

	bgPalette := pick(paletteNames, rng)
	bgShade := pick(shadeKeys, rng)
	bgColor := palettes[bgPalette][bgShade]

	fgPalette := pick(paletteNames, rng)
	fgPool := shadesWithContrast([]int{bgShade}, minShadeDistance)
	var fgShade int
	if len(fgPool) > 0 {
		fgShade = pick(fgPool, rng)
	} else {
		fgShade = pickExcluding(shadeKeys, rng, []int{bgShade})
	}
	fgColor := palettes[fgPalette][fgShade]

	sides := int(math.Floor(rangeFloat(rng, 3, 9))) // 3–8
	edgeStyle := pick(edgeStyles, rng)
	rotation := rangeFloat(rng, 0, 2*math.Pi)
	radius := rangeFloat(rng, 0.55, 0.9)
	spikeDepth := rangeFloat(rng, 0.3, 0.7)
	curveAmount := rangeFloat(rng, 0.2, 0.5)

	path := buildPolygonPath(PolygonParams{
		Sides:       sides,
		EdgeStyle:   edgeStyle,
		Rotation:    rotation,
		Radius:      radius,
		SpikeDepth:  spikeDepth,
		CurveAmount: curveAmount,
	}, size)

	eyePalette := pick(paletteNames, rng)
	eyePool := shadesWithContrast([]int{bgShade, fgShade}, minShadeDistance)
	var eyeShade int
	if len(eyePool) > 0 {
		eyeShade = pick(eyePool, rng)
	} else {
		eyeShade = pickExcluding(shadeKeys, rng, []int{bgShade, fgShade})
	}
	eyeColor := palettes[eyePalette][eyeShade]

	leftEyeStyle := pick(eyeStyles, rng)
	// ~10% chance of mismatched eyes; the rng() call happens unconditionally here.
	var rightEyeStyle EyeStyle
	if rng() < 0.1 {
		rightEyeStyle = pickExcluding(eyeStyles, rng, []EyeStyle{leftEyeStyle})
	} else {
		rightEyeStyle = leftEyeStyle
	}
	eyeSpacing := rangeFloat(rng, 0.15, 0.35)
	eyeSize := rangeFloat(rng, 0.04, 0.1)

	eyes := buildEyes(EyeParams{
		LeftStyle:  leftEyeStyle,
		RightStyle: rightEyeStyle,
		Spacing:    eyeSpacing,
		EyeSize:    eyeSize,
	}, size, eyeColor)

	choices := identityChoices{
		Size:          size,
		BGPalette:     bgPalette,
		BGShade:       bgShade,
		FGPalette:     fgPalette,
		FGShade:       fgShade,
		EyePalette:    eyePalette,
		EyeShade:      eyeShade,
		Sides:         sides,
		EdgeStyle:     edgeStyle,
		Rotation:      rotation,
		Radius:        radius,
		SpikeDepth:    spikeDepth,
		CurveAmount:   curveAmount,
		LeftEyeStyle:  leftEyeStyle,
		RightEyeStyle: rightEyeStyle,
		EyeSpacing:    eyeSpacing,
		EyeSize:       eyeSize,
	}
	return assembleSVG(size, bgColor, fgColor, path, eyes), choices
}

func assembleSVG(size int, bgColor, fgColor, path, eyes string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, size, size, size, size)
	b.WriteByte('\n')
	fmt.Fprintf(&b, `  <rect width="%d" height="%d" fill="%s" />`, size, size, bgColor)
	b.WriteByte('\n')
	fmt.Fprintf(&b, `  <path d="%s" fill="%s" />`, path, fgColor)
	b.WriteByte('\n')
	b.WriteString("  ")
	b.WriteString(eyes)
	b.WriteByte('\n')
	b.WriteString(`</svg>`)
	return b.String()
}

// pick selects an element from arr using the next rng output, mirroring
// `arr[Math.floor(rng() * arr.length)]`.
func pick[T any](arr []T, rng func() float64) T {
	return arr[int(math.Floor(rng()*float64(len(arr))))]
}

// pickExcluding is pick, but drawing from (arr - exclude); if that's empty it
// falls back to arr so behavior matches the TS reference.
func pickExcluding[T comparable](arr []T, rng func() float64, exclude []T) T {
	filtered := make([]T, 0, len(arr))
	for _, v := range arr {
		if !slices.Contains(exclude, v) {
			filtered = append(filtered, v)
		}
	}
	pool := filtered
	if len(pool) == 0 {
		pool = arr
	}
	return pool[int(math.Floor(rng()*float64(len(pool))))]
}

// shadesWithContrast returns the shades at least `minDistance` steps away from
// every shade in `used` in the shadeKeys ordering.
func shadesWithContrast(used []int, minDistance int) []int {
	out := make([]int, 0, len(shadeKeys))
	for _, s := range shadeKeys {
		si := slices.Index(shadeKeys, s)
		ok := true
		for _, u := range used {
			if abs(si-slices.Index(shadeKeys, u)) < minDistance {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, s)
		}
	}
	return out
}

// rangeFloat maps a rng() output to [min, max).
func rangeFloat(rng func() float64, minV, maxV float64) float64 {
	return minV + rng()*(maxV-minV)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
