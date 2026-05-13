package donkeykong

import (
	"strings"
	"testing"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// TestStageLooksRight is an eyeball check, not a strict assertion — it
// dumps a glyph map of the canvas (one character per pixel cluster) so
// developers can confirm the layout still resembles Donkey Kong after a
// physics tweak. It's intentionally tolerant: it only fails if NOTHING is
// drawn, which would indicate a regression in the draw pipeline.
func TestStageLooksRight(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	c := engine.NewCanvas(80, 48)
	// Force into mid-play with a couple of barrels so the snapshot shows
	// them on the field.
	p.state = psPlaying
	p.timeT = 5
	for i := 0; i < 3; i++ {
		p.throwBarrel()
	}
	p.Draw(c)

	w, h := c.Width(), c.Height()
	any := false
	var b strings.Builder
	b.WriteString("\n")
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			col := c.Get(x, y)
			if col.A == 0 || (col.R == 0 && col.G == 0 && col.B == 0) {
				b.WriteByte('.')
				continue
			}
			any = true
			b.WriteByte(glyphFor(col))
		}
		b.WriteByte('\n')
	}
	if !any {
		t.Fatalf("canvas is entirely empty:\n%s", b.String())
	}
	// Always emit the dump for inspection when -v is on.
	t.Logf("stage snapshot:%s", b.String())
}

func glyphFor(col engine.Color) byte {
	switch {
	case col == marioRed:
		return 'R'
	case col == marioBlue:
		return 'B'
	case col == marioSkin:
		return 'S'
	case col == marioShoe:
		return 's'
	case col == marioHair:
		return 'h'
	case col == dkDark:
		return 'D'
	case col == dkLight:
		return 'L'
	case col == dkWhite:
		return 'w'
	case col == dkBlack:
		return 'k'
	case col == dkMouth:
		return 'm'
	case col == paulineHair:
		return 'Y'
	case col == paulineDress:
		return 'd'
	case col == barrelMain:
		return 'O'
	case col == barrelDark:
		return 'x'
	case col == girderRed:
		return '='
	case col == girderDark:
		return '-'
	case col == ladderYellow:
		return '|'
	case col == ladderDark:
		return '~'
	case col == hammerHead:
		return 'H'
	case col == hammerHandle:
		return 'A'
	case col == oilMain:
		return '#'
	case col == oilDark:
		return '*'
	case col == flameOut, col == flameMid, col == flameIn:
		return 'f'
	}
	return '?'
}
