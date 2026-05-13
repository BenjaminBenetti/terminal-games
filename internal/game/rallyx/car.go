package rallyx

import "math"

// direction encodes the four cardinal moves plus a "stopped" state.
// The order matches the Pac-Man package convention so AI tie-breaks
// can read up/left/down/right deterministically.
type direction uint8

const (
	dirNone direction = iota
	dirUp
	dirLeft
	dirDown
	dirRight
)

var allDirs = [...]direction{dirUp, dirLeft, dirDown, dirRight}

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

// car is the shared state every drivable entity carries: the player's
// race car and each of the four enemy chasers. Positions are in TILE
// units, with (c+0.5, r+0.5) the centre of tile (c, r). Speed is in
// tiles per second.
//
// Motion is grid-aligned: turns happen at tile centres, with the
// special exception that a 180° reversal is permitted instantly. The
// caller buffers a desired direction; the buffered desire is consumed
// when the entity arrives at the next intersection or replaced by a
// fresh input before that.
type car struct {
	x, y    float64
	dir     direction
	desired direction
	speed   float64

	// alive == false freezes the car in place — used for the player
	// during the death animation and for enemies that have been
	// destroyed (by smoke or by another wreck).
	alive bool

	// crashed is the death state for enemies after they hit a rock or
	// drove into smoke. They sit on their crashed tile until cleaned
	// up after a short delay; dead enemies don't collide with the
	// player.
	crashed  bool
	crashedT float64

	// smokeT counts down the duration this car (if an enemy) is
	// disabled by a smoke screen — during this time the enemy stops
	// moving but is not destroyed.
	smokeT float64
}

// tileX / tileY are the integer tile coordinates of the cell the car
// currently occupies.
func (c *car) tileX() int { return int(math.Floor(c.x)) }
func (c *car) tileY() int { return int(math.Floor(c.y)) }

// canPasser reports whether tile (col, row) may be entered by this
// car. Wrapping it as a function lets one mover serve both the player
// (rocks count as walls — driving into one means crashing, so motion
// stops at the rock-adjacent centre) and the AI (which also treats
// rocks as impassable so it doesn't drive into them).
type canPasser func(col, row int) bool

// advance moves c by speed * dt tiles, stopping at walls and
// snapping to tile centres at every intersection. The loop alternates
// between "at centre" and "mid-tile" states, capping each iteration
// to at most one tile of motion so the entity can't skip past an
// intersection even on a large dt.
func (c *car) advance(dt float64, canPass canPasser) {
	if !c.alive || c.crashed || c.smokeT > 0 {
		return
	}
	if c.dir == dirNone && c.desired == dirNone {
		return
	}

	// Instant 180° reversal is allowed (handles the player flicking
	// the opposite key mid-corridor). The desired direction is *not*
	// cleared so the same turn can be reconsidered at the next centre.
	if c.desired != dirNone && c.desired == c.dir.opposite() {
		c.dir = c.desired
	}

	remaining := c.speed * dt
	const eps = 1e-7

	for remaining > eps {
		cx := float64(c.tileX()) + 0.5
		cy := float64(c.tileY()) + 0.5
		atCentre := math.Abs(c.x-cx) < eps && math.Abs(c.y-cy) < eps

		if atCentre {
			c.x, c.y = cx, cy

			// Try the buffered turn first.
			if c.desired != dirNone && c.desired != c.dir {
				tx, ty := c.tileX()+c.desired.dx(), c.tileY()+c.desired.dy()
				if canPass(tx, ty) {
					c.dir = c.desired
				}
			}

			if c.dir == dirNone {
				return
			}
			ntx, nty := c.tileX()+c.dir.dx(), c.tileY()+c.dir.dy()
			if !canPass(ntx, nty) {
				c.dir = dirNone
				return
			}
		} else if c.dir == dirNone {
			return
		}

		var distToNext float64
		switch c.dir {
		case dirLeft:
			if c.x > cx+eps {
				distToNext = c.x - cx
			} else {
				distToNext = c.x - (cx - 1)
			}
		case dirRight:
			if c.x < cx-eps {
				distToNext = cx - c.x
			} else {
				distToNext = (cx + 1) - c.x
			}
		case dirUp:
			if c.y > cy+eps {
				distToNext = c.y - cy
			} else {
				distToNext = c.y - (cy - 1)
			}
		case dirDown:
			if c.y < cy-eps {
				distToNext = cy - c.y
			} else {
				distToNext = (cy + 1) - c.y
			}
		}

		step := distToNext
		if step > remaining {
			step = remaining
		}

		switch c.dir {
		case dirLeft:
			c.x -= step
		case dirRight:
			c.x += step
		case dirUp:
			c.y -= step
		case dirDown:
			c.y += step
		}
		remaining -= step
	}
}

// distSq returns the squared tile-space distance between two cars —
// used everywhere a collision check needs a cheap radius compare.
func (c *car) distSq(o *car) float64 {
	dx := c.x - o.x
	dy := c.y - o.y
	return dx*dx + dy*dy
}

