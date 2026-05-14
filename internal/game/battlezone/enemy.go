package battlezone

import (
	"math"
	"math/rand"
)

// enemyKind is the species of an active enemy. Only one enemy is alive
// at any time, matching the original cabinet.
type enemyKind int

const (
	enemyTank enemyKind = iota
	enemySuperTank
	enemyMissile
	enemySaucer
)

// Score values match the values on the marquee of the 1980 cabinet.
const (
	scoreTank      = 1000
	scoreSuperTank = 3000
	scoreMissile   = 2000
	scoreSaucer    = 5000
)

// Per-enemy tuning. Speeds are in world units per second.
const (
	tankMoveSpeed    = 5.5
	tankTurnSpeed    = 1.0  // rad/s
	tankFireRange    = 50.0 // start considering shots within this range
	tankFireCooldown = 2.6  // seconds between shots
	tankAimTol       = 0.10 // rad — must aim within this to fire

	superTankMoveSpeed    = 8.4
	superTankTurnSpeed    = 1.5
	superTankFireRange    = 55.0
	superTankFireCooldown = 1.9
	superTankAimTol       = 0.08

	missileSpeed       = 11.0
	missileTurnSpeed   = 1.4
	missileLifetime    = 16.0
	missileMinDistKill = 1.6

	saucerSpeed     = 3.2
	saucerLifetime  = 14.0
	saucerHeight    = 5.0 // above ground
	saucerWanderAmp = 0.6
)

// enemy is the single live opponent. Fields used differ by kind.
type enemy struct {
	kind enemyKind
	pos  vec3 // ground centre for tanks/missile; sky position for saucer
	yaw  float64
	// Tank/super-tank fields.
	fireCool float64
	// State timer that drives kind-specific behaviour (e.g. saucer
	// wander, missile lifetime).
	life float64
	// AI sub-state for tanks: 0 = chase, 1 = strafe to clear LOS.
	subState int
	subT     float64
}

// projectile is an in-flight shell (player or enemy) or the homing
// missile body's exhaust trail. fromPlayer separates collisions; if
// fromMissile is set, the projectile is purely visual.
type projectile struct {
	pos        vec3
	vel        vec3 // world units per second
	life       float64
	fromPlayer bool
	// For player/enemy shells: a previous-position pin used to draw a
	// tiny tracer line and to do swept collision against obstacles
	// rather than relying on a single-frame point check.
	prev vec3
}

const (
	playerShellSpeed = 38.0
	enemyShellSpeed  = 28.0
	shellLifetime    = 1.4
	shellHitRadius   = 1.1
)

// tickEnemy advances the enemy one step. dt is the frame delta in
// seconds. Returns true if the enemy should be removed (saucer flew
// out, missile expired, etc.). The play scene injects the player's
// position so the AI can target it without a back-pointer.
func (p *playScene) tickEnemy(dt float64) bool {
	e := p.enemy
	if e == nil {
		return false
	}
	e.life += dt
	switch e.kind {
	case enemyTank, enemySuperTank:
		return p.tickTankAI(e, dt)
	case enemyMissile:
		return p.tickMissileAI(e, dt)
	case enemySaucer:
		return p.tickSaucerAI(e, dt)
	}
	return false
}

// tickTankAI handles both standard and super tank movement. The tank
// turns to face the player and rolls forward, slowing if an obstacle
// blocks line of sight and strafing sideways for a second to clear it.
// Whenever its cannon is roughly aligned and within range, it fires.
func (p *playScene) tickTankAI(e *enemy, dt float64) bool {
	var moveSpd, turnSpd, fireCoolReset, fireRange, aimTol float64
	if e.kind == enemySuperTank {
		moveSpd = superTankMoveSpeed
		turnSpd = superTankTurnSpeed
		fireCoolReset = superTankFireCooldown
		fireRange = superTankFireRange
		aimTol = superTankAimTol
	} else {
		moveSpd = tankMoveSpeed
		turnSpd = tankTurnSpeed
		fireCoolReset = tankFireCooldown
		fireRange = tankFireRange
		aimTol = tankAimTol
	}

	// Aim toward player using shortest toroidal delta.
	toward := shortestDelta(e.pos, p.cam.pos)
	desired := math.Atan2(toward.x, toward.z)
	diff := normalizeAngle(desired - e.yaw)
	maxStep := turnSpd * dt
	if diff > maxStep {
		diff = maxStep
	} else if diff < -maxStep {
		diff = -maxStep
	}
	e.yaw = normalizeAngle(e.yaw + diff)

	// Move forward. If a strafe sub-state is active, blend in a small
	// lateral component to dodge around blockers.
	fwd := vec3{x: math.Sin(e.yaw), z: math.Cos(e.yaw)}
	dist := math.Hypot(toward.x, toward.z)
	speed := moveSpd
	if dist < 6 {
		// Maintain a small standoff distance so the tank doesn't ram.
		speed *= 0.2
	}
	dx := fwd.x * speed * dt
	dz := fwd.z * speed * dt
	if e.subState == 1 {
		// Strafing — perpendicular to facing.
		side := math.Sin(e.subT * 3) // oscillate left/right
		perp := vec3{x: math.Cos(e.yaw), z: -math.Sin(e.yaw)}
		dx += perp.x * speed * 0.4 * dt * side
		dz += perp.z * speed * 0.4 * dt * side
		e.subT += dt
		if e.subT > 1.2 {
			e.subState = 0
			e.subT = 0
		}
	}
	// Don't drive INTO an obstacle.
	newPos := vec3{x: wrapWorld(e.pos.x + dx), y: e.pos.y, z: wrapWorld(e.pos.z + dz)}
	if !obstacleAt(p.obstacles, newPos, 1.3) {
		e.pos = newPos
	} else if e.subState == 0 {
		// Switch to strafing for a moment.
		e.subState = 1
		e.subT = 0
	}

	// Fire if aim is good, range is good, line-of-sight is clear, and
	// we have no shell currently in flight.
	e.fireCool -= dt
	if e.fireCool <= 0 && math.Abs(diff) < aimTol && dist < fireRange && !p.enemyShellInFlight() {
		// Check line-of-sight: cast a segment from the cannon mouth
		// toward the player. If it hits an obstacle, don't fire.
		nose := vec3{
			x: e.pos.x + fwd.x*1.7,
			y: 1.0,
			z: e.pos.z + fwd.z*1.7,
		}
		target := vec3{x: p.cam.pos.x, y: 1.0, z: p.cam.pos.z}
		if _, blocked := segmentBlocked(p.obstacles, nose, target); !blocked {
			p.spawnEnemyShell(nose, fwd)
			e.fireCool = fireCoolReset
		} else {
			// Slight delay before re-checking so we don't try to fire
			// every frame while blocked.
			e.fireCool = 0.4
		}
	}
	return false
}

// tickMissileAI flies the guided missile toward the player. It homes
// gently, can be blocked/destroyed by hitting an obstacle, and kills
// the player on contact.
func (p *playScene) tickMissileAI(e *enemy, dt float64) bool {
	if e.life > missileLifetime {
		return true
	}
	toward := shortestDelta(e.pos, p.cam.pos)
	desired := math.Atan2(toward.x, toward.z)
	diff := normalizeAngle(desired - e.yaw)
	step := missileTurnSpeed * dt
	if diff > step {
		diff = step
	} else if diff < -step {
		diff = -step
	}
	e.yaw = normalizeAngle(e.yaw + diff)

	fwd := vec3{x: math.Sin(e.yaw), z: math.Cos(e.yaw)}
	dx := fwd.x * missileSpeed * dt
	dz := fwd.z * missileSpeed * dt
	newPos := vec3{x: wrapWorld(e.pos.x + dx), y: e.pos.y, z: wrapWorld(e.pos.z + dz)}
	// Missile is destroyed by hitting an obstacle.
	if obstacleAt(p.obstacles, newPos, 0.7) {
		p.spawnExplosion(newPos, 0.9)
		return true
	}
	e.pos = newPos
	// Spawn an occasional dust spark behind the missile so the trail
	// reads as motion when the screen is otherwise sparse.
	if p.rng.Float64() < 0.3 {
		back := vec3{x: e.pos.x - fwd.x*0.7, y: e.pos.y + 0.4, z: e.pos.z - fwd.z*0.7}
		p.spawnSpark(back)
	}
	// Hit the player?
	if math.Hypot(toward.x, toward.z) < missileMinDistKill {
		p.spawnExplosion(p.cam.pos, 1.2)
		p.killPlayer()
		return true
	}
	return false
}

// tickSaucerAI drifts the saucer across the playfield. The saucer
// doesn't shoot and doesn't crash on obstacles — it floats above them.
// It despawns after its lifetime expires.
func (p *playScene) tickSaucerAI(e *enemy, dt float64) bool {
	if e.life > saucerLifetime {
		return true
	}
	fwd := vec3{x: math.Sin(e.yaw), z: math.Cos(e.yaw)}
	// Tiny sinusoidal vertical drift makes the saucer "wobble".
	bob := saucerWanderAmp * math.Sin(e.life*1.2)
	e.pos.x = wrapWorld(e.pos.x + fwd.x*saucerSpeed*dt)
	e.pos.z = wrapWorld(e.pos.z + fwd.z*saucerSpeed*dt)
	e.pos.y = saucerHeight + bob
	return false
}

// spawnEnemy chooses a new enemy by kind and places it at a random
// distance from the player. The kind is picked by the play scene's
// progression rules.
func (p *playScene) spawnEnemy(kind enemyKind) {
	// Pick a position around the player at a comfortable distance. We
	// bias spawns toward the front 240° of the player's heading so an
	// enemy can't materialize directly behind and shoot before the
	// player has a chance to find them on radar — a small mercy the
	// original made up for with arcade-cabinet awareness. Distances
	// are chosen so the enemy enters at the radar's outer rim and has
	// to close the gap before firing, giving the player real time to
	// pick a strategy.
	dist := 64.0 + p.rng.Float64()*32.0
	frontHalf := math.Pi * 2.0 / 3.0 // ±120° from facing
	ang := p.cam.yaw + (p.rng.Float64()*2-1)*frontHalf
	pos := vec3{
		x: wrapWorld(p.cam.pos.x + math.Sin(ang)*dist),
		y: 0,
		z: wrapWorld(p.cam.pos.z + math.Cos(ang)*dist),
	}
	// New tank/missile spawn pointed in a random direction — they need
	// to acquire the player first. Original Battlezone enemies didn't
	// teleport pre-aimed either.
	yaw := p.rng.Float64() * 2 * math.Pi
	// Initial fire cooldown is long enough for the player to spot the
	// blip on radar and rotate to face the threat. Subsequent shots
	// from the same enemy use the per-kind reload rate.
	initialCool := 4.0
	switch kind {
	case enemyMissile:
		// Missiles fly higher off the ground and are oriented forward
		// from the start — they're meant to feel urgent, not generous.
		pos.y = 0.6
		towards := shortestDelta(pos, p.cam.pos)
		yaw = math.Atan2(towards.x, towards.z)
	case enemySuperTank:
		// Super tanks reload faster but still need the player to react.
		initialCool = 3.0
	case enemySaucer:
		// Pick a tangential heading so the saucer flies past, not at us.
		towards := shortestDelta(pos, p.cam.pos)
		yaw = math.Atan2(towards.z, -towards.x)
		pos.y = saucerHeight
	}
	p.enemy = &enemy{
		kind:     kind,
		pos:      pos,
		yaw:      yaw,
		fireCool: initialCool,
	}
}

// spawnPlayerShell fires the player's shell. Only one is in flight at
// a time — the play scene must check before calling.
func (p *playScene) spawnPlayerShell() {
	fwd := vec3{x: math.Sin(p.cam.yaw), z: math.Cos(p.cam.yaw)}
	mouth := vec3{
		x: p.cam.pos.x + fwd.x*1.2,
		y: 1.1,
		z: p.cam.pos.z + fwd.z*1.2,
	}
	p.projectiles = append(p.projectiles, &projectile{
		pos:        mouth,
		prev:       mouth,
		vel:        vec3{x: fwd.x * playerShellSpeed, z: fwd.z * playerShellSpeed},
		life:       shellLifetime,
		fromPlayer: true,
	})
}

// spawnEnemyShell fires an enemy tank's shell from the given cannon
// mouth in the given forward direction.
func (p *playScene) spawnEnemyShell(mouth, fwd vec3) {
	p.projectiles = append(p.projectiles, &projectile{
		pos:        mouth,
		prev:       mouth,
		vel:        vec3{x: fwd.x * enemyShellSpeed, z: fwd.z * enemyShellSpeed},
		life:       shellLifetime,
		fromPlayer: false,
	})
}

// playerShellInFlight reports whether the player has an active shell.
// Used to enforce the one-shot-at-a-time limit.
func (p *playScene) playerShellInFlight() bool {
	for _, pr := range p.projectiles {
		if pr.fromPlayer {
			return true
		}
	}
	return false
}

// enemyShellInFlight reports whether an enemy shell is already alive.
// Used so a tank doesn't unload a stream of shells per frame.
func (p *playScene) enemyShellInFlight() bool {
	for _, pr := range p.projectiles {
		if !pr.fromPlayer {
			return true
		}
	}
	return false
}

// obstacleAt reports whether the given world point is inside any
// obstacle's XZ bounding circle, expanded by extraRadius.
func obstacleAt(obstacles []*obstacle, p vec3, extraRadius float64) bool {
	for _, o := range obstacles {
		pos := nearestCopy(p, o.pos)
		dx := p.x - pos.x
		dz := p.z - pos.z
		r := o.radius + extraRadius
		if dx*dx+dz*dz < r*r {
			return true
		}
	}
	return false
}

// pickEnemyKind decides what to spawn next based on the player's score.
// Battlezone's progression introduces super tanks at modest scores and
// missiles once the player has demonstrated some skill; saucers appear
// as rare bonus opportunities throughout.
func pickEnemyKind(rng *rand.Rand, score int) enemyKind {
	// 1-in-10 chance of saucer at any time once the player has cleared
	// at least one foe — pure bonus, doesn't shoot back.
	if score >= scoreTank && rng.Float64() < 0.08 {
		return enemySaucer
	}
	switch {
	case score < 5000:
		return enemyTank
	case score < 10000:
		// Mix in occasional super tanks.
		if rng.Float64() < 0.35 {
			return enemySuperTank
		}
		return enemyTank
	case score < 20000:
		// Add missiles to the mix.
		r := rng.Float64()
		switch {
		case r < 0.55:
			return enemyTank
		case r < 0.85:
			return enemySuperTank
		default:
			return enemyMissile
		}
	default:
		// Late game: super tanks and missiles dominate.
		r := rng.Float64()
		switch {
		case r < 0.35:
			return enemyTank
		case r < 0.7:
			return enemySuperTank
		default:
			return enemyMissile
		}
	}
}
