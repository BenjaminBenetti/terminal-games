package pacman

import "math"

// direction encodes the four cardinal moves plus a "stopped" state.
// The order matters: ghost AI breaks ties on direction in the order
// up, left, down, right (see ghost.go), and allMoves enumerates them
// in that same order.
type direction uint8

const (
	dirNone direction = iota
	dirUp
	dirLeft
	dirDown
	dirRight
)

// allMoves lists the four real directions in the original arcade's
// tie-break order: up, left, down, right.
var allMoves = [...]direction{dirUp, dirLeft, dirDown, dirRight}

// dx / dy returns the unit-tile delta for a direction.
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

// opposite returns the 180° reversal of d.
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

// entity is the shared state every moving body (Pac-Man and the four
// ghosts) carries. Positions are in TILE units, with (c+0.5, r+0.5)
// the centre of tile (c, r). Speed is tiles per second.
//
// Movement is grid-aligned: non-180° turns happen only at tile centres,
// a 180° reversal is permitted instantly at any position, and a
// buffered "desired" direction lets the player aim a turn slightly
// before the intersection arrives. Buffered desire persists until
// either consumed by an intersection or replaced by a new key.
type entity struct {
	x, y    float64
	dir     direction
	desired direction
	speed   float64
}

// tileX / tileY are the integer tile coordinates of the cell the
// entity's centre currently occupies.
func (e *entity) tileX() int { return int(math.Floor(e.x)) }
func (e *entity) tileY() int { return int(math.Floor(e.y)) }

// canPasser reports whether tile (col, row) is walkable for the entity
// in question. Wrapping it as a function lets one mover serve both
// Pac-Man (forbidden from the ghost house) and each ghost (allowed
// through the door only conditionally).
type canPasser func(col, row int) bool

// advance moves e by speed * dt tiles, handling tile-centre decisions,
// wall stops, and tunnel wrap. The loop alternates between two states:
//
//  1. Aligned at a tile centre — attempt the buffered turn, then check
//     whether the chosen direction leads into a wall (in which case
//     the entity stops). The distance to the *next* tile centre in the
//     chosen direction is exactly 1 tile.
//  2. Mid-tile — the entity is moving along one axis; the distance to
//     the next centre is less than 1. Walk up to that centre, then
//     return to state 1.
//
// Each loop iteration consumes at most one tile-centre worth of
// movement, so the entity can never skip past a decision point even
// when dt is large.
func (e *entity) advance(dt float64, canPass canPasser) {
	if e.dir == dirNone && e.desired == dirNone {
		return
	}

	// A 180° reversal is permitted instantly — don't make the player
	// wait for an intersection. The desired direction is *not* cleared,
	// so the same turn can still be re-considered at the next centre
	// in case the player is queueing through a corner.
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
			// Clean up float drift before any decisions.
			e.x, e.y = cx, cy

			// Try the buffered turn.
			if e.desired != dirNone && e.desired != e.dir {
				tx, ty := e.tileX()+e.desired.dx(), e.tileY()+e.desired.dy()
				if canPass(tx, ty) {
					e.dir = e.desired
				}
			}

			// If the current direction now leads into a wall, halt.
			if e.dir == dirNone {
				return
			}
			ntx, nty := e.tileX()+e.dir.dx(), e.tileY()+e.dir.dy()
			if !canPass(ntx, nty) {
				e.dir = dirNone
				return
			}
		} else if e.dir == dirNone {
			return
		}

		// Distance from the current position to the *next* tile centre
		// along the moving axis. When at a centre, the next centre is
		// a full tile away in the chosen direction. Mid-tile, it's
		// just the remaining fraction.
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
	}

	// Tunnel wrap. The ±0.5 padding lets the entity emerge from the
	// wrapped side already past the tunnel mouth, matching the
	// continuous-feeling wrap in the arcade.
	if e.tileY() == tunnelRow {
		if e.x < -0.5 {
			e.x += mazeCols + 1
		} else if e.x > float64(mazeCols)+0.5 {
			e.x -= mazeCols + 1
		}
	}
}
