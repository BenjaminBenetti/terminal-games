package main

import (
	"math"
	"time"
	"unicode/utf8"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// pickerScene is the interactive game launcher. It renders a scrollable
// list of registered games over a Tron-style perspective grid, and
// reports the picked one (if any) via the picked field once Run returns.
type pickerScene struct {
	e        *engine.Engine
	games    []registry.Game
	selected int
	viewport int
	picked   string // name of the selected game, or "" if the user quit

	// startTime is set on the first Update call. It governs both the
	// grace period before the Kitty-keyboard warning surfaces and the
	// time origin for every animation in Draw.
	startTime time.Time
}

// kittyDetectionGrace is how long the picker waits before deciding a
// terminal genuinely doesn't speak the Kitty keyboard protocol. The
// terminal's reply to our \x1b[?u query typically arrives in <50 ms,
// but we give some headroom to avoid a one-frame flash of the warning
// on supporting terminals.
const kittyDetectionGrace = 250 * time.Millisecond

const kittyHelpURL = "https://terminaltrove.com/compare/terminals/?features=kitty-keyboard-protocol"

// Tron palette — cyan/teal for "the system", orange for the user.
var (
	tronCyan      = engine.Color{R: 110, G: 240, B: 255, A: 255}
	tronCyanMid   = engine.Color{R: 60, G: 170, B: 215, A: 255}
	tronCyanDim   = engine.Color{R: 30, G: 90, B: 130, A: 255}
	tronOrange    = engine.Color{R: 255, G: 150, B: 40, A: 255}
	tronWhite     = engine.Color{R: 220, G: 245, B: 255, A: 255}
	tronBg        = engine.Color{R: 4, G: 8, B: 18, A: 255}
)

func newPickerScene(e *engine.Engine, games []registry.Game) *pickerScene {
	return &pickerScene{e: e, games: games}
}

func (p *pickerScene) Update(time.Duration) error {
	if p.startTime.IsZero() {
		p.startTime = time.Now()
	}
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return nil
		}
		switch k.Code {
		case engine.KeyUp:
			p.moveUp()
		case engine.KeyDown:
			p.moveDown()
		case engine.KeyEnter:
			p.picked = p.games[p.selected].Name()
			return engine.ErrQuit
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				return engine.ErrQuit
			case 'j', 'J':
				p.moveDown()
			case 'k', 'K':
				p.moveUp()
			}
		}
	}
}

func (p *pickerScene) moveUp() {
	if p.selected > 0 {
		p.selected--
	}
}

func (p *pickerScene) moveDown() {
	if p.selected < len(p.games)-1 {
		p.selected++
	}
}

func (p *pickerScene) Draw(c *engine.Canvas) {
	c.Clear(tronBg)

	cols, rows := c.Cols(), c.Rows()
	width := c.Width()
	n := len(p.games)
	if n == 0 {
		return
	}

	t := 0.0
	if !p.startTime.IsZero() {
		t = time.Since(p.startTime).Seconds()
	}

	// Perspective grid first, so everything else stacks over it.
	drawTronGrid(c, t)

	titleBottomRow := drawTronTitle(c, t)

	showWarning := !p.startTime.IsZero() &&
		time.Since(p.startTime) > kittyDetectionGrace &&
		!p.e.KittyKeyboardDetected()

	frameTop := titleBottomRow + 1
	frameBottom := rows - 2 // exclusive; row rows-1 is the hint
	if showWarning {
		frameBottom = rows - 4
	}
	if frameBottom-frameTop < 5 {
		frameTop = frameBottom - 5
		if frameTop < 1 {
			frameTop = 1
		}
	}

	// Dim the grid behind the list panel so names stay readable. Keeps
	// the grid as a faint backdrop instead of wiping it entirely.
	dimRegion(c, 0, frameTop*2, width, (frameBottom-frameTop)*2, 0.32)

	drawTronFrame(c, 0, frameTop, cols, frameBottom-frameTop, t)

	listTop := frameTop + 1
	listBottom := frameBottom - 1
	visibleRows := listBottom - listTop
	if visibleRows < 1 {
		visibleRows = 1
	}

	if p.selected < p.viewport {
		p.viewport = p.selected
	} else if p.selected >= p.viewport+visibleRows {
		p.viewport = p.selected - visibleRows + 1
	}

	maxNameLen := 0
	for _, g := range p.games {
		if l := utf8.RuneCountInString(g.Name()); l > maxNameLen {
			maxNameLen = l
		}
	}
	const sidePad = 4
	barWidth := maxNameLen + sidePad*2
	if barWidth > cols-4 {
		barWidth = cols - 4
	}
	barCol := (cols - barWidth) / 2
	if barCol < 1 {
		barCol = 1
	}

	itemsShown := n - p.viewport
	if itemsShown > visibleRows {
		itemsShown = visibleRows
	}
	startRow := listTop + (visibleRows-itemsShown)/2

	// Tron-orange highlight, with a subtle brightness pulse.
	pulse := 0.85 + 0.15*math.Sin(t*4)
	highlight := scaleColor(tronOrange, pulse)
	bracketOn := math.Mod(t, 0.8) < 0.6

	for row := 0; row < itemsShown; row++ {
		idx := p.viewport + row
		g := p.games[idx]
		y := startRow + row
		name := g.Name()
		nameLen := utf8.RuneCountInString(name)
		nameCol := (cols - nameLen) / 2

		if idx == p.selected {
			c.FillRect(barCol, y*2, barWidth, 2, highlight)
			c.Print(nameCol, y, name, engine.Black)
			if bracketOn {
				if nameCol-3 >= barCol {
					c.Print(nameCol-3, y, ">", engine.Black)
				}
				if nameCol+nameLen+2 < barCol+barWidth {
					c.Print(nameCol+nameLen+2, y, "<", engine.Black)
				}
			}
		} else {
			// Alternate bright/mid cyan to suggest CRT scanlines.
			col := tronCyan
			if (idx & 1) == 1 {
				col = tronCyanMid
			}
			c.Print(nameCol, y, name, col)
		}
	}

	chev := tronCyan
	if p.viewport > 0 {
		c.Print(cols-2, listTop, "▲", chev)
	}
	if p.viewport+visibleRows < n {
		c.Print(cols-2, listBottom-1, "▼", chev)
	}

	if showWarning {
		drawKittyWarning(c, rows-3)
	}

	drawTronHint(c, rows-1, t)
}

// drawTronGrid paints a perspective floor grid: vertical lines
// converging to a vanishing point on the horizon, plus horizontal lines
// scrolling toward the viewer with quadratic perspective falloff.
func drawTronGrid(c *engine.Canvas, t float64) {
	width, height := c.Width(), c.Height()
	// Push the horizon down to ~40% so it sits below the title area.
	horizonY := int(float64(height) * 0.42)
	floorH := height - horizonY
	if floorH < 6 {
		return
	}

	// Vertical converging lines.
	vanishX := width / 2
	spread := 5
	for i := -spread; i <= spread; i++ {
		// Linear spread at the bottom edge.
		floorX := vanishX + i*(width/spread)
		col := tronCyanDim
		if i == 0 {
			col = tronCyanMid
		}
		c.DrawLine(vanishX, horizonY, floorX, height-1, col)
	}

	// Horizontal scrolling perspective lines.
	const numH = 9
	phase := math.Mod(t*0.35, 1.0)
	for i := 0; i < numH; i++ {
		p := (float64(i) + phase) / float64(numH)
		if p > 1 {
			p -= 1
		}
		yp := p * p // perspective falloff: lines bunch near horizon
		y := horizonY + int(float64(floorH-1)*yp)
		bright := 0.20 + 0.80*yp
		col := scaleColor(tronCyan, bright*0.75)
		for x := 0; x < width; x++ {
			c.Set(x, y, col)
		}
	}

	// Bright horizon line + thin glow above it.
	for x := 0; x < width; x++ {
		c.Set(x, horizonY, tronCyanMid)
		if horizonY-1 >= 0 {
			c.Set(x, horizonY-1, tronCyanDim)
		}
	}
}

// drawTronTitle paints "TERMINAL GAMES" with a cyan halo. Returns the
// bottom-most cell-row the title occupies.
//
// Layouts tried widest-first: single-line pixel-art, two-line pixel-art,
// then a native-font fallback for very narrow terminals.
func drawTronTitle(c *engine.Canvas, t float64) int {
	cols := c.Cols()
	width := c.Width()

	pulse := 0.85 + 0.15*math.Sin(t*1.8)
	main := scaleColor(tronCyan, pulse)
	glow := scaleColor(tronCyan, 0.32)
	halo := []struct{ dx, dy int }{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

	const single = "TERMINAL GAMES"
	if singlePx := engine.TextWidth(single); singlePx <= width-4 {
		x := (width - singlePx) / 2
		y := 2
		for _, off := range halo {
			c.DrawText(x+off.dx, y+off.dy, single, glow)
		}
		c.DrawText(x, y, single, main)
		return (y + engine.FontHeight + 1) / 2
	}

	const top = "TERMINAL"
	const bot = "GAMES"
	topPx := engine.TextWidth(top)
	botPx := engine.TextWidth(bot)
	if topPx <= width-2 && botPx <= width-2 {
		y1 := 1
		y2 := y1 + engine.FontHeight + 2
		xTop := (width - topPx) / 2
		xBot := (width - botPx) / 2
		for _, off := range halo {
			c.DrawText(xTop+off.dx, y1+off.dy, top, glow)
			c.DrawText(xBot+off.dx, y2+off.dy, bot, glow)
		}
		c.DrawText(xTop, y1, top, main)
		c.DrawText(xBot, y2, bot, main)
		return (y2 + engine.FontHeight + 1) / 2
	}

	titleCol := (cols - utf8.RuneCountInString(single)) / 2
	if titleCol < 0 {
		titleCol = 0
	}
	c.Print(titleCol, 0, single, main)
	return 0
}

// drawTronFrame paints a thin cyan frame with bright corner accents,
// orange HUD tick marks, and a chasing orange "data packet" that runs
// along the top edge (mirrored cyan packet runs the opposite way along
// the bottom).
func drawTronFrame(c *engine.Canvas, col, row, w, h int, t float64) {
	if w < 2 || h < 2 {
		return
	}
	wall := tronCyanMid
	accent := tronCyan

	for x := col + 1; x < col+w-1; x++ {
		c.Print(x, row, "─", wall)
		c.Print(x, row+h-1, "─", wall)
	}
	for y := row + 1; y < row+h-1; y++ {
		c.Print(col, y, "│", wall)
		c.Print(col+w-1, y, "│", wall)
	}
	c.Print(col, row, "┌", accent)
	c.Print(col+w-1, row, "┐", accent)
	c.Print(col, row+h-1, "└", accent)
	c.Print(col+w-1, row+h-1, "┘", accent)

	// Orange HUD ticks just inside each corner.
	if w >= 6 && h >= 4 {
		c.Print(col+1, row, "┬", tronOrange)
		c.Print(col+w-2, row, "┬", tronOrange)
		c.Print(col+1, row+h-1, "┴", tronOrange)
		c.Print(col+w-2, row+h-1, "┴", tronOrange)
	}

	// Chasing data packets along the top and bottom rails. A short
	// fading trail sells the motion.
	inner := w - 2
	if inner <= 0 {
		return
	}
	trailLen := 5
	headTop := int(math.Mod(t*14, float64(inner)))
	headBot := inner - 1 - headTop
	for i := 0; i < trailLen; i++ {
		b := 1.0 - float64(i)*0.18
		if b < 0.15 {
			b = 0.15
		}
		// Top: orange packet sweeping left→right.
		pos := headTop - i
		for pos < 0 {
			pos += inner
		}
		c.Print(col+1+pos, row, "═", scaleColor(tronOrange, b))
		// Bottom: cyan packet sweeping right→left.
		pos = headBot + i
		for pos >= inner {
			pos -= inner
		}
		c.Print(col+1+pos, row+h-1, "═", scaleColor(tronCyan, b))
	}
}

// drawTronHint paints the bottom hint with bracketed key indicators and
// a blinking ENTER call-to-action.
func drawTronHint(c *engine.Canvas, row int, t float64) {
	cols := c.Cols()
	dim := tronCyanDim
	label := tronWhite
	keyHot := tronOrange

	on := math.Mod(t, 1.0) < 0.7
	execCol := keyHot
	if !on {
		execCol = scaleColor(tronOrange, 0.45)
	}

	long := []hintPart{
		{"[↑↓] ", keyHot},
		{"SELECT", label},
		{"   ", dim},
		{"[ENTER] ", execCol},
		{"EXECUTE", label},
		{"   ", dim},
		{"[Q] ", keyHot},
		{"EXIT", label},
	}
	short := []hintPart{
		{"[↑↓] SELECT  ", label},
		{"[ENTER] ", execCol},
		{"EXEC  [Q] EXIT", label},
	}

	parts := long
	if hintLen(long) > cols {
		parts = short
	}

	total := hintLen(parts)
	colStart := (cols - total) / 2
	if colStart < 0 {
		colStart = 0
	}
	for _, p := range parts {
		c.Print(colStart, row, p.text, p.colour)
		colStart += utf8.RuneCountInString(p.text)
	}
}

type hintPart struct {
	text   string
	colour engine.Color
}

func hintLen(parts []hintPart) int {
	n := 0
	for _, p := range parts {
		n += utf8.RuneCountInString(p.text)
	}
	return n
}

// dimRegion scales every pixel inside the given pixel-coord rectangle by
// factor. Used to fade the perspective grid behind the list panel so
// names stay legible without losing the grid entirely.
func dimRegion(c *engine.Canvas, x, y, w, h int, factor float64) {
	if w <= 0 || h <= 0 {
		return
	}
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			c.Set(xx, yy, scaleColor(c.Get(xx, yy), factor))
		}
	}
}

// scaleColor multiplies an opaque colour's RGB channels by factor
// (clamped to [0, 1]).
func scaleColor(c engine.Color, factor float64) engine.Color {
	if factor < 0 {
		factor = 0
	}
	if factor > 1 {
		factor = 1
	}
	return engine.Color{
		R: uint8(float64(c.R) * factor),
		G: uint8(float64(c.G) * factor),
		B: uint8(float64(c.B) * factor),
		A: c.A,
	}
}

// drawKittyWarning renders a two-line "no kitty kbd" message centred on
// firstRow (line 1) and firstRow+1 (line 2). The terminal-trove URL is
// shortened progressively when the canvas is narrower than the full
// link.
func drawKittyWarning(c *engine.Canvas, firstRow int) {
	cols := c.Cols()
	amber := engine.Color{R: 220, G: 160, B: 80, A: 255}
	url := engine.Cyan

	msg := "⚠  no kitty keyboard protocol — works best with a kitty-aware terminal"
	if utf8.RuneCountInString(msg) > cols {
		msg = "⚠  no kitty keyboard protocol"
	}
	c.Print((cols-utf8.RuneCountInString(msg))/2, firstRow, msg, amber)

	link := kittyHelpURL
	if utf8.RuneCountInString(link) > cols {
		link = "terminaltrove.com/compare/terminals/"
	}
	if utf8.RuneCountInString(link) > cols {
		link = "terminaltrove.com"
	}
	c.Print((cols-utf8.RuneCountInString(link))/2, firstRow+1, link, url)
}
