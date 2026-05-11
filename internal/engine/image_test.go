package engine_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

func TestDrawImageCopiesPixels(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	img.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	img.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 255})

	c := engine.NewCanvas(4, 4)
	c.DrawImage(1, 1, img)

	cases := []struct {
		x, y int
		want engine.Color
	}{
		{1, 1, engine.Red},
		{2, 1, engine.Green},
		{1, 2, engine.Blue},
		{2, 2, engine.White},
		{0, 0, engine.Transparent}, // untouched
		{3, 3, engine.Transparent}, // untouched
	}
	for _, tc := range cases {
		if got := c.Get(tc.x, tc.y); got != tc.want {
			t.Errorf("Get(%d,%d) = %+v, want %+v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestDrawImageSkipsTransparentSourcePixels(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})           // opaque red
	img.SetNRGBA(1, 0, color.NRGBA{R: 0, G: 255, B: 0, A: 0}) // alpha 0

	c := engine.NewCanvas(4, 2)
	c.Clear(engine.Blue)
	c.DrawImage(0, 0, img)

	if got := c.Get(0, 0); got != engine.Red {
		t.Errorf("(0,0) = %+v, want Red", got)
	}
	if got := c.Get(1, 0); got != engine.Blue {
		t.Errorf("(1,0) = %+v, want Blue (transparent source must not overwrite)", got)
	}
}

func TestDrawImageClipsToCanvasBounds(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for sy := 0; sy < 4; sy++ {
		for sx := 0; sx < 4; sx++ {
			img.SetNRGBA(sx, sy, color.NRGBA{R: 255, A: 255})
		}
	}

	c := engine.NewCanvas(4, 4)
	// Position so only the bottom-right 2×2 of the image overlaps the canvas.
	c.DrawImage(-2, -2, img)

	if got := c.Get(0, 0); got != engine.Red {
		t.Errorf("(0,0) = %+v, want Red", got)
	}
	if got := c.Get(1, 1); got != engine.Red {
		t.Errorf("(1,1) = %+v, want Red", got)
	}
	if got := c.Get(2, 2); got != engine.Transparent {
		t.Errorf("(2,2) = %+v, want Transparent (outside image)", got)
	}
}

func TestDrawImageHonoursNonZeroBoundsMin(t *testing.T) {
	// Bounds.Min != (0,0): the image's logical top-left should still map
	// to the destination (x, y) regardless of where Min is.
	img := image.NewNRGBA(image.Rect(5, 5, 7, 6))
	img.SetNRGBA(5, 5, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(6, 5, color.NRGBA{G: 255, A: 255})

	c := engine.NewCanvas(4, 2)
	c.DrawImage(0, 0, img)

	if got := c.Get(0, 0); got != engine.Red {
		t.Errorf("(0,0) = %+v, want Red", got)
	}
	if got := c.Get(1, 0); got != engine.Green {
		t.Errorf("(1,0) = %+v, want Green", got)
	}
}

func TestDrawImagePalettedSource(t *testing.T) {
	// *image.Paletted goes through color model conversion — make sure
	// the conversion path produces the expected RGBA8 result.
	palette := color.Palette{
		color.NRGBA{R: 255, A: 255},
		color.NRGBA{G: 255, A: 255},
	}
	img := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	img.SetColorIndex(0, 0, 0)
	img.SetColorIndex(1, 0, 1)

	c := engine.NewCanvas(2, 2)
	c.DrawImage(0, 0, img)
	if got := c.Get(0, 0); got != engine.Red {
		t.Errorf("(0,0) = %+v, want Red", got)
	}
	if got := c.Get(1, 0); got != engine.Green {
		t.Errorf("(1,0) = %+v, want Green", got)
	}
}
