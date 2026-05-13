package centipede

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// segment is one node of a centipede chain. Each segment carries its
// own direction state so chains can split cleanly — the new head of a
// split-off tail keeps whatever direction the chain had at that point.
type segment struct {
	col, row int

	// dx is the horizontal step direction: -1 left, +1 right.
	dx int
	// dy is the vertical step direction: +1 descending, -1 ascending.
	// Only the head consults dy when it bumps an obstacle.
	dy int

	// diving and diveRows handle the "hit a poisoned mushroom" state:
	// the head plunges straight down through its column for the next
	// diveRows ticks, ignoring further mushrooms.
	diving   bool
	diveRows int

	// animT advances per real-time second and animates the body
	// frame independently of cell-step ticks.
	animT float64
	frame int

	// spawnT is the elapsed time since this segment came into being,
	// used for a brief render fade-in on freshly-split chain heads.
	spawnT float64
}

// centipedeChain is one (possibly head-less) snake of segments. After a
// split there may be many of these running on the same field at once.
type centipedeChain struct {
	segments []*segment

	// moveT accumulates dt; when it crosses moveInterval, the chain
	// takes one discrete cell-step.
	moveT        float64
	moveInterval float64
}

// newCentipede builds a fresh chain of `length` segments entering the
// field at the top, starting from the right edge and heading left.
// Each chain at level start uses the level-tuned moveInterval.
func newCentipede(length int, cols int, moveInterval float64) *centipedeChain {
	if length < 1 {
		length = 1
	}
	segs := make([]*segment, length)
	for i := 0; i < length; i++ {
		col := cols - 1 - i // tail trails to the right of the head
		if col < 0 {
			col = 0
		}
		segs[i] = &segment{
			col: col,
			row: 0,
			dx:  -1, // entering heads left
			dy:  1,  // descending
		}
	}
	return &centipedeChain{
		segments:     segs,
		moveInterval: moveInterval,
	}
}

// length returns how many segments remain in the chain.
func (cp *centipedeChain) length() int { return len(cp.segments) }

// tick advances the chain. It handles per-second animation every frame
// and triggers a cell-step whenever the move timer elapses.
func (cp *centipedeChain) tick(dt float64, f *field) {
	for _, s := range cp.segments {
		s.animT += dt
		s.spawnT += dt
		if s.animT >= 0.18 {
			s.animT -= 0.18
			s.frame = 1 - s.frame
		}
	}
	cp.moveT += dt
	for cp.moveT >= cp.moveInterval {
		cp.moveT -= cp.moveInterval
		cp.step(f)
		if len(cp.segments) == 0 {
			return
		}
	}
}

// step performs one cell-step. The head's logic is the interesting
// part; body segments simply trail-follow the segment in front using a
// pre-step snapshot.
func (cp *centipedeChain) step(f *field) {
	if len(cp.segments) == 0 {
		return
	}

	// Snapshot every segment before any move so the trail-follow uses
	// pre-step positions consistently regardless of iteration order.
	snap := make([]segment, len(cp.segments))
	for i, s := range cp.segments {
		snap[i] = *s
	}

	cp.stepHead(f, cp.segments[0])

	for i := 1; i < len(cp.segments); i++ {
		s := cp.segments[i]
		prev := snap[i-1]
		s.col = prev.col
		s.row = prev.row
		s.dx = prev.dx
		s.dy = prev.dy
		s.diving = prev.diving
		s.diveRows = prev.diveRows
	}
}

// stepHead moves the head one cell. Order of precedence:
//
//  1. If we're diving, just keep going straight down until we exit the
//     dive state (rows-budget exhausted or we reach the floor).
//  2. Otherwise, try a horizontal step. If blocked by a mushroom or the
//     side wall: drop one row in the current vertical direction (or
//     bounce vertically if we're already at the top/bottom), reverse
//     the horizontal direction, and stay put horizontally for this
//     tick. If the obstacle was a poisoned mushroom, also enter the
//     dive state so the next tick plunges.
//  3. If the horizontal step succeeded, just move there.
func (cp *centipedeChain) stepHead(f *field, h *segment) {
	if h.diving {
		h.row++
		h.diveRows--
		if h.row >= f.rows-1 {
			h.row = f.rows - 1
			h.diving = false
			h.dy = -1
		} else if h.diveRows <= 0 {
			h.diving = false
			// Resume a sensible heading: bounce off the player zone if
			// we just plunged into it.
			if h.row >= f.playerZoneTop {
				h.dy = -1
			} else {
				h.dy = 1
			}
		}
		return
	}

	nextCol := h.col + h.dx
	blocked := false
	hitPoisoned := false
	switch {
	case nextCol < 0 || nextCol >= f.cols:
		blocked = true
	case f.hasMushroom(nextCol, h.row):
		blocked = true
		if f.isPoisoned(nextCol, h.row) {
			hitPoisoned = true
		}
	}

	if !blocked {
		h.col = nextCol
		return
	}

	// Drop or rise one row, reverse direction. If we'd step out of the
	// playfield, flip dy first and step the other way.
	nextRow := h.row + h.dy
	if nextRow < 0 || nextRow >= f.rows {
		h.dy = -h.dy
		nextRow = h.row + h.dy
	}
	h.row = nextRow
	h.dx = -h.dx

	if hitPoisoned {
		// Plunge straight down through the column until we reach the
		// player zone or the floor.
		h.diving = true
		h.diveRows = f.playerZoneTop - h.row + 3
		if h.diveRows < 1 {
			h.diveRows = 1
		}
	}
}

// applyHitAt removes segment idx and splits the chain. The killed
// segment's cell should have a mushroom planted by the caller. Returns:
//
//   - splitOff: the new chain spawned from the tail (or nil if no tail
//     remained, i.e. the killed segment was the last one in the chain
//     or the killed segment was the head and there was nothing behind).
//   - wasHead: true if idx == 0 (worth 100 pts; otherwise 10).
//   - empty: true if this chain has no segments left after the cut.
//
// The new head of the tail-chain has its horizontal direction flipped
// so the two halves visibly drift apart, matching arcade behaviour.
func (cp *centipedeChain) applyHitAt(idx int) (splitOff *centipedeChain, wasHead bool, empty bool) {
	if idx < 0 || idx >= len(cp.segments) {
		return nil, false, len(cp.segments) == 0
	}
	wasHead = (idx == 0)

	var tail []*segment
	if idx+1 < len(cp.segments) {
		tail = make([]*segment, len(cp.segments)-idx-1)
		copy(tail, cp.segments[idx+1:])
	}
	cp.segments = cp.segments[:idx]
	empty = len(cp.segments) == 0

	if len(tail) == 0 {
		return nil, wasHead, empty
	}
	// Promote new head, flip its horizontal direction.
	newHead := tail[0]
	newHead.dx = -newHead.dx
	newHead.spawnT = 0
	splitOff = &centipedeChain{
		segments:     tail,
		moveInterval: cp.moveInterval,
	}
	return splitOff, wasHead, empty
}

// segmentRect returns the AABB the segment occupies in canvas pixels.
func segmentRect(f *field, s *segment) rect {
	x, y := f.cellPixel(s.col, s.row)
	return rect{x0: x, y0: y, x1: x + cellW, y1: y + cellH}
}

// draw renders all of cp's segments. The head uses its own sprite/color;
// body segments use the body sprites in alternating frames.
func (cp *centipedeChain) draw(c *engine.Canvas, f *field) {
	for i, s := range cp.segments {
		x, y := f.cellPixel(s.col, s.row)
		var spr sprite
		var col engine.Color
		if i == 0 {
			if s.frame == 0 {
				spr = centipedeHeadA
			} else {
				spr = centipedeHeadB
			}
			col = colorCentipedeHead
		} else {
			if s.frame == 0 {
				spr = centipedeBodyA
			} else {
				spr = centipedeBodyB
			}
			col = colorCentipedeBody
		}
		// Mirror the sprite horizontally when moving left, so the head
		// "faces" its direction of travel. Body segments inherit this
		// for visual consistency.
		if s.dx < 0 {
			drawSpriteMirror(c, x, y, spr, col)
		} else {
			drawSprite(c, x, y, spr, col)
		}
	}
}
