package main

import (
	"time"
	"unicode/utf8"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// pickerScene is the interactive game launcher. It renders a scrollable
// list of registered games and reports the picked one (if any) via the
// picked field once Run returns.
type pickerScene struct {
	e        *engine.Engine
	games    []registry.Game
	selected int
	viewport int
	picked   string // name of the selected game, or "" if the user quit

	// startTime is set on the first Update call. We use it to delay the
	// "kitty kbd not detected" warning by a short grace period so a
	// supporting terminal's flags reply has time to arrive before we
	// decide nothing came back.
	startTime time.Time
}

// kittyDetectionGrace is how long the picker waits before deciding a
// terminal genuinely doesn't speak the Kitty keyboard protocol. The
// terminal's reply to our \x1b[?u query typically arrives in <50 ms,
// but we give some headroom to avoid a one-frame flash of the warning
// on supporting terminals.
const kittyDetectionGrace = 250 * time.Millisecond

const kittyHelpURL = "https://terminaltrove.com/compare/terminals/?features=kitty-keyboard-protocol"

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
	c.Clear(engine.Color{R: 12, G: 12, B: 24, A: 255})

	cols, rows := c.Cols(), c.Rows()
	n := len(p.games)
	if n == 0 {
		return
	}

	// Title.
	title := "TERMINAL GAMES"
	titleCol := (cols - utf8.RuneCountInString(title)) / 2
	if titleCol < 0 {
		titleCol = 0
	}
	c.Print(titleCol, 0, title, engine.White)

	// Decide whether to show the "no Kitty keyboard" warning. Wait a
	// little after startup so a supporting terminal's flags reply has
	// time to arrive.
	showWarning := !p.startTime.IsZero() &&
		time.Since(p.startTime) > kittyDetectionGrace &&
		!p.e.KittyKeyboardDetected()

	// List area sits between the title and the hint, with the optional
	// warning carved out of the bottom.
	listTop := 2
	listBottom := rows - 2 // exclusive; row rows-1 is the hint
	if showWarning {
		listBottom = rows - 4 // make room for 2 warning lines at rows-3, rows-2
	}
	visibleRows := listBottom - listTop
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Keep the selected item in view.
	if p.selected < p.viewport {
		p.viewport = p.selected
	} else if p.selected >= p.viewport+visibleRows {
		p.viewport = p.selected - visibleRows + 1
	}

	// Centre the highlight bar around the widest name so the list reads
	// as a tidy column rather than spanning the full canvas width.
	maxNameLen := 0
	for _, g := range p.games {
		if l := utf8.RuneCountInString(g.Name()); l > maxNameLen {
			maxNameLen = l
		}
	}
	const sidePad = 2
	barWidth := maxNameLen + sidePad*2
	if barWidth > cols {
		barWidth = cols
	}
	barCol := (cols - barWidth) / 2
	if barCol < 0 {
		barCol = 0
	}

	// Vertically centre the visible chunk of the list inside the band.
	itemsShown := n - p.viewport
	if itemsShown > visibleRows {
		itemsShown = visibleRows
	}
	startRow := listTop + (visibleRows-itemsShown)/2

	for row := 0; row < itemsShown; row++ {
		idx := p.viewport + row
		g := p.games[idx]
		y := startRow + row
		nameLen := utf8.RuneCountInString(g.Name())
		nameCol := (cols - nameLen) / 2
		colour := engine.Gray
		if idx == p.selected {
			colour = engine.Black
			c.FillRect(barCol, y*2, barWidth, 2, engine.Yellow)
		}
		c.Print(nameCol, y, g.Name(), colour)
	}

	// Scroll chevrons in the right margin when more entries exist.
	if p.viewport > 0 {
		c.Print(cols-2, listTop, "↑", engine.Cyan)
	}
	if p.viewport+visibleRows < n {
		c.Print(cols-2, listBottom-1, "↓", engine.Cyan)
	}

	// Kitty-keyboard not-detected warning, if applicable.
	if showWarning {
		drawKittyWarning(c, rows-3)
	}

	// Hint footer.
	hint := "↑↓ select   enter play   q quit"
	c.Print((cols-utf8.RuneCountInString(hint))/2, rows-1, hint, engine.Gray)
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
