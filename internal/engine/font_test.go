package engine_test

import (
	"testing"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

func TestTextWidth(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"A", engine.FontWidth},
		{"AB", engine.FontWidth*2 + 1},
		{"ABC", engine.FontWidth*3 + 2},
	}
	for _, tc := range cases {
		if got := engine.TextWidth(tc.text); got != tc.want {
			t.Errorf("TextWidth(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestDrawTextReturnsCursor(t *testing.T) {
	c := engine.NewCanvas(60, 12)
	end := c.DrawText(0, 0, "HI", engine.White)
	if end != engine.FontAdvance*2 {
		t.Errorf("DrawText end = %d, want %d", end, engine.FontAdvance*2)
	}
}

func TestDrawTextSetsExpectedPixels(t *testing.T) {
	c := engine.NewCanvas(20, 10)
	c.DrawText(0, 0, "I", engine.Red)
	// 'I' glyph: top and bottom rows are "#####", middle three rows are
	// "..#..". Verify a couple of representative pixels.
	if got := c.Get(0, 0); got != engine.Red {
		t.Errorf("(0,0) = %+v, want Red", got)
	}
	if got := c.Get(4, 0); got != engine.Red {
		t.Errorf("(4,0) = %+v, want Red", got)
	}
	if got := c.Get(2, 3); got != engine.Red {
		t.Errorf("(2,3) = %+v, want Red (vertical stem)", got)
	}
	if got := c.Get(0, 3); got != engine.Transparent {
		t.Errorf("(0,3) = %+v, want Transparent", got)
	}
	if got := c.Get(0, 6); got != engine.Red {
		t.Errorf("(0,6) = %+v, want Red (bottom bar)", got)
	}
}

func TestDrawTextLowercaseFoldedToUpper(t *testing.T) {
	c1 := engine.NewCanvas(20, 10)
	c2 := engine.NewCanvas(20, 10)
	c1.DrawText(0, 0, "a", engine.White)
	c2.DrawText(0, 0, "A", engine.White)
	for y := 0; y < c1.Height(); y++ {
		for x := 0; x < c1.Width(); x++ {
			if c1.Get(x, y) != c2.Get(x, y) {
				t.Fatalf("(%d,%d) differs: lower=%v upper=%v", x, y, c1.Get(x, y), c2.Get(x, y))
			}
		}
	}
}

func TestDrawTextUnknownGlyphRendersBox(t *testing.T) {
	c := engine.NewCanvas(20, 10)
	c.DrawText(0, 0, "~", engine.Red) // '~' is not in the font
	// Missing glyph is a solid 5x7 outlined box: corners should be red.
	corners := [][2]int{{0, 0}, {4, 0}, {0, 6}, {4, 6}}
	for _, p := range corners {
		if got := c.Get(p[0], p[1]); got != engine.Red {
			t.Errorf("missing-glyph corner (%d,%d) = %+v, want Red", p[0], p[1], got)
		}
	}
}
