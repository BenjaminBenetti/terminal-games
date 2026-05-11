package enginedemo

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/jpeg"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

//go:embed cat.jpg
var catBytes []byte

// catImage is decoded once at init. If decoding fails, the demo shows a
// fallback message instead of bringing down the whole package.
var catImage image.Image

func init() {
	if img, _, err := image.Decode(bytes.NewReader(catBytes)); err == nil {
		catImage = img
	}
}

type catDemo struct {
	cached           *image.NRGBA
	cachedW, cachedH int
}

func newCatDemo() demoScene { return &catDemo{} }

func (d *catDemo) Update(time.Duration) error { return nil }

func (d *catDemo) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 16, G: 16, B: 24, A: 255})

	if catImage == nil {
		drawFooter(c, "cat (decode failed)   •   esc back")
		return
	}

	maxW := c.Width()
	maxH := c.Height() - 4 // leave room for the footer

	if d.cached == nil || d.cachedW != maxW || d.cachedH != maxH {
		d.cached = fitImageNRGBA(catImage, maxW, maxH)
		d.cachedW = maxW
		d.cachedH = maxH
	}

	b := d.cached.Bounds()
	x := (c.Width() - b.Dx()) / 2
	y := (maxH - b.Dy()) / 2
	c.DrawImage(x, y, d.cached)

	drawFooter(c, "uwu  •  cat  •  esc back")
}

// fitImageNRGBA returns src scaled with nearest-neighbour sampling to fit
// inside maxW × maxH while preserving aspect ratio.
func fitImageNRGBA(src image.Image, maxW, maxH int) *image.NRGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw == 0 || sh == 0 || maxW < 1 || maxH < 1 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}

	// Smaller of the two scale factors so both dims fit.
	scaleW := float64(maxW) / float64(sw)
	scaleH := float64(maxH) / float64(sh)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}

	dstW := int(float64(sw) * scale)
	dstH := int(float64(sh) * scale)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}

	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		sy := sb.Min.Y + y*sh/dstH
		for x := 0; x < dstW; x++ {
			sx := sb.Min.X + x*sw/dstW
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}
