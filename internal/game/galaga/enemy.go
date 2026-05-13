package galaga

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// enemyKind identifies the three enemy archetypes that occupy the
// Galaga formation.
type enemyKind int

const (
	enemyBee       enemyKind = iota // Zako — 50 / 100 pts. Rows 3-4.
	enemyButterfly                  // Goei — 80 / 160 pts. Rows 1-2.
	enemyBoss                       // Boss Galaga — 150 / 400 pts. Row 0.
)

// formationScore returns the point value when killed while in or
// returning to the formation (not actively diving).
func (k enemyKind) formationScore() int {
	switch k {
	case enemyBee:
		return 50
	case enemyButterfly:
		return 80
	case enemyBoss:
		return 150
	}
	return 0
}

// flightScore returns the point value when killed while diving. Diving
// kills are worth 2x the formation value (Boss is 400, the original
// arcade also has 800 / 1600 bonuses for killing a Boss with escorts —
// not modelled in this build).
func (k enemyKind) flightScore() int {
	switch k {
	case enemyBee:
		return 100
	case enemyButterfly:
		return 160
	case enemyBoss:
		return 400
	}
	return 0
}

// color returns the iconic display colour for this enemy kind.
func (k enemyKind) color() engine.Color {
	switch k {
	case enemyBee:
		return engine.Color{R: 90, G: 180, B: 255, A: 255}
	case enemyButterfly:
		return engine.Color{R: 240, G: 100, B: 120, A: 255}
	case enemyBoss:
		return engine.Color{R: 100, G: 230, B: 150, A: 255}
	}
	return engine.White
}

// frames returns the two wing-flap animation frames for this kind.
func (k enemyKind) frames() (sprite, sprite) {
	switch k {
	case enemyBee:
		return beeA, beeB
	case enemyButterfly:
		return butterflyA, butterflyB
	case enemyBoss:
		return bossA, bossB
	}
	return beeA, beeB
}

// hitsToKill reports how many player bullets are required to destroy
// this enemy kind. In the original arcade Bosses take two shots while
// alone in formation (they turn blue after the first); Bees and
// Butterflies die in one. While diving every enemy dies in one shot.
func (k enemyKind) hitsToKill(diving bool) int {
	if diving {
		return 1
	}
	if k == enemyBoss {
		return 2
	}
	return 1
}

// enemyState is the per-enemy life-cycle state.
type enemyState int

const (
	esEntering  enemyState = iota // flying along entry path to formation
	esFormation                   // resting in formation slot (subject to sway)
	esDiving                      // peeled off, flying along dive path
	esReturning                   // dive complete, flying back to formation
	esHoverBeam                   // boss is deploying tractor beam
	esCarryShip                   // boss returning to formation with captured ship
	esDying                       // animating explosion
	esGone                        // ready to be removed from the entity list
)

// enemy is a single insectoid. Bosses, Butterflies, and Bees share this
// struct — kind differentiates appearance and score. State drives the
// behaviour each frame.
type enemy struct {
	kind enemyKind

	// Formation slot — fixed for the life of this enemy. While in
	// esFormation, x/y are derived from this slot + sway.
	slotRow, slotCol int

	state enemyState

	x, y float64

	// Path data (used in entering/diving/returning/carryShip).
	path     *path
	pathDist float64
	pathSpd  float64

	// Wing-flap animation.
	frame  int
	frameT float64

	// Hit points (Boss turns blue after first hit then dies on second).
	hits int

	// Boss-only: tractor beam animation, captured player carry.
	beamT        float64 // tractor-beam elapsed seconds (in esHoverBeam)
	carryHasShip bool    // true if this boss is currently carrying the player's captured ship
	// tractorDive is set when startDive issued a tractor-beam dive path
	// for this boss. tickEnemies inspects this on path-completion to
	// decide whether to transition to esHoverBeam (tractor) or
	// esReturning (regular swoop/loop that went off-screen).
	tractorDive bool

	// Diving bomb cadence.
	bombCooldown float64

	// Explosion animation timer.
	dyingT float64
}

// inFormation reports whether this enemy is currently sitting in the
// formation grid (so it counts toward stage completion).
func (e *enemy) inFormation() bool {
	return e.state == esFormation
}

// alive reports whether the enemy still exists for collision / scoring
// purposes (i.e. not exploded and not removed).
func (e *enemy) alive() bool {
	return e.state != esDying && e.state != esGone
}

// canFire reports whether the enemy is in a state where it makes sense
// to drop bombs at the player. In the arcade only diving enemies fire,
// not formation-bound ones, which is what this preserves.
func (e *enemy) canFire() bool {
	return e.state == esDiving
}

// boundingBox returns the AABB of this enemy in canvas pixels for
// collision tests. Width/height come from the kind's sprite — all
// three kinds share 7x5 dimensions in this build, so we just hard-code.
func (e *enemy) boundingBox() rect {
	w, h := 7, 5
	return rect{x0: int(e.x), y0: int(e.y), x1: int(e.x) + w, y1: int(e.y) + h}
}

// applyHit damages the enemy. Returns true if the hit killed it.
// Bosses absorb the first hit (turn blue / lose green tint) and die on
// the second. While diving, every enemy dies in one hit.
func (e *enemy) applyHit(diving bool) bool {
	need := e.kind.hitsToKill(diving)
	e.hits++
	return e.hits >= need
}
