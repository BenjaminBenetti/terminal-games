package donkeykong

// Layout of the iconic "barrel" stage:
//
//     ┌─ HUD (top of screen) ──────────────────────────┐
//     │ Pauline on a small pedestal, top-right         │
//     │ DK on his platform, top-left                   │
//     ├────────────────────────────────────────────────┤
//     │ Girder 0  (DK platform, flat, full width)      │
//     │ Girder 1  (slanted, barrels roll LEFT)         │
//     │ Girder 2  (slanted, barrels roll RIGHT)        │
//     │ Girder 3  (slanted, barrels roll LEFT)         │
//     │ Girder 4  (Mario start, flat, full width)      │
//     │     [oil drum + flame on the left]             │
//     └────────────────────────────────────────────────┘
//
// Slopes are 1 pixel of rise/fall across the full canvas width, alternating
// direction. Spacing between adjacent girders is uniform; the layout helper
// picks the largest spacing that fits the canvas while leaving room above
// for DK and below for the oil drum.

const (
	// Roll direction encoded on each girder. Barrels on a girder always move
	// in this direction. A flat girder gets dirNone and barrels keep their
	// current sign (or default to +1 if they have none).
	dirLeft  = -1
	dirNone  = 0
	dirRight = +1

	ladderWidth = 3 // two rails + 1 px gap between
)

// girder is a single horizontal platform spanning the canvas. The slope is
// expressed as the y at the left and right edges (in canvas pixel rows).
type girder struct {
	leftX, rightX int
	leftY, rightY int
	rollDir       int // direction a barrel rolls on this girder
}

// yAt returns the canvas pixel row of the top of this girder at column x.
// The girder is treated as a 1-pixel-thick line interpolated linearly
// between (leftX, leftY) and (rightX, rightY); the result is clipped to
// the girder's x range and rounded to the nearest pixel row (with correct
// rounding for negative slopes).
func (g girder) yAt(x int) int {
	if x <= g.leftX {
		return g.leftY
	}
	if x >= g.rightX {
		return g.rightY
	}
	span := g.rightX - g.leftX
	if span == 0 {
		return g.leftY
	}
	delta := g.rightY - g.leftY
	num := delta * (x - g.leftX)
	// Round to nearest — bias toward 0 for both signs.
	var off int
	if num >= 0 {
		off = (num + span/2) / span
	} else {
		off = (num - span/2) / span
	}
	return g.leftY + off
}

// contains reports whether x lies within the girder's x range.
func (g girder) contains(x int) bool {
	return x >= g.leftX && x <= g.rightX
}

// ladder connects two adjacent girders. The rails span [x, x+ladderWidth-1]
// in canvas columns. topY is the pixel row where the ladder meets the
// upper girder (i.e. its highest point); bottomY is where it meets the
// lower girder. broken=true shortens the climbable range — Mario can step
// onto the ladder from the lower girder, climb partway, but can never
// reach the upper girder.
type ladder struct {
	x       int
	topY    int
	bottomY int
	broken  bool
}

// climbTopY returns the highest row Mario's feet can reach when climbing
// this ladder. For an intact ladder this is the upper girder; for a broken
// ladder it's halfway up.
func (l ladder) climbTopY() int {
	if !l.broken {
		return l.topY
	}
	return l.topY + (l.bottomY-l.topY)/2
}

// containsX reports whether px is within this ladder's column range.
func (l ladder) containsX(px int) bool {
	return px >= l.x && px < l.x+ladderWidth
}

// level is the static stage geometry: girders + ladders, plus the anchor
// positions for the actors that don't move (DK, Pauline, oil drum).
type level struct {
	girders []girder
	// ladders[i] is the slice of ladders between girders[i] and girders[i+1].
	ladders [][]ladder

	width  int
	height int

	// Top-of-stage anchors.
	dkX, dkY                 int // top-left of the DK sprite
	paulineX, paulineY       int // top-left of Pauline sprite
	paulinePedestalX, paulinePedestalY, paulinePedestalW int

	// Oil drum + flame anchors.
	oilDrumX, oilDrumY int
	flameX, flameY     int

	// Mario start.
	marioStartX     int
	marioStartGIdx  int // index into girders the player begins on (bottom)

	// The girder Mario must reach to win (top of stage).
	marioGoalGIdx int
}

// buildLevel constructs the stage geometry for the given canvas dimensions.
// All offsets are derived so the game still scales to terminal sizes other
// than the canonical 80x48, within reason.
func buildLevel(canvasW, canvasH int) *level {
	lvl := &level{
		width:  canvasW,
		height: canvasH,
	}

	// HUD takes the top cell row (= 2 px). DK + Pauline live in the space
	// above the topmost girder; oil drum + a thin floor strip live below
	// the bottommost girder. The remaining vertical span is divided
	// equally among 5 girders (DK + 3 slanted + Mario flat).
	hudPx := 2
	topMargin := hudPx + 8 // DK sprite top can dip into the row below the HUD
	bottomMargin := 2

	// Pick a girder spacing that fits AND leaves enough headroom for Mario
	// + a visible 2-pixel tilt on the slanted girders. With T=2 alternating
	// slopes, the minimum gap at the convergent corner is (spacing - 2);
	// Mario is 6 px tall, so we need spacing >= 8 to clear his head.
	usableH := canvasH - topMargin - bottomMargin
	spacing := usableH / 4
	if spacing < 8 {
		spacing = 8
	}

	g0Y := topMargin
	g1Y := g0Y + spacing
	g2Y := g1Y + spacing
	g3Y := g2Y + spacing
	g4Y := g3Y + spacing

	tilt := 2
	if spacing < 9 {
		// Tight layout — fall back to a 1-pixel tilt so Mario still
		// fits in the converging corner.
		tilt = 1
	}

	// Slanted girders end short on their LOW side so barrels can fall off
	// the end and land on the HIGH side of the next girder below. This is
	// the classic zig-zag arrangement: each girder reaches one canvas edge
	// and is "cropped" on the other.
	fullL, fullR := 0, canvasW-1
	cropL, cropR := 10, canvasW-11

	mk := func(lX, rX, leftY, rightY, dir int) girder {
		return girder{leftX: lX, rightX: rX, leftY: leftY, rightY: rightY, rollDir: dir}
	}

	// Tilts: ±tilt across the girder's span, alternating direction. The
	// HIGH end of each slanted girder reaches the canvas edge; the LOW
	// end stops short to leave a fall-off point.
	lvl.girders = []girder{
		mk(fullL, fullR, g0Y, g0Y, dirNone),         // 0: DK platform, flat full width
		mk(cropL, fullR, g1Y+tilt, g1Y, dirLeft),    // 1: low-left, high-right
		mk(fullL, cropR, g2Y, g2Y+tilt, dirRight),   // 2: high-left, low-right
		mk(cropL, fullR, g3Y+tilt, g3Y, dirLeft),    // 3: low-left, high-right
		mk(fullL, fullR, g4Y, g4Y, dirNone),         // 4: Mario start, flat full width
	}

	// --- Ladders ------------------------------------------------------
	// One ladder between DK platform and G1 (intact, near right) so Mario
	// has a clear path up the final step to Pauline. Between every other
	// pair, two ladders — one intact, one broken — at staggered columns.
	// The intact ladders alternate sides so the player has to zig-zag.
	w := canvasW
	lvl.ladders = make([][]ladder, 4)

	// DK -> G1: one full ladder near the right side.
	lvl.ladders[0] = []ladder{
		mkLadder(lvl, 0, w*3/4-2, false),
	}
	// G1 -> G2: full on left, broken on right.
	lvl.ladders[1] = []ladder{
		mkLadder(lvl, 1, w/5, false),
		mkLadder(lvl, 1, w*4/5-2, true),
	}
	// G2 -> G3: broken on left, full on right.
	lvl.ladders[2] = []ladder{
		mkLadder(lvl, 2, w/6, true),
		mkLadder(lvl, 2, w*5/8, false),
	}
	// G3 -> G4: full on left and right (Mario has two routes off the start).
	lvl.ladders[3] = []ladder{
		mkLadder(lvl, 3, w/5, false),
		mkLadder(lvl, 3, w*3/4-2, false),
	}

	// --- Top-of-stage anchors ----------------------------------------
	// DK sits near the left of his platform; Pauline on a small pedestal
	// near the right. The pedestal is positioned so Pauline's head clears
	// the HUD text row at the very top of the screen.
	lvl.dkX = 4
	lvl.dkY = g0Y - dkSprite().height()
	pedW := paulineSprite.width() + 4
	lvl.paulinePedestalW = pedW
	lvl.paulinePedestalX = canvasW - pedW - 6
	// Pedestal top is just above the DK platform; Pauline's head clears
	// the HUD (which occupies pixel rows 0..hudPx-1).
	pedY := g0Y - 2
	if pedY < paulineSprite.height()+hudPx {
		pedY = paulineSprite.height() + hudPx
	}
	lvl.paulinePedestalY = pedY
	lvl.paulineX = lvl.paulinePedestalX + 2
	lvl.paulineY = lvl.paulinePedestalY - paulineSprite.height()

	// --- Oil drum + flame ---------------------------------------------
	lvl.oilDrumX = 1
	lvl.oilDrumY = g4Y - oilDrum.height() + 1
	lvl.flameX = lvl.oilDrumX + 1
	lvl.flameY = lvl.oilDrumY - flameA.height()

	// --- Mario start --------------------------------------------------
	lvl.marioStartGIdx = 4
	lvl.marioStartX = lvl.oilDrumX + oilDrum.width() + 12
	lvl.marioGoalGIdx = 0

	return lvl
}

// dkSprite picks the right idle/throw frame; the geometry helper only
// needs the height so it can position DK above the platform.
func dkSprite() colorSprite { return dkIdle }

// mkLadder builds a ladder between girders[gIdx] (upper) and
// girders[gIdx+1] (lower) at the given x. The endpoints are pinned to the
// girder y-values at that x so the ladder visually meets each girder.
func mkLadder(l *level, gIdx, x int, broken bool) ladder {
	upper := l.girders[gIdx]
	lower := l.girders[gIdx+1]
	topY := upper.yAt(x + ladderWidth/2)
	bottomY := lower.yAt(x + ladderWidth/2)
	return ladder{x: x, topY: topY, bottomY: bottomY, broken: broken}
}

