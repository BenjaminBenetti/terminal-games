package engine

import (
	"bytes"
	"fmt"
	"io"
)

// upperHalfBlock displays the foreground colour as the top half of the cell
// and the background colour as the bottom half.
const upperHalfBlock = "▀"

// cellState is the rendered state of one terminal cell. It is either a
// half-block "pixel cell" (isText=false, fg=top pixel colour, bg=bottom
// pixel colour, r unused) or a "text cell" placed via Canvas.Print
// (isText=true, fg=text colour, bg=averaged underlying pixel colours,
// r=rune).
type cellState struct {
	isText bool
	r      rune
	fg, bg Color
}

// renderer translates a Canvas into ANSI escape sequences and only writes
// the cells that changed since the previous frame.
type renderer struct {
	front  []cellState
	width  int
	height int
	buf    bytes.Buffer
}

// render writes the diff between c and the renderer's last known state to out.
// The first call (or any call after the canvas size changes) emits every cell.
func (r *renderer) render(c *Canvas, out io.Writer) error {
	r.syncSize(c)
	r.buf.Reset()

	var lastFG, lastBG Color
	colorsSet := false
	lastRow, lastCol := -2, -2
	cellRows := r.height / 2

	for row := 0; row < cellRows; row++ {
		rowBase := row * r.width
		topStart := (row * 2) * r.width
		botStart := topStart + r.width
		for col := 0; col < r.width; col++ {
			cellIdx := rowBase + col
			top := c.pixels[topStart+col]
			bot := c.pixels[botStart+col]

			var current cellState
			if tc, ok := c.text[cellIdx]; ok {
				current = cellState{
					isText: true,
					r:      tc.r,
					fg:     displayColor(tc.fg),
					bg:     averageColor(top, bot),
				}
			} else {
				current = cellState{
					isText: false,
					fg:     displayColor(top),
					bg:     displayColor(bot),
				}
			}

			if current == r.front[cellIdx] {
				continue
			}

			if row != lastRow || col != lastCol+1 {
				fmt.Fprintf(&r.buf, "\x1b[%d;%dH", row+1, col+1)
			}
			if !colorsSet || current.fg != lastFG {
				fmt.Fprintf(&r.buf, "\x1b[38;2;%d;%d;%dm", current.fg.R, current.fg.G, current.fg.B)
				lastFG = current.fg
			}
			if !colorsSet || current.bg != lastBG {
				fmt.Fprintf(&r.buf, "\x1b[48;2;%d;%d;%dm", current.bg.R, current.bg.G, current.bg.B)
				lastBG = current.bg
			}
			colorsSet = true
			if current.isText {
				r.buf.WriteRune(current.r)
			} else {
				r.buf.WriteString(upperHalfBlock)
			}

			r.front[cellIdx] = current
			lastRow = row
			lastCol = col
		}
	}

	if r.buf.Len() == 0 {
		return nil
	}
	r.buf.WriteString("\x1b[0m")
	_, err := out.Write(r.buf.Bytes())
	return err
}

// syncSize ensures the front buffer matches the canvas dimensions. When the
// canvas resizes, the front buffer is reset to a sentinel value so every
// cell is treated as dirty on the next render.
func (r *renderer) syncSize(c *Canvas) {
	cellCount := c.width * (c.height / 2)
	if r.width == c.width && r.height == c.height && len(r.front) == cellCount {
		return
	}
	r.width = c.width
	r.height = c.height
	r.front = make([]cellState, cellCount)
	sentinel := cellState{fg: Color{R: 1, G: 2, B: 3, A: 0}}
	for i := range r.front {
		r.front[i] = sentinel
	}
}

// displayColor maps the stored pixel colour to what should actually be drawn.
// Transparent pixels render as black since the terminal cannot show nothing.
func displayColor(c Color) Color {
	if c.A == 0 {
		return Color{R: 0, G: 0, B: 0, A: 255}
	}
	return c
}

// averageColor returns the per-channel mean of two colours after mapping
// transparency to black. Used to derive a single background colour for a
// text cell that sits on top of two stacked pixels.
func averageColor(a, b Color) Color {
	da := displayColor(a)
	db := displayColor(b)
	return Color{
		R: uint8((uint16(da.R) + uint16(db.R)) / 2),
		G: uint8((uint16(da.G) + uint16(db.G)) / 2),
		B: uint8((uint16(da.B) + uint16(db.B)) / 2),
		A: 255,
	}
}
