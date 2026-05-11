package enginedemo

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// keysDemo visualises the engine's held-key state. A cursor moves with
// arrow keys or WASD, accelerating diagonally when two direction keys
// are held at once. Four indicators across the top light up while their
// respective direction is held — the proof that multiple keys are being
// reported simultaneously.
type keysDemo struct {
	e     *engine.Engine
	x, y  float64
	trail []trailPoint
}

type trailPoint struct {
	x, y int
}

func newKeysDemo(e *engine.Engine) demoScene {
	return &keysDemo{
		e: e,
		x: float64(e.Canvas().Width()) / 2,
		y: float64(e.Canvas().Height()) / 2,
	}
}

func (d *keysDemo) up() bool {
	return d.e.IsKeyDown(engine.KeyUp) || d.e.IsCharDown('w') || d.e.IsCharDown('W')
}
func (d *keysDemo) down() bool {
	return d.e.IsKeyDown(engine.KeyDown) || d.e.IsCharDown('s') || d.e.IsCharDown('S')
}
func (d *keysDemo) left() bool {
	return d.e.IsKeyDown(engine.KeyLeft) || d.e.IsCharDown('a') || d.e.IsCharDown('A')
}
func (d *keysDemo) right() bool {
	return d.e.IsKeyDown(engine.KeyRight) || d.e.IsCharDown('d') || d.e.IsCharDown('D')
}

func (d *keysDemo) Update(dt time.Duration) error {
	speed := 70.0 * dt.Seconds()
	moved := false
	if d.up() {
		d.y -= speed
		moved = true
	}
	if d.down() {
		d.y += speed
		moved = true
	}
	if d.left() {
		d.x -= speed
		moved = true
	}
	if d.right() {
		d.x += speed
		moved = true
	}

	w := d.e.Canvas().Width()
	h := d.e.Canvas().Height()
	if d.x < 2 {
		d.x = 2
	}
	if d.y < 2 {
		d.y = 2
	}
	if d.x > float64(w-3) {
		d.x = float64(w - 3)
	}
	if d.y > float64(h-3) {
		d.y = float64(h - 3)
	}

	if moved {
		d.trail = append(d.trail, trailPoint{int(d.x), int(d.y)})
		const maxTrail = 80
		if len(d.trail) > maxTrail {
			d.trail = d.trail[len(d.trail)-maxTrail:]
		}
	}
	return nil
}

func (d *keysDemo) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 8, G: 8, B: 24, A: 255})

	// Fading trail — older positions are darker.
	n := len(d.trail)
	for i, p := range d.trail {
		t := uint8(255 * (i + 1) / n)
		c.Set(p.x, p.y, engine.Color{R: t, G: t / 3, B: t / 8, A: 255})
	}

	// Cursor.
	cx, cy := int(d.x), int(d.y)
	c.FillCircle(cx, cy, 2, engine.Yellow)
	c.DrawCircle(cx, cy, 4, engine.White)

	cols := c.Cols()

	// Title.
	title := "hold arrows or WASD — combine for diagonal motion"
	tc := (cols - utf8.RuneCountInString(title)) / 2
	if tc < 0 {
		tc = 0
	}
	c.Print(tc, 0, title, engine.White)

	// Four direction indicators in a row. Each cell lights up when its
	// direction key is held — two of them on at once is what proves
	// multi-key input is working.
	indicators := []struct {
		label string
		held  bool
	}{
		{"↑", d.up()},
		{"↓", d.down()},
		{"←", d.left()},
		{"→", d.right()},
	}
	const slotWidth = 6 // cells per indicator
	indicatorRow := 2
	startCol := (cols - slotWidth*len(indicators)) / 2
	if startCol < 0 {
		startCol = 0
	}
	for i, ind := range indicators {
		col := startCol + i*slotWidth
		fg := engine.Gray
		if ind.held {
			fg = engine.Black
			// 3-cell-wide yellow chip behind the glyph.
			c.FillRect(col-1, indicatorRow*2, 3, 2, engine.Yellow)
		}
		c.Print(col, indicatorRow, ind.label, fg)
	}

	// Diagnostic: show what the terminal actually accepted from our
	// progressive-enhancement query. Flag 2 (event types) is the one that
	// matters for combos — without it we're stuck with the auto-repeat
	// fallback and the OS will only repeat the most-recently pressed key.
	flags := d.e.KittyKeyboardFlags()
	talked := d.e.TerminalReplied()
	var status string
	var statusColor engine.Color
	orange := engine.Color{R: 220, G: 120, B: 80, A: 255}
	switch {
	case flags < 0 && !talked:
		status = "no replies — DA1 silent too, input plumbing issue?"
		statusColor = orange
	case flags < 0 && talked:
		status = "DA1 ok, kitty query ignored — terminal lacks kitty kbd"
		statusColor = orange
	case flags == 0:
		status = "kitty: replied with 0 flags (legacy mode)"
		statusColor = orange
	default:
		status = fmt.Sprintf("kitty flags: %d (%s)", flags, flagNames(flags))
		if flags&2 != 0 {
			statusColor = engine.Cyan
		} else {
			statusColor = engine.Yellow
		}
	}
	statusRow := c.Rows() - 4
	c.Print((cols-utf8.RuneCountInString(status))/2, statusRow, status, statusColor)

	// Per-key press/release counters. If a key shows "p:N r:0" after you
	// pressed and released it, the terminal isn't sending release events
	// for that key (or our parser isn't recognising them).
	type counterDef struct {
		label string
		p, r  int
	}
	upP, upR := d.e.KeyEventCounts(engine.KeyUp)
	downP, downR := d.e.KeyEventCounts(engine.KeyDown)
	leftP, leftR := d.e.KeyEventCounts(engine.KeyLeft)
	rightP, rightR := d.e.KeyEventCounts(engine.KeyRight)
	wP, wR := d.e.CharEventCounts('w')
	counters := []counterDef{
		{"↑", upP, upR},
		{"↓", downP, downR},
		{"←", leftP, leftR},
		{"→", rightP, rightR},
		{"w", wP, wR},
	}
	parts := make([]string, len(counters))
	for i, c := range counters {
		parts[i] = fmt.Sprintf("%s p:%d r:%d", c.label, c.p, c.r)
	}
	line := strings.Join(parts, "   ")
	c.Print((cols-utf8.RuneCountInString(line))/2, c.Rows()-3, line, engine.Gray)

	drawFooter(c, "multi-key  •  esc back")
}

// flagNames returns a comma-separated list of Kitty keyboard protocol
// flag names that are set in the bitmask.
func flagNames(flags int) string {
	var parts []string
	if flags&1 != 0 {
		parts = append(parts, "disambiguate")
	}
	if flags&2 != 0 {
		parts = append(parts, "events")
	}
	if flags&4 != 0 {
		parts = append(parts, "alt-keys")
	}
	if flags&8 != 0 {
		parts = append(parts, "all-escapes")
	}
	if flags&16 != 0 {
		parts = append(parts, "text")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "+")
}
