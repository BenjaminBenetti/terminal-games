package frogger

// Static playfield layout. The arcade Frogger plays on a fixed grid:
// five road lanes between two safe strips, a median strip, then five
// river lanes ending in five home slots. We mirror that exactly. The
// playfield is rendered at a fixed 80×48 pixel size and centred on the
// canvas; everything in this file is expressed in playfield-local
// coordinates (origin = top-left of the playfield).
//
//   +----------------------------------------+  y = 0
//   |  HUD  (score / hi / level)             |  4
//   +----------------------------------------+
//   |  homes   [ ]  [ ]  [ ]  [ ]  [ ]       |  9   (5 px, top hedge)
//   +----------------------------------------+
//   |  river   turtles  ←                    |  12  (3 px)
//   |          long log →                    |  15
//   |          turtles  ←                    |  18
//   |          med log + lady →              |  21
//   |          short log →                   |  24
//   +----------------------------------------+
//   |  median (purple safe strip)            |  27  (3 px)
//   +----------------------------------------+
//   |  road    trucks   ←                    |  30
//   |          fast cars →                   |  33
//   |          cars     ←                    |  36
//   |          dozers   →                    |  39
//   |          cars     ←                    |  42
//   +----------------------------------------+
//   |  start strip (frog spawns here)        |  46  (4 px)
//   +----------------------------------------+
//   |  time bar                              |  48
//   +----------------------------------------+

const (
	// Playfield height is fixed (lane structure depends on it).
	playfieldH = 48

	// Playfield width is computed at runtime to fit the canvas — defaults
	// up to playfieldTargetW on wide terminals, falls back to canvas
	// width on narrower ones, never below playfieldMinW.
	playfieldTargetW = 160
	playfieldMinW    = 80

	// HUD lives at the top of the playfield. 2 cell rows = 4 pixel rows.
	hudH = 4

	// Home strip (hedge + slot openings).
	homeStripY = hudH      // top of hedge
	homeStripH = 5         // 5 px tall

	// Lane height (uniform for road + river).
	laneH = 3

	// River occupies 5 lanes immediately below the home strip.
	riverY0 = homeStripY + homeStripH // y of top river lane
	riverH  = laneH * 5

	// Median strip — same height as a lane.
	medianY = riverY0 + riverH
	medianH = 3

	// Road — 5 lanes.
	roadY0 = medianY + medianH
	roadH  = laneH * 5

	// Start strip + time bar at the bottom.
	startY = roadY0 + roadH
	startH = 4
	timeY  = startY + startH // 2 px tall, rest of the playfield

	// Frog grid: hops in steps of colStep horizontally, snaps vertically
	// to a fixed row centre. There are 14 row centres: home (one virtual
	// row at the top), 5 river lanes, median, 5 road lanes, and start.
	colStep = 6 // pixels per horizontal hop

	// Frog sprite is 5x3 with a 1-px transparent gutter on right side.
	frogW = 5
	frogH = 3

	// Home slots.
	numHomes = 5
	homeSlotW = 8 // pixel width of each opening
)

// laneKind classifies a lane for frog-collision purposes.
type laneKind int

const (
	laneStart   laneKind = iota // safe (frog spawns here)
	laneRoad                    // vehicles — collision = death
	laneMedian                  // safe
	laneRiverWater              // river open water — drown if not on a rider
	laneHomeRow                 // top hedge (homes embedded)
)

// laneSpec describes one lane: where it sits, what entities populate
// it, how they move, what colour the lane surface is. Vehicle and log
// spawn data lives here so playScene can rebuild the lane state for
// each new wave.
type laneSpec struct {
	yTop int      // pixel-top of the lane (3 px tall by convention)
	kind laneKind
	// Direction: +1 = left→right, -1 = right→left.
	dir int
	// Pixel-per-second speed of all entities in this lane (positive).
	speed float64

	// Vehicle / log / turtle population for this lane. Mutually
	// exclusive — only the field matching kind is meaningful.
	// "Slot" length is the on-lane footprint (sprite + trailing gap).
	entityW int    // visible width of one entity (px)
	entitySpan int // entityW + gap between adjacent entities (px)
	count    int   // number of entities running in this lane

	// Vehicle sprite picker (road lanes only). For variety the first
	// car is sprite[0], second sprite[1%len], etc.
	carSprites []colorSprite

	// River-lane variants.
	isLog    bool // entity is a log
	isTurtle bool // entity is a row of turtles
	// Diving turtles: per-cycle dive on the LANE, not per turtle. All
	// turtles in this lane dive together (matching the arcade). 0 = no
	// diving for this lane.
	diveCycle float64 // total seconds of one full surface→dive→surface cycle
}

// frogRow is one of the 14 vertical hop positions, indexed 0 (home) to
// 13 (start). centreY returns the pixel row the frog's top-left sits on
// when at that row.
type frogRow int

const (
	rowHome frogRow = iota
	rowRiverT1
	rowRiverL1
	rowRiverT2
	rowRiverL2
	rowRiverS1
	rowMedian
	rowRoad5
	rowRoad4
	rowRoad3
	rowRoad2
	rowRoad1
	rowStart
	numFrogRows
)

// rowCenterY returns the pixel row where the frog sprite's TOP edge sits
// when occupying r. Frog is 3 px tall, so it visually centres on
// (yTop + 1) for a 3-px-tall lane.
func rowCenterY(r frogRow) int {
	switch r {
	case rowHome:
		return homeStripY + 1
	case rowRiverT1:
		return riverY0 + 0*laneH
	case rowRiverL1:
		return riverY0 + 1*laneH
	case rowRiverT2:
		return riverY0 + 2*laneH
	case rowRiverL2:
		return riverY0 + 3*laneH
	case rowRiverS1:
		return riverY0 + 4*laneH
	case rowMedian:
		return medianY
	case rowRoad5:
		return roadY0 + 0*laneH
	case rowRoad4:
		return roadY0 + 1*laneH
	case rowRoad3:
		return roadY0 + 2*laneH
	case rowRoad2:
		return roadY0 + 3*laneH
	case rowRoad1:
		return roadY0 + 4*laneH
	case rowStart:
		return startY
	}
	return startY
}

// rowKind classifies a row for collision logic.
func rowKind(r frogRow) laneKind {
	switch r {
	case rowHome:
		return laneHomeRow
	case rowMedian, rowStart:
		return laneMedian
	case rowRoad1, rowRoad2, rowRoad3, rowRoad4, rowRoad5:
		return laneRoad
	case rowRiverS1, rowRiverL1, rowRiverL2, rowRiverT1, rowRiverT2:
		return laneRiverWater
	}
	return laneStart
}

// laneSpecForRow returns the moving-lane index for r, or -1 if the row
// is a safe strip (home, median, start). The index addresses
// playScene.lanes, which holds all populated lanes (road + river)
// in row order.
func laneSpecForRow(r frogRow) int {
	switch r {
	case rowRiverT1:
		return 0
	case rowRiverL1:
		return 1
	case rowRiverT2:
		return 2
	case rowRiverL2:
		return 3
	case rowRiverS1:
		return 4
	case rowRoad5:
		return 5
	case rowRoad4:
		return 6
	case rowRoad3:
		return 7
	case rowRoad2:
		return 8
	case rowRoad1:
		return 9
	}
	return -1
}

// computePlayfieldW picks the playfield width to use for a given canvas
// width. The playfield always uses at least playfieldMinW so lane
// entities fit; caps at playfieldTargetW so absurdly wide terminals
// don't produce hop-counts that no one would want to traverse.
func computePlayfieldW(canvasW int) int {
	w := canvasW
	if w > playfieldTargetW {
		w = playfieldTargetW
	}
	if w < playfieldMinW {
		w = playfieldMinW
	}
	return w
}

// requiredCount returns the minimum lane entity count needed for the
// cycle period (count*entitySpan) to be at least playfieldW + entityW —
// the constraint that prevents a wrapping entity from being visible at
// both ends of the playfield at the same instant.
func requiredCount(playfieldW, entityW, entitySpan int) int {
	need := playfieldW + entityW
	c := (need + entitySpan - 1) / entitySpan // ceil division
	if c < 2 {
		c = 2
	}
	return c
}

// buildLaneSpecs builds the canonical lane configuration. Speeds match
// the arcade feel — the road has weave-able gaps, the river is denser
// at the bottom and sparser at the top. Counts scale with playfieldW
// so a wider playfield gets proportionally more entities per lane
// (i.e. lane density stays roughly constant rather than entities
// getting stretched thin).
func buildLaneSpecs(playfieldW int, waveScale float64) []laneSpec {
	// All entity dimensions and spacings are in pixels. Spans (= entity
	// width + gap) are chosen so the right number of copies fit around
	// the playfield with rhythm-readable timing.
	specs := []laneSpec{
		// --- River (top → bottom) -----------------------------------
		// rowRiverT1: turtle lane, groups of 3, moving LEFT, divers.
		{
			yTop: riverY0 + 0*laneH, kind: laneRiverWater, dir: -1,
			speed:      9.0 * waveScale,
			entityW:    17, // 3 turtles × 5 + 2 px gaps
			entitySpan: 33,
			isTurtle:   true,
			diveCycle:  10.0,
		},
		// rowRiverL1: long log lane (24 px) moving RIGHT.
		{
			yTop: riverY0 + 1*laneH, kind: laneRiverWater, dir: +1,
			speed:      8.0 * waveScale,
			entityW:    24,
			entitySpan: 54,
			isLog:      true,
		},
		// rowRiverT2: turtle lane, groups of 2 turtles, moving LEFT.
		{
			yTop: riverY0 + 2*laneH, kind: laneRiverWater, dir: -1,
			speed:      11.0 * waveScale,
			entityW:    11, // 2 turtles × 5 + 1 px gap
			entitySpan: 32,
			isTurtle:   true,
			diveCycle:  8.0,
		},
		// rowRiverL2: medium log lane (18 px) — lady frog rides here.
		{
			yTop: riverY0 + 3*laneH, kind: laneRiverWater, dir: +1,
			speed:      10.0 * waveScale,
			entityW:    18,
			entitySpan: 50,
			isLog:      true,
		},
		// rowRiverS1: short log lane (12 px) moving RIGHT, fast.
		{
			yTop: riverY0 + 4*laneH, kind: laneRiverWater, dir: +1,
			speed:      13.0 * waveScale,
			entityW:    12,
			entitySpan: 32,
			isLog:      true,
		},

		// --- Road (top → bottom, closest-to-median first) -----------
		// rowRoad5: slow cars moving LEFT.
		{
			yTop: roadY0 + 0*laneH, kind: laneRoad, dir: -1,
			speed:      7.0 * waveScale,
			entityW:    6,
			entitySpan: 30,
			carSprites: []colorSprite{carPurpleSpr, carRedSpr},
		},
		// rowRoad4: bulldozers moving RIGHT, medium speed.
		{
			yTop: roadY0 + 1*laneH, kind: laneRoad, dir: +1,
			speed:      9.0 * waveScale,
			entityW:    6,
			entitySpan: 30,
			carSprites: []colorSprite{dozerSprite},
		},
		// rowRoad3: cars moving LEFT.
		{
			yTop: roadY0 + 2*laneH, kind: laneRoad, dir: -1,
			speed:      11.0 * waveScale,
			entityW:    6,
			entitySpan: 32,
			carSprites: []colorSprite{carPinkSpr, carCyanSpr},
		},
		// rowRoad2: fast cars moving RIGHT (the "speed lane").
		{
			yTop: roadY0 + 3*laneH, kind: laneRoad, dir: +1,
			speed:      16.0 * waveScale,
			entityW:    6,
			entitySpan: 24,
			carSprites: []colorSprite{carYellowSpr},
		},
		// rowRoad1: trucks moving LEFT, slow.
		{
			yTop: roadY0 + 4*laneH, kind: laneRoad, dir: -1,
			speed:      6.0 * waveScale,
			entityW:    13,
			entitySpan: 50,
			carSprites: []colorSprite{truckSprite},
		},
	}
	// Count per lane is derived from the playfield width so density
	// stays constant — wider playfield = more entities per lane, not
	// the same entities stretched thin.
	for i := range specs {
		specs[i].count = requiredCount(playfieldW, specs[i].entityW, specs[i].entitySpan)
	}
	return specs
}

// homeSlotX returns the x range [x0, x1) of the i-th home slot
// (0 ≤ i < numHomes). Slots are evenly distributed across the playfield
// with a hedge-coloured divider between every pair.
func homeSlotX(i, playfieldW int) (int, int) {
	totalSlot := numHomes * homeSlotW
	totalDiv := playfieldW - totalSlot
	div := totalDiv / (numHomes + 1)
	x0 := div + i*(homeSlotW+div)
	return x0, x0 + homeSlotW
}

