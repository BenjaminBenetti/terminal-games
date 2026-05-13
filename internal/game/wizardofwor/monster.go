package wizardofwor

import (
	"math"
	"math/rand"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// monsterKind enumerates the four enemy tiers. The first three are the
// standard dungeon roster; worluk and wizard are the bonus enemies
// that appear at end-of-dungeon and on rare occasions.
type monsterKind int

const (
	mkBurwor monsterKind = iota
	mkGarwor
	mkThorwor
	mkWorluk
	mkWizard
)

// monsterBaseSpeed is the per-kind starting speed in tiles/sec. Higher
// dungeons multiply this — see play.dungeonSpeedMul.
var monsterBaseSpeed = map[monsterKind]float64{
	mkBurwor:  1.6,
	mkGarwor:  2.4,
	mkThorwor: 3.2,
	mkWorluk:  6.0,
	mkWizard:  2.0,
}

// monsterScore is the point value awarded for shooting one. Worluk and
// Wizard kills also trigger the doubled-score and 2500-pt bonuses
// respectively, applied separately in play.killMonster.
var monsterScore = map[monsterKind]int{
	mkBurwor:  100,
	mkGarwor:  200,
	mkThorwor: 500,
	mkWorluk:  1000,
	mkWizard:  2500,
}

// monsterColor returns the body colour for the given kind. Worluk and
// Wizard have their own dedicated sprite, so this is consulted only
// for the three standard monsters and for the radar dot.
func monsterColor(k monsterKind) engine.Color {
	switch k {
	case mkBurwor:
		return burworBody
	case mkGarwor:
		return garworBody
	case mkThorwor:
		return thorworBody
	case mkWorluk:
		return worlukBody
	case mkWizard:
		return wizardRobe
	}
	return engine.White
}

// monsterState is the per-instance lifecycle. Monsters start in the
// cage waiting for the spawn timer, walk out the door, hunt the player
// until shot, and either die in an explosion or escape with Worluk.
type monsterState int

const (
	msInCage     monsterState = iota // bobbing in the cage, waiting for release
	msEmerging                       // scripted slide from cage cell to door cell
	msHunting                        // normal AI on the playfield
	msDying                          // explosion frames before removal
)

// monster is one active enemy. The embedded entity drives its grid
// motion; the rest is bookkeeping for AI, invisibility, and shooting.
type monster struct {
	entity
	kind    monsterKind
	state   monsterState
	emergeT float64 // 0..1 lerp from cage to door during msEmerging
	walkPhase float64 // for sprite leg animation
	hideT   float64   // counts down visibility timer for Garwor/Thorwor
	visible bool      // currently rendered on the maze (radar always shows)
	shootT  float64   // seconds until next shot is allowed (post-fire cooldown)
	// aimDir / aimT implement the shot windup. The arcade's monsters
	// don't fire the instant they line up on the player — they pause
	// briefly first, giving the player a window to react and break
	// the line of sight. While aimDir != dirNone, the monster is
	// aiming in that direction; when aimT hits zero the shot fires.
	// Losing the line during windup cancels the aim.
	aimDir  direction
	aimT    float64
	dieT    float64 // age of dying explosion (0..dyingHold)
}

// newMonster returns a monster spawned in the cage of the given kind.
// Speed is set from the per-kind base multiplied by the dungeon speed
// multiplier.
func newMonster(kind monsterKind, speedMul float64, rng *rand.Rand) *monster {
	m := &monster{
		kind:    kind,
		state:   msInCage,
		visible: true,
	}
	m.entity.x = float64(cageCol) + 0.5
	m.entity.y = float64(cageRow) + 0.5
	m.entity.dir = dirUp
	m.entity.desired = dirUp
	m.entity.speed = monsterBaseSpeed[kind] * speedMul
	// Stagger the next-shoot timer so monsters don't all fire on the
	// same frame as soon as one decides to.
	m.shootT = 1.2 + rng.Float64()*2.0
	return m
}

// transform mutates an existing monster into a stronger kind, keeping
// its position and direction but updating colour / speed / shoot
// cadence. Used by the "Burwor → Garwor" timer during a dungeon.
func (m *monster) transform(to monsterKind, speedMul float64, rng *rand.Rand) {
	m.kind = to
	m.entity.speed = monsterBaseSpeed[to] * speedMul
	// Garwor/Thorwor start in a visible phase, then begin cycling.
	m.visible = true
	m.hideT = 2.5 + rng.Float64()*2.5
}

// pickAtJunction is the AI decision: at each cell centre, look at the
// open neighbours and pick the one that takes us closer to the
// player. A configurable amount of randomness keeps the dungeon
// surprising (and matches the original's spotty, sometimes-erratic
// hunting behaviour).
func (m *monster) pickAtJunction(maze *maze, playerC, playerR int, rng *rand.Rand, randomness float64) direction {
	c, r := m.tileX(), m.tileY()
	forbidden := m.dir.opposite()

	options := make([]direction, 0, 4)
	for _, d := range allMoves {
		if d == forbidden {
			continue
		}
		if maze.canMove(c, r, d) {
			options = append(options, d)
		}
	}
	if len(options) == 0 {
		// Dead-end — fall back to reversing.
		if forbidden != dirNone && maze.canMove(c, r, forbidden) {
			return forbidden
		}
		return dirNone
	}
	if len(options) == 1 {
		return options[0]
	}

	if rng.Float64() < randomness {
		return options[rng.Intn(len(options))]
	}

	// Greedy: pick the option whose next cell is closest to the player.
	best := options[0]
	bestDist := math.Inf(1)
	for _, d := range options {
		nx := c + d.dx()
		ny := r + d.dy()
		dx := float64(nx - playerC)
		dy := float64(ny - playerR)
		dist := dx*dx + dy*dy
		if dist < bestDist {
			best = d
			bestDist = dist
		}
	}
	return best
}

// hasLineOfFire checks whether (fromC, fromR) and (toC, toR) lie on
// the same row or column with no walls between them. Used by monster
// shooting and the wizard's fireball-vs-player aim.
func hasLineOfFire(maze *maze, fromC, fromR, toC, toR int) (direction, bool) {
	if fromR == toR {
		if fromC == toC {
			return dirNone, false
		}
		step := 1
		dir := dirRight
		if toC < fromC {
			step = -1
			dir = dirLeft
		}
		// Walk along the row checking that each adjacent wall is clear.
		for c := fromC; c != toC; c += step {
			if step > 0 {
				// moving right: wall to the right of c is vwalls[r][c+1].
				if maze.hasWallLeft(c+1, fromR) {
					return dirNone, false
				}
			} else {
				if maze.hasWallLeft(c, fromR) {
					return dirNone, false
				}
			}
		}
		return dir, true
	}
	if fromC == toC {
		step := 1
		dir := dirDown
		if toR < fromR {
			step = -1
			dir = dirUp
		}
		for r := fromR; r != toR; r += step {
			if step > 0 {
				if maze.hasWallTop(fromC, r+1) {
					return dirNone, false
				}
			} else {
				if maze.hasWallTop(fromC, r) {
					return dirNone, false
				}
			}
		}
		return dir, true
	}
	return dirNone, false
}

// shouldShoot runs the monster's shot decision for this frame, in
// three phases:
//
//  1. Post-fire cooldown — shootT > 0 — the monster is reloading and
//     can't fire. Aim is cleared.
//  2. Idle scanning — no current aim. If a line of fire exists, start
//     a windup timer (aimT) and remember the direction (aimDir).
//  3. Aiming — aimDir set. Verify the line still exists each frame
//     (cancel if the player broke it). When aimT hits zero, fire.
//
// The windup gives the player a window to dodge. It's short for
// Thorwors and notably longer for Burwors — that's why Burwors feel
// sluggish and Thorwors feel scary in the arcade.
func (m *monster) shouldShoot(maze *maze, playerC, playerR int, dt float64, rng *rand.Rand) (direction, bool) {
	if m.state != msHunting {
		m.aimDir = dirNone
		return dirNone, false
	}

	// Phase 1: post-fire cooldown.
	if m.shootT > 0 {
		m.shootT -= dt
		m.aimDir = dirNone
		return dirNone, false
	}

	dir, ok := hasLineOfFire(maze, m.tileX(), m.tileY(), playerC, playerR)
	if !ok {
		// Line lost — drop any in-progress aim. Don't burn cooldown;
		// the monster keeps watching for a fresh sightline.
		m.aimDir = dirNone
		return dirNone, false
	}

	// Phase 2: newly acquired line of sight — begin the windup.
	if m.aimDir != dir {
		m.aimDir = dir
		m.aimT = aimWindup(m.kind, rng)
		return dirNone, false
	}

	// Phase 3: hold the aim, count down.
	m.aimT -= dt
	if m.aimT > 0 {
		return dirNone, false
	}

	// Fire.
	m.aimDir = dirNone
	switch m.kind {
	case mkBurwor:
		m.shootT = 4 + rng.Float64()*4
	case mkGarwor:
		m.shootT = 2.5 + rng.Float64()*2.5
	case mkThorwor:
		m.shootT = 1.5 + rng.Float64()*1.5
	default:
		m.shootT = 3
	}
	return dir, true
}

// aimWindup is the reaction window the player gets between a monster
// acquiring a line of fire and the actual shot. Tier-dependent: a
// Burwor lumbers up its aim, a Thorwor snaps to fire.
func aimWindup(k monsterKind, rng *rand.Rand) float64 {
	switch k {
	case mkBurwor:
		return 0.85 + rng.Float64()*0.4
	case mkGarwor:
		return 0.55 + rng.Float64()*0.3
	case mkThorwor:
		return 0.30 + rng.Float64()*0.2
	}
	return 0.6
}

// isAiming reports whether the monster is currently telegraphing a
// shot. The renderer uses this to draw a brighter body so the player
// can see the threat coming.
func (m *monster) isAiming() bool {
	return m.state == msHunting && m.aimDir != dirNone
}

// updateVisibility ticks the invisibility cycle for Garwors and
// Thorwors. Burwors are always visible. Other kinds (Worluk, Wizard)
// have their own per-state rules and don't go through here.
func (m *monster) updateVisibility(dt float64, rng *rand.Rand) {
	if m.kind != mkGarwor && m.kind != mkThorwor {
		m.visible = true
		return
	}
	if m.state != msHunting {
		m.visible = true
		return
	}
	m.hideT -= dt
	if m.hideT <= 0 {
		m.visible = !m.visible
		if m.visible {
			// Visible window: short for Thorwor, longer for Garwor.
			if m.kind == mkThorwor {
				m.hideT = 1.0 + rng.Float64()*1.0
			} else {
				m.hideT = 1.5 + rng.Float64()*1.5
			}
		} else {
			if m.kind == mkThorwor {
				m.hideT = 2.0 + rng.Float64()*2.0
			} else {
				m.hideT = 1.8 + rng.Float64()*1.8
			}
		}
	}
}

// wave describes the monster composition for a dungeon: how many of
// each kind start in the cage, and the transformation timers.
type wave struct {
	burwors  int
	garwors  int
	thorwors int
	// transformAfter is the time (seconds) after a dungeon starts at
	// which a single Burwor upgrades to Garwor. Subsequent transforms
	// happen on the same interval. 0 disables transforms.
	transformAfter float64
	speedMul       float64
}

// waveFor returns the spawn / difficulty plan for the given dungeon
// number (1-based). The pool grows and the timer shrinks as the
// player progresses; the original game ramps similarly.
func waveFor(dungeon int) wave {
	d := dungeon
	if d < 1 {
		d = 1
	}
	w := wave{
		burwors:        6,
		garwors:        0,
		thorwors:       0,
		transformAfter: 0,
		speedMul:       1.0 + 0.07*float64(d-1),
	}
	switch {
	case d == 1:
		// Pure Burwors, no transforms.
	case d == 2:
		w.burwors = 5
		w.garwors = 1
		w.transformAfter = 18
	case d == 3:
		w.burwors = 4
		w.garwors = 2
		w.transformAfter = 14
	case d == 4:
		w.burwors = 3
		w.garwors = 2
		w.thorwors = 1
		w.transformAfter = 12
	case d == 5:
		w.burwors = 2
		w.garwors = 3
		w.thorwors = 1
		w.transformAfter = 10
	default:
		// Dungeons 6+ get denser and faster.
		w.burwors = 1
		w.garwors = 3
		w.thorwors = 2
		w.transformAfter = 8 + 1.5/float64(d-5)
	}
	// Difficulty cap: clamp the speed multiplier so late dungeons remain
	// at least nominally playable.
	if w.speedMul > 1.9 {
		w.speedMul = 1.9
	}
	return w
}
