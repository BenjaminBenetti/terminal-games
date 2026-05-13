package brickbreaker

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// brickType selects sprite, hit points, and score for a single brick slot
// in a level pattern. Slots are encoded as runes in the level rows.
type brickType int

const (
	brickEmpty  brickType = iota
	brickWeak             // 1 hit — '#'
	brickStrong           // 2 hits — '@'
	brickTough            // 3 hits — '*'
)

func (b brickType) hits() int {
	switch b {
	case brickWeak:
		return 1
	case brickStrong:
		return 2
	case brickTough:
		return 3
	}
	return 0
}

// baseColor returns the brick's full-health colour. Damaged bricks dim
// from this base via color().
func (b brickType) baseColor() engine.Color {
	switch b {
	case brickWeak:
		return engine.Color{R: 90, G: 200, B: 255, A: 255}
	case brickStrong:
		return engine.Color{R: 110, G: 230, B: 130, A: 255}
	case brickTough:
		return engine.Color{R: 255, G: 130, B: 80, A: 255}
	}
	return engine.Transparent
}

// color returns the brick's display colour given its remaining HP. A
// brick darkens as it takes damage so the player can read how close it
// is to breaking.
func (b brickType) color(hpRemaining int) engine.Color {
	base := b.baseColor()
	max := b.hits()
	if max <= 0 || hpRemaining >= max {
		return base
	}
	factor := float64(hpRemaining) / float64(max)
	dim := 0.4 + 0.6*factor
	return engine.Color{
		R: uint8(float64(base.R) * dim),
		G: uint8(float64(base.G) * dim),
		B: uint8(float64(base.B) * dim),
		A: 255,
	}
}

// score returns the points awarded for destroying a brick of this type.
func (b brickType) score() int {
	switch b {
	case brickWeak:
		return 10
	case brickStrong:
		return 25
	case brickTough:
		return 50
	}
	return 0
}

// brickFromRune maps a level-pattern rune to a brick type. Any rune not
// in the table is treated as an empty slot.
func brickFromRune(r byte) brickType {
	switch r {
	case '#':
		return brickWeak
	case '@':
		return brickStrong
	case '*':
		return brickTough
	}
	return brickEmpty
}

// levelDef is a hand-authored level. The brick pattern is a list of rows
// where each rune is a brick slot; the slot width adapts to the canvas.
type levelDef struct {
	name        string
	summary     string
	ballSpeed   float64 // px/s
	paddleWidth int     // px
	paddleSpeed float64 // px/s
	rows        []string
}

// levels is the canonical set of selectable levels. Index in this slice
// is the user-facing level number minus one.
//
// The patterns are designed for "combo runs": a ball that breaches the
// wall and ricochets between the top of the play area and the brick
// ceiling racks up huge multipliers. Each level has a deliberate entry
// path (gaps, channels, or thin spots) the player can aim for.
var levels = []levelDef{
	{
		name:        "PYRAMID PUNCH",
		summary:     "Soft sides, dense top — climb the wings",
		ballSpeed:   30,
		paddleWidth: 16,
		paddleSpeed: 50,
		rows: []string{
			"....######....",
			"...########...",
			"..##########..",
			".############.",
			"##############",
		},
	},
	{
		name:        "TWIN TOWERS",
		summary:     "Thread the middle channel",
		ballSpeed:   38,
		paddleWidth: 12,
		paddleSpeed: 56,
		rows: []string{
			"#####....#####",
			"#####....#####",
			"@@@##....##@@@",
			"@@@@@....@@@@@",
			"@@@@@@@@@@@@@@",
			"##############",
		},
	},
	{
		name:        "VAULT",
		summary:     "Armored fortress, narrow breaches",
		ballSpeed:   48,
		paddleWidth: 10,
		paddleSpeed: 62,
		rows: []string{
			"**************",
			"@@##########@@",
			"##@@@@@@@@@@##",
			"##@@****@@@@##",
			"##@@@@@@@@@@##",
			"@@##########@@",
			"##############",
		},
	},
}
