package starcastle

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning. Positions in canvas pixels, velocities in px/s, timers in
// seconds. Hand-picked to match the feel of the original cabinet on
// a small terminal field.
const (
	// --- Ship ---------------------------------------------------------
	shipRotateSpeed  = 4.2  // rad/s
	shipThrustAccel  = 55.0 // px/s² along facing
	shipMaxSpeed     = 70.0
	shipDrag         = 0.35
	shipFireGap      = 0.18 // min seconds between shots
	shipMaxBullets   = 4    // simultaneous in-flight player bullets
	shipBulletSpeed  = 100.0
	shipBulletLife   = 1.4
	shipRadius       = 2.4
	shipInvulDur     = 2.4
	shipRespawnDelay = 1.3
	shipExplodeDur   = 1.4

	// --- Cannon -------------------------------------------------------
	cannonTrackRate  = 1.6  // rad/s — how fast it swivels toward the player
	cannonFirePeriod = 2.6  // base seconds between fireballs
	cannonFireJitter = 0.6  // ± seconds randomization
	mineSpeed        = 16.0 // px/s outward
	mineHomeRate     = 0.9  // rad/s turn rate toward player
	mineLife         = 9.0
	mineRadius       = 1.6
	mineSpawnSpread  = 0.20 // initial angle jitter from cannon facing (radians)
	mineMaxOnScreen  = 4

	// --- Scoring ------------------------------------------------------
	scoreOuterSeg  = 10
	scoreMiddleSeg = 20
	scoreInnerSeg  = 30
	scoreMine      = 100
	scoreCannon    = 1000
	bonusLifeEvery = 10000
	startingLives  = 3
	startingWave   = 1

	// --- Level progression --------------------------------------------
	levelClearedDelay = 2.4 // pause on the "STAGE N CLEAR" banner
)

// scoresPerRing keeps the per-ring point values aligned with the ring
// indices used elsewhere.
var scoresPerRing = [3]int{scoreOuterSeg, scoreMiddleSeg, scoreInnerSeg}

// playState is the gameplay sub-state machine. The outer scene only
// knows title vs play; everything below lives in here.
type playState int

const (
	psPlaying      playState = iota
	psShipDying              // exploding; transitions to respawn or game over
	psRespawning             // dead, waiting for safe spawn
	psLevelCleared           // cannon killed; brief banner, then next level
	psGameOver
)

// ship is the player's vessel. Physics matches Asteroids: rotation,
// thrust along facing, mild drag, hard speed cap, torus wrap.
type ship struct {
	x, y, vx, vy float64
	angle        float64
	thrust       bool
	cooldown     float64
	alive        bool
	invul        float64
	flameFlick   float64
	flameOn      bool
}

// bullet is a player projectile. Bullets have no homing — they're
// linear, time-limited dots that disintegrate on the first segment
// they touch.
type bullet struct {
	x, y, vx, vy float64
	life         float64
}

// mine is the cannon's fireball. Slow, drifts outward, homes mildly
// toward the player's current position. Destroyed by player bullets
// or by collision with a ring segment (it's a ball of fire and goes
// out against the ring's energy field, in the lore).
type mine struct {
	x, y   float64
	vx, vy float64
	life   float64
	spinT  float64 // for the rotating cross visual
	dyingT float64 // brief death-flash before removal; 0 == alive
}

// particle is debris from explosions. Fades by life/dur.
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

	geom geometry

	rings     []ring
	coreAlive bool
	coreAng   float64 // current facing direction
	coreSpin  float64 // additional spin during death animation
	coreT     float64 // generic timer (used for fire scheduling)
	coreFireT float64 // countdown until next attempted fireball

	ship      ship
	bullets   []*bullet
	mines     []*mine
	particles []particle

	score     int
	hiScore   int
	lives     int
	wave      int
	nextBonus int

	state  playState
	stateT float64

	rng *rand.Rand

	// Difficulty scaling factors applied at wave start.
	ringSpeedMul float64
	regenMul     float64
	cannonRateK  float64

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
		nextBonus: bonusLifeEvery,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.geom = computeGeometry(p.w, p.h)
	p.startLevel(startingWave)
	p.resetShip(true)
	return p
}

// startLevel rebuilds the rings and cannon for level n. Speed and
// regen scale per level to keep ramping difficulty after the cannon
// is destroyed and a fresh castle spawns.
func (p *playScene) startLevel(wave int) {
	p.wave = wave
	p.state = psPlaying
	p.stateT = 0
	p.bullets = nil
	p.mines = nil
	p.particles = nil
	// Difficulty curve: rings spin ~10% faster and regen 8% faster per
	// level, capped so it never becomes literally impossible.
	p.ringSpeedMul = math.Min(1.0+0.10*float64(wave-1), 2.0)
	p.regenMul = math.Max(1.0-0.08*float64(wave-1), 0.55)
	p.cannonRateK = math.Max(1.0-0.06*float64(wave-1), 0.50)
	p.rings = make([]ring, numRings)
	for i := 0; i < numRings; i++ {
		r := newRing(i, p.geom)
		r.spinRate *= p.ringSpeedMul
		r.regenDelay *= p.regenMul
		p.rings[i] = r
	}
	p.coreAlive = true
	p.coreAng = -math.Pi / 2 // aim up to start
	p.coreSpin = 0
	p.coreFireT = 1.4 // grace period before first shot
}

// resetShip places the ship at a safe spawn — far enough from the
// castle that it isn't immediately gibbed by a segment. Original
// arcade spawned the player at the bottom-centre of the screen; we
// match that.
func (p *playScene) resetShip(invul bool) {
	p.ship = ship{
		x:     float64(p.w) / 2,
		y:     float64(p.h) - 4, // bottom-centre, like the cabinet
		angle: -math.Pi / 2,     // facing up toward the castle
		alive: true,
	}
	if invul {
		p.ship.invul = shipInvulDur
	}
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
		p.tickRings(s)
		p.tickBullets(s)
		p.tickCannon(s)
		p.tickMines(s)
		p.tickParticles(s)
		p.resolveCollisions()
	case psShipDying:
		// Castle keeps simulating so the field doesn't freeze visually.
		p.tickRings(s)
		p.tickBullets(s)
		p.tickCannon(s)
		p.tickMines(s)
		p.tickParticles(s)
		p.resolveBulletAndMineHits()
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
		p.tickRings(s)
		p.tickBullets(s)
		p.tickCannon(s)
		p.tickMines(s)
		p.tickParticles(s)
		p.resolveBulletAndMineHits()
		if p.stateT >= shipRespawnDelay && p.spawnAreaClear() {
			p.resetShip(true)
			p.state = psPlaying
			p.stateT = 0
		}
	case psLevelCleared:
		p.tickRings(s)
		p.tickBullets(s)
		p.tickParticles(s)
		p.tickShip(s)
		// Spin the dying core faster and faster for drama.
		p.coreSpin += s * 6.0
		if p.stateT >= levelClearedDelay {
			p.startLevel(p.wave + 1)
			// Player keeps their ship and inertia for continuity.
		}
	case psGameOver:
		p.tickRings(s)
		p.tickCannon(s)
		p.tickMines(s)
		p.tickParticles(s)
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	return nil
}

func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psPlaying:
			p.handlePlayKey(k)
		case psShipDying, psRespawning, psLevelCleared:
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
		geom:      p.geom,
		hiScore:   hi,
		lives:     startingLives,
		nextBonus: bonusLifeEvery,
		rng:       p.rng,
	}
	p.startLevel(startingWave)
	p.resetShip(true)
}

// tryFire spawns a bullet from the ship nose if cooldown and bullet
// budget allow. Velocity is shipBulletSpeed along facing PLUS the
// ship's own velocity — kept for tactical backward shots.
func (p *playScene) tryFire() {
	if !p.ship.alive {
		return
	}
	if p.ship.cooldown > 0 {
		return
	}
	if len(p.bullets) >= shipMaxBullets {
		return
	}
	nx := math.Cos(p.ship.angle)
	ny := math.Sin(p.ship.angle)
	bx := p.ship.x + nx*4
	by := p.ship.y + ny*4
	p.bullets = append(p.bullets, &bullet{
		x:    bx,
		y:    by,
		vx:   p.ship.vx + nx*shipBulletSpeed,
		vy:   p.ship.vy + ny*shipBulletSpeed,
		life: shipBulletLife,
	})
	p.ship.cooldown = shipFireGap
}

// --- Ticks --------------------------------------------------------------

func (p *playScene) tickShip(s float64) {
	if p.ship.cooldown > 0 {
		p.ship.cooldown -= s
	}
	if p.ship.invul > 0 {
		p.ship.invul -= s
	}
	if !p.ship.alive {
		return
	}

	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	switch {
	case left && !right:
		p.ship.angle -= shipRotateSpeed * s
	case right && !left:
		p.ship.angle += shipRotateSpeed * s
	}

	thrust := p.e.IsKeyDown(engine.KeyUp) ||
		p.e.IsCharDown('w') || p.e.IsCharDown('W')
	p.ship.thrust = thrust
	if thrust {
		p.ship.vx += math.Cos(p.ship.angle) * shipThrustAccel * s
		p.ship.vy += math.Sin(p.ship.angle) * shipThrustAccel * s
		sp := math.Hypot(p.ship.vx, p.ship.vy)
		if sp > shipMaxSpeed {
			scale := shipMaxSpeed / sp
			p.ship.vx *= scale
			p.ship.vy *= scale
		}
	}

	dragK := 1.0 - shipDrag*s
	if dragK < 0 {
		dragK = 0
	}
	p.ship.vx *= dragK
	p.ship.vy *= dragK

	p.ship.x = wrapF(p.ship.x+p.ship.vx*s, float64(p.w))
	p.ship.y = wrapF(p.ship.y+p.ship.vy*s, float64(p.h))

	p.ship.flameFlick += s
	if p.ship.flameFlick > 0.06 {
		p.ship.flameFlick = 0
		p.ship.flameOn = !p.ship.flameOn
	}
}

func (p *playScene) tickRings(s float64) {
	for i := range p.rings {
		updateRing(&p.rings[i], s)
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

// tickCannon aims the central cannon toward the player and schedules
// fireballs. Aiming is rate-limited so the cannon can't perfectly
// snap to the player — gives the player a window to dodge.
func (p *playScene) tickCannon(s float64) {
	if !p.coreAlive {
		p.coreSpin += s * 8.0
		return
	}
	// Aim toward the (possibly dead) ship. Even during respawn we keep
	// pointing at the last spot so the cannon visual stays alive.
	tx := p.ship.x
	ty := p.ship.y
	desired := math.Atan2(ty-p.geom.cy, tx-p.geom.cx)
	delta := wrapPi(desired - p.coreAng)
	maxTurn := cannonTrackRate * s
	if delta > maxTurn {
		delta = maxTurn
	} else if delta < -maxTurn {
		delta = -maxTurn
	}
	p.coreAng += delta

	// Schedule fireballs. We attempt to fire on a cooldown; if too many
	// mines are already on screen, we skip and try again next tick.
	if p.state == psPlaying {
		p.coreFireT -= s
		if p.coreFireT <= 0 {
			if p.countLiveMines() < mineMaxOnScreen {
				p.cannonFire()
			}
			period := cannonFirePeriod * p.cannonRateK
			jitter := (p.rng.Float64()*2 - 1) * cannonFireJitter
			p.coreFireT = period + jitter
			if p.coreFireT < 0.5 {
				p.coreFireT = 0.5
			}
		}
	}
}

// cannonFire spits a single fireball from the cannon along the
// current aim line. The mine spawns at the cannon's edge (so it's
// visible immediately) and homes mildly toward the player.
func (p *playScene) cannonFire() {
	jitter := (p.rng.Float64()*2 - 1) * mineSpawnSpread
	ang := p.coreAng + jitter
	r := p.geom.coreR + 1.5
	mx := p.geom.cx + math.Cos(ang)*r
	my := p.geom.cy + math.Sin(ang)*r
	p.mines = append(p.mines, &mine{
		x:    mx,
		y:    my,
		vx:   math.Cos(ang) * mineSpeed,
		vy:   math.Sin(ang) * mineSpeed,
		life: mineLife,
	})
}

func (p *playScene) countLiveMines() int {
	n := 0
	for _, m := range p.mines {
		if m.dyingT == 0 {
			n++
		}
	}
	return n
}

// tickMines advances each mine: homes toward the player, dies if it
// hits a segment (mines are weaker than bullets and pop on first
// segment contact). Dying mines fade out their flash before being
// removed.
func (p *playScene) tickMines(s float64) {
	kept := p.mines[:0]
	for _, m := range p.mines {
		if m.dyingT > 0 {
			m.dyingT -= s
			if m.dyingT > 0 {
				kept = append(kept, m)
			}
			continue
		}
		m.life -= s
		if m.life <= 0 {
			continue
		}
		m.spinT += s

		// Home toward player if the ship is alive.
		if p.ship.alive {
			cur := math.Atan2(m.vy, m.vx)
			want := math.Atan2(p.ship.y-m.y, p.ship.x-m.x)
			delta := wrapPi(want - cur)
			maxTurn := mineHomeRate * s
			if delta > maxTurn {
				delta = maxTurn
			} else if delta < -maxTurn {
				delta = -maxTurn
			}
			cur += delta
			sp := math.Hypot(m.vx, m.vy)
			m.vx = math.Cos(cur) * sp
			m.vy = math.Sin(cur) * sp
		}

		m.x += m.vx * s
		m.y += m.vy * s

		// Bounce off the bounds (mines don't wrap — they'd cheat the
		// player by attacking from the back). Clamp and reflect.
		bounced := false
		if m.x < 0 {
			m.x = 0
			m.vx = -m.vx
			bounced = true
		}
		if m.x > float64(p.w-1) {
			m.x = float64(p.w - 1)
			m.vx = -m.vx
			bounced = true
		}
		if m.y < 0 {
			m.y = 0
			m.vy = -m.vy
			bounced = true
		}
		if m.y > float64(p.h-1) {
			m.y = float64(p.h - 1)
			m.vy = -m.vy
			bounced = true
		}
		_ = bounced

		kept = append(kept, m)
	}
	p.mines = kept
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

// --- Collisions ---------------------------------------------------------

func (p *playScene) resolveCollisions() {
	p.resolveBulletAndMineHits()
	if !p.ship.alive || p.ship.invul > 0 {
		return
	}
	// Ship vs any alive segment (sampled at several points around the
	// ship body so the triangle nose registers even before the centre).
	if p.shipTouchesAnyRing() >= 0 {
		p.killShip()
		return
	}
	// Ship vs core.
	if p.coreAlive {
		dx := p.ship.x - p.geom.cx
		dy := p.ship.y - p.geom.cy
		if dx*dx+dy*dy <= (p.geom.coreR+shipRadius)*(p.geom.coreR+shipRadius) {
			p.killShip()
			return
		}
	}
	// Ship vs mine.
	for _, m := range p.mines {
		if m.dyingT > 0 {
			continue
		}
		dx := p.ship.x - m.x
		dy := p.ship.y - m.y
		rr := mineRadius + shipRadius
		if dx*dx+dy*dy <= rr*rr {
			// Take both out.
			m.dyingT = 0.18
			p.spawnExplosion(m.x, m.y, 8,
				engine.Color{R: 255, G: 160, B: 80, A: 255})
			p.killShip()
			return
		}
	}
}

// shipTouchesAnyRing samples a small ring of points around the ship's
// hull and returns the ring index it intersects, or -1 if clear. We
// don't bother awarding score — the ship's own death triggers a small
// explosion either way.
func (p *playScene) shipTouchesAnyRing() int {
	// 6 sample points around the hull plus the centre.
	samples := 6
	for k := 0; k <= samples; k++ {
		var sx, sy float64
		if k == 0 {
			sx, sy = p.ship.x, p.ship.y
		} else {
			a := float64(k-1) * 2 * math.Pi / float64(samples)
			sx = p.ship.x + math.Cos(a)*shipRadius
			sy = p.ship.y + math.Sin(a)*shipRadius
		}
		for i := range p.rings {
			if pointHitsRing(&p.rings[i], p.geom, sx, sy) >= 0 {
				return i
			}
		}
	}
	return -1
}

// resolveBulletAndMineHits handles every projectile in flight. Called
// during play and respawn states so the world keeps simulating while
// the player can't act.
func (p *playScene) resolveBulletAndMineHits() {
	// Player bullets.
	keptB := p.bullets[:0]
	for _, b := range p.bullets {
		consumed := false

		// Bullet vs core (only if there's a clean shot through the
		// rings at the bullet's path). For simplicity, just check
		// distance: if it's inside the core radius, it hit.
		if p.coreAlive {
			dx := b.x - p.geom.cx
			dy := b.y - p.geom.cy
			if dx*dx+dy*dy <= p.geom.coreR*p.geom.coreR {
				p.killCore()
				consumed = true
			}
		}

		// Bullet vs ring segment — closest ring first so an outer
		// segment intercepts before an inner one at the same pixel.
		if !consumed {
			for i := range p.rings {
				idx := pointHitsRing(&p.rings[i], p.geom, b.x, b.y)
				if idx >= 0 {
					destroySegment(&p.rings[i], idx)
					p.rings[i].segments[idx].hitT = segmentHitFlash
					p.addScore(scoresPerRing[i])
					col := ringHitColor(i)
					p.spawnExplosion(b.x, b.y, 4, col)
					consumed = true
					break
				}
			}
		}

		// Bullet vs mine.
		if !consumed {
			for _, m := range p.mines {
				if m.dyingT > 0 {
					continue
				}
				dx := b.x - m.x
				dy := b.y - m.y
				rr := mineRadius + 0.6
				if dx*dx+dy*dy <= rr*rr {
					m.dyingT = 0.18
					p.addScore(scoreMine)
					p.spawnExplosion(m.x, m.y, 10,
						engine.Color{R: 255, G: 160, B: 80, A: 255})
					consumed = true
					break
				}
			}
		}

		if !consumed {
			keptB = append(keptB, b)
		}
	}
	p.bullets = keptB

	// Mines vs ring segments: mines extinguish on the first segment
	// they cross. They're moving outward so this means they protect
	// the player when the rings are intact and break through when
	// gaps line up.
	for _, m := range p.mines {
		if m.dyingT > 0 {
			continue
		}
		for i := range p.rings {
			idx := pointHitsRing(&p.rings[i], p.geom, m.x, m.y)
			if idx >= 0 {
				// Segment is unhurt (mines don't damage the castle),
				// but the mine dies.
				m.dyingT = 0.18
				p.spawnExplosion(m.x, m.y, 6,
					engine.Color{R: 255, G: 130, B: 60, A: 255})
				break
			}
		}
	}
}

// killCore destroys the cannon, plays the celebration, and queues the
// next level.
func (p *playScene) killCore() {
	if !p.coreAlive {
		return
	}
	p.coreAlive = false
	p.addScore(scoreCannon)
	p.state = psLevelCleared
	p.stateT = 0
	// Big explosion at the centre.
	p.spawnExplosion(p.geom.cx, p.geom.cy, 40,
		engine.Color{R: 255, G: 220, B: 90, A: 255})
	p.spawnExplosion(p.geom.cx, p.geom.cy, 22,
		engine.Color{R: 255, G: 110, B: 60, A: 255})
}

func (p *playScene) addScore(delta int) {
	p.score += delta
	for p.score >= p.nextBonus {
		p.lives++
		p.nextBonus += bonusLifeEvery
	}
}

// killShip burns a life and starts the death animation. Whether the
// player gets to respawn is decided once the explosion finishes.
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

// spawnExplosion radiates `count` particles outward from (x, y).
func (p *playScene) spawnExplosion(x, y float64, count int, color engine.Color) {
	for i := 0; i < count; i++ {
		ang := p.rng.Float64() * 2 * math.Pi
		spd := 15 + p.rng.Float64()*30
		dur := 0.4 + p.rng.Float64()*0.5
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

func (p *playScene) spawnShipExplosion() {
	colors := []engine.Color{
		{R: 255, G: 240, B: 120, A: 255},
		{R: 255, G: 180, B: 80, A: 255},
		{R: 240, G: 90, B: 60, A: 255},
		{R: 200, G: 220, B: 255, A: 255},
	}
	for i := 0; i < 26; i++ {
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

// spawnAreaClear gates respawn: the bottom-centre area (where the
// ship comes back) must be free of mines so we don't insta-kill the
// player. We don't need to worry about ring segments — the spawn is
// far enough outside the castle.
func (p *playScene) spawnAreaClear() bool {
	sx := float64(p.w) / 2
	sy := float64(p.h) - 4
	margin := 10.0
	for _, m := range p.mines {
		if m.dyingT > 0 {
			continue
		}
		dx := m.x - sx
		dy := m.y - sy
		if dx*dx+dy*dy < margin*margin {
			return false
		}
	}
	return true
}

// --- Helpers ------------------------------------------------------------

// wrapPi normalizes an angle to (-π, π].
func wrapPi(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a <= -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// wrapF normalizes v into [0, max).
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

// --- Rendering ----------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Black)

	// Rings first, inner→outer so outline pass overlays cleanly.
	for i := numRings - 1; i >= 0; i-- {
		drawRing(c, &p.rings[i], p.geom, ringColor(i))
	}
	for i := numRings - 1; i >= 0; i-- {
		drawRingOutlines(c, &p.rings[i], p.geom, ringOutlineColor(i))
	}

	// Core under everything below it.
	coreCol := engine.Color{R: 255, G: 200, B: 80, A: 255}
	drawCore(c, p.geom, p.coreAng+p.coreSpin, p.coreAlive, coreCol)

	p.drawMines(c)
	p.drawBullets(c)
	p.drawParticles(c)
	p.drawShip(c)
	p.drawHUD(c)

	switch p.state {
	case psLevelCleared:
		p.drawCentreBanner(c, fmt.Sprintf("STAGE %d CLEAR", p.wave), engine.Yellow)
	case psGameOver:
		p.drawGameOver(c)
	}
}

func ringColor(idx int) engine.Color {
	switch idx {
	case ringOuter:
		return engine.Color{R: 100, G: 230, B: 230, A: 255}
	case ringMiddle:
		return engine.Color{R: 90, G: 200, B: 240, A: 255}
	default:
		return engine.Color{R: 130, G: 180, B: 255, A: 255}
	}
}

func ringOutlineColor(idx int) engine.Color {
	c := ringColor(idx)
	// Darken to make the radial dividers read against the filled
	// segments.
	return engine.Color{
		R: c.R / 3,
		G: c.G / 3,
		B: c.B / 3,
		A: 255,
	}
}

func ringHitColor(idx int) engine.Color {
	c := ringColor(idx)
	// Brighter for the burst.
	return engine.Color{
		R: 200 + c.R/5,
		G: 220 + c.G/12,
		B: 220 + c.B/12,
		A: 255,
	}
}

func (p *playScene) drawBullets(c *engine.Canvas) {
	col := engine.Color{R: 240, G: 240, B: 240, A: 255}
	for _, b := range p.bullets {
		c.Set(int(b.x), int(b.y), col)
		// Tracer.
		tx := int(b.x - b.vx*0.012)
		ty := int(b.y - b.vy*0.012)
		c.Set(tx, ty, col)
	}
}

func (p *playScene) drawMines(c *engine.Canvas) {
	hot := engine.Color{R: 255, G: 180, B: 80, A: 255}
	cool := engine.Color{R: 255, G: 100, B: 40, A: 255}
	for _, m := range p.mines {
		col := hot
		if m.dyingT > 0 {
			// Pop in white as it dies.
			col = engine.Color{R: 255, G: 255, B: 240, A: 255}
		}
		// Body — small filled circle.
		c.FillCircle(int(m.x), int(m.y), int(math.Floor(mineRadius)), col)
		// Spinning cross — distinguishes mines from bullet sparks.
		if m.dyingT == 0 {
			ang := m.spinT * 8.0
			r := mineRadius + 1.4
			x0 := m.x + math.Cos(ang)*r
			y0 := m.y + math.Sin(ang)*r
			x1 := m.x - math.Cos(ang)*r
			y1 := m.y - math.Sin(ang)*r
			c.DrawLine(int(x0), int(y0), int(x1), int(y1), cool)
			x2 := m.x + math.Cos(ang+math.Pi/2)*r
			y2 := m.y + math.Sin(ang+math.Pi/2)*r
			x3 := m.x - math.Cos(ang+math.Pi/2)*r
			y3 := m.y - math.Sin(ang+math.Pi/2)*r
			c.DrawLine(int(x2), int(y2), int(x3), int(y3), cool)
		}
	}
}

func (p *playScene) drawParticles(c *engine.Canvas) {
	for _, pr := range p.particles {
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

func (p *playScene) drawShip(c *engine.Canvas) {
	if !p.ship.alive {
		return
	}
	// Invulnerability blink.
	if p.ship.invul > 0 {
		cycle := math.Mod(p.ship.invul, 0.18)
		if cycle < 0.07 {
			return
		}
	}
	col := engine.Color{R: 200, G: 240, B: 255, A: 255}
	drawShipBody(c, p.ship.x, p.ship.y, p.ship.angle, col)
	if p.ship.thrust && p.ship.flameOn {
		drawShipFlame(c, p.ship.x, p.ship.y, p.ship.angle,
			engine.Color{R: 255, G: 160, B: 80, A: 255})
	}
}

// drawShipBody renders the ship triangle at (cx, cy) rotated to angle.
func drawShipBody(c *engine.Canvas, cx, cy, angle float64, color engine.Color) {
	type v struct{ x, y float64 }
	verts := []v{
		{4.0, 0},
		{-2.0, 2.4},
		{-1.0, 0},
		{-2.0, -2.4},
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

func drawShipFlame(c *engine.Canvas, cx, cy, angle float64, color engine.Color) {
	type v struct{ x, y float64 }
	tail := v{-4.5, 0}
	left := v{-1.8, 1.1}
	right := v{-1.8, -1.1}
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

func (p *playScene) drawHUD(c *engine.Canvas) {
	scoreText := fmt.Sprintf("%06d", p.score)
	hiText := "HI " + zeroPad(p.hiScore, 6)
	waveText := fmt.Sprintf("STAGE %d", p.wave)
	cols := c.Cols()

	c.Print(1, 0, scoreText, engine.White)
	c.Print((cols-len(hiText))/2, 0, hiText, engine.Yellow)
	c.Print(cols-len(waveText)-1, 0, waveText, engine.Cyan)

	// Mini ship icons for remaining lives.
	count := p.lives
	if p.ship.alive {
		count--
	}
	if count < 0 {
		count = 0
	}
	for i := 0; i < count; i++ {
		x := 4 + i*7
		y := 8
		drawShipBody(c, float64(x), float64(y), -math.Pi/2,
			engine.Color{R: 180, G: 220, B: 240, A: 255})
	}
}

func (p *playScene) drawCentreBanner(c *engine.Canvas, text string, col engine.Color) {
	w := engine.TextWidth(text)
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2
	c.FillRect(x-3, y-2, w+6, engine.FontHeight+4,
		engine.Color{R: 8, G: 8, B: 16, A: 255})
	c.DrawText(x, y, text, col)
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	w := engine.TextWidth("GAME OVER")
	x := (p.w - w) / 2
	y := (p.h-engine.FontHeight)/2 - 4
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4,
		engine.Color{R: 8, G: 8, B: 16, A: 255})
	c.DrawText(x, y, "GAME OVER",
		engine.Color{R: 255, G: 80, B: 80, A: 255})
	hint := "ENTER PLAY AGAIN   ESC QUIT"
	c.Print((c.Cols()-len(hint))/2, c.Rows()/2+2, hint, engine.White)
}
