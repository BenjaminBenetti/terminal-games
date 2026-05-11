package enginedemo

import (
	"math"
	"time"
	"unicode/utf8"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// drawFooter paints a label on the bottom terminal row using the terminal's
// native font, backed by a black bar so it stays readable over busy demos.
func drawFooter(c *engine.Canvas, label string) {
	cols, rows := c.Cols(), c.Rows()
	row := rows - 1
	c.FillRect(0, row*2, cols, 2, engine.Color{R: 0, G: 0, B: 0, A: 255})
	n := utf8.RuneCountInString(label)
	col := (cols - n) / 2
	if col < 0 {
		col = 0
	}
	c.Print(col, row, label, engine.White)
}

// --- Colour palette ---------------------------------------------------------

type paletteDemo struct{}

func newPaletteDemo() demoScene { return &paletteDemo{} }

func (p *paletteDemo) Update(time.Duration) error { return nil }

func (p *paletteDemo) Draw(c *engine.Canvas) {
	c.Clear(engine.Black)

	swatches := []engine.Color{
		engine.Red, engine.Green, engine.Blue,
		engine.Yellow, engine.Cyan, engine.Magenta,
		engine.White, engine.Gray,
	}
	rows := 2
	cols := (len(swatches) + rows - 1) / rows
	padX, padY := 6, 6
	gapX, gapY := 4, 4
	usableW := c.Width() - 2*padX - gapX*(cols-1)
	usableH := c.Height() - 2*padY - gapY*(rows-1) - engine.FontHeight - 6
	sw := usableW / cols
	sh := usableH / rows
	for i, col := range swatches {
		row := i / cols
		colIdx := i % cols
		x := padX + colIdx*(sw+gapX)
		y := padY + row*(sh+gapY)
		c.FillRect(x, y, sw, sh, col)
		c.DrawRect(x, y, sw, sh, engine.White)
	}

	// Continuous greyscale ramp.
	rampY := padY + rows*(sh+gapY)
	rampH := 4
	for x := padX; x < c.Width()-padX; x++ {
		t := float64(x-padX) / float64(c.Width()-2*padX-1)
		v := uint8(255 * t)
		c.FillRect(x, rampY, 1, rampH, engine.Color{R: v, G: v, B: v, A: 255})
	}

	drawFooter(c, "color palette   •   esc back")
}

// --- Bouncing ball ----------------------------------------------------------

type bouncingBall struct {
	x, y   float64
	vx, vy float64
	radius int
	color  engine.Color
}

func newBouncingBallDemo() demoScene {
	return &bouncingBall{
		x: 30, y: 20,
		vx: 55, vy: 33,
		radius: 4,
		color:  engine.Yellow,
	}
}

func (b *bouncingBall) Update(dt time.Duration) error {
	s := dt.Seconds()
	b.x += b.vx * s
	b.y += b.vy * s
	return nil
}

func (b *bouncingBall) Draw(c *engine.Canvas) {
	w, h := c.Width(), c.Height()
	r := float64(b.radius)
	if b.x < r {
		b.x = r
		b.vx = -b.vx
	}
	if b.x > float64(w)-r-1 {
		b.x = float64(w) - r - 1
		b.vx = -b.vx
	}
	if b.y < r {
		b.y = r
		b.vy = -b.vy
	}
	if b.y > float64(h)-r-1 {
		b.y = float64(h) - r - 1
		b.vy = -b.vy
	}
	c.Clear(engine.Color{R: 8, G: 16, B: 32, A: 255})
	c.DrawRect(0, 0, w, h, engine.Cyan)
	c.FillCircle(int(b.x), int(b.y), b.radius, b.color)
	c.DrawCircle(int(b.x), int(b.y), b.radius+2, engine.White)
	drawFooter(c, "bouncing ball   •   esc back")
}

// --- Shapes -----------------------------------------------------------------

type shapesDemo struct {
	t float64
}

func newShapesDemo() demoScene { return &shapesDemo{} }

func (s *shapesDemo) Update(dt time.Duration) error {
	s.t += dt.Seconds()
	return nil
}

func (s *shapesDemo) Draw(c *engine.Canvas) {
	c.Clear(engine.Black)
	w, h := c.Width(), c.Height()

	// Row of rectangles: filled vs stroked.
	c.FillRect(4, 4, 16, 10, engine.Red)
	c.DrawRect(24, 4, 16, 10, engine.Red)
	c.FillRect(44, 4, 16, 10, engine.Green)
	c.DrawRect(64, 4, 16, 10, engine.Green)

	// Row of circles.
	c.FillCircle(12, 26, 6, engine.Blue)
	c.DrawCircle(32, 26, 6, engine.Blue)
	c.FillCircle(52, 26, 6, engine.Magenta)
	c.DrawCircle(72, 26, 6, engine.Magenta)

	// Spinning line fan in the lower half.
	cx, cy := w/2, h*3/4
	const spokes = 12
	for i := 0; i < spokes; i++ {
		a := s.t*0.6 + float64(i)*math.Pi*2/float64(spokes)
		r := 10.0
		x2 := cx + int(r*math.Cos(a))
		y2 := cy + int(r*math.Sin(a))
		col := engine.Color{
			R: uint8(127 + 127*math.Sin(a)),
			G: uint8(127 + 127*math.Sin(a+2)),
			B: uint8(127 + 127*math.Sin(a+4)),
			A: 255,
		}
		c.DrawLine(cx, cy, x2, y2, col)
	}

	drawFooter(c, "shapes   •   esc back")
}

// --- Plasma -----------------------------------------------------------------

type plasmaDemo struct {
	t float64
}

func newPlasmaDemo() demoScene { return &plasmaDemo{} }

func (p *plasmaDemo) Update(dt time.Duration) error {
	p.t += dt.Seconds()
	return nil
}

func (p *plasmaDemo) Draw(c *engine.Canvas) {
	c.Clear(engine.Black)
	w, h := c.Width(), c.Height()
	t := p.t
	for y := 0; y < h; y++ {
		fy := float64(y)
		for x := 0; x < w; x++ {
			fx := float64(x)
			v := math.Sin(fx*0.2+t) +
				math.Sin(fy*0.25+t*1.3) +
				math.Sin((fx+fy)*0.15+t*0.7) +
				math.Sin(math.Sqrt(fx*fx+fy*fy)*0.2+t)
			n := (v + 4) / 8 // [0,1]
			r := uint8(127.5 * (1 + math.Sin(n*math.Pi*2)))
			g := uint8(127.5 * (1 + math.Sin(n*math.Pi*2+2)))
			b := uint8(127.5 * (1 + math.Sin(n*math.Pi*2+4)))
			c.Set(x, y, engine.Color{R: r, G: g, B: b, A: 255})
		}
	}
	drawFooter(c, "plasma   •   esc back")
}
