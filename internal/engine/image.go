package engine

import (
	"image"
	"image/color"
)

// DrawImage blits img onto the canvas with its top-left aligned to pixel
// coordinates (x, y). Each source pixel is converted to RGBA8 and written
// to the destination one-for-one — there is no scaling and no blending.
// Source pixels with alpha == 0 are skipped so DrawImage behaves like a
// sprite blit over the existing canvas content.
//
// Any standard image.Image works — *image.RGBA, *image.NRGBA, decoded
// PNG/JPEG/GIF, *image.Paletted, *image.YCbCr, etc. The image's
// Bounds().Min is used as its logical top-left when computing destination
// coordinates, so subimages and images with non-zero origins are handled
// correctly.
//
// Destination pixels outside the canvas are silently clipped.
func (c *Canvas) DrawImage(x, y int, img image.Image) {
	bounds := img.Bounds()
	for sy := bounds.Min.Y; sy < bounds.Max.Y; sy++ {
		dy := y + (sy - bounds.Min.Y)
		if dy < 0 || dy >= c.height {
			continue
		}
		row := dy * c.width
		for sx := bounds.Min.X; sx < bounds.Max.X; sx++ {
			dx := x + (sx - bounds.Min.X)
			if dx < 0 || dx >= c.width {
				continue
			}
			nrgba := color.NRGBAModel.Convert(img.At(sx, sy)).(color.NRGBA)
			if nrgba.A == 0 {
				continue
			}
			c.pixels[row+dx] = Color{
				R: nrgba.R,
				G: nrgba.G,
				B: nrgba.B,
				A: nrgba.A,
			}
		}
	}
}
