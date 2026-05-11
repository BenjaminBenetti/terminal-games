package enginedemo

import (
	"unicode/utf8"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

type menuAction int

const (
	menuNone menuAction = iota
	menuLaunch
)

// menuScene renders a list of selectable demo names and exposes the
// currently highlighted index.
type menuScene struct {
	items    []string
	selected int
}

func (m *menuScene) handleKey(k engine.Key) menuAction {
	switch k.Code {
	case engine.KeyUp:
		if m.selected > 0 {
			m.selected--
		}
	case engine.KeyDown:
		if m.selected < len(m.items)-1 {
			m.selected++
		}
	case engine.KeyEnter:
		return menuLaunch
	case engine.KeyChar:
		// 1..N digit shortcuts.
		if k.Rune >= '1' && int(k.Rune-'0') <= len(m.items) {
			m.selected = int(k.Rune - '1')
			return menuLaunch
		}
	}
	return menuNone
}

func (m *menuScene) draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 12, G: 12, B: 24, A: 255})

	cols, rows := c.Cols(), c.Rows()
	n := len(m.items)
	if n == 0 {
		return
	}

	title := "ENGINE DEMOS"
	titleCol := (cols - len(title)) / 2
	if titleCol < 0 {
		titleCol = 0
	}
	c.Print(titleCol, 0, title, engine.White)

	maxNameLen := 0
	for _, name := range m.items {
		if l := utf8.RuneCountInString(name); l > maxNameLen {
			maxNameLen = l
		}
	}
	// Reserve two cells of padding on each side of the longest name for
	// the highlight bar to breathe.
	const sidePad = 2
	itemCol := (cols-maxNameLen)/2 - sidePad
	if itemCol < 0 {
		itemCol = 0
	}
	barWidth := maxNameLen + sidePad*2
	if barWidth > cols {
		barWidth = cols
	}

	// Centre the items vertically in the space between title and hint.
	const topMargin = 2
	const bottomMargin = 2
	available := rows - topMargin - bottomMargin
	startRow := topMargin + (available-n)/2
	if startRow < topMargin {
		startRow = topMargin
	}

	for i, name := range m.items {
		row := startRow + i
		if row >= rows-1 {
			break
		}
		colour := engine.Gray
		if i == m.selected {
			colour = engine.Black
			// Paint the highlight bar so Print's bg pulls yellow from the
			// underlying canvas pixels.
			c.FillRect(itemCol, row*2, barWidth, 2, engine.Yellow)
		}
		c.Print(itemCol+sidePad, row, name, colour)
	}

	hint := "↑↓ select   enter play   q quit   esc back"
	hintCells := utf8.RuneCountInString(hint)
	if hintCells > cols {
		hint = "↑↓ select   enter play   q quit"
		hintCells = utf8.RuneCountInString(hint)
	}
	if hintCells > cols {
		hint = "q quit"
		hintCells = utf8.RuneCountInString(hint)
	}
	c.Print((cols-hintCells)/2, rows-1, hint, engine.Gray)
}
