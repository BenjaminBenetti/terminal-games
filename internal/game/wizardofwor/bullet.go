package wizardofwor

import "math"

// bullet is a single projectile travelling in one of the four
// directions at constant speed (in tile units / sec). One bullet is
// allowed per shooter at a time; the play loop enforces that.
//
// Shooter identifies which entity fired the bullet so it can't kill
// itself, and so we can score correctly when it hits.
type shooterID int

const (
	shooterNone   shooterID = iota
	shooterPlayer
	shooterMonster
	shooterWizard
)

type bullet struct {
	x, y    float64
	dir     direction
	speed   float64
	shooter shooterID
	// monsterIdx is the index into play.monsters of the shooter, when
	// shooter is shooterMonster. Used for collision exemption and
	// score attribution. -1 for player / wizard bullets.
	monsterIdx int
	// alive becomes false once the bullet has hit a wall or entity;
	// the play loop sweeps dead bullets out at the end of the frame.
	alive bool
	// trail is a short historical trail used for visual smear. Each
	// entry is a tile-space (x, y) snapshot kept for the last few
	// frames. Three samples is plenty for the smear and very cheap.
	trail [3]struct{ x, y float64 }
	trailN int
}

// bulletSpeedPlayer / bulletSpeedMonster are the tile-per-second
// velocities. Player shots are faster than monster shots — the player
// has the survival edge in the original because their reach is longer.
const (
	bulletSpeedPlayer  = 18.0
	bulletSpeedMonster = 11.0
	bulletSpeedWizard  = 13.0
)

// newPlayerBullet spawns a bullet at the player's position headed in
// the player's facing direction. The caller owns the returned value.
func newPlayerBullet(originX, originY float64, dir direction) bullet {
	b := bullet{
		x: originX, y: originY,
		dir:        dir,
		speed:      bulletSpeedPlayer,
		shooter:    shooterPlayer,
		monsterIdx: -1,
		alive:      true,
	}
	for i := range b.trail {
		b.trail[i] = struct{ x, y float64 }{originX, originY}
	}
	return b
}

func newMonsterBullet(originX, originY float64, dir direction, monsterIdx int) bullet {
	b := bullet{
		x: originX, y: originY,
		dir:        dir,
		speed:      bulletSpeedMonster,
		shooter:    shooterMonster,
		monsterIdx: monsterIdx,
		alive:      true,
	}
	for i := range b.trail {
		b.trail[i] = struct{ x, y float64 }{originX, originY}
	}
	return b
}

func newWizardBullet(originX, originY float64, dir direction) bullet {
	b := bullet{
		x: originX, y: originY,
		dir:        dir,
		speed:      bulletSpeedWizard,
		shooter:    shooterWizard,
		monsterIdx: -1,
		alive:      true,
	}
	for i := range b.trail {
		b.trail[i] = struct{ x, y float64 }{originX, originY}
	}
	return b
}

// advance moves the bullet by speed*dt, checking for wall crossings
// along the way. The bullet dies the instant it crosses a wall edge —
// it never overruns into the next cell. Movement is broken into
// sub-steps no larger than half a tile so the wall check stays
// reliable even at high speeds and long dt.
//
// Returns true if the bullet is still alive at the end of the call.
func (b *bullet) advance(dt float64, m *maze) bool {
	if !b.alive || b.dir == dirNone {
		return b.alive
	}

	// Snapshot the trail BEFORE moving so the smear represents the
	// last few frames' positions.
	b.trail[b.trailN%len(b.trail)] = struct{ x, y float64 }{b.x, b.y}
	b.trailN++

	remaining := b.speed * dt
	const maxStep = 0.45

	for remaining > 1e-9 && b.alive {
		step := remaining
		if step > maxStep {
			step = maxStep
		}

		oldC := int(math.Floor(b.x))
		oldR := int(math.Floor(b.y))

		switch b.dir {
		case dirLeft:
			b.x -= step
		case dirRight:
			b.x += step
		case dirUp:
			b.y -= step
		case dirDown:
			b.y += step
		}

		// Tunnel-row wrap so player and monster bullets can chase the
		// Worluk through the side warps.
		if oldR == tunnelRow {
			if b.x < -0.5 {
				b.x += mazeCols + 1
			} else if b.x > float64(mazeCols)+0.5 {
				b.x -= mazeCols + 1
			}
		}

		newC := int(math.Floor(b.x))
		newR := int(math.Floor(b.y))

		// If we crossed a cell boundary, check whether a wall edge
		// stood between the two cells.
		if newC != oldC || newR != oldR {
			if newR == oldR {
				// Horizontal crossing.
				if newC > oldC {
					// moving right — wall to the right of oldC.
					if m.hasWallLeft(oldC+1, oldR) {
						b.alive = false
						// Pin the bullet against the wall for visual.
						b.x = float64(oldC + 1)
						return false
					}
				} else {
					if m.hasWallLeft(oldC, oldR) {
						b.alive = false
						b.x = float64(oldC)
						return false
					}
				}
			} else if newC == oldC {
				if newR > oldR {
					if m.hasWallTop(oldC, oldR+1) {
						b.alive = false
						b.y = float64(oldR + 1)
						return false
					}
				} else {
					if m.hasWallTop(oldC, oldR) {
						b.alive = false
						b.y = float64(oldR)
						return false
					}
				}
			}
		}

		// Also kill the bullet if it left the playfield off the top or
		// bottom (rare; the maze borders are walls everywhere except
		// the tunnel mouths, which we already handled by wrapping).
		if b.y < 0 || b.y > float64(mazeRows) {
			b.alive = false
			return false
		}

		remaining -= step
	}
	return b.alive
}

// hitsEntity returns true if the bullet's current position lies within
// `radius` (in tile units) of the entity's centre.
func (b *bullet) hitsEntity(ex, ey, radius float64) bool {
	dx := b.x - ex
	dy := b.y - ey
	return dx*dx+dy*dy <= radius*radius
}
