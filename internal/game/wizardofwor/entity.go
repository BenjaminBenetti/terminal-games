package wizardofwor

import "math"

// direction encodes the four cardinal moves plus a "stopped" state.
// The order matters for arcade-style tie-breaking in monster AI and is
// kept consistent with what allMoves enumerates.
type direction uint8

const (
	dirNone direction = iota
	dirUp
	dirDown
	dirLeft
	dirRight
)

// allMoves lists the four real directions for AI loops. Order chosen
// to favour up-then-side-then-down at ties, which feels close to the
// arcade's tie-breaking habits.
var allMoves = [...]direction{dirUp, dirLeft, dirDown, dirRight}

func (d direction) dx() int {
	switch d {
	case dirLeft:
		return -1
	case dirRight:
		return 1
	}
	return 0
}

func (d direction) dy() int {
	switch d {
	case dirUp:
		return -1
	case dirDown:
		return 1
	}
	return 0
}

func (d direction) opposite() direction {
	switch d {
	case dirUp:
		return dirDown
	case dirDown:
		return dirUp
	case dirLeft:
		return dirRight
	case dirRight:
		return dirLeft
	}
	return dirNone
}

// entity is the shared state for every moving body — the player and
// each monster. Position is in TILE units: (c+0.5, r+0.5) is the
// centre of cell (c, r). Movement is grid-aligned: turns only happen
// at cell centres (except an instant 180° reversal, which the arcade
// allows mid-tile).
//
// `desired` is the buffered next direction. The player's input writes
// it directly; monster AI writes it at each junction.
type entity struct {
	x, y    float64
	dir     direction
	desired direction
	speed   float64 // tiles per second
}

// tileX returns the integer column of the cell the entity's centre is in.
func (e *entity) tileX() int { return int(math.Floor(e.x)) }

// tileY returns the integer row of the cell the entity's centre is in.
func (e *entity) tileY() int { return int(math.Floor(e.y)) }

// advance moves e by speed*dt, handling the buffered turn, wall stops,
// and the side-tunnel wrap. The loop never consumes more than one tile
// of motion in a single step so we don't ever skip a decision point
// when dt is unusually large.
//
// canPass(c, r, d) reports whether the entity may step from (c, r) in
// direction d. The caller passes a closure bound to the maze so the
// same advance logic works for the player and every monster.
func (e *entity) advance(dt float64, canPass func(c, r int, d direction) bool) {
	if e.dir == dirNone && e.desired == dirNone {
		return
	}

	// Instant 180° reversal at any position. The original arcade lets a
	// monster (and the player) turn around between intersections.
	if e.desired != dirNone && e.desired == e.dir.opposite() {
		e.dir = e.desired
	}

	remaining := e.speed * dt
	const eps = 1e-7

	for remaining > eps {
		cx := float64(e.tileX()) + 0.5
		cy := float64(e.tileY()) + 0.5
		atCentre := math.Abs(e.x-cx) < eps && math.Abs(e.y-cy) < eps

		if atCentre {
			// Snap to clean up float drift.
			e.x, e.y = cx, cy

			// Try the buffered turn.
			if e.desired != dirNone && e.desired != e.dir {
				if canPass(e.tileX(), e.tileY(), e.desired) {
					e.dir = e.desired
				}
			}

			if e.dir == dirNone {
				return
			}
			if !canPass(e.tileX(), e.tileY(), e.dir) {
				e.dir = dirNone
				return
			}
		} else if e.dir == dirNone {
			return
		}

		// Distance from current position to the next tile-centre along
		// the current axis. When already at centre, the next centre is
		// one tile away in the moving direction; mid-tile it's the
		// fractional remainder.
		var distToNext float64
		switch e.dir {
		case dirLeft:
			if e.x > cx+eps {
				distToNext = e.x - cx
			} else {
				distToNext = e.x - (cx - 1)
			}
		case dirRight:
			if e.x < cx-eps {
				distToNext = cx - e.x
			} else {
				distToNext = (cx + 1) - e.x
			}
		case dirUp:
			if e.y > cy+eps {
				distToNext = e.y - cy
			} else {
				distToNext = e.y - (cy - 1)
			}
		case dirDown:
			if e.y < cy-eps {
				distToNext = cy - e.y
			} else {
				distToNext = (cy + 1) - e.y
			}
		}

		step := distToNext
		if step > remaining {
			step = remaining
		}

		switch e.dir {
		case dirLeft:
			e.x -= step
		case dirRight:
			e.x += step
		case dirUp:
			e.y -= step
		case dirDown:
			e.y += step
		}
		remaining -= step

		// Side-tunnel wrap. Once the entity walks off the playfield
		// horizontally on the tunnel row, jump it to the opposite end
		// minus a small overshoot so it emerges smoothly.
		if e.tileY() == tunnelRow {
			if e.x < -0.5 {
				e.x += mazeCols + 1
			} else if e.x > float64(mazeCols)+0.5 {
				e.x -= mazeCols + 1
			}
		}
	}
}

// atCentre reports whether the entity is currently aligned with the
// cell centre on both axes. Monster AI uses this to know when to
// re-pick its desired direction.
func (e *entity) atCentre() bool {
	const eps = 1e-3
	cx := float64(e.tileX()) + 0.5
	cy := float64(e.tileY()) + 0.5
	return math.Abs(e.x-cx) < eps && math.Abs(e.y-cy) < eps
}
