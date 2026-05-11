package engine

// Canvas is a 2D grid of RGBA pixels that scenes draw into each frame.
//
// The internal pixel height is always even: each terminal cell renders two
// vertically-stacked pixels using a half-block character, so a canvas with
// height H occupies H/2 terminal rows.
//
// All drawing primitives clip to the canvas bounds. Set on an out-of-bounds
// coordinate is a no-op rather than a panic.
//
// A Canvas can also carry a text overlay populated by Print: cells covered
// by Print are rendered using the terminal's native font (one cell per
// rune) instead of the half-block pixel rendering. The text overlay is
// cleared by Clear so it behaves like an immediate-mode draw call.
type Canvas struct {
	width, height int
	pixels        []Color
	text          map[int]textCell
}

// textCell is a single rune placed onto the terminal cell grid via Print.
type textCell struct {
	r  rune
	fg Color
}

// NewCanvas returns a Canvas with the given dimensions in pixels. Width and
// height are clamped to at least 1, and height is rounded up to the next even
// value since two pixels share one terminal row.
func NewCanvas(width, height int) *Canvas {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if height%2 != 0 {
		height++
	}
	return &Canvas{
		width:  width,
		height: height,
		pixels: make([]Color, width*height),
	}
}

// Width returns the canvas width in pixels.
func (c *Canvas) Width() int { return c.width }

// Height returns the canvas height in pixels.
func (c *Canvas) Height() int { return c.height }

// Cols returns the canvas width in terminal cells (identical to Width since
// one pixel column maps to one cell column).
func (c *Canvas) Cols() int { return c.width }

// Rows returns the canvas height in terminal cells (half the pixel height).
func (c *Canvas) Rows() int { return c.height / 2 }

// Clear fills every pixel with color and removes any text written via Print.
func (c *Canvas) Clear(color Color) {
	for i := range c.pixels {
		c.pixels[i] = color
	}
	clear(c.text)
}

// Print writes text at terminal-cell coordinates (col, row), where one
// rune occupies exactly one terminal cell using the terminal's native
// font — i.e. text appears at the terminal's actual character size, not
// as the chunky pixel-art font produced by DrawText.
//
// The background colour of each cell is taken from the canvas pixels at
// that position. Call FillRect or Set first if you want a coloured
// background behind the text (e.g. a highlight bar).
//
// Out-of-bounds characters are clipped. Call Clear to remove the overlay.
func (c *Canvas) Print(col, row int, text string, fg Color) {
	if row < 0 || row >= c.Rows() {
		return
	}
	if c.text == nil {
		c.text = make(map[int]textCell)
	}
	for _, r := range text {
		if col >= c.width {
			return
		}
		if col >= 0 {
			c.text[row*c.width+col] = textCell{r: r, fg: fg}
		}
		col++
	}
}

// Set assigns color to the pixel at (x, y). Out-of-bounds calls are ignored.
func (c *Canvas) Set(x, y int, color Color) {
	if x < 0 || y < 0 || x >= c.width || y >= c.height {
		return
	}
	c.pixels[y*c.width+x] = color
}

// Get returns the color at (x, y), or Transparent if out of bounds.
func (c *Canvas) Get(x, y int) Color {
	if x < 0 || y < 0 || x >= c.width || y >= c.height {
		return Transparent
	}
	return c.pixels[y*c.width+x]
}

// FillRect fills the rectangle [x, x+w) × [y, y+h) with color, clipped to
// the canvas bounds.
func (c *Canvas) FillRect(x, y, w, h int, color Color) {
	if w <= 0 || h <= 0 {
		return
	}
	x0, y0, x1, y1 := x, y, x+w, y+h
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > c.width {
		x1 = c.width
	}
	if y1 > c.height {
		y1 = c.height
	}
	for yy := y0; yy < y1; yy++ {
		row := yy * c.width
		for xx := x0; xx < x1; xx++ {
			c.pixels[row+xx] = color
		}
	}
}

// DrawRect strokes the outline of the rectangle [x, x+w) × [y, y+h).
func (c *Canvas) DrawRect(x, y, w, h int, color Color) {
	if w <= 0 || h <= 0 {
		return
	}
	for xx := x; xx < x+w; xx++ {
		c.Set(xx, y, color)
		c.Set(xx, y+h-1, color)
	}
	for yy := y; yy < y+h; yy++ {
		c.Set(x, yy, color)
		c.Set(x+w-1, yy, color)
	}
}

// DrawLine draws a line from (x0, y0) to (x1, y1) using Bresenham's algorithm.
func (c *Canvas) DrawLine(x0, y0, x1, y1 int, color Color) {
	dx := absInt(x1 - x0)
	dy := -absInt(y1 - y0)
	sx := 1
	if x0 >= x1 {
		sx = -1
	}
	sy := 1
	if y0 >= y1 {
		sy = -1
	}
	err := dx + dy
	for {
		c.Set(x0, y0, color)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// FillCircle fills the disc of radius r centred at (cx, cy).
func (c *Canvas) FillCircle(cx, cy, r int, color Color) {
	if r < 0 {
		return
	}
	r2 := r * r
	for dy := -r; dy <= r; dy++ {
		dy2 := dy * dy
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy2 <= r2 {
				c.Set(cx+dx, cy+dy, color)
			}
		}
	}
}

// DrawCircle strokes the outline of a circle of radius r centred at (cx, cy)
// using the midpoint circle algorithm.
func (c *Canvas) DrawCircle(cx, cy, r int, color Color) {
	if r < 0 {
		return
	}
	x, y := r, 0
	err := 1 - x
	for x >= y {
		c.Set(cx+x, cy+y, color)
		c.Set(cx+y, cy+x, color)
		c.Set(cx-x, cy+y, color)
		c.Set(cx-y, cy+x, color)
		c.Set(cx-x, cy-y, color)
		c.Set(cx-y, cy-x, color)
		c.Set(cx+x, cy-y, color)
		c.Set(cx+y, cy-x, color)
		y++
		if err < 0 {
			err += 2*y + 1
		} else {
			x--
			err += 2*(y-x) + 1
		}
	}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
