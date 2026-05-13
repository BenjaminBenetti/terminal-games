package galaga

import "math"

// Formation geometry. The classic Galaga formation is 4 Boss Galagas
// (top row, centred), 16 Butterflies (two middle rows of 8), and 20
// Bees (two bottom rows of 10). The terminal canvas is landscape-oriented
// and shorter than the arcade portrait screen, so this build uses a
// 5x8 grid with 4 + 16 + 16 = 36 enemies, with the 4 Bosses occupying
// the middle four columns of row 0.
const (
	formationRows = 5
	formationCols = 8

	// Per-slot horizontal pitch. 7-px wide sprite + 1-px gap = 8 px.
	slotPitchX = 8
	// Per-slot vertical pitch. 5-px tall sprite + 1-px gap = 6 px.
	slotPitchY = 6

	// Maximum sway amplitude in pixels, peak-to-centre.
	formSwayAmp = 4.0
	// Seconds per full sway cycle.
	formSwayPeriod = 6.0
)

// formation describes the resting positions of every enemy slot. Per-
// enemy state lives on the enemy struct; this struct only owns the
// shared origin and sway phase.
type formation struct {
	originX float64 // canvas pixel x of slot (row=0, col=0)
	originY float64 // canvas pixel y of slot (row=0, col=0)
	swayT   float64 // accumulated seconds for the sway oscillator
}

// slotOffsetX returns the current sway offset in pixels (signed).
func (f *formation) swayOffset() float64 {
	return math.Sin(2*math.Pi*f.swayT/formSwayPeriod) * formSwayAmp
}

// slotPos returns the current pixel top-left of the enemy in slot
// (row, col), taking the current sway phase into account. Coordinates
// are in canvas pixels.
func (f *formation) slotPos(row, col int) vec2 {
	return vec2{
		x: f.originX + float64(col*slotPitchX) + f.swayOffset(),
		y: f.originY + float64(row*slotPitchY),
	}
}

// slotIsOccupied reports whether the formation has an enemy slotted at
// (row, col). All slots are occupied except row 0 columns 0, 1, 6, 7
// which are kept empty so the Boss row reads as four enemies clustered
// in the centre.
func slotIsOccupied(row, col int) bool {
	if row < 0 || row >= formationRows || col < 0 || col >= formationCols {
		return false
	}
	if row == 0 {
		return col >= 2 && col <= 5
	}
	return true
}

// kindForSlot returns the enemy kind that lives in formation slot
// (row, col). Slot kinds are fixed for the entire game.
func kindForSlot(row, col int) enemyKind {
	switch row {
	case 0:
		return enemyBoss
	case 1, 2:
		return enemyButterfly
	default:
		return enemyBee
	}
}

// formationWidthPx returns the pixel width occupied by the formation
// (rightmost slot's right edge minus leftmost slot's left edge), used
// for centring the origin on the canvas.
func formationWidthPx(spriteW int) int {
	return (formationCols-1)*slotPitchX + spriteW
}

// totalFormationSlots returns the number of occupied slots in the grid.
// For the 4-boss-row layout that's 4 + 8 + 8 + 8 + 8 = 36.
func totalFormationSlots() int {
	n := 0
	for r := 0; r < formationRows; r++ {
		for c := 0; c < formationCols; c++ {
			if slotIsOccupied(r, c) {
				n++
			}
		}
	}
	return n
}
