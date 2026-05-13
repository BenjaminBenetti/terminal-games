package galaga

// A stage in classic Galaga is a sequence of "waves" — small flights of
// enemies that share an entry path and travel into the formation in a
// staggered single-file line. There are five waves per stage:
//   1. 8 Bees enter from upper-left, loop, fill row 4 (bottom).
//   2. 8 Bees enter from upper-right, loop, fill row 3.
//   3. 8 Butterflies enter from upper-left, loop, fill row 2.
//   4. 8 Butterflies enter from upper-right, loop, fill row 1.
//   5. 4 Bosses enter from top centre, fill row 0.
//
// Each wave's enemies are spawned in sequence with a small per-enemy
// delay so they fly nose-to-tail along the same Bezier curve before
// each peels off into its own formation slot.

// entryPathKind selects which path builder is used for a wave.
type entryPathKind int

const (
	pathTopLoopLeft entryPathKind = iota
	pathTopLoopRight
	pathBottomSweepLeft
	pathBottomSweepRight
	pathTopDirect
)

// waveDef is the static description of a single wave: which slot list
// to fill, in what order, along which entry path, with what spacing.
type waveDef struct {
	pathKind entryPathKind
	slots    []slotRC // formation slots to populate, in entry order
	spacing  float64  // seconds between successive enemy spawns
	startT   float64  // seconds into the stage at which this wave begins
	speed    float64  // path speed in pixels/second
}

// slotRC is a (row, col) tuple for formation slot addressing.
type slotRC struct{ row, col int }

// stagePlan is the ordered set of waves that build a single stage's
// formation. The same plan is reused for every stage in this build —
// difficulty escalates via dive frequency and bomb speed, not wave order.
func stagePlan() []waveDef {
	// Bees: row 4 fills left-to-right via upper-left loop; row 3 fills
	// right-to-left via upper-right loop. The asymmetric ordering keeps
	// the two streams from intersecting visually.
	beesBottom := make([]slotRC, 0, 8)
	for c := 0; c < formationCols; c++ {
		beesBottom = append(beesBottom, slotRC{row: 4, col: c})
	}
	beesUpper := make([]slotRC, 0, 8)
	for c := formationCols - 1; c >= 0; c-- {
		beesUpper = append(beesUpper, slotRC{row: 3, col: c})
	}
	// Butterflies: same pattern, two rows higher.
	butterflyBottom := make([]slotRC, 0, 8)
	for c := 0; c < formationCols; c++ {
		butterflyBottom = append(butterflyBottom, slotRC{row: 2, col: c})
	}
	butterflyTop := make([]slotRC, 0, 8)
	for c := formationCols - 1; c >= 0; c-- {
		butterflyTop = append(butterflyTop, slotRC{row: 1, col: c})
	}
	// Bosses: row 0 columns 2..5, in order.
	bosses := []slotRC{
		{row: 0, col: 2},
		{row: 0, col: 3},
		{row: 0, col: 4},
		{row: 0, col: 5},
	}
	return []waveDef{
		{pathKind: pathTopLoopLeft, slots: beesBottom, spacing: 0.35, startT: 0.0, speed: 70},
		{pathKind: pathTopLoopRight, slots: beesUpper, spacing: 0.35, startT: 2.2, speed: 70},
		{pathKind: pathBottomSweepLeft, slots: butterflyBottom, spacing: 0.35, startT: 4.6, speed: 70},
		{pathKind: pathBottomSweepRight, slots: butterflyTop, spacing: 0.35, startT: 7.0, speed: 70},
		{pathKind: pathTopDirect, slots: bosses, spacing: 0.55, startT: 9.4, speed: 60},
	}
}

// stageDuration returns how long it takes for the entry script to be
// fully scheduled. This is the latest start of any enemy spawn — the
// engine still has to fly them in after that, but we use it to decide
// when to enable diving attacks.
func stageDuration(plan []waveDef) float64 {
	var maxT float64
	for _, w := range plan {
		// Approximate the last enemy's "started flying" time.
		t := w.startT + float64(len(w.slots)-1)*w.spacing
		if t > maxT {
			maxT = t
		}
	}
	return maxT
}
