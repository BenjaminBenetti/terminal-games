package rallyx

import (
	"math"
	"math/rand"
)

// Enemy chase AI. In the arcade, the four red cars use a coarse
// "head toward the player" heuristic: at each intersection they pick
// the cardinal direction that minimises Manhattan distance to the
// player, with a 180°-reversal restriction so they don't dither in
// place. We mirror that here, with a small chance of picking a
// "wrong" turn each intersection to keep the chase from feeling
// surgical.
//
// The cardinal-decision-at-tile-centre pattern is shared with the
// Pac-Man port's ghost movement; the per-frame motion loop lives on
// car.advance and the AI only fills in car.desired.

// enemyState describes the high-level behaviour of one enemy car.
type enemyState uint8

const (
	enemyChasing enemyState = iota
	enemyCrashed            // hit a rock or driven into smoke
)

// enemy adds AI bookkeeping on top of the car struct.
type enemy struct {
	car
	state  enemyState
	rng    *rand.Rand
	wander float64 // 0..1, probability of choosing a non-optimal direction
	lastTile [2]int
}

// newEnemy constructs an enemy at (x, y) with a per-instance RNG seed
// so the four enemies don't pick identical wander rolls every frame.
func newEnemy(x, y float64, seed int64) *enemy {
	return &enemy{
		car: car{
			x:     x,
			y:     y,
			dir:   dirLeft,
			alive: true,
			speed: 0,
		},
		state:    enemyChasing,
		rng:      rand.New(rand.NewSource(seed)),
		wander:   0.25,
		lastTile: [2]int{-1, -1},
	}
}

// drive picks a new direction at each tile centre using a "move
// toward the player, but maybe veer off" heuristic. It does NOT
// advance the car — call car.advance afterwards to apply motion.
func (e *enemy) drive(playerX, playerY float64, m *maze) {
	if !e.alive || e.crashed || e.smokeT > 0 {
		return
	}

	cur := [2]int{e.tileX(), e.tileY()}
	if cur == e.lastTile {
		// Mid-tile; nothing to decide.
		return
	}
	e.lastTile = cur

	// Enumerate the legal moves (excluding the reversal of the current
	// heading; the arcade's pursuit AI refuses to U-turn at an
	// intersection, which is what keeps the chase tense).
	options := make([]direction, 0, 4)
	for _, d := range allDirs {
		if d == e.dir.opposite() && e.dir != dirNone {
			continue
		}
		tx := cur[0] + d.dx()
		ty := cur[1] + d.dy()
		if m.passable(tx, ty) {
			options = append(options, d)
		}
	}
	if len(options) == 0 {
		// Trapped — allow reversal as a last resort.
		rev := e.dir.opposite()
		tx := cur[0] + rev.dx()
		ty := cur[1] + rev.dy()
		if m.passable(tx, ty) {
			e.desired = rev
			return
		}
		return
	}

	// "Best" direction is the one whose post-move tile minimises
	// Manhattan distance to the player. Ties (which are common in
	// open chambers) are broken by allDirs ordering so the choice is
	// stable frame-over-frame.
	bestIdx := 0
	bestScore := math.Inf(1)
	for i, d := range options {
		nx := float64(cur[0]+d.dx()) + 0.5
		ny := float64(cur[1]+d.dy()) + 0.5
		score := math.Abs(nx-playerX) + math.Abs(ny-playerY)
		if score < bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	choice := options[bestIdx]
	// Small chance to wander — picks a random legal option. This is
	// the "Pac-Man clyde" knob: too low and the enemies are unfair,
	// too high and they're harmless.
	if len(options) > 1 && e.rng.Float64() < e.wander {
		choice = options[e.rng.Intn(len(options))]
	}
	e.desired = choice
}

// stunBySmoke marks the enemy as disabled by a smoke screen for
// `duration` seconds. The car freezes; collisions stop registering
// against the player until the timer expires.
func (e *enemy) stunBySmoke(duration float64) {
	if !e.alive || e.crashed {
		return
	}
	if duration > e.smokeT {
		e.smokeT = duration
	}
}

// tickSmoke decays the smoke-stun timer toward zero.
func (e *enemy) tickSmoke(dt float64) {
	if e.smokeT > 0 {
		e.smokeT -= dt
		if e.smokeT < 0 {
			e.smokeT = 0
		}
	}
}

// crash records that the enemy has been destroyed (rock or wreck).
func (e *enemy) crash() {
	e.crashed = true
	e.alive = false
	e.crashedT = 0
}
