package engine_test

import (
	"testing"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

func TestNewCanvasRoundsOddHeightUp(t *testing.T) {
	c := engine.NewCanvas(10, 5)
	if c.Height() != 6 {
		t.Errorf("Height() = %d, want 6", c.Height())
	}
	if c.Width() != 10 {
		t.Errorf("Width() = %d, want 10", c.Width())
	}
}

func TestNewCanvasClampsToOne(t *testing.T) {
	c := engine.NewCanvas(0, 0)
	if c.Width() != 1 || c.Height() != 2 {
		t.Errorf("NewCanvas(0,0) -> %dx%d, want 1x2", c.Width(), c.Height())
	}
}

func TestSetGetInBounds(t *testing.T) {
	c := engine.NewCanvas(4, 4)
	c.Set(2, 1, engine.Red)
	if got := c.Get(2, 1); got != engine.Red {
		t.Errorf("Get(2,1) = %+v, want Red", got)
	}
	// untouched pixel
	if got := c.Get(0, 0); got != engine.Transparent {
		t.Errorf("Get(0,0) = %+v, want Transparent", got)
	}
}

func TestSetOutOfBoundsIsNoOp(t *testing.T) {
	c := engine.NewCanvas(4, 4)
	c.Set(-1, 0, engine.Red)
	c.Set(0, -1, engine.Red)
	c.Set(4, 0, engine.Red)
	c.Set(0, 4, engine.Red)
	for y := 0; y < c.Height(); y++ {
		for x := 0; x < c.Width(); x++ {
			if got := c.Get(x, y); got != engine.Transparent {
				t.Errorf("Get(%d,%d) = %+v, want Transparent", x, y, got)
			}
		}
	}
}

func TestGetOutOfBoundsReturnsTransparent(t *testing.T) {
	c := engine.NewCanvas(4, 4)
	if got := c.Get(-1, 0); got != engine.Transparent {
		t.Errorf("Get(-1,0) = %+v, want Transparent", got)
	}
	if got := c.Get(0, 99); got != engine.Transparent {
		t.Errorf("Get(0,99) = %+v, want Transparent", got)
	}
}

func TestClear(t *testing.T) {
	c := engine.NewCanvas(3, 4)
	c.Clear(engine.Blue)
	for y := 0; y < c.Height(); y++ {
		for x := 0; x < c.Width(); x++ {
			if got := c.Get(x, y); got != engine.Blue {
				t.Errorf("Get(%d,%d) = %+v, want Blue", x, y, got)
			}
		}
	}
}

func TestFillRectClipsToBounds(t *testing.T) {
	c := engine.NewCanvas(4, 4)
	c.FillRect(-2, -2, 6, 6, engine.Red)
	for y := 0; y < c.Height(); y++ {
		for x := 0; x < c.Width(); x++ {
			if got := c.Get(x, y); got != engine.Red {
				t.Errorf("Get(%d,%d) = %+v, want Red", x, y, got)
			}
		}
	}
}

func TestFillRectInterior(t *testing.T) {
	c := engine.NewCanvas(6, 6)
	c.FillRect(2, 1, 3, 2, engine.Green)
	for y := 0; y < c.Height(); y++ {
		for x := 0; x < c.Width(); x++ {
			inside := x >= 2 && x < 5 && y >= 1 && y < 3
			want := engine.Transparent
			if inside {
				want = engine.Green
			}
			if got := c.Get(x, y); got != want {
				t.Errorf("Get(%d,%d) = %+v, want %+v", x, y, got, want)
			}
		}
	}
}

func TestFillRectZeroSize(t *testing.T) {
	c := engine.NewCanvas(4, 4)
	c.FillRect(1, 1, 0, 5, engine.Red)
	c.FillRect(1, 1, 5, 0, engine.Red)
	for y := 0; y < c.Height(); y++ {
		for x := 0; x < c.Width(); x++ {
			if got := c.Get(x, y); got != engine.Transparent {
				t.Errorf("Get(%d,%d) = %+v, want Transparent", x, y, got)
			}
		}
	}
}

func TestDrawRectOutline(t *testing.T) {
	c := engine.NewCanvas(4, 4)
	c.DrawRect(0, 0, 4, 4, engine.Red)
	for i := 0; i < 4; i++ {
		if got := c.Get(i, 0); got != engine.Red {
			t.Errorf("top edge (%d,0) = %+v, want Red", i, got)
		}
		if got := c.Get(i, 3); got != engine.Red {
			t.Errorf("bottom edge (%d,3) = %+v, want Red", i, got)
		}
		if got := c.Get(0, i); got != engine.Red {
			t.Errorf("left edge (0,%d) = %+v, want Red", i, got)
		}
		if got := c.Get(3, i); got != engine.Red {
			t.Errorf("right edge (3,%d) = %+v, want Red", i, got)
		}
	}
	if got := c.Get(1, 1); got != engine.Transparent {
		t.Errorf("interior (1,1) = %+v, want Transparent", got)
	}
	if got := c.Get(2, 2); got != engine.Transparent {
		t.Errorf("interior (2,2) = %+v, want Transparent", got)
	}
}

func TestDrawLineHorizontal(t *testing.T) {
	c := engine.NewCanvas(8, 4)
	c.DrawLine(1, 2, 6, 2, engine.Green)
	for x := 1; x <= 6; x++ {
		if got := c.Get(x, 2); got != engine.Green {
			t.Errorf("Get(%d,2) = %+v, want Green", x, got)
		}
	}
	if got := c.Get(0, 2); got != engine.Transparent {
		t.Errorf("Get(0,2) = %+v, want Transparent", got)
	}
	if got := c.Get(7, 2); got != engine.Transparent {
		t.Errorf("Get(7,2) = %+v, want Transparent", got)
	}
}

func TestDrawLineVertical(t *testing.T) {
	c := engine.NewCanvas(4, 8)
	c.DrawLine(2, 1, 2, 6, engine.Cyan)
	for y := 1; y <= 6; y++ {
		if got := c.Get(2, y); got != engine.Cyan {
			t.Errorf("Get(2,%d) = %+v, want Cyan", y, got)
		}
	}
}

func TestDrawLineDiagonalEndpoints(t *testing.T) {
	c := engine.NewCanvas(8, 8)
	c.DrawLine(0, 0, 7, 7, engine.Magenta)
	if got := c.Get(0, 0); got != engine.Magenta {
		t.Errorf("Get(0,0) = %+v, want Magenta", got)
	}
	if got := c.Get(7, 7); got != engine.Magenta {
		t.Errorf("Get(7,7) = %+v, want Magenta", got)
	}
	if got := c.Get(3, 3); got != engine.Magenta {
		t.Errorf("Get(3,3) = %+v, want Magenta", got)
	}
}

func TestFillCircle(t *testing.T) {
	c := engine.NewCanvas(8, 8)
	c.FillCircle(4, 4, 2, engine.Yellow)
	if got := c.Get(4, 4); got != engine.Yellow {
		t.Errorf("center (4,4) = %+v, want Yellow", got)
	}
	if got := c.Get(4, 6); got != engine.Yellow {
		t.Errorf("south point (4,6) = %+v, want Yellow", got)
	}
	if got := c.Get(6, 4); got != engine.Yellow {
		t.Errorf("east point (6,4) = %+v, want Yellow", got)
	}
	if got := c.Get(7, 7); got != engine.Transparent {
		t.Errorf("outside (7,7) = %+v, want Transparent", got)
	}
}

func TestDrawCircle(t *testing.T) {
	c := engine.NewCanvas(8, 8)
	c.DrawCircle(4, 4, 2, engine.Cyan)
	if got := c.Get(4, 4); got != engine.Transparent {
		t.Errorf("center should not be stroked, got %+v", got)
	}
	if got := c.Get(6, 4); got != engine.Cyan {
		t.Errorf("east point (6,4) = %+v, want Cyan", got)
	}
	if got := c.Get(2, 4); got != engine.Cyan {
		t.Errorf("west point (2,4) = %+v, want Cyan", got)
	}
	if got := c.Get(4, 6); got != engine.Cyan {
		t.Errorf("south point (4,6) = %+v, want Cyan", got)
	}
	if got := c.Get(4, 2); got != engine.Cyan {
		t.Errorf("north point (4,2) = %+v, want Cyan", got)
	}
}
