package identitygen

import (
	"math"
	"strconv"
	"strings"
)

// EdgeStyle names a polygon edge treatment.
type EdgeStyle string

const (
	EdgeFlat             EdgeStyle = "flat"
	EdgeSpikey           EdgeStyle = "spikey"
	EdgeScalloped        EdgeStyle = "scalloped"
	EdgeInverseScalloped EdgeStyle = "inverse-scalloped"
)

// edgeStyles is the ordered list generateIdentity picks from. Order is
// load-bearing — rng indexes into it.
var edgeStyles = []EdgeStyle{
	EdgeFlat,
	EdgeSpikey,
	EdgeScalloped,
	EdgeInverseScalloped,
}

// PolygonParams describes the polygon layer of an identity.
type PolygonParams struct {
	Sides       int       // 3–8
	EdgeStyle   EdgeStyle
	Rotation    float64   // radians
	Radius      float64   // fraction of half-viewBox
	SpikeDepth  float64   // inner-radius ratio for spikey
	CurveAmount float64   // control-point offset for scalloped
}

type point struct {
	x, y float64
}

// regularVertices returns the vertices of a regular polygon centered at (cx, cy).
func regularVertices(sides int, cx, cy, r, rotation float64) []point {
	pts := make([]point, sides)
	for i := range sides {
		angle := rotation + 2*math.Pi*float64(i)/float64(sides)
		pts[i] = point{x: cx + r*math.Cos(angle), y: cy + r*math.Sin(angle)}
	}
	return pts
}

func flatPath(vertices []point) string {
	var b strings.Builder
	b.WriteString("M ")
	writeFloat(&b, vertices[0].x)
	b.WriteByte(' ')
	writeFloat(&b, vertices[0].y)
	for _, p := range vertices[1:] {
		b.WriteString(" L ")
		writeFloat(&b, p.x)
		b.WriteByte(' ')
		writeFloat(&b, p.y)
	}
	b.WriteString(" Z")
	return b.String()
}

// spikeyPath alternates outer and inner radii to build a star-like shape.
func spikeyPath(sides int, cx, cy, outerR, innerR, rotation float64) string {
	pts := make([]point, 0, sides*2)
	for i := range sides {
		outerAngle := rotation + 2*math.Pi*float64(i)/float64(sides)
		pts = append(pts, point{
			x: cx + outerR*math.Cos(outerAngle),
			y: cy + outerR*math.Sin(outerAngle),
		})
		innerAngle := outerAngle + math.Pi/float64(sides)
		pts = append(pts, point{
			x: cx + innerR*math.Cos(innerAngle),
			y: cy + innerR*math.Sin(innerAngle),
		})
	}
	return flatPath(pts)
}

// scallopedPath draws Bezier arcs with control points pushed outward (inward=false)
// or inward (inward=true) relative to the polygon center.
func scallopedPath(vertices []point, cx, cy, curveAmount float64, inward bool) string {
	var b strings.Builder
	b.WriteString("M ")
	writeFloat(&b, vertices[0].x)
	b.WriteByte(' ')
	writeFloat(&b, vertices[0].y)

	n := len(vertices)
	sign := 1.0
	if inward {
		sign = -1
	}
	for i := range n {
		a := vertices[i]
		bp := vertices[(i+1)%n]
		mx := (a.x + bp.x) / 2
		my := (a.y + bp.y) / 2
		dx := mx - cx
		dy := my - cy
		dist := math.Sqrt(dx*dx + dy*dy)
		nx := dx / dist
		ny := dy / dist
		offset := curveAmount * dist
		cpx := mx + sign*nx*offset
		cpy := my + sign*ny*offset
		b.WriteString(" Q ")
		writeFloat(&b, cpx)
		b.WriteByte(' ')
		writeFloat(&b, cpy)
		b.WriteByte(' ')
		writeFloat(&b, bp.x)
		b.WriteByte(' ')
		writeFloat(&b, bp.y)
	}

	b.WriteString(" Z")
	return b.String()
}

// buildPolygonPath returns the `d` attribute for a polygon defined by params
// drawn inside a size×size viewBox.
func buildPolygonPath(params PolygonParams, size int) string {
	sz := float64(size)
	cx := sz / 2
	cy := sz / 2
	r := params.Radius * sz / 2

	switch params.EdgeStyle {
	case EdgeFlat:
		return flatPath(regularVertices(params.Sides, cx, cy, r, params.Rotation))
	case EdgeSpikey:
		return spikeyPath(params.Sides, cx, cy, r, r*params.SpikeDepth, params.Rotation)
	case EdgeScalloped:
		return scallopedPath(regularVertices(params.Sides, cx, cy, r, params.Rotation), cx, cy, params.CurveAmount, false)
	case EdgeInverseScalloped:
		return scallopedPath(regularVertices(params.Sides, cx, cy, r, params.Rotation), cx, cy, params.CurveAmount, true)
	}
	return ""
}

// writeFloat formats a float coordinate into b using Go's shortest round-trip
// decimal representation. Formatting need not match the TS implementation —
// SVG renderers accept any valid decimal.
func writeFloat(b *strings.Builder, v float64) {
	b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
}
