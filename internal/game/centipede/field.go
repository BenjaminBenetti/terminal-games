package centipede

import (
	"math/rand"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// mushroom is a single cell occupant. hp is remaining hits before
// destruction; 0 means the cell is empty. poisoned is set by a scorpion
// passing through and triggers the centipede's straight-dive behaviour.
type mushroom struct {
	hp       int  // 1..mushroomHP when present, 0 when empty
	poisoned bool
}

const (
	mushroomHP = 4

	// initialMushroomDensity is the fraction of cells seeded as
	// mushrooms when a fresh field is generated. The arcade had a
	// fairly dense field; ~12% gives a good blocker count without
	// suffocating the playfield.
	initialMushroomDensity = 0.12

	// fleaMushroomTriggerCount is the cumulative count of mushrooms in
	// the lower band of the field below which a flea will spawn. Lower
	// counts make fleas more frequent.
	fleaMushroomTriggerCount = 4
)

// field is the mushroom grid. It owns the cell array plus the geometry
// needed to translate between cell and pixel coordinates.
type field struct {
	cols, rows int
	// originX, originY are the pixel coordinates of cell (0, 0).
	originX, originY int
	cells            [][]mushroom

	// playerZoneTop is the cell row at which the player zone begins;
	// mushrooms are not seeded above this — wait, vice versa: this is
	// the row at which the player zone starts (mushrooms still allowed,
	// but the player may move freely from this row down).
	playerZoneTop int
}

// newField builds a field sized to fit inside (canvasW × (canvasH -
// hudHeightPx)) pixels and seeds an initial mushroom layout using rng.
func newField(canvasW, canvasH, hudHeightPx int, rng *rand.Rand) *field {
	cols := canvasW / cellW
	rows := (canvasH - hudHeightPx) / cellH

	f := &field{
		cols:    cols,
		rows:    rows,
		originX: 0,
		originY: hudHeightPx,
		cells:   make([][]mushroom, rows),
	}
	for r := 0; r < rows; r++ {
		f.cells[r] = make([]mushroom, cols)
	}
	// Player zone: bottom 4 cell rows. Centipede also enters this zone
	// (bouncing back up) so it's not literally off-limits — just a band
	// the player can roam.
	f.playerZoneTop = rows - 4
	if f.playerZoneTop < rows-1 {
		// guard against tiny terminals
	}
	f.seed(rng)
	return f
}

// seed plants the initial random mushroom layout. The top row and the
// bottom-most row are kept clear so the centipede has somewhere to
// enter and the player doesn't spawn inside a mushroom.
func (f *field) seed(rng *rand.Rand) {
	target := int(float64(f.cols*f.rows) * initialMushroomDensity)
	planted := 0
	for tries := 0; tries < target*4 && planted < target; tries++ {
		c := rng.Intn(f.cols)
		r := 1 + rng.Intn(f.rows-2)
		if r >= f.rows-1 {
			continue
		}
		if f.cells[r][c].hp > 0 {
			continue
		}
		f.cells[r][c] = mushroom{hp: mushroomHP}
		planted++
	}
}

// regrow tops the field back up to the initial density, restoring any
// damaged mushrooms to full health and converting any poisoned ones
// back to normal. Called between waves.
func (f *field) regrow(rng *rand.Rand) {
	count := 0
	for r := 0; r < f.rows; r++ {
		for c := 0; c < f.cols; c++ {
			if f.cells[r][c].hp > 0 {
				// Restore damaged mushrooms to full and un-poison.
				f.cells[r][c].hp = mushroomHP
				f.cells[r][c].poisoned = false
				count++
			}
		}
	}
	target := int(float64(f.cols*f.rows) * initialMushroomDensity)
	for tries := 0; tries < target*4 && count < target; tries++ {
		c := rng.Intn(f.cols)
		r := 1 + rng.Intn(f.rows-2)
		if r >= f.rows-1 {
			continue
		}
		if f.cells[r][c].hp > 0 {
			continue
		}
		f.cells[r][c] = mushroom{hp: mushroomHP}
		count++
	}
}

// inBounds reports whether (col, row) is a valid cell index.
func (f *field) inBounds(col, row int) bool {
	return col >= 0 && col < f.cols && row >= 0 && row < f.rows
}

// hasMushroom reports whether cell (col, row) holds a live mushroom.
func (f *field) hasMushroom(col, row int) bool {
	if !f.inBounds(col, row) {
		return false
	}
	return f.cells[row][col].hp > 0
}

// isPoisoned reports whether the live mushroom at (col, row) is
// poisoned. Returns false for empty cells.
func (f *field) isPoisoned(col, row int) bool {
	if !f.hasMushroom(col, row) {
		return false
	}
	return f.cells[row][col].poisoned
}

// plant grows a fresh full-health mushroom in (col, row) if the cell is
// empty and in bounds. Returns true on success.
func (f *field) plant(col, row int) bool {
	if !f.inBounds(col, row) || f.cells[row][col].hp > 0 {
		return false
	}
	f.cells[row][col] = mushroom{hp: mushroomHP}
	return true
}

// damage applies a single hit to the cell. Returns the score awarded
// (1 for the killing hit on a normal mushroom, 5 for a poisoned one;
// 0 otherwise) plus a flag indicating whether the mushroom was killed.
func (f *field) damage(col, row int) (score int, destroyed bool) {
	if !f.hasMushroom(col, row) {
		return 0, false
	}
	f.cells[row][col].hp--
	if f.cells[row][col].hp <= 0 {
		poisoned := f.cells[row][col].poisoned
		f.cells[row][col] = mushroom{}
		if poisoned {
			return 5, true
		}
		return 1, true
	}
	return 0, false
}

// eat removes a mushroom outright (no score). Used by the spider.
func (f *field) eat(col, row int) bool {
	if !f.hasMushroom(col, row) {
		return false
	}
	f.cells[row][col] = mushroom{}
	return true
}

// poison flags the mushroom in (col, row) as poisoned, if it exists.
func (f *field) poison(col, row int) {
	if !f.hasMushroom(col, row) {
		return
	}
	f.cells[row][col].poisoned = true
}

// lowerCount returns the number of mushrooms in the bottom band of the
// field — used by the flea spawn heuristic.
func (f *field) lowerCount() int {
	count := 0
	for r := f.playerZoneTop - 2; r < f.rows; r++ {
		if r < 0 {
			continue
		}
		for c := 0; c < f.cols; c++ {
			if f.cells[r][c].hp > 0 {
				count++
			}
		}
	}
	return count
}

// cellPixel returns the top-left pixel coordinate of cell (col, row).
func (f *field) cellPixel(col, row int) (int, int) {
	return f.originX + col*cellW, f.originY + row*cellH
}

// cellAtPixel converts a pixel coordinate to its enclosing cell (or
// (-1,-1) if outside the grid).
func (f *field) cellAtPixel(x, y int) (col, row int) {
	if x < f.originX || y < f.originY {
		return -1, -1
	}
	c := (x - f.originX) / cellW
	r := (y - f.originY) / cellH
	if !f.inBounds(c, r) {
		return -1, -1
	}
	return c, r
}

// draw renders every mushroom into the canvas with the correct damage
// frame and colour.
func (f *field) draw(c *engine.Canvas) {
	for r := 0; r < f.rows; r++ {
		for col := 0; col < f.cols; col++ {
			m := f.cells[r][col]
			if m.hp <= 0 {
				continue
			}
			frame := mushroomHP - m.hp
			if frame < 0 {
				frame = 0
			}
			if frame >= len(mushroomFrames) {
				frame = len(mushroomFrames) - 1
			}
			x, y := f.cellPixel(col, r)
			colr := colorMushroom
			if m.poisoned {
				colr = colorMushroomPoisoned
			}
			drawSprite(c, x, y, mushroomFrames[frame], colr)
		}
	}
}
