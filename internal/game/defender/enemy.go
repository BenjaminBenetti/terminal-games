package defender

import (
	"math"
	"math/rand"
)

// enemyKind enumerates the six enemy archetypes from Defender.
type enemyKind int

const (
	kLander  enemyKind = iota // descends, abducts humans, mutates if it reaches the top
	kMutant                   // ex-lander; fast, aggressive, pursues the player
	kBomber                   // slow, drifts in straight line, drops mines
	kPod                      // bursts into 4 swarmers when shot
	kSwarmer                  // small fast pack predator
	kBaiter                   // wave-stall punisher; fast yo-yo
)

// enemyState distinguishes per-frame AI sub-modes for landers. Other
// kinds use only stateActive / stateDying / stateGone.
type enemyState int

const (
	esActive    enemyState = iota // default behaviour
	esDescend                     // lander descending toward a target human
	esAbducting                   // lander locked to a human, rising
	esEscaped                     // lander reached the top with human → about to mutate
	esDying                       // explosion animation
	esGone                        // ready to be reaped
)

// enemy is the do-it-all entity for every hostile in the wave. Per-kind
// fields are gated on `kind`; the alternative is six separate types
// each with their own slice, which buys nothing because the game logic
// already branches on kind everywhere.
type enemy struct {
	kind  enemyKind
	state enemyState

	worldX, y  float64
	vx, vy     float64
	frame      int     // 0 or 1, cycled for two-frame wing flap
	frameT     float64 // accumulator for animation
	fireT      float64 // cooldown until this enemy can fire again
	dyingT     float64 // elapsed time in esDying
	chaseT     float64 // time-since-last-AI-decision (used by mutants/swarmers)
	dropT      float64 // bomber mine-drop cool-down
	mutateAtY  float64 // y at which a lander finishes "escaping" and mutates
	carrying   *humanoid
	targetH    *humanoid // chosen abductee (for landers in esDescend)
	beenSeen   bool      // true once this enemy has been on-screen at least once
	homeY      float64   // baiter centre-of-yoyo
	homePhase  float64   // baiter phase offset so spawned baiters don't sync
}

func (e *enemy) alive() bool {
	return e.state != esDying && e.state != esGone
}

func (e *enemy) sprite() (a, b sprite) {
	switch e.kind {
	case kLander:
		return landerA, landerB
	case kMutant:
		return mutantA, mutantB
	case kBomber:
		return bomberA, bomberB
	case kPod:
		return podA, podB
	case kSwarmer:
		return swarmerA, swarmerB
	case kBaiter:
		return baiterA, baiterB
	}
	return landerA, landerB
}

// kindScore returns the points awarded for shooting an enemy of this
// kind. Values follow the original cabinet (with Pod's 1000 bonus
// applied here, not deferred to the burst).
func kindScore(k enemyKind) int {
	switch k {
	case kLander:
		return 150
	case kMutant:
		return 150
	case kBomber:
		return 250
	case kPod:
		return 1000
	case kSwarmer:
		return 150
	case kBaiter:
		return 200
	}
	return 0
}

// boundingBox returns the per-enemy AABB in world+pixel coords used by
// the bullet/mine collision tests.
func (e *enemy) boundingBox() (x0, y0, x1, y1 float64) {
	a, _ := e.sprite()
	w := a.width()
	h := a.height()
	return e.worldX, e.y, e.worldX + float64(w), e.y + float64(h)
}

// updateEnemies advances every enemy's AI, position, and animation for
// dt seconds. It also enqueues enemy bolts as it goes.
func (p *playScene) updateEnemies(dt float64) {
	kept := p.enemies[:0]
	for _, e := range p.enemies {
		// Animation: every kind flickers between two frames.
		e.frameT += dt
		if e.frameT >= enemyAnimPeriod {
			e.frameT = 0
			e.frame = 1 - e.frame
		}

		switch e.state {
		case esDying:
			e.dyingT += dt
			if e.dyingT >= enemyExplodeDur {
				e.state = esGone
			}
		default:
			switch e.kind {
			case kLander:
				p.tickLander(e, dt)
			case kMutant:
				p.tickMutant(e, dt)
			case kBomber:
				p.tickBomber(e, dt)
			case kPod:
				p.tickPod(e, dt)
			case kSwarmer:
				p.tickSwarmer(e, dt)
			case kBaiter:
				p.tickBaiter(e, dt)
			}
		}
		if e.state != esGone {
			kept = append(kept, e)
		}
	}
	p.enemies = kept
}

// ---- Lander -----------------------------------------------------------

// Landers are the wave's backbone. They:
//  1. drift sideways at high altitude scanning for humans (esActive),
//  2. lock on to a nearest human and dive (esDescend),
//  3. once close, "grab" them and ascend (esAbducting), and
//  4. mutate into a Mutant if they reach the top edge (esEscaped).
//
// Any lander that's shot during esAbducting drops its human into
// freefall — the player can catch them mid-air.
func (p *playScene) tickLander(e *enemy, dt float64) {
	switch e.state {
	case esActive:
		// Drift horizontally — random target velocity, occasionally re-rolled.
		e.chaseT -= dt
		if e.chaseT <= 0 {
			e.chaseT = 1.0 + p.rng.Float64()*1.5
			dir := float64(p.rng.Intn(3) - 1) // -1, 0, +1
			e.vx = dir * landerCruise
			// Drift up/down a little to avoid forming a perfect line.
			e.vy = (p.rng.Float64() - 0.5) * 8
		}
		e.worldX = p.world.wrapX(e.worldX + e.vx*dt)
		e.y += e.vy * dt
		// Clamp to upper portion of the play zone.
		if e.y < float64(p.world.playZoneTop) {
			e.y = float64(p.world.playZoneTop)
			e.vy = math.Abs(e.vy)
		}
		topBand := float64(p.world.playZoneTop) + 24
		if e.y > topBand {
			e.y = topBand
			e.vy = -math.Abs(e.vy)
		}
		// Decide to start descending toward a human.
		if p.rng.Float64() < landerDescendProb*dt {
			tgt := p.pickClosestHumanForLander(e.worldX)
			if tgt != nil {
				e.targetH = tgt
				e.state = esDescend
			}
		}
		// Fire occasionally.
		p.maybeEnemyFire(e, landerFireGap, dt)
	case esDescend:
		if e.targetH == nil || e.targetH.dead || e.targetH.state != humanWalking {
			// Lost target — go back to roaming.
			e.targetH = nil
			e.state = esActive
			return
		}
		// Steer horizontally toward target, descend at fixed rate.
		dx := p.world.wrapDelta(e.worldX, e.targetH.worldX)
		stepX := math.Copysign(math.Min(math.Abs(dx), landerSteerX*dt), dx)
		e.worldX = p.world.wrapX(e.worldX + stepX)
		e.y += landerDescendSpd * dt
		// Did we get close enough to grab?
		dy := e.targetH.y - (e.y + 5)
		if math.Abs(dx) < 3 && dy < 4 && dy > -2 {
			// Latch onto the human.
			e.carrying = e.targetH
			e.targetH.carrier = e
			e.targetH.state = humanLifted
			e.state = esAbducting
			e.mutateAtY = float64(p.world.playZoneTop) + 2
		}
		p.maybeEnemyFire(e, landerFireGap, dt)
	case esAbducting:
		// Ascend, carrying human in tow.
		e.y -= landerAbductSpd * dt
		if e.carrying != nil {
			e.carrying.worldX = e.worldX + 2
			e.carrying.y = e.y + 5
			if e.carrying.dead {
				// Defensive: shouldn't normally happen.
				e.carrying = nil
				e.state = esActive
			}
		}
		// Reached the top — mutate.
		if e.y <= e.mutateAtY {
			p.mutateLander(e)
		}
	}
}

// mutateLander converts an escaping lander (and its captive) into a
// Mutant and a corpse, respectively. Kills the carried human off
// (Defender's classic punishment for letting an abduction succeed).
func (p *playScene) mutateLander(e *enemy) {
	if e.carrying != nil {
		p.killHuman(e.carrying)
		e.carrying = nil
	}
	e.kind = kMutant
	e.state = esActive
	e.vx = 0
	e.vy = 0
}

// ---- Mutant -----------------------------------------------------------

// Mutants chase the player at high speed, weaving up and down. They
// fire more often than landers.
func (p *playScene) tickMutant(e *enemy, dt float64) {
	dx := p.world.wrapDelta(e.worldX, p.player.worldX)
	dy := p.player.y - e.y
	// Normalised pursuit vector, plus a sinusoidal jitter so two
	// mutants on the same path don't perfectly overlap.
	d := math.Hypot(dx, dy)
	if d < 0.01 {
		d = 0.01
	}
	jitter := math.Sin(e.frameT*7 + e.homePhase)
	e.vx = (dx/d)*mutantSpeed + jitter*8
	e.vy = (dy/d)*mutantSpeed + math.Cos(e.frameT*5+e.homePhase)*6
	e.worldX = p.world.wrapX(e.worldX + e.vx*dt)
	e.y += e.vy * dt
	// Stay within play zone.
	if e.y < float64(p.world.playZoneTop) {
		e.y = float64(p.world.playZoneTop)
	}
	if e.y > float64(p.world.playZoneBot)-6 {
		e.y = float64(p.world.playZoneBot) - 6
	}
	p.maybeEnemyFire(e, mutantFireGap, dt)
}

// ---- Bomber ----------------------------------------------------------

// Bombers cruise along a horizontal lane at the top of the world,
// dropping cross-shaped mines that hang in place for several seconds.
func (p *playScene) tickBomber(e *enemy, dt float64) {
	e.worldX = p.world.wrapX(e.worldX + e.vx*dt)
	// Slow vertical drift to lend some variety.
	e.y += e.vy * dt
	if e.y < float64(p.world.playZoneTop)+4 {
		e.y = float64(p.world.playZoneTop) + 4
		e.vy = math.Abs(e.vy)
	}
	if e.y > float64(p.world.playZoneTop)+20 {
		e.y = float64(p.world.playZoneTop) + 20
		e.vy = -math.Abs(e.vy)
	}
	e.dropT -= dt
	if e.dropT <= 0 {
		// Only drop while on-screen, otherwise mines pile up off-screen.
		sx := p.world.toScreen(e.worldX)
		if sx > -10 && sx < p.w+10 {
			p.mines = append(p.mines, &mine{
				worldX: e.worldX + 5,
				y:      e.y + 5,
				life:   mineLifetime,
			})
		}
		e.dropT = bomberDropGap + p.rng.Float64()*0.8
	}
}

// ---- Pod & Swarmer ---------------------------------------------------

// Pods drift slowly in a straight line. Their entire role is to
// rupture into 4 swarmers when shot — see resolveBulletEnemyHits.
func (p *playScene) tickPod(e *enemy, dt float64) {
	e.worldX = p.world.wrapX(e.worldX + e.vx*dt)
	e.y += e.vy * dt
	// Soft-bound to the upper play zone.
	if e.y < float64(p.world.playZoneTop)+4 {
		e.vy = math.Abs(e.vy)
	}
	if e.y > float64(p.world.playZoneTop)+30 {
		e.vy = -math.Abs(e.vy)
	}
}

// Swarmers behave like mini-mutants: chase the player aggressively with
// extra wobble.
func (p *playScene) tickSwarmer(e *enemy, dt float64) {
	dx := p.world.wrapDelta(e.worldX, p.player.worldX)
	dy := p.player.y - e.y
	d := math.Hypot(dx, dy)
	if d < 0.01 {
		d = 0.01
	}
	wobble := math.Sin(e.frameT*9 + e.homePhase)
	e.vx = (dx/d)*swarmerSpeed + wobble*14
	e.vy = (dy/d)*swarmerSpeed + math.Cos(e.frameT*8+e.homePhase)*12
	e.worldX = p.world.wrapX(e.worldX + e.vx*dt)
	e.y += e.vy * dt
	if e.y < float64(p.world.playZoneTop) {
		e.y = float64(p.world.playZoneTop)
	}
	if e.y > float64(p.world.playZoneBot)-4 {
		e.y = float64(p.world.playZoneBot) - 4
	}
	// Swarmers don't fire — they damage by contact only.
}

// ---- Baiter ----------------------------------------------------------

// Baiters spawn when the wave is dragging. They scream sideways past
// the player, bobbing vertically — much faster than any other enemy.
func (p *playScene) tickBaiter(e *enemy, dt float64) {
	// Always chase player horizontally at high speed; bob vertically
	// around a homeY sine.
	dx := p.world.wrapDelta(e.worldX, p.player.worldX)
	if dx >= 0 {
		e.vx = baiterSpeed
	} else {
		e.vx = -baiterSpeed
	}
	e.worldX = p.world.wrapX(e.worldX + e.vx*dt)
	e.homePhase += dt * 5
	e.y = e.homeY + 8*math.Sin(e.homePhase)
	p.maybeEnemyFire(e, baiterFireGap, dt)
}

// maybeEnemyFire enqueues an enemy bolt aimed at the player when the
// enemy's per-shot cool-down has elapsed AND there's a reasonable
// line-of-sight (i.e. enemy is on-screen).
func (p *playScene) maybeEnemyFire(e *enemy, gap float64, dt float64) {
	e.fireT -= dt
	if e.fireT > 0 {
		return
	}
	// Only fire while on-screen — off-screen shots are wasted bytes.
	sx := p.world.toScreen(e.worldX)
	if sx < -5 || sx >= p.w+5 {
		e.fireT = 0.4
		return
	}
	if len(p.enemyBolts) >= enemyBoltMax {
		e.fireT = 0.3
		return
	}
	dx := p.world.wrapDelta(e.worldX, p.player.worldX)
	dy := p.player.y - e.y
	d := math.Hypot(dx, dy)
	if d < 0.5 {
		e.fireT = gap
		return
	}
	speed := enemyBoltSpeed
	b := &enemyBoltEntity{
		worldX: e.worldX + 3,
		y:      e.y + 2,
		vx:     dx / d * speed,
		vy:     dy / d * speed,
	}
	p.enemyBolts = append(p.enemyBolts, b)
	e.fireT = gap + p.rng.Float64()*0.5
}

// burstPod replaces the pod entity with 4 swarmers radiating outward.
func (p *playScene) burstPod(e *enemy) {
	for i := 0; i < 4; i++ {
		ang := float64(i) * (math.Pi / 2)
		sw := &enemy{
			kind:      kSwarmer,
			state:     esActive,
			worldX:    e.worldX,
			y:         e.y,
			vx:        math.Cos(ang) * swarmerSpeed,
			vy:        math.Sin(ang) * swarmerSpeed,
			homePhase: p.rng.Float64() * 6.28,
		}
		p.enemies = append(p.enemies, sw)
	}
}

// spawnLander spawns one lander at a random world x near the top of
// the play zone. Used by the wave director.
func spawnLander(w *world, rng *rand.Rand) *enemy {
	return &enemy{
		kind:    kLander,
		state:   esActive,
		worldX:  rng.Float64() * float64(w.worldW),
		y:       float64(w.playZoneTop) + 6 + rng.Float64()*10,
		vx:      (rng.Float64() - 0.5) * 2 * landerCruise,
		chaseT:  rng.Float64() * 1.5,
	}
}

func spawnBomber(w *world, rng *rand.Rand) *enemy {
	dir := 1.0
	if rng.Intn(2) == 0 {
		dir = -1
	}
	return &enemy{
		kind:    kBomber,
		state:   esActive,
		worldX:  rng.Float64() * float64(w.worldW),
		y:       float64(w.playZoneTop) + 8 + rng.Float64()*10,
		vx:      dir * bomberSpeed,
		vy:      (rng.Float64() - 0.5) * 4,
		dropT:   1.0 + rng.Float64()*1.5,
	}
}

func spawnPod(w *world, rng *rand.Rand) *enemy {
	dir := 1.0
	if rng.Intn(2) == 0 {
		dir = -1
	}
	return &enemy{
		kind:   kPod,
		state:  esActive,
		worldX: rng.Float64() * float64(w.worldW),
		y:      float64(w.playZoneTop) + 12 + rng.Float64()*8,
		vx:     dir * podSpeed,
		vy:     (rng.Float64() - 0.5) * 6,
	}
}

func spawnBaiter(w *world, rng *rand.Rand, playerY float64) *enemy {
	return &enemy{
		kind:      kBaiter,
		state:     esActive,
		worldX:    rng.Float64() * float64(w.worldW),
		homeY:     playerY,
		y:         playerY,
		homePhase: rng.Float64() * 6.28,
	}
}

// Tuning constants for enemies — speeds in px/s.
const (
	enemyAnimPeriod   = 0.18
	enemyExplodeDur   = 0.45
	enemyBoltMax      = 24
	enemyBoltSpeed    = 60.0

	landerCruise      = 10.0
	landerDescendSpd  = 14.0
	landerSteerX      = 18.0
	landerAbductSpd   = 12.0
	landerFireGap     = 2.2
	landerDescendProb = 0.25 // chance per second of starting a descent

	mutantSpeed   = 40.0
	mutantFireGap = 1.2

	bomberSpeed   = 18.0
	bomberDropGap = 1.6
	mineLifetime  = 4.0

	podSpeed     = 10.0
	swarmerSpeed = 55.0

	baiterSpeed   = 65.0
	baiterFireGap = 1.8
)
