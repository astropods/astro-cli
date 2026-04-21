package identitygen

import (
	"encoding/xml"
	"math"
	"strings"
	"testing"
)

func TestRegularVerticesKnown(t *testing.T) {
	// Triangle at rotation=0, centered at (50, 50), r=50. First vertex sits on
	// the +x axis.
	v := regularVertices(3, 50, 50, 50, 0)
	if len(v) != 3 {
		t.Fatalf("expected 3 vertices, got %d", len(v))
	}
	if math.Abs(v[0].x-100) > 1e-9 || math.Abs(v[0].y-50) > 1e-9 {
		t.Fatalf("v0 = (%v, %v), want (100, 50)", v[0].x, v[0].y)
	}
	// 2nd vertex at 120° from +x: cos=-0.5, sin=sqrt(3)/2.
	wantX := 50 + 50*math.Cos(2*math.Pi/3)
	wantY := 50 + 50*math.Sin(2*math.Pi/3)
	if math.Abs(v[1].x-wantX) > 1e-9 || math.Abs(v[1].y-wantY) > 1e-9 {
		t.Fatalf("v1 = (%v, %v), want (%v, %v)", v[1].x, v[1].y, wantX, wantY)
	}
}

func TestBuildPolygonPathAllEdgeStyles(t *testing.T) {
	base := PolygonParams{
		Sides:       6,
		Rotation:    0.3,
		Radius:      0.8,
		SpikeDepth:  0.5,
		CurveAmount: 0.3,
	}
	for _, style := range edgeStyles {
		t.Run(string(style), func(t *testing.T) {
			p := base
			p.EdgeStyle = style
			d := buildPolygonPath(p, 128)
			if d == "" {
				t.Fatal("empty path")
			}
			if !strings.HasPrefix(d, "M ") {
				t.Fatalf("path must start with M, got %q", d[:min(16, len(d))])
			}
			if !strings.HasSuffix(d, " Z") {
				t.Fatalf("path must end with Z, got %q", d[max(0, len(d)-8):])
			}
			// Embed in a minimal SVG and verify it parses as XML.
			svg := `<svg xmlns="http://www.w3.org/2000/svg"><path d="` + d + `"/></svg>`
			if err := xml.Unmarshal([]byte(svg), new(struct{ XMLName xml.Name })); err != nil {
				t.Fatalf("svg did not parse: %v (d=%q)", err, d)
			}
		})
	}
}

func TestBuildPolygonPathUnknownStyleReturnsEmpty(t *testing.T) {
	// Defensive: unknown style shouldn't panic; returns "". This is dead code in
	// practice since EdgeStyle values all come from the fixed `edgeStyles` slice.
	p := PolygonParams{Sides: 4, EdgeStyle: "bogus"}
	if got := buildPolygonPath(p, 128); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
