package lunarlander

import (
	"math/rand"
	"sort"
)

// padSpec describes one flat landing zone. xStart and xEnd are inclusive /
// exclusive pixel columns; y is the constant terrain height across the
// pad; mult is the score multiplier displayed alongside it.
type padSpec struct {
	xStart, xEnd int
	y            int
	mult         int
}

// width returns the pad width in pixels.
func (p padSpec) width() int { return p.xEnd - p.xStart }

// terrain is a 1-D heightmap that tells you, for every pixel column,
// which y the surface starts at (terrain occupies [height[x], h)). It
// also carries the list of landing pads carved into the heightmap so
// the play scene can score touchdowns and label pads.
type terrain struct {
	height []int
	pads   []padSpec
	w, h   int
	// horizon is the lowest y any terrain peak is allowed to climb to.
	// Used purely for clamping during generation and for the play scene
	// to know the top of the playable region above terrain.
	horizon int
}

// generateTerrain builds a fresh moonscape covering w pixels wide on a
// canvas h pixels tall. Surface generation is midpoint-displacement
// (a.k.a. the diamond-square 1-D variant) seeded from rng, followed by
// carving four flat pads of escalating multiplier into the result.
func generateTerrain(w, h int, rng *rand.Rand) *terrain {
	if w < 8 {
		w = 8
	}
	if h < 16 {
		h = 16
	}
	t := &terrain{
		height:  make([]int, w),
		w:       w,
		h:       h,
		horizon: h / 3,
	}

	// Baseline sits in the lower third so the player has plenty of sky.
	baseline := h*3/4 + rng.Intn(5) - 2
	t.height[0] = baseline
	t.height[w-1] = baseline + rng.Intn(7) - 3
	if t.height[w-1] < t.horizon {
		t.height[w-1] = t.horizon
	}

	// Initial vertical displacement scales with canvas height so the
	// terrain looks similarly jagged at any sane window size.
	displacement := h / 5
	if displacement < 6 {
		displacement = 6
	}
	t.fractalFill(0, w-1, rng, displacement)

	t.clampHeights()
	t.placePads(rng)
	return t
}

// fractalFill recursively bisects [l, r], displacing each midpoint by a
// shrinking random amount. The displacement halves at every level which
// produces self-similar terrain (1/f noise-ish) — the resulting profile
// reads as believably mountainous rather than uniformly noisy.
func (t *terrain) fractalFill(l, r int, rng *rand.Rand, displacement int) {
	if r-l <= 1 {
		return
	}
	m := (l + r) / 2
	avg := (t.height[l] + t.height[r]) / 2
	jitter := 0
	if displacement > 0 {
		jitter = rng.Intn(2*displacement+1) - displacement
	}
	t.height[m] = avg + jitter

	next := displacement * 6 / 10
	if next < 1 {
		next = 1
	}
	t.fractalFill(l, m, rng, next)
	t.fractalFill(m, r, rng, next)
}

// clampHeights forces every column into a sane vertical band so peaks
// can't punch the HUD or floor through the canvas bottom.
func (t *terrain) clampHeights() {
	minY := t.horizon
	maxY := t.h - 1
	for i := range t.height {
		if t.height[i] < minY {
			t.height[i] = minY
		}
		if t.height[i] > maxY {
			t.height[i] = maxY
		}
	}
}

// padCandidates returns the four pad templates used per mission, biggest
// first (so the algorithm places the easy 2x pad before squeezing
// smaller pads into whatever room is left).
func padCandidates() []padSpec {
	return []padSpec{
		{xEnd: 22, mult: 2}, // widest, easiest target — a bus could land here
		{xEnd: 17, mult: 3},
		{xEnd: 13, mult: 4},
		{xEnd: 10, mult: 5}, // smallest, highest reward
	}
}

// placePads carves the four landing zones into the heightmap. Each pad
// is positioned over the average local height with a guard buffer so
// pads can't touch one another (which would look like one giant pad).
func (t *terrain) placePads(rng *rand.Rand) {
	const sideMargin = 4
	const padGap = 8 // minimum empty columns between pads
	const maxAttempts = 80

	candidates := padCandidates()
	// Shuffle the *order in which we attempt* sizes so the 2x doesn't
	// always end up dead-centre. The candidate widths themselves stay
	// sorted-by-multiplier; we just permute placement order.
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	placed := []padSpec{}
	for _, c := range candidates {
		width := c.xEnd
		if width >= t.w-2*sideMargin {
			continue
		}
		for attempt := 0; attempt < maxAttempts; attempt++ {
			start := sideMargin + rng.Intn(t.w-width-2*sideMargin)
			end := start + width
			if overlapsExisting(start, end, padGap, placed) {
				continue
			}
			// Flatten the strip to the local average height — picking the
			// minimum would make every pad sit on a plateau, the average
			// blends each pad into its surrounding terrain.
			sum := 0
			for i := start; i < end; i++ {
				sum += t.height[i]
			}
			avg := sum / width
			if avg < t.horizon {
				avg = t.horizon
			}
			for i := start; i < end; i++ {
				t.height[i] = avg
			}
			placed = append(placed, padSpec{
				xStart: start,
				xEnd:   end,
				y:      avg,
				mult:   c.mult,
			})
			break
		}
	}

	sort.Slice(placed, func(i, j int) bool {
		return placed[i].xStart < placed[j].xStart
	})
	t.pads = placed
}

// overlapsExisting reports whether [start, end) is too close to any
// previously-placed pad.
func overlapsExisting(start, end, gap int, placed []padSpec) bool {
	for _, p := range placed {
		if start < p.xEnd+gap && end > p.xStart-gap {
			return true
		}
	}
	return false
}

// padAtRange returns the pad fully covering [x0, x1), or nil if the
// range straddles a slope or two pads. Used to decide whether both
// landing feet are sharing a single pad.
func (t *terrain) padAtRange(x0, x1 int) *padSpec {
	if x0 < 0 || x1 > t.w || x1 <= x0 {
		return nil
	}
	for i := range t.pads {
		p := &t.pads[i]
		if x0 >= p.xStart && x1 <= p.xEnd {
			return p
		}
	}
	return nil
}

// heightAt returns the terrain height at integer column x, clamped to
// the canvas. Used by collision tests against arbitrary lander points.
func (t *terrain) heightAt(x int) int {
	if x < 0 {
		x = 0
	}
	if x >= t.w {
		x = t.w - 1
	}
	return t.height[x]
}
