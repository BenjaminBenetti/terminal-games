package defender

import (
	"math"
	"math/rand"
)

// humanoidState is the per-human state machine. Humans wander on the
// ground, can be grabbed and lifted by Landers, can be set free by
// the player shooting the lifter (entering freefall), and can be
// caught by the player and gently carried back to the surface for a
// big bonus.
type humanoidState int

const (
	humanWalking  humanoidState = iota // strolling on the surface
	humanLifted                        // being carried upward by a lander
	humanFalling                       // dropped — falling toward the ground
	humanCarried                       // being held by the player
	humanLanded                        // just landed safely (briefly held in pose)
	humanDead                          // killed (off-board)
)

// humanoid is one of the 10 humans the player is defending. They walk
// idly until something happens to them; everything interesting is
// driven by their state field.
type humanoid struct {
	worldX  float64
	y       float64
	state   humanoidState
	dirX    float64 // walk direction (-1 or +1), changed at random
	walkT   float64 // time since last direction change
	carrier *enemy  // lander currently lifting this human, when state == humanLifted
	fallV   float64 // current downward speed when falling
	dead    bool    // shortcut for ==humanDead, used by some collision paths
	rescued bool    // set true once a successful catch-and-drop is awarded a bonus
	bonusT  float64 // time-remaining for the "+500" flash above the human after rescue
}

// spawnHumans seeds the planet with `count` humans at random world-x
// positions on the terrain surface. They walk left/right idly.
func spawnHumans(w *world, count int, rng *rand.Rand) []*humanoid {
	humans := make([]*humanoid, count)
	for i := 0; i < count; i++ {
		x := rng.Float64() * float64(w.worldW)
		y := w.terrainAt(x) - float64(humanoidSprite.height())
		humans[i] = &humanoid{
			worldX: x,
			y:      y,
			state:  humanWalking,
			dirX:   pickDir(rng),
		}
	}
	return humans
}

func pickDir(rng *rand.Rand) float64 {
	if rng.Intn(2) == 0 {
		return -1
	}
	return 1
}

// updateHumanoids advances every human's state for `dt` seconds.
// `dt` is passed in seconds (so callers don't have to convert).
func (p *playScene) updateHumanoids(dt float64) {
	for _, h := range p.humans {
		if h.dead {
			continue
		}
		switch h.state {
		case humanWalking:
			h.walkT += dt
			// Re-roll direction every couple seconds, with some
			// probability of pausing (dirX = 0).
			if h.walkT > 1.5+p.rng.Float64()*1.5 {
				h.walkT = 0
				r := p.rng.Float64()
				switch {
				case r < 0.35:
					h.dirX = -1
				case r < 0.70:
					h.dirX = 1
				default:
					h.dirX = 0
				}
			}
			h.worldX = p.world.wrapX(h.worldX + h.dirX*humanWalkSpeed*dt)
			// Track terrain — humans walk on the surface, not floating.
			h.y = p.world.terrainAt(h.worldX) - float64(humanoidSprite.height())
		case humanLifted:
			// Position is driven by the carrier lander in updateEnemies.
			if h.carrier == nil || !h.carrier.alive() {
				// Carrier killed by something else (most often a player
				// shot). Drop into freefall from current y.
				h.state = humanFalling
				h.carrier = nil
				h.fallV = 0
			}
		case humanFalling:
			h.fallV += humanFallAccel * dt
			if h.fallV > humanFallMax {
				h.fallV = humanFallMax
			}
			h.y += h.fallV * dt
			ground := p.world.terrainAt(h.worldX) - float64(humanoidSprite.height())
			if h.y >= ground {
				// Touched ground. If they were falling from "too high"
				// (≥ killFallHeight), they die. Otherwise they survive
				// and resume walking — and the player gets a bonus if
				// they hadn't already been credited via mid-air catch.
				h.y = ground
				if h.fallV >= humanFatalFallSpeed {
					p.killHuman(h)
				} else {
					h.state = humanWalking
					h.dirX = pickDir(p.rng)
					h.fallV = 0
					if !h.rescued {
						p.score += humanLandBonus
						h.rescued = true
						h.bonusT = 1.0
					}
				}
			}
		case humanCarried:
			// Position is driven by the player in updatePlayer.
		case humanLanded:
			// Brief celebratory pose; transition handled in updatePlayer
			// when the carry-and-set-down completes.
		}
		if h.bonusT > 0 {
			h.bonusT -= dt
		}
	}
}

// killHuman marks the human dead. If the last human just died (and we
// hadn't already triggered planet-explode), planet explosion fires.
func (p *playScene) killHuman(h *humanoid) {
	if h.dead {
		return
	}
	h.dead = true
	h.state = humanDead
	if h.carrier != nil {
		h.carrier.carrying = nil
		h.carrier = nil
	}
	// Has every human been wiped out?
	for _, x := range p.humans {
		if !x.dead {
			return
		}
	}
	p.beginPlanetExplosion()
}

// pickClosestHumanForLander finds the nearest still-walking human
// within a reasonable abduction radius. Returns nil if none qualify.
func (p *playScene) pickClosestHumanForLander(landerX float64) *humanoid {
	best := -1
	bestDist := math.MaxFloat64
	for i, h := range p.humans {
		if h.dead || h.state != humanWalking {
			continue
		}
		d := math.Abs(p.world.wrapDelta(landerX, h.worldX))
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	if best < 0 {
		return nil
	}
	return p.humans[best]
}

// Tuning. These are exported as package-level constants because they're
// shared with the player carry logic in player.go.
const (
	humanWalkSpeed       = 5.0  // px/s — slow shuffle along the surface
	humanFallAccel       = 50.0 // px/s² — gravity for freefall
	humanFallMax         = 80.0 // px/s — terminal velocity
	humanFatalFallSpeed  = 65.0 // px/s — anything faster than this kills on impact
	humanLandBonus       = 250  // points for a survived freefall (or carried delivery)
	humanRescueDeliver   = 500  // points for delivering a carried human to the ground
)
