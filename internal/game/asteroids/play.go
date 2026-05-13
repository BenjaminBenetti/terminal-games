package asteroids

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Positions in canvas pixels; velocities in px/s;
// timers in seconds. Most numbers chosen by feel against the original
// arcade game on a roughly 4:3 display.
const (
	// --- Ship ---------------------------------------------------------
	shipRotateSpeed   = 4.4   // rad/s
	shipThrustAccel   = 60.0  // px/s² along facing
	shipMaxSpeed      = 80.0  // hard cap so terminal field stays sane
	shipDrag          = 0.35  // gentle space drag — keeps the small terminal field playable
	shipFireGap       = 0.16  // min seconds between shots
	shipMaxBullets    = 4     // simultaneous in-flight player bullets
	shipBulletSpeed   = 105.0 // bullet speed in ship's frame (added to ship velocity)
	shipBulletLife    = 0.85
	shipRadius        = 3.2 // collision radius
	shipInvulDur      = 2.6 // seconds of post-respawn invulnerability
	shipRespawnDelay  = 1.4 // delay between explosion finishing and respawn
	shipExplodeDur    = 1.4
	shipHyperRisk     = 0.10 // chance hyperspace kills the ship
	shipHyperLockout  = 1.0  // min seconds between hyperspace jumps

	// --- Asteroids ----------------------------------------------------
	sizeSmall  = 0
	sizeMedium = 1
	sizeLarge  = 2

	// --- Saucers ------------------------------------------------------
	saucerLargeRad    = 4.0
	saucerSmallRad    = 2.6
	saucerLargeSpd    = 18.0
	saucerSmallSpd    = 23.0
	saucerLargeFire   = 1.4
	saucerSmallFire   = 1.1
	saucerLargeScore  = 200
	saucerSmallScore  = 1000
	saucerBulletSpeed = 64.0
	saucerBulletLife  = 1.4
	saucerSpawnMin    = 12.0
	saucerSpawnMax    = 24.0
	saucerDirMin      = 1.8
	saucerDirMax      = 4.2

	// --- Game ---------------------------------------------------------
	waveStartCount   = 4
	waveExtraPerWave = 2
	waveMaxCount     = 11
	waveClearedDelay = 1.6
	bonusLifeEvery   = 10000
	startingLives    = 3
)

// Per-size asteroid properties. Index by sizeSmall / sizeMedium / sizeLarge.
var (
	asteroidRadii   = [3]float64{3.0, 6.0, 10.0}
	asteroidScores  = [3]int{100, 50, 20}
	asteroidSpdMin  = [3]float64{20.0, 14.0, 8.0}
	asteroidSpdMax  = [3]float64{32.0, 24.0, 16.0}
	asteroidColors  = [3]engine.Color{
		{R: 200, G: 200, B: 220, A: 255},
		{R: 220, G: 200, B: 180, A: 255},
		{R: 220, G: 210, B: 170, A: 255},
	}
)

// playState is the gameplay sub-state machine. The top-level scene only
// distinguishes title / play; this state lives inside playScene.
type playState int

const (
	psPlaying    playState = iota
	psShipDying            // ship exploding; if lives remain, transitions to psRespawning
	psRespawning           // dead but center-clear timer running; spawns when safe
	psWaveCleared
	psGameOver
)

// saucerKind distinguishes the two flying-saucer variants.
type saucerKind int

const (
	saucerLarge saucerKind = iota // 200 pts, random shots
	saucerSmall                   // 1000 pts, aimed shots
)

// --- Entities -----------------------------------------------------------

// ship is the player. Position is the centroid; angle 0 == facing +X
// (right); -π/2 == facing up. Movement is Newtonian with a small drag.
type ship struct {
	x, y, vx, vy float64
	angle        float64
	thrust       bool // set each frame from input, drives both physics and render
	cooldown     float64
	hyperCool    float64
	alive        bool // false while exploding / waiting to respawn
	invul        float64
	flameFlick   float64 // sub-frame accumulator for visual thrust flicker
	flameOn      bool
}

// asteroid is one rock. The verts slice stores per-vertex radius
// multipliers around a circle so each asteroid has its own irregular
// silhouette; angle/spin animate that silhouette over time.
type asteroid struct {
	x, y, vx, vy float64
	angle        float64
	spin         float64
	size         int
	verts        []float64
}

// bullet is a projectile. fromPlayer separates player shots from saucer
// shots — they have different collision rules and different colors.
type bullet struct {
	x, y, vx, vy float64
	life         float64
	fromPlayer   bool
}

// saucer is the flying-saucer enemy. The active flag is checked by the
// play loop to know whether to tick it; nil means none on screen.
type saucer struct {
	x, y, vx, vy float64
	kind         saucerKind
	fireT        float64
	dirT         float64 // until next direction change
	hasBullet    *bullet // single live bullet, nil if none
}

// particle is one piece of explosion debris. It moves linearly and fades
// out; positions wrap with the rest of the world.
type particle struct {
	x, y, vx, vy float64
	life         float64
	dur          float64
	color        engine.Color
}

// playScene owns the full match state.
type playScene struct {
	e    *engine.Engine
	w, h int

	ship       ship
	bullets    []*bullet
	asteroids  []*asteroid
	saucer     *saucer
	particles  []particle

	score        int
	hiScore      int
	lives        int
	wave         int
	nextBonus    int // score threshold for next extra life

	state    playState
	stateT   float64

	saucerSpawnT float64 // seconds until next saucer attempt

	rng *rand.Rand

	// Quit signal — top-level scene reads this and drops back to title.
	wantQuit bool
}

// newPlayScene constructs a play scene sized to the engine's canvas.
func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:         e,
		w:         c.Width(),
		h:         c.Height(),
		hiScore:   hiScore,
		lives:     startingLives,
		wave:      0,
		nextBonus: bonusLifeEvery,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.resetShip(true)
	p.startWave(1)
	return p
}

// resetShip moves the ship to canvas centre, stops it, and (if invul=true)
// gives it the usual post-respawn grace period.
func (p *playScene) resetShip(invul bool) {
	p.ship = ship{
		x:     float64(p.w) / 2,
		y:     float64(p.h) / 2,
		angle: -math.Pi / 2, // face up
		alive: true,
	}
	if invul {
		p.ship.invul = shipInvulDur
	}
}

// startWave fills the field with the appropriate number of large
// asteroids for the given wave, far from the ship's spawn point.
func (p *playScene) startWave(wave int) {
	p.wave = wave
	p.state = psPlaying
	p.stateT = 0
	p.bullets = nil
	p.particles = nil
	p.saucer = nil
	p.saucerSpawnT = saucerSpawnMin + p.rng.Float64()*(saucerSpawnMax-saucerSpawnMin)

	n := waveStartCount + (wave-1)*waveExtraPerWave
	if n > waveMaxCount {
		n = waveMaxCount
	}

	// Spawn asteroids at random edge positions so they don't materialize
	// on top of the player. We keep a safe radius around the centre.
	cx := float64(p.w) / 2
	cy := float64(p.h) / 2
	safe := math.Min(float64(p.w), float64(p.h)) * 0.35
	p.asteroids = nil
	for i := 0; i < n; i++ {
		var x, y float64
		for attempts := 0; attempts < 30; attempts++ {
			x = p.rng.Float64() * float64(p.w)
			y = p.rng.Float64() * float64(p.h)
			if math.Hypot(x-cx, y-cy) >= safe {
				break
			}
		}
		p.asteroids = append(p.asteroids, newAsteroid(p.rng, x, y, sizeLarge))
	}
}

// newAsteroid builds an asteroid at (x, y) with the given size class.
// Velocity, rotation, and silhouette are randomised. The wave scales
// the speed range slightly so later waves feel faster.
func newAsteroid(rng *rand.Rand, x, y float64, size int) *asteroid {
	speed := asteroidSpdMin[size] + rng.Float64()*(asteroidSpdMax[size]-asteroidSpdMin[size])
	dir := rng.Float64() * 2 * math.Pi
	a := &asteroid{
		x:     x,
		y:     y,
		vx:    math.Cos(dir) * speed,
		vy:    math.Sin(dir) * speed,
		angle: rng.Float64() * 2 * math.Pi,
		spin:  (rng.Float64()*2 - 1) * 1.2,
		size:  size,
	}
	n := 9 + rng.Intn(4) // 9–12 vertices
	radius := asteroidRadii[size]
	a.verts = make([]float64, n)
	for i := 0; i < n; i++ {
		// Each vertex radius is between 0.7r and 1.15r so the silhouette
		// reads as a chunky rock rather than a circle.
		a.verts[i] = radius * (0.7 + rng.Float64()*0.45)
	}
	return a
}

// --- Update -------------------------------------------------------------

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}

	s := dt.Seconds()
	p.stateT += s

	switch p.state {
	case psPlaying:
		p.tickShip(s)
		p.tickAsteroids(s)
		p.tickBullets(s)
		p.tickSaucer(s)
		p.tickParticles(s)
		p.resolveCollisions()
		if len(p.asteroids) == 0 && p.saucer == nil {
			p.state = psWaveCleared
			p.stateT = 0
		}
	case psShipDying:
		// Field keeps moving so explosions look right.
		p.tickAsteroids(s)
		p.tickBullets(s)
		p.tickSaucer(s)
		p.tickParticles(s)
		// Saucer can still kill us indirectly while we're already dying,
		// but the visible effect is just more particles — we ignore
		// further ship-collision checks while dead.
		p.resolveBulletCollisions()
		if p.stateT >= shipExplodeDur {
			if p.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.state = psRespawning
				p.stateT = 0
			}
		}
	case psRespawning:
		p.tickAsteroids(s)
		p.tickBullets(s)
		p.tickSaucer(s)
		p.tickParticles(s)
		p.resolveBulletCollisions()
		// Wait for the centre to clear AND the respawn delay to expire.
		if p.stateT >= shipRespawnDelay && p.centerClear() {
			p.resetShip(true)
			p.state = psPlaying
			p.stateT = 0
		}
	case psWaveCleared:
		p.tickAsteroids(s)
		p.tickBullets(s)
		p.tickParticles(s)
		// Keep the ship alive but let it drift; nothing to collide with.
		p.tickShip(s)
		if p.stateT >= waveClearedDelay {
			p.startWave(p.wave + 1)
		}
	case psGameOver:
		p.tickAsteroids(s)
		p.tickParticles(s)
		// Wait for the player to acknowledge.
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	return nil
}

// handleInput drains the engine's key queue. Discrete actions (fire,
// hyperspace, quit) are handled here; held movement (rotate, thrust)
// is polled directly in tickShip via IsKeyDown / IsCharDown.
func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psPlaying:
			p.handlePlayKey(k)
		case psShipDying, psRespawning, psWaveCleared:
			if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
			}
		case psGameOver:
			switch k.Code {
			case engine.KeyEnter:
				p.restartMatch()
			case engine.KeyEsc:
				p.wantQuit = true
			case engine.KeyChar:
				switch k.Rune {
				case 'q', 'Q':
					p.wantQuit = true
				case 'r', 'R', ' ':
					p.restartMatch()
				}
			}
		}
	}
}

func (p *playScene) handlePlayKey(k engine.Key) {
	switch k.Code {
	case engine.KeyChar:
		switch k.Rune {
		case ' ':
			p.tryFire()
		case 'h', 'H':
			p.tryHyperspace()
		case 'q', 'Q':
			p.wantQuit = true
		}
	case engine.KeyEsc:
		p.wantQuit = true
	}
}

func (p *playScene) restartMatch() {
	hi := p.hiScore
	*p = playScene{
		e:         p.e,
		w:         p.w,
		h:         p.h,
		hiScore:   hi,
		lives:     startingLives,
		nextBonus: bonusLifeEvery,
		rng:       p.rng,
	}
	p.resetShip(true)
	p.startWave(1)
}

// tryFire spawns a bullet from the ship nose if allowed.
func (p *playScene) tryFire() {
	if !p.ship.alive {
		return
	}
	if p.ship.cooldown > 0 {
		return
	}
	if countPlayerBullets(p.bullets) >= shipMaxBullets {
		return
	}
	// Spawn at the nose; velocity is shipBulletSpeed along facing PLUS the
	// ship's own velocity, so backwards-firing is a real tactical option
	// (a hallmark of the original).
	nx := math.Cos(p.ship.angle)
	ny := math.Sin(p.ship.angle)
	bx := p.ship.x + nx*5
	by := p.ship.y + ny*5
	p.bullets = append(p.bullets, &bullet{
		x:          bx,
		y:          by,
		vx:         p.ship.vx + nx*shipBulletSpeed,
		vy:         p.ship.vy + ny*shipBulletSpeed,
		life:       shipBulletLife,
		fromPlayer: true,
	})
	p.ship.cooldown = shipFireGap
}

// tryHyperspace teleports the ship to a random location with a small
// chance of self-destruct — classic Asteroids' "panic button". Velocity
// resets to zero, which is the original's behaviour.
func (p *playScene) tryHyperspace() {
	if !p.ship.alive {
		return
	}
	if p.ship.hyperCool > 0 {
		return
	}
	p.ship.hyperCool = shipHyperLockout
	if p.rng.Float64() < shipHyperRisk {
		// "Failed" jump — destroy the ship.
		p.killShip()
		return
	}
	// Pick a random spot anywhere on the field. Don't try to be safe —
	// hyperspace is a gamble.
	p.ship.x = p.rng.Float64() * float64(p.w)
	p.ship.y = p.rng.Float64() * float64(p.h)
	p.ship.vx = 0
	p.ship.vy = 0
}

// tickShip advances the ship one frame: rotation, thrust, drag, wrap.
func (p *playScene) tickShip(s float64) {
	if p.ship.cooldown > 0 {
		p.ship.cooldown -= s
	}
	if p.ship.hyperCool > 0 {
		p.ship.hyperCool -= s
	}
	if p.ship.invul > 0 {
		p.ship.invul -= s
	}
	if !p.ship.alive {
		return
	}

	// Rotation is held-key state: simultaneously left+right cancels.
	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	switch {
	case left && !right:
		p.ship.angle -= shipRotateSpeed * s
	case right && !left:
		p.ship.angle += shipRotateSpeed * s
	}

	// Thrust along facing. We accumulate velocity rather than snapping —
	// inertia is the point of the game.
	thrust := p.e.IsKeyDown(engine.KeyUp) || p.e.IsCharDown('w') || p.e.IsCharDown('W')
	p.ship.thrust = thrust
	if thrust {
		p.ship.vx += math.Cos(p.ship.angle) * shipThrustAccel * s
		p.ship.vy += math.Sin(p.ship.angle) * shipThrustAccel * s
		// Cap total speed.
		sp := math.Hypot(p.ship.vx, p.ship.vy)
		if sp > shipMaxSpeed {
			scale := shipMaxSpeed / sp
			p.ship.vx *= scale
			p.ship.vy *= scale
		}
	}

	// Subtle drag — keeps the small terminal field playable without
	// killing the inertia feel.
	dragK := 1.0 - shipDrag*s
	if dragK < 0 {
		dragK = 0
	}
	p.ship.vx *= dragK
	p.ship.vy *= dragK

	p.ship.x = wrapF(p.ship.x+p.ship.vx*s, float64(p.w))
	p.ship.y = wrapF(p.ship.y+p.ship.vy*s, float64(p.h))

	// Flame flicker for the engine plume.
	p.ship.flameFlick += s
	if p.ship.flameFlick > 0.06 {
		p.ship.flameFlick = 0
		p.ship.flameOn = !p.ship.flameOn
	}
}

func (p *playScene) tickAsteroids(s float64) {
	for _, a := range p.asteroids {
		a.x = wrapF(a.x+a.vx*s, float64(p.w))
		a.y = wrapF(a.y+a.vy*s, float64(p.h))
		a.angle += a.spin * s
	}
}

func (p *playScene) tickBullets(s float64) {
	kept := p.bullets[:0]
	for _, b := range p.bullets {
		b.life -= s
		if b.life <= 0 {
			continue
		}
		b.x = wrapF(b.x+b.vx*s, float64(p.w))
		b.y = wrapF(b.y+b.vy*s, float64(p.h))
		kept = append(kept, b)
	}
	p.bullets = kept
}

func (p *playScene) tickSaucer(s float64) {
	if p.saucer == nil {
		p.saucerSpawnT -= s
		if p.saucerSpawnT <= 0 {
			p.spawnSaucer()
		}
		return
	}
	su := p.saucer
	// Move; saucer X wraps when off-screen the trailing way it entered,
	// so it can do multiple passes. Y is clamped roughly to the play area.
	su.x += su.vx * s
	su.y += su.vy * s
	if su.x < -10 || su.x > float64(p.w)+10 {
		// Saucer left the screen; despawn.
		p.saucer = nil
		p.saucerSpawnT = saucerSpawnMin + p.rng.Float64()*(saucerSpawnMax-saucerSpawnMin)
		return
	}
	if su.y < 4 {
		su.y = 4
		su.vy = -su.vy
	}
	if su.y > float64(p.h)-4 {
		su.y = float64(p.h) - 4
		su.vy = -su.vy
	}

	// Periodic direction zig-zag.
	su.dirT -= s
	if su.dirT <= 0 {
		su.dirT = saucerDirMin + p.rng.Float64()*(saucerDirMax-saucerDirMin)
		// Pick a new small vertical component; horizontal direction stays.
		var spd float64
		if su.kind == saucerLarge {
			spd = saucerLargeSpd
		} else {
			spd = saucerSmallSpd
		}
		// Slight angle tilt: ±0.4 rad off horizontal.
		angle := (p.rng.Float64()*0.8 - 0.4)
		dir := 1.0
		if su.vx < 0 {
			dir = -1.0
		}
		su.vx = dir * spd * math.Cos(angle)
		su.vy = spd * math.Sin(angle)
	}

	// Fire.
	su.fireT -= s
	if su.fireT <= 0 {
		p.saucerFire()
		if su.kind == saucerLarge {
			su.fireT = saucerLargeFire
		} else {
			su.fireT = saucerSmallFire
		}
	}
}

func (p *playScene) tickParticles(s float64) {
	kept := p.particles[:0]
	for i := range p.particles {
		pr := &p.particles[i]
		pr.life -= s
		if pr.life <= 0 {
			continue
		}
		pr.x = wrapF(pr.x+pr.vx*s, float64(p.w))
		pr.y = wrapF(pr.y+pr.vy*s, float64(p.h))
		kept = append(kept, *pr)
	}
	p.particles = kept
}

// spawnSaucer chooses a saucer type weighted by current score, picks an
// edge to enter from, and sets initial velocity / timers.
func (p *playScene) spawnSaucer() {
	// Type selection: more small (deadly) saucers as score climbs.
	smallProb := 0.15 + float64(p.score)/40000.0
	if smallProb > 0.85 {
		smallProb = 0.85
	}
	kind := saucerLarge
	if p.rng.Float64() < smallProb {
		kind = saucerSmall
	}

	var spd float64
	var rad float64
	if kind == saucerLarge {
		spd = saucerLargeSpd
		rad = saucerLargeRad
	} else {
		spd = saucerSmallSpd
		rad = saucerSmallRad
	}

	// Enter from a random side.
	var x float64
	dir := 1.0
	if p.rng.Intn(2) == 0 {
		x = -rad
		dir = 1.0
	} else {
		x = float64(p.w) + rad
		dir = -1.0
	}
	// Y somewhere in the middle band so it has room to bounce.
	y := 8 + p.rng.Float64()*float64(p.h-16)

	p.saucer = &saucer{
		x:     x,
		y:     y,
		vx:    dir * spd,
		vy:    0,
		kind:  kind,
		fireT: 0.6 + p.rng.Float64()*0.6,
		dirT:  saucerDirMin + p.rng.Float64()*(saucerDirMax-saucerDirMin),
	}
}

// saucerFire spawns a single bullet from the saucer. Large saucer fires
// randomly; small saucer aims at the ship with a small inaccuracy that
// shrinks as the player's score grows.
func (p *playScene) saucerFire() {
	su := p.saucer
	if su == nil {
		return
	}
	var angle float64
	if su.kind == saucerSmall && p.ship.alive {
		// Aim toward ship — accounting for wrap by picking the shortest
		// vector across torus.
		dx := wrapDelta(p.ship.x-su.x, float64(p.w))
		dy := wrapDelta(p.ship.y-su.y, float64(p.h))
		angle = math.Atan2(dy, dx)
		// Accuracy improves with score; minimum jitter ±0.06 rad.
		jitter := 0.45 - float64(p.score)/35000.0*0.4
		if jitter < 0.06 {
			jitter = 0.06
		}
		angle += (p.rng.Float64()*2 - 1) * jitter
	} else {
		angle = p.rng.Float64() * 2 * math.Pi
	}
	b := &bullet{
		x:          su.x,
		y:          su.y,
		vx:         math.Cos(angle) * saucerBulletSpeed,
		vy:         math.Sin(angle) * saucerBulletSpeed,
		life:       saucerBulletLife,
		fromPlayer: false,
	}
	p.bullets = append(p.bullets, b)
}

// --- Collisions ---------------------------------------------------------

func (p *playScene) resolveCollisions() {
	p.resolveBulletCollisions()
	if !p.ship.alive || p.ship.invul > 0 {
		return
	}
	// Ship vs asteroids.
	for _, a := range p.asteroids {
		if p.circlesOverlap(p.ship.x, p.ship.y, shipRadius, a.x, a.y, asteroidRadii[a.size]*0.85) {
			p.scoreAsteroid(a)
			p.splitAsteroid(a)
			p.killShip()
			return
		}
	}
	// Ship vs saucer.
	if p.saucer != nil {
		srad := saucerLargeRad
		if p.saucer.kind == saucerSmall {
			srad = saucerSmallRad
		}
		if p.circlesOverlap(p.ship.x, p.ship.y, shipRadius, p.saucer.x, p.saucer.y, srad) {
			// Both die. No points for ramming a saucer in the original.
			p.spawnExplosion(p.saucer.x, p.saucer.y, 12, engine.Color{R: 240, G: 120, B: 240, A: 255})
			p.saucer = nil
			p.saucerSpawnT = saucerSpawnMin + p.rng.Float64()*(saucerSpawnMax-saucerSpawnMin)
			p.killShip()
			return
		}
	}
}

// resolveBulletCollisions handles every projectile in flight. Called
// during all states so explosions and saucer fire continue mid-respawn.
func (p *playScene) resolveBulletCollisions() {
	kept := p.bullets[:0]
	for _, b := range p.bullets {
		consumed := false

		// Bullet vs asteroids.
		for i := 0; i < len(p.asteroids); i++ {
			a := p.asteroids[i]
			r := asteroidRadii[a.size]
			if p.circlesOverlap(b.x, b.y, 0.5, a.x, a.y, r*0.85) {
				if b.fromPlayer {
					p.scoreAsteroid(a)
				}
				p.splitAsteroid(a)
				consumed = true
				break
			}
		}
		if consumed {
			continue
		}

		// Bullet vs saucer — only player bullets.
		if b.fromPlayer && p.saucer != nil {
			srad := saucerLargeRad
			score := saucerLargeScore
			if p.saucer.kind == saucerSmall {
				srad = saucerSmallRad
				score = saucerSmallScore
			}
			if p.circlesOverlap(b.x, b.y, 0.5, p.saucer.x, p.saucer.y, srad) {
				p.addScore(score)
				p.spawnExplosion(p.saucer.x, p.saucer.y, 14, engine.Color{R: 240, G: 120, B: 240, A: 255})
				p.saucer = nil
				p.saucerSpawnT = saucerSpawnMin + p.rng.Float64()*(saucerSpawnMax-saucerSpawnMin)
				continue
			}
		}

		// Bullet vs ship — only saucer bullets, and only when vulnerable.
		if !b.fromPlayer && p.ship.alive && p.ship.invul <= 0 {
			if p.circlesOverlap(b.x, b.y, 0.5, p.ship.x, p.ship.y, shipRadius) {
				p.killShip()
				continue
			}
		}

		kept = append(kept, b)
	}
	p.bullets = kept
}

// splitAsteroid replaces a with two smaller asteroids spawned at its
// position, or removes it outright if it's already the smallest. Particle
// debris is spawned regardless so the destruction reads.
func (p *playScene) splitAsteroid(a *asteroid) {
	p.spawnExplosion(a.x, a.y, 6+a.size*3, asteroidColors[a.size])
	// Remove a from the slice.
	idx := -1
	for i, x := range p.asteroids {
		if x == a {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	p.asteroids = append(p.asteroids[:idx], p.asteroids[idx+1:]...)
	if a.size == sizeSmall {
		return
	}
	newSize := a.size - 1
	for i := 0; i < 2; i++ {
		child := newAsteroid(p.rng, a.x, a.y, newSize)
		// Give children some lateral spread so they don't co-travel.
		spread := (p.rng.Float64()*0.6 + 0.3)
		if i == 0 {
			spread = -spread
		}
		ang := math.Atan2(a.vy, a.vx) + spread*math.Pi/3
		spd := math.Hypot(a.vx, a.vy) * 1.15
		// Floor the speed so a near-stationary parent doesn't produce
		// effectively-stationary children.
		minSpd := asteroidSpdMin[newSize]
		if spd < minSpd {
			spd = minSpd
		}
		child.vx = math.Cos(ang) * spd
		child.vy = math.Sin(ang) * spd
		p.asteroids = append(p.asteroids, child)
	}
}

func (p *playScene) scoreAsteroid(a *asteroid) {
	p.addScore(asteroidScores[a.size])
}

func (p *playScene) addScore(delta int) {
	p.score += delta
	for p.score >= p.nextBonus {
		p.lives++
		p.nextBonus += bonusLifeEvery
	}
}

// killShip plays the explosion and burns a life. Whether the player
// gets to respawn is decided when the explosion finishes.
func (p *playScene) killShip() {
	if !p.ship.alive {
		return
	}
	p.ship.alive = false
	p.lives--
	p.spawnShipExplosion()
	p.state = psShipDying
	p.stateT = 0
}

// spawnExplosion appends `count` particles radiating from (x, y).
func (p *playScene) spawnExplosion(x, y float64, count int, color engine.Color) {
	for i := 0; i < count; i++ {
		ang := p.rng.Float64() * 2 * math.Pi
		spd := 18 + p.rng.Float64()*28
		dur := 0.45 + p.rng.Float64()*0.45
		p.particles = append(p.particles, particle{
			x:     x,
			y:     y,
			vx:    math.Cos(ang) * spd,
			vy:    math.Sin(ang) * spd,
			life:  dur,
			dur:   dur,
			color: color,
		})
	}
}

// spawnShipExplosion is the bigger, longer-lasting particle burst when
// the player ship dies. Uses warm colours to read distinctly from rock
// debris.
func (p *playScene) spawnShipExplosion() {
	colors := []engine.Color{
		{R: 255, G: 240, B: 120, A: 255},
		{R: 255, G: 180, B: 80, A: 255},
		{R: 240, G: 90, B: 60, A: 255},
	}
	for i := 0; i < 22; i++ {
		ang := p.rng.Float64() * 2 * math.Pi
		spd := 22 + p.rng.Float64()*45
		dur := 0.7 + p.rng.Float64()*0.7
		col := colors[p.rng.Intn(len(colors))]
		p.particles = append(p.particles, particle{
			x:     p.ship.x,
			y:     p.ship.y,
			vx:    math.Cos(ang) * spd,
			vy:    math.Sin(ang) * spd,
			life:  dur,
			dur:   dur,
			color: col,
		})
	}
}

// centerClear reports whether the area around the canvas centre is free
// of asteroids and saucers and saucer bullets — used to gate respawning.
func (p *playScene) centerClear() bool {
	cx := float64(p.w) / 2
	cy := float64(p.h) / 2
	margin := 14.0
	for _, a := range p.asteroids {
		if p.circlesOverlap(cx, cy, margin, a.x, a.y, asteroidRadii[a.size]) {
			return false
		}
	}
	if p.saucer != nil {
		srad := saucerLargeRad
		if p.saucer.kind == saucerSmall {
			srad = saucerSmallRad
		}
		if p.circlesOverlap(cx, cy, margin, p.saucer.x, p.saucer.y, srad) {
			return false
		}
	}
	for _, b := range p.bullets {
		if b.fromPlayer {
			continue
		}
		if p.circlesOverlap(cx, cy, margin*0.7, b.x, b.y, 1) {
			return false
		}
	}
	return true
}

// circlesOverlap is a toroidal distance check — two entities are
// overlapping if their bounding circles do, taking screen wrap into
// account.
func (p *playScene) circlesOverlap(x1, y1, r1, x2, y2, r2 float64) bool {
	dx := wrapDelta(x1-x2, float64(p.w))
	dy := wrapDelta(y1-y2, float64(p.h))
	sum := r1 + r2
	return dx*dx+dy*dy <= sum*sum
}

// wrapDelta returns the shortest signed distance from b to a on a
// circular axis of length max. The result is in (-max/2, max/2].
func wrapDelta(d, max float64) float64 {
	if max <= 0 {
		return d
	}
	for d > max/2 {
		d -= max
	}
	for d < -max/2 {
		d += max
	}
	return d
}

// wrapF returns v reduced to [0, max) on a torus.
func wrapF(v, max float64) float64 {
	if max <= 0 {
		return v
	}
	for v < 0 {
		v += max
	}
	for v >= max {
		v -= max
	}
	return v
}

// countPlayerBullets returns how many in-flight bullets came from the
// player. Saucer fire doesn't count against the player's limit.
func countPlayerBullets(bs []*bullet) int {
	n := 0
	for _, b := range bs {
		if b.fromPlayer {
			n++
		}
	}
	return n
}

// --- Rendering ----------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Black)

	p.drawAsteroids(c)
	p.drawSaucer(c)
	p.drawBullets(c)
	p.drawParticles(c)
	p.drawShip(c)
	p.drawHUD(c)

	switch p.state {
	case psWaveCleared:
		msg := fmt.Sprintf("WAVE %d", p.wave+1)
		p.drawCentreBanner(c, msg, engine.Yellow)
	case psGameOver:
		p.drawGameOver(c)
	}
}

func (p *playScene) drawAsteroids(c *engine.Canvas) {
	for _, a := range p.asteroids {
		col := asteroidColors[a.size]
		drawWrapped(c, a.x, a.y, asteroidRadii[a.size]+1, func(ox, oy int) {
			drawAsteroidAt(c, a, ox, oy, col)
		})
	}
}

func (p *playScene) drawSaucer(c *engine.Canvas) {
	if p.saucer == nil {
		return
	}
	rad := saucerLargeRad
	col := engine.Color{R: 240, G: 120, B: 240, A: 255}
	if p.saucer.kind == saucerSmall {
		rad = saucerSmallRad
		col = engine.Color{R: 255, G: 90, B: 120, A: 255}
	}
	drawWrapped(c, p.saucer.x, p.saucer.y, rad+1, func(ox, oy int) {
		drawSaucerAt(c, ox, oy, rad, col)
	})
}

func (p *playScene) drawBullets(c *engine.Canvas) {
	playerCol := engine.Color{R: 240, G: 240, B: 240, A: 255}
	saucerCol := engine.Color{R: 250, G: 200, B: 80, A: 255}
	for _, b := range p.bullets {
		col := playerCol
		if !b.fromPlayer {
			col = saucerCol
		}
		// Two-pixel "tracer" for a slightly more visible projectile in
		// terminal pixels.
		c.Set(int(b.x), int(b.y), col)
		c.Set(int(b.x)-int(b.vx*0.01), int(b.y)-int(b.vy*0.01), col)
	}
}

func (p *playScene) drawParticles(c *engine.Canvas) {
	for _, pr := range p.particles {
		// Fade by reducing intensity proportional to remaining life. The
		// terminal doesn't blend, so we approximate by darkening RGB.
		k := pr.life / pr.dur
		if k > 1 {
			k = 1
		}
		col := engine.Color{
			R: uint8(float64(pr.color.R) * k),
			G: uint8(float64(pr.color.G) * k),
			B: uint8(float64(pr.color.B) * k),
			A: 255,
		}
		c.Set(int(pr.x), int(pr.y), col)
	}
}

// drawShip renders the player ship — vector triangle with engine notch,
// optional thrust flame. Invulnerable ships blink. While dying / waiting
// to respawn, nothing is drawn (the explosion particles speak for it).
func (p *playScene) drawShip(c *engine.Canvas) {
	if !p.ship.alive {
		return
	}
	// Invulnerability blink: visible 0.07s, hidden 0.04s.
	if p.ship.invul > 0 {
		cycle := math.Mod(p.ship.invul, 0.18)
		if cycle < 0.07 {
			return
		}
	}
	col := engine.Color{R: 200, G: 240, B: 255, A: 255}
	drawWrapped(c, p.ship.x, p.ship.y, 8, func(ox, oy int) {
		drawShipBody(c, float64(ox), float64(oy), p.ship.angle, col)
		if p.ship.thrust && p.ship.flameOn {
			drawShipFlame(c, float64(ox), float64(oy), p.ship.angle,
				engine.Color{R: 255, G: 160, B: 80, A: 255})
		}
	})
}

// drawHUD draws the score, hi-score, lives indicator (mini ships), and
// wave number across the very top of the screen.
func (p *playScene) drawHUD(c *engine.Canvas) {
	scoreText := fmt.Sprintf("%05d", p.score)
	hiText := "HI " + zeroPad(p.hiScore, 5)
	waveText := fmt.Sprintf("WAVE %d", p.wave)
	cols := c.Cols()

	c.Print(1, 0, scoreText, engine.White)
	c.Print((cols-len(hiText))/2, 0, hiText, engine.Yellow)
	c.Print(cols-len(waveText)-1, 0, waveText, engine.Cyan)

	// Mini ship icons for remaining lives. While the ship is alive it's
	// already counted in p.lives, so we draw (lives - 1) extras. During
	// dying/respawning, p.lives has already been decremented and equals
	// the number of ships left after this one, so draw exactly p.lives.
	count := p.lives
	if p.ship.alive {
		count--
	}
	if count < 0 {
		count = 0
	}
	for i := 0; i < count; i++ {
		x := 4 + i*8
		y := 8
		drawShipBody(c, float64(x), float64(y), -math.Pi/2,
			engine.Color{R: 180, G: 220, B: 240, A: 255})
	}
}

func (p *playScene) drawCentreBanner(c *engine.Canvas, text string, col engine.Color) {
	w := engine.TextWidth(text)
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2
	c.FillRect(x-3, y-2, w+6, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 16, A: 255})
	c.DrawText(x, y, text, col)
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	w := engine.TextWidth("GAME OVER")
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2 - 4
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 16, A: 255})
	c.DrawText(x, y, "GAME OVER", engine.Color{R: 255, G: 80, B: 80, A: 255})
	hint := "ENTER PLAY AGAIN   ESC QUIT"
	c.Print((c.Cols()-len(hint))/2, c.Rows()/2+2, hint, engine.White)
}

// --- Shape primitives ---------------------------------------------------

// drawAsteroidAt draws an asteroid centred at the integer pixel (cx, cy).
// Allows the same asteroid to be drawn at multiple toroidal positions
// when it's near a wrap edge.
func drawAsteroidAt(c *engine.Canvas, a *asteroid, cx, cy int, color engine.Color) {
	n := len(a.verts)
	if n < 3 {
		return
	}
	var prevX, prevY int
	for i := 0; i <= n; i++ {
		idx := i % n
		r := a.verts[idx]
		theta := a.angle + 2*math.Pi*float64(idx)/float64(n)
		px := cx + int(r*math.Cos(theta))
		py := cy + int(r*math.Sin(theta))
		if i > 0 {
			c.DrawLine(prevX, prevY, px, py, color)
		}
		prevX, prevY = px, py
	}
}

// drawShipBody renders the ship outline at (cx, cy) rotated to angle.
// The outline goes nose → tailR → notchR → notchL → tailL → nose so the
// engine notch reads as the classic Asteroids silhouette.
func drawShipBody(c *engine.Canvas, cx, cy, angle float64, color engine.Color) {
	type v struct{ x, y float64 }
	verts := []v{
		{6.0, 0},
		{-3.0, 3.0},
		{-1.5, 1.5},
		{-1.5, -1.5},
		{-3.0, -3.0},
	}
	rot := func(p v) (int, int) {
		rx := p.x*math.Cos(angle) - p.y*math.Sin(angle)
		ry := p.x*math.Sin(angle) + p.y*math.Cos(angle)
		return int(cx + rx), int(cy + ry)
	}
	for i := 0; i < len(verts); i++ {
		x0, y0 := rot(verts[i])
		x1, y1 := rot(verts[(i+1)%len(verts)])
		c.DrawLine(x0, y0, x1, y1, color)
	}
}

// drawShipFlame draws a small triangle behind the ship's rear notch to
// indicate active thrust. Flicker is handled by the caller.
func drawShipFlame(c *engine.Canvas, cx, cy, angle float64, color engine.Color) {
	type v struct{ x, y float64 }
	tail := v{-5.5, 0}
	left := v{-2.5, 1.4}
	right := v{-2.5, -1.4}
	rot := func(p v) (int, int) {
		rx := p.x*math.Cos(angle) - p.y*math.Sin(angle)
		ry := p.x*math.Sin(angle) + p.y*math.Cos(angle)
		return int(cx + rx), int(cy + ry)
	}
	tx, ty := rot(tail)
	lx, ly := rot(left)
	rx, ry := rot(right)
	c.DrawLine(lx, ly, tx, ty, color)
	c.DrawLine(tx, ty, rx, ry, color)
}

// drawSaucerAt renders a flying saucer centred at (cx, cy). radius
// scales the whole shape; the topology is fixed (dome + body + rim).
func drawSaucerAt(c *engine.Canvas, cx, cy int, radius float64, color engine.Color) {
	type ln struct{ x0, y0, x1, y1 float64 }
	lines := []ln{
		// Dome.
		{-0.35, -0.85, 0.35, -0.85},
		{-0.35, -0.85, -0.55, -0.35},
		{0.35, -0.85, 0.55, -0.35},
		// Body upper.
		{-1.00, 0.00, -0.55, -0.35},
		{-0.55, -0.35, 0.55, -0.35},
		{0.55, -0.35, 1.00, 0.00},
		// Body lower.
		{-1.00, 0.00, -0.55, 0.35},
		{-0.55, 0.35, 0.55, 0.35},
		{0.55, 0.35, 1.00, 0.00},
		// Rim.
		{-1.00, 0.00, 1.00, 0.00},
	}
	for _, l := range lines {
		c.DrawLine(
			cx+int(l.x0*radius), cy+int(l.y0*radius),
			cx+int(l.x1*radius), cy+int(l.y1*radius),
			color,
		)
	}
}

// drawWrapped invokes draw for each toroidal copy of an entity whose
// bounding box (centred at x,y with radius r) overlaps the visible
// canvas. This is what makes an asteroid straddling the right edge
// show up on the left simultaneously.
func drawWrapped(c *engine.Canvas, x, y, r float64, draw func(ox, oy int)) {
	w := c.Width()
	h := c.Height()
	for ox := -1; ox <= 1; ox++ {
		for oy := -1; oy <= 1; oy++ {
			cx := x + float64(ox*w)
			cy := y + float64(oy*h)
			if cx+r < 0 || cx-r >= float64(w) {
				continue
			}
			if cy+r < 0 || cy-r >= float64(h) {
				continue
			}
			draw(int(cx), int(cy))
		}
	}
}
