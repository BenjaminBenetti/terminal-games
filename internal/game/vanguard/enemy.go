package vanguard

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// enemyKind identifies a specific enemy archetype. Each kind has a
// fixed sprite pair, score value, default hp, and an AI behaviour
// (chosen by tickEnemy via a switch on kind).
type enemyKind int

const (
	ekKemleyL  enemyKind = iota // mountain rocket from left
	ekKemleyR                   // mountain rocket from right
	ekHelm                      // mountain hover-shooter
	ekBringer                   // mountain heavy
	ekBear                      // stripe chaser
	ekFloater                   // bleak drifter
	ekDancer                    // rainbow descender
	ekStyxTurret                // styx wall turret
)

// enemyState is the per-enemy lifecycle. esActive enemies drive their
// own AI; esDying plays the explosion; esGone is collected next tick.
type enemyState int

const (
	esActive enemyState = iota
	esDying
	esGone
)

// enemy is a single in-flight enemy. AI lives in the Update method on
// playScene (tickEnemy); this struct only carries state.
type enemy struct {
	kind    enemyKind
	x, y    float64
	vx, vy  float64
	hp      int
	state   enemyState
	frame   int
	frameT  float64
	dyingT  float64
	aiT     float64 // generic ai timer used per-kind
	fireCD  float64 // bomb cooldown
	born    float64 // stageT when spawned, for time-based animations
}

// frames returns the two animation sprites for this kind.
func (k enemyKind) frames() (a, b sprite) {
	switch k {
	case ekKemleyL, ekKemleyR:
		return kemleyA, kemleyB
	case ekHelm:
		return helmA, helmB
	case ekBringer:
		return bringerA, bringerB
	case ekBear:
		return bearA, bearB
	case ekFloater:
		return floaterA, floaterB
	case ekDancer:
		return dancerA, dancerB
	case ekStyxTurret:
		return helmA, helmB
	}
	return helmA, helmB
}

func (k enemyKind) score() int {
	switch k {
	case ekKemleyL, ekKemleyR:
		return 150
	case ekHelm:
		return 100
	case ekBringer:
		return 250
	case ekBear:
		return 200
	case ekFloater:
		return 300
	case ekDancer:
		return 200
	case ekStyxTurret:
		return 350
	}
	return 100
}

func (k enemyKind) defaultHP() int {
	switch k {
	case ekBringer, ekStyxTurret:
		return 2
	}
	return 1
}

func (k enemyKind) color() engine.Color {
	switch k {
	case ekKemleyL, ekKemleyR:
		return engine.Color{R: 240, G: 160, B: 80, A: 255}
	case ekHelm:
		return engine.Color{R: 220, G: 100, B: 240, A: 255}
	case ekBringer:
		return engine.Color{R: 240, G: 90, B: 90, A: 255}
	case ekBear:
		return engine.Color{R: 240, G: 200, B: 120, A: 255}
	case ekFloater:
		return engine.Color{R: 120, G: 240, B: 200, A: 255}
	case ekDancer:
		return engine.Color{R: 240, G: 100, B: 200, A: 255}
	case ekStyxTurret:
		return engine.Color{R: 200, G: 60, B: 200, A: 255}
	}
	return engine.White
}

// alive returns true while the enemy can still affect the simulation
// (collide, fire, score). Dying explosions don't count.
func (e *enemy) alive() bool { return e.state == esActive }

// boundingBox returns the AABB used for both bullet/enemy and player/
// enemy collisions. Shrunk slightly inside the sprite to feel forgiving.
func (e *enemy) boundingBox() rect {
	a, _ := e.kind.frames()
	w := a.width()
	h := a.height()
	return rect{
		x0: int(e.x) + 1,
		y0: int(e.y) + 1,
		x1: int(e.x) + w - 1,
		y1: int(e.y) + h - 1,
	}
}

// spawnEnemy is a small constructor that fills in animation defaults
// and sets hp from the enemy kind. Each spawn site sets x, y, and the
// initial velocity components separately.
func spawnEnemy(kind enemyKind, x, y float64, stageT float64) *enemy {
	return &enemy{
		kind:  kind,
		x:     x,
		y:     y,
		hp:    kind.defaultHP(),
		state: esActive,
		born:  stageT,
	}
}

// tickEnemy advances one enemy by `dt` seconds. It returns true if the
// enemy fired a bomb this frame (caller spawns the bomb), along with
// the bomb origin coordinates.
//
// The pattern logic is intentionally simple — each kind has its own
// stylised motion that suits the zone it appears in. The point is for
// the *mix* of behaviours within a stage to feel busy, not for any
// single enemy to be a chess problem.
func (p *playScene) tickEnemy(e *enemy, dt float64) (fired bool, fx, fy float64) {
	// Animation tick is shared by every enemy.
	e.frameT += dt
	if e.frameT >= 0.25 {
		e.frameT = 0
		e.frame = 1 - e.frame
	}
	e.aiT += dt
	e.fireCD -= dt

	switch e.kind {
	case ekKemleyL:
		// Straight-line rocket flying right; very fast.
		e.x += 60 * dt
		// Slight vertical sway.
		e.y += 6 * dt * math.Sin(e.aiT*3)
	case ekKemleyR:
		e.x -= 60 * dt
		e.y += 6 * dt * math.Sin(e.aiT*3+1.0)
	case ekHelm:
		// Hovers in place, drifts slightly toward the player x, fires
		// straight down on a cool-down.
		dx := p.player.x - e.x
		if math.Abs(dx) > 1 {
			e.x += sign(dx) * 12 * dt
		}
		// Bob slightly.
		e.y += 4 * dt * math.Sin(e.aiT*2)
		if e.fireCD <= 0 && e.y < float64(p.h)*0.55 {
			e.fireCD = 1.4 + p.rng.Float64()*0.8
			return true, e.x + 2, e.y + float64(helmA.height())
		}
	case ekBringer:
		// Slow crawl with a sweeping vertical pattern. Heavy and tanky.
		e.x -= 14 * dt
		e.y += 10 * dt * math.Sin(e.aiT*1.4)
		if e.fireCD <= 0 {
			e.fireCD = 1.7 + p.rng.Float64()*1.0
			return true, e.x + 3, e.y + float64(bringerA.height())
		}
	case ekBear:
		// Tracks the player. Faster than Bringer, drops bombs more often.
		dx := p.player.x - e.x
		dy := p.player.y - e.y
		dist := math.Hypot(dx, dy) + 0.001
		e.x += (dx / dist) * 18 * dt
		e.y += (dy / dist) * 10 * dt
		if e.fireCD <= 0 && p.rng.Float64() < 0.5 {
			e.fireCD = 1.2 + p.rng.Float64()*0.7
			return true, e.x + 3, e.y + float64(bearA.height())
		}
	case ekFloater:
		// Drifts left while bobbing on a sin wave. Fires occasional
		// straight-down bombs.
		e.x -= 10 * dt
		e.y = e.y + 18*dt*math.Sin(e.aiT*1.3)
		if e.fireCD <= 0 && p.rng.Float64() < 0.6 {
			e.fireCD = 1.6 + p.rng.Float64()*0.9
			return true, e.x + 2, e.y + float64(floaterA.height())
		}
	case ekDancer:
		// Descends through the rainbow scroll, weaving side to side.
		e.y += 24 * dt
		e.x += 22 * dt * math.Sin(e.aiT*2.5)
	case ekStyxTurret:
		// Stationary in world coords — drifts up with the world scroll
		// because the play scene treats it as locked to the wall. Fires
		// inward toward the centre of the play area.
		if e.fireCD <= 0 {
			e.fireCD = 1.5 + p.rng.Float64()*0.8
			return true, e.x + 2, e.y + float64(helmA.height())
		}
	}
	return false, 0, 0
}

// sign returns -1, 0, or 1 — convenient for nudging values toward a
// target without having to write the conditional inline.
func sign(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
