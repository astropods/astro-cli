package identitygen

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"strings"

	"golang.org/x/image/draw"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"

	"github.com/astropods/astro/apps/astro-server/internal/avatar"
)

// rasterSize is the pixel dimension of the rasterized JPEG output. Matches
// avatar.OutputSize so a generated placeholder and a user-uploaded/processed
// avatar share the same resolution and storage size characteristics.
const rasterSize = avatar.OutputSize

// supersampleFactor is how many times larger the intermediate raster buffer is
// before we downsample to rasterSize. 2× supersampling produces substantially
// cleaner edge anti-aliasing on the flat-color geometric shapes, which JPEG
// then compresses more faithfully than jaggy edges.
const supersampleFactor = 2

// jpegQuality is higher than avatar.processImage's 85 because these generated
// placeholders have sharp color transitions (polygon and eye edges) that pick
// up visible DCT ringing at lower qualities. 92 roughly doubles file size over
// 85 but is still well below 100's diminishing returns.
const jpegQuality = 92

// GenerateIdentityJPEG renders a deterministic identity for the given seed as a
// JPEG. Internally it parses the SVG produced by GenerateIdentity and rasters
// it — there is no separate rendering path.
//
// The caller's Size option is ignored; the SVG is always produced at the
// supersampled raster size so oksvg's coordinate space matches the buffer
// and no stroke-scaling transform is needed (oksvg doesn't rescale stroke
// widths when its target rect differs from the SVG width/height).
func GenerateIdentityJPEG(opts IdentityOptions) ([]byte, error) {
	svg := GenerateIdentity(IdentityOptions{Seed: opts.Seed, Size: rasterSize * supersampleFactor})
	return rasterizeSVGToJPEG(svg)
}

// rasterizeSVGToJPEG parses an SVG string produced by this package, rasterizes
// it at supersampleFactor × rasterSize, downsamples to rasterSize with a
// high-quality filter, then JPEG-encodes. Supersampling + downsampling is
// where the crispness comes from — oksvg's built-in AA is adequate but the
// extra pass tightens edges noticeably.
func rasterizeSVGToJPEG(svg string) ([]byte, error) {
	icon, err := oksvg.ReadIconStream(strings.NewReader(svg))
	if err != nil {
		return nil, fmt.Errorf("identitygen: parse svg: %w", err)
	}

	superSize := rasterSize * supersampleFactor
	super := image.NewRGBA(image.Rect(0, 0, superSize, superSize))
	icon.SetTarget(0, 0, float64(superSize), float64(superSize))
	scanner := rasterx.NewScannerGV(superSize, superSize, super, super.Bounds())
	dasher := rasterx.NewDasher(superSize, superSize, scanner)
	icon.Draw(dasher, 1.0)

	// Downsample with Catmull-Rom — same filter avatar.processImage uses for
	// user-uploaded avatars, so the placeholder and upload paths share the
	// same scaling character.
	out := image.NewRGBA(image.Rect(0, 0, rasterSize, rasterSize))
	draw.CatmullRom.Scale(out, out.Bounds(), super, super.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("identitygen: encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}
