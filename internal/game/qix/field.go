package qix

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// cellState is the per-cell tag in the playfield grid. There are four
// distinct values rather than two so:
//
//   - cellOpen        — Qix territory, free for the player to dive into.
//     Sparx and the player cannot stand on these.
//   - cellClaimed     — already won by the player. Solid, impassable to
//     every entity; rendered as a filled block.
//   - cellBorder      — the boundary the marker (and Sparx) walks on.
//     A border cell has at least one cellOpen 4-neighbour, so the
//     border is exactly the outline of the unclaimed region.
//   - cellDraw        — part of the player's currently-in-progress
//     polyline. The Qix dies on contact with these (kills the player).
//     Sparx ignore cellDraw — they only walk cellBorder.
type cellState uint8

const (
	cellOpen cellState = iota
	cellClaimed
	cellBorder
	cellDraw
)

// point is a discrete grid cell. Used everywhere — trails, flood-fill
// results, sparx coordinates.
type point struct {
	x, y int
}

// field is the playfield grid in cell coords. Pixel coords are simply
// the same numbers — one grid cell maps to one canvas pixel. Origin
// (0, 0) is the top-left of the playfield, not of the canvas; the
// playScene applies a per-frame pixel offset when drawing.
type field struct {
	w, h  int
	cells []cellState

	// claimColor is the fill colour for cellClaimed cells; changes per
	// level for a bit of visual variety.
	claimColor engine.Color
	// borderColor / drawFastColor / drawSlowColor are the colours used
	// when rendering the corresponding cell types.
	borderColor   engine.Color
	drawFastColor engine.Color
	drawSlowColor engine.Color

	// openCount is the cached count of cellOpen cells. Tracked
	// incrementally so percent-claimed is O(1).
	openCount int
	// totalCells is w*h minus the initial border, i.e. the original
	// open-area size at level start. Percent-claimed is computed
	// against this.
	totalCells int
}

// newField builds a rectangular playfield with a one-cell-thick border
// outline and the interior filled with cellOpen.
func newField(w, h int) *field {
	f := &field{
		w:             w,
		h:             h,
		cells:         make([]cellState, w*h),
		claimColor:    engine.Color{R: 30, G: 60, B: 160, A: 255},
		borderColor:   engine.White,
		drawFastColor: engine.Color{R: 240, G: 230, B: 110, A: 255},
		drawSlowColor: engine.Color{R: 240, G: 100, B: 100, A: 255},
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x == 0 || x == w-1 || y == 0 || y == h-1 {
				f.cells[f.idx(x, y)] = cellBorder
			} else {
				f.cells[f.idx(x, y)] = cellOpen
				f.openCount++
			}
		}
	}
	f.totalCells = f.openCount
	return f
}

func (f *field) idx(x, y int) int { return y*f.w + x }

func (f *field) inBounds(x, y int) bool {
	return x >= 0 && x < f.w && y >= 0 && y < f.h
}

// at returns the cell state at (x, y). Out-of-bounds reads as
// cellClaimed — treating it as a wall keeps every "can I move here"
// check tidy without an explicit bounds branch at every callsite.
func (f *field) at(x, y int) cellState {
	if !f.inBounds(x, y) {
		return cellClaimed
	}
	return f.cells[f.idx(x, y)]
}

// set updates the cell at (x, y) and keeps openCount in sync. No-op
// on out-of-bounds.
func (f *field) set(x, y int, s cellState) {
	if !f.inBounds(x, y) {
		return
	}
	i := f.idx(x, y)
	prev := f.cells[i]
	if prev == s {
		return
	}
	if prev == cellOpen {
		f.openCount--
	}
	if s == cellOpen {
		f.openCount++
	}
	f.cells[i] = s
}

func (f *field) isOpen(x, y int) bool   { return f.at(x, y) == cellOpen }
func (f *field) isBorder(x, y int) bool { return f.at(x, y) == cellBorder }
func (f *field) isDraw(x, y int) bool   { return f.at(x, y) == cellDraw }

// percentClaimed returns the percentage of the original open area that
// has been claimed, rounded down. 0–100.
func (f *field) percentClaimed() int {
	if f.totalCells <= 0 {
		return 0
	}
	claimed := f.totalCells - f.openCount
	return claimed * 100 / f.totalCells
}

// neighbors4 yields the 4-connected neighbours of (x, y). Out-of-bounds
// neighbours are still returned — callers handle the bound check via
// field.at, which already collapses OOB to cellClaimed.
var neighborOffsets = [4]point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// commitDraw converts every cellDraw on the field into cellBorder. Used
// when a player completes a polyline back to the existing border.
//
// trail is the ordered list of cells the player walked, from the
// initial border step through to the cell that re-touched the border.
// The two endpoints are already cellBorder and stay that way; every
// interior cell flips draw → border.
func (f *field) commitDraw(trail []point) {
	for _, p := range trail {
		if f.at(p.x, p.y) == cellDraw {
			f.set(p.x, p.y, cellBorder)
		}
	}
}

// cancelDraw reverts every cellDraw in trail to cellOpen. Used when
// the player dies mid-line.
func (f *field) cancelDraw(trail []point) {
	for _, p := range trail {
		if f.at(p.x, p.y) == cellDraw {
			f.set(p.x, p.y, cellOpen)
		}
	}
}

// allOpenRegions enumerates every connected component of cellOpen
// cells in the field. The result is a slice of regions; each region
// is a slice of cells.
func (f *field) allOpenRegions() [][]point {
	visited := make([]bool, len(f.cells))
	var regions [][]point
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			i := f.idx(x, y)
			if visited[i] || f.cells[i] != cellOpen {
				continue
			}
			// BFS, marking as we go so the outer loop skips them.
			queue := []point{{x, y}}
			visited[i] = true
			var region []point
			for len(queue) > 0 {
				c := queue[0]
				queue = queue[1:]
				region = append(region, c)
				for _, off := range neighborOffsets {
					nx, ny := c.x+off.x, c.y+off.y
					if !f.inBounds(nx, ny) {
						continue
					}
					j := f.idx(nx, ny)
					if visited[j] || f.cells[j] != cellOpen {
						continue
					}
					visited[j] = true
					queue = append(queue, point{nx, ny})
				}
			}
			regions = append(regions, region)
		}
	}
	return regions
}

// claim flips every cellOpen in region to cellClaimed and returns the
// number of cells flipped.
func (f *field) claim(region []point) int {
	n := 0
	for _, p := range region {
		if f.isOpen(p.x, p.y) {
			f.set(p.x, p.y, cellClaimed)
			n++
		}
	}
	return n
}

// resolveClaim is the post-draw bookkeeping the player invokes once a
// polyline completes back to the existing border.
//
// 1. The trail's draw cells become border.
// 2. Every remaining open region is identified.
// 3. Every region that does NOT contain any of qixPositions is claimed.
//
// Returns the total number of cells claimed across all such regions.
// If no claim happened (zero or one region, or every region contained
// the Qix) the trail still becomes border but no claim count accrues.
//
// qixPositions is a small set of probe points — one per joint of the
// Qix monster. Because the Qix's joints are connected by lines that
// can't cross a border, every joint should live in the same region,
// but we sample all of them for robustness.
func (f *field) resolveClaim(trail []point, qixPositions []point) int {
	f.commitDraw(trail)

	regions := f.allOpenRegions()
	if len(regions) <= 1 {
		// No split — degenerate trail (e.g. straight out, hit nothing
		// to enclose). Trail is already committed to border; nothing
		// more to do.
		return 0
	}
	claimed := 0
	for _, region := range regions {
		if regionContainsAny(region, qixPositions) {
			continue
		}
		claimed += f.claim(region)
	}
	return claimed
}

// regionContainsAny is a linear scan that asks "is any of probes
// inside region?". Region sizes are bounded by the playfield area and
// probe counts by the Qix's joint count (≈8), so the O(|region|*|probes|)
// cost is comfortably small and avoids materialising another set.
func regionContainsAny(region []point, probes []point) bool {
	if len(probes) == 0 {
		return false
	}
	set := make(map[point]struct{}, len(region))
	for _, p := range region {
		set[p] = struct{}{}
	}
	for _, p := range probes {
		if _, ok := set[p]; ok {
			return true
		}
	}
	return false
}

