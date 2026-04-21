package identitygen

import (
	"fmt"
	"math"
	"strings"
)

// EyeStyle names one of the eight eye rendering styles.
type EyeStyle string

const (
	EyeDots        EyeStyle = "dots"
	EyeRings       EyeStyle = "rings"
	EyeSlits       EyeStyle = "slits"
	EyeTriangles   EyeStyle = "triangles"
	EyeDashes      EyeStyle = "dashes"
	EyeSquares     EyeStyle = "squares"
	EyeSemicircles EyeStyle = "semicircles"
	EyeDiamonds    EyeStyle = "diamonds"
)

// eyeStyles is the ordered list generateIdentity picks from. Order is
// load-bearing — rng indexes into it.
var eyeStyles = []EyeStyle{
	EyeDots,
	EyeRings,
	EyeSlits,
	EyeTriangles,
	EyeDashes,
	EyeSquares,
	EyeSemicircles,
	EyeDiamonds,
}

// EyeParams describes the eye pair of an identity.
type EyeParams struct {
	LeftStyle  EyeStyle
	RightStyle EyeStyle
	Spacing    float64 // fraction of size, 0.15–0.35
	EyeSize    float64 // fraction of size, 0.04–0.1
}

// buildEye returns an SVG fragment for a single eye centered at (ex, ey).
func buildEye(style EyeStyle, ex, ey, r float64, color string) string {
	sw := math.Max(1, r*0.4)
	switch style {
	case EyeDots:
		return fmt.Sprintf(
			`<circle class="dp-eye" cx="%s" cy="%s" r="%s" fill="%s" />`,
			f(ex), f(ey), f(r), color,
		)
	case EyeRings:
		return fmt.Sprintf(
			`<circle class="dp-eye" cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="%s" />`,
			f(ex), f(ey), f(r), color, f(sw),
		)
	case EyeSlits:
		return fmt.Sprintf(
			`<line class="dp-eye" x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s" stroke-linecap="round" />`,
			f(ex), f(ey-r), f(ex), f(ey+r), color, f(sw),
		)
	case EyeTriangles:
		h := r * 1.2
		return fmt.Sprintf(
			`<polygon class="dp-eye" points="%s,%s %s,%s %s,%s" fill="%s" />`,
			f(ex), f(ey-h), f(ex-h), f(ey+h*0.6), f(ex+h), f(ey+h*0.6), color,
		)
	case EyeDashes:
		return fmt.Sprintf(
			`<line class="dp-eye" x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s" stroke-linecap="round" />`,
			f(ex-r), f(ey), f(ex+r), f(ey), color, f(sw),
		)
	case EyeSquares:
		return fmt.Sprintf(
			`<rect class="dp-eye" x="%s" y="%s" width="%s" height="%s" fill="%s" />`,
			f(ex-r), f(ey-r), f(r*2), f(r*2), color,
		)
	case EyeSemicircles:
		return fmt.Sprintf(
			`<path class="dp-eye" d="M %s %s A %s %s 0 0 1 %s %s" fill="%s" />`,
			f(ex-r), f(ey), f(r), f(r), f(ex+r), f(ey), color,
		)
	case EyeDiamonds:
		d := r
		return fmt.Sprintf(
			`<polygon class="dp-eye" points="%s,%s %s,%s %s,%s %s,%s" fill="%s" />`,
			f(ex), f(ey-d), f(ex+d), f(ey), f(ex), f(ey+d), f(ex-d), f(ey), color,
		)
	}
	return ""
}

// buildEyes renders both eyes centered on a size×size viewBox.
func buildEyes(params EyeParams, size int, color string) string {
	sz := float64(size)
	cx := sz / 2
	cy := sz / 2
	gap := params.Spacing * sz
	lx := cx - gap/2
	rx := cx + gap/2
	r := params.EyeSize * sz

	var b strings.Builder
	b.WriteString(buildEye(params.LeftStyle, lx, cy, r, color))
	b.WriteString("\n  ")
	b.WriteString(buildEye(params.RightStyle, rx, cy, r, color))
	return b.String()
}

// f formats a float for SVG attribute output.
func f(v float64) string {
	return formatCoord(v)
}

// formatCoord is a single-point helper so all coord formatting is consistent.
func formatCoord(v float64) string {
	var b strings.Builder
	writeFloat(&b, v)
	return b.String()
}
