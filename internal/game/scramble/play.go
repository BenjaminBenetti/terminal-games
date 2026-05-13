package scramble

import (
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Velocities are in pixels per second; times in
// seconds. Everything that affects "how it feels" lives here so it's
// easy to find when balancing.
const (
	scrollSpdBase = 22.0  // world scroll speed at stage 1
	scrollPerSt   = 1.8   // added px/sec per subsequent stage
	scrollLoopAdd = 4.0   // added px/sec each time we loop the run

	playerHSpd = 30.0 // arrow-key horizontal speed (relative to camera)
	playerVSpd = 38.0 // arrow-key vertical speed

	laserSpd       = 105.0 // world-x velocity of player laser
	laserFireGap   = 0.12  // min seconds between laser shots
	maxLasers      = 3     // simultaneous lasers in flight

	bombGravity    = 70.0  // px/sec^2 downward on a dropped bomb
	bombVxBonus    = 14.0  // forward push relative to camera on drop
	bombFireGap    = 0.20  // min seconds between bomb drops
	maxBombs       = 3     // simultaneous bombs in flight

	rocketLaunchSpd = 36.0 // upward velocity of a launched rocket
	rocketTriggerDx = 80.0 // world-x distance ahead of player that wakes a rocket

	ufoHoverAmp = 8.0  // UFO vertical sinus amplitude
	ufoHoverHz  = 0.6  // UFO vertical sinus frequency in Hz

	towerFireGap = 1.6  // seconds between tower missile launches
	missileSpd   = 38.0 // upward speed of an anti-air missile

	fuelMax    = 100.0
	fuelDrain  = 1.4 // per second
	fuelRefill = 28.0
	fuelLowAt  = 25.0

	playerExplodeDur = 1.2
	playerRespawnDur = 1.4
	stageBannerDur   = 2.0
	victoryDur       = 3.2

	starCount = 30
	starSpeed = 10.0

	// extraLifeAt is the score threshold at which the player earns an
	// additional ship.
	extraLifeAt = 10000
)

// playState is the sub-state machine driven by playScene.
type playState int

const (
	psStageIntro playState = iota
	psPlaying
	psPlayerHit
	psStageCleared
	psVictory
	psGameOver
)

// playScene is the per-run state. One playScene is constructed when the
// player starts a fresh run and survives across all six sectors; only on
// game-over does it return to the title scene.
type playScene struct {
	e    *engine.Engine
	w, h int // canvas size in pixels

	pfTop int // playfield top y
	pfBot int // playfield bottom y

	stage   int
	loop    int     // how many times we've cleared the base
	worldW  int
	cameraX float64

	terrain *terrain

	enemies  []*entity
	missiles []*entity
	bullets  []*playerBulletEntity
	bombs    []*playerBombEntity
	booms    []*entity // explosions (kind = entExplosion)
	stars    []star

	player playerEntity

	fuel      float64
	score     int
	hiScore   int
	extraLife bool // toggled true once the extra-life bonus has been awarded

	state  playState
	stateT float64

	rng *rand.Rand

	// True once the player asks to bail out of the run back to the
	// title screen — the parent scene picks this up after Update.
	wantQuit bool
}

// newPlayScene constructs the play state sized to the engine's canvas.
func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		hiScore: hiScore,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.computeLayout()
	p.player.lives = 3
	p.fuel = fuelMax
	p.spawnStars()
	p.beginStage(1)
	return p
}

// computeLayout pins the playfield region inside the canvas. The HUD
// reserves the top two terminal rows (4 pixels); the playfield runs
// from pfTop (inclusive) to pfBot (exclusive) — pfBot == canvas height
// means the playfield runs all the way to the bottom row.
func (p *playScene) computeLayout() {
	p.pfTop = 4
	p.pfBot = p.h
	if p.h-p.pfTop < 12 {
		// Tiny terminal — give up some HUD to keep the playfield viable.
		p.pfTop = 2
	}
}

// scrollSpd returns the current world scroll speed factoring in stage
// difficulty and how many times we've already looped the base.
func (p *playScene) scrollSpd() float64 {
	return scrollSpdBase + scrollPerSt*float64(p.stage-1) + scrollLoopAdd*float64(p.loop)
}

// stageScrollLockX returns the world-x at which the camera stops
// scrolling for the current stage. For stages 1–5 it's worldW − canvasW
// (i.e. the scroll continues until the far edge is reached). For stage
// 6 the camera locks once the reactor is fully on-screen so the player
// has to destroy it before progressing.
func (p *playScene) stageScrollLockX() float64 {
	end := float64(p.worldW - p.w)
	if p.stage == 6 {
		// Lock so the reactor sits comfortably on the right side of the
		// screen rather than tucked against the very edge.
		end = float64(p.worldW - p.w + 8)
		if end < 0 {
			end = 0
		}
	}
	if end < 0 {
		end = 0
	}
	return end
}

// beginStage resets the world for the given stage number, generating a
// fresh terrain and enemy list. Player position and fuel carry over;
// score and lives carry across stages.
func (p *playScene) beginStage(stage int) {
	p.stage = stage
	p.state = psStageIntro
	p.stateT = 0
	p.cameraX = 0
	p.bullets = nil
	p.bombs = nil
	p.missiles = nil
	p.booms = nil

	p.worldW = stageWorldWidth(stage, p.w)
	p.terrain = newTerrain(stage, p.worldW, p.pfTop, p.pfBot, p.rng)
	p.enemies = populateStage(stage, p.terrain, p.worldW, p.rng)

	p.player.x = float64(p.w) * 0.12
	p.player.y = p.safeRespawnY()
	p.player.cooldownLaser = 0
	p.player.cooldownBomb = 0
	p.player.explodeT = 0
	p.player.respawnT = playerRespawnDur
}

// safeRespawnY finds a y for the player ship that sits inside the
// terrain corridor at the player's current x. It centres the ship in
// the tightest cross-section under the sprite footprint so that very
// uneven terrain (a building face inside the player's column range on
// stage 5, a stalactite in stage 4) still produces a survivable spawn.
func (p *playScene) safeRespawnY() float64 {
	pw := playerSprite.width()
	ph := playerSprite.height()
	worldX := int(p.cameraX + p.player.x)
	minG := p.pfBot
	maxC := p.pfTop - 1
	for x := worldX; x < worldX+pw; x++ {
		g, c := p.terrain.at(x)
		if g < minG {
			minG = g
		}
		if c > maxC {
			maxC = c
		}
	}
	corridorTop := maxC + 1
	corridorBot := minG
	y := (corridorTop + corridorBot - ph) / 2
	if y < p.pfTop+1 {
		y = p.pfTop + 1
	}
	if y+ph > p.pfBot-1 {
		y = p.pfBot - 1 - ph
	}
	return float64(y)
}

// spawnStars seeds the parallax-background star field. Stars live in
// canvas pixel space (not world space) and just scroll horizontally —
// they're vibes, not gameplay.
func (p *playScene) spawnStars() {
	p.stars = make([]star, starCount)
	for i := range p.stars {
		p.stars[i] = star{
			x:     p.rng.Float64() * float64(p.w),
			y:     float64(p.pfTop) + p.rng.Float64()*float64(p.pfBot-p.pfTop)*0.55,
			c:     p.rng.Intn(len(starPalette)),
			twink: p.rng.Float64(),
		}
	}
}

// -------- Update path -----------------------------------------------------

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s
	p.tickStars(s)

	switch p.state {
	case psStageIntro:
		if p.stateT >= stageBannerDur {
			p.state = psPlaying
			p.stateT = 0
		}
	case psPlaying:
		p.updatePlaying(s)
	case psPlayerHit:
		// World scroll continues at a token pace so the explosion plays
		// out in motion, but no spawning or input.
		p.cameraX += p.scrollSpd() * 0.4 * s
		if p.cameraX > p.stageScrollLockX() {
			p.cameraX = p.stageScrollLockX()
		}
		p.tickProjectiles(s)
		p.tickBooms(s)
		p.player.explodeT -= s
		if p.player.explodeT <= 0 {
			if p.player.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.respawnPlayer()
				p.state = psPlaying
				p.stateT = 0
			}
		}
	case psStageCleared:
		if p.stateT >= stageBannerDur {
			if p.stage >= 6 {
				// Already handled by psVictory path; defensive fallback.
				p.loop++
				p.beginStage(1)
			} else {
				p.beginStage(p.stage + 1)
			}
		}
	case psVictory:
		p.tickBooms(s)
		if p.stateT >= victoryDur {
			p.loop++
			p.beginStage(1)
		}
	case psGameOver:
		// wait for player input.
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	if !p.extraLife && p.score >= extraLifeAt {
		p.player.lives++
		p.extraLife = true
	}
	return nil
}

// handleInput drains the press-event queue. Discrete events handle
// firing (laser, bomb) and quit; continuous movement is read from
// IsKeyDown / IsCharDown inside updatePlaying.
func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psPlaying:
			p.handlePlayKey(k)
		case psGameOver:
			if k.Code == engine.KeyEnter ||
				(k.Code == engine.KeyChar && (k.Rune == 'r' || k.Rune == 'R')) {
				hi := p.hiScore
				p.score = 0
				p.player.lives = 3
				p.fuel = fuelMax
				p.extraLife = false
				p.loop = 0
				p.hiScore = hi
				p.beginStage(1)
			} else if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
			}
		default:
			if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
			}
		}
	}
}

func (p *playScene) handlePlayKey(k engine.Key) {
	switch k.Code {
	case engine.KeyEsc:
		p.wantQuit = true
	case engine.KeyChar:
		switch k.Rune {
		case ' ':
			p.tryFireLaser()
		case 'b', 'B', 'z', 'Z', 'x', 'X':
			p.tryDropBomb()
		case 'q', 'Q':
			p.wantQuit = true
		}
	}
}

// tryFireLaser spawns a forward laser if the cool-down has elapsed and
// the player has bullets to spare.
func (p *playScene) tryFireLaser() {
	if p.player.explodeT > 0 {
		return
	}
	if len(p.bullets) >= maxLasers || p.player.cooldownLaser > 0 {
		return
	}
	worldX := p.cameraX + p.player.x + float64(playerSprite.width())
	y := p.player.y + float64(playerSprite.height()/2)
	p.bullets = append(p.bullets, &playerBulletEntity{
		x:  worldX,
		y:  y,
		vx: laserSpd,
	})
	p.player.cooldownLaser = laserFireGap
}

// tryDropBomb spawns a bomb if the cool-down has elapsed.
func (p *playScene) tryDropBomb() {
	if p.player.explodeT > 0 {
		return
	}
	if len(p.bombs) >= maxBombs || p.player.cooldownBomb > 0 {
		return
	}
	worldX := p.cameraX + p.player.x + float64(playerSprite.width()/2)
	y := p.player.y + float64(playerSprite.height())
	p.bombs = append(p.bombs, &playerBombEntity{
		x:  worldX,
		y:  y,
		vx: p.scrollSpd() + bombVxBonus,
		vy: 6.0,
	})
	p.player.cooldownBomb = bombFireGap
}

// respawnPlayer restores the player ship at a safe centre-left location
// in the playfield with the standard invincibility blink. Called from
// Update once the explosion animation completes.
func (p *playScene) respawnPlayer() {
	p.player.x = float64(p.w) * 0.12
	p.player.y = p.safeRespawnY()
	p.player.explodeT = 0
	p.player.respawnT = playerRespawnDur
	p.player.cooldownLaser = 0
	p.player.cooldownBomb = 0
	if p.fuel < 30 {
		p.fuel = 30
	}
}

// updatePlaying is the main gameplay tick.
func (p *playScene) updatePlaying(s float64) {
	// Camera scroll.
	maxCam := p.stageScrollLockX()
	if p.cameraX < maxCam {
		p.cameraX += p.scrollSpd() * s
		if p.cameraX > maxCam {
			p.cameraX = maxCam
		}
	}

	// Fuel drain. Running dry kills the player like terrain impact.
	p.fuel -= fuelDrain * s
	if p.fuel <= 0 {
		p.fuel = 0
		p.killPlayer()
		return
	}

	// Player motion.
	if p.player.respawnT > 0 {
		p.player.respawnT -= s
	}
	p.movePlayer(s)
	if p.player.cooldownLaser > 0 {
		p.player.cooldownLaser -= s
	}
	if p.player.cooldownBomb > 0 {
		p.player.cooldownBomb -= s
	}

	// Held-key shortcuts so the player can hold space / b instead of
	// tapping. Cool-down logic in tryFire/tryDrop keeps this honest.
	if p.e.IsCharDown(' ') {
		p.tryFireLaser()
	}
	if p.e.IsCharDown('b') || p.e.IsCharDown('B') ||
		p.e.IsCharDown('z') || p.e.IsCharDown('Z') ||
		p.e.IsCharDown('x') || p.e.IsCharDown('X') {
		p.tryDropBomb()
	}

	p.tickEnemies(s)
	p.tickProjectiles(s)
	p.tickBooms(s)

	p.resolveCollisions()

	// Stage clear conditions.
	if p.stage == 6 {
		if !p.reactorAlive() && p.state == psPlaying {
			p.state = psVictory
			p.stateT = 0
			// Add a final pyrotechnics burst.
			p.spawnBoom(p.cameraX+float64(p.w)*0.75, float64(p.pfTop+(p.pfBot-p.pfTop)/2))
		}
	} else if p.state == psPlaying && p.cameraX >= maxCam-0.001 &&
		p.player.x+float64(playerSprite.width()) >= float64(p.w)-1 {
		// The original auto-advances when the player flies off the right
		// edge of the level. Once the camera reaches its scroll lock the
		// player can press forward to commit to the next stage.
		p.state = psStageCleared
		p.stateT = 0
	}
}

// movePlayer applies arrow-key motion clamped to the playfield, and
// checks for terrain collision at the new position.
func (p *playScene) movePlayer(s float64) {
	if p.player.explodeT > 0 {
		return
	}
	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	up := p.e.IsKeyDown(engine.KeyUp) || p.e.IsCharDown('w') || p.e.IsCharDown('W')
	down := p.e.IsKeyDown(engine.KeyDown) || p.e.IsCharDown('s') || p.e.IsCharDown('S')

	var dx, dy float64
	switch {
	case left && !right:
		dx = -playerHSpd
	case right && !left:
		dx = playerHSpd
	}
	switch {
	case up && !down:
		dy = -playerVSpd
	case down && !up:
		dy = playerVSpd
	}
	p.player.x += dx * s
	p.player.y += dy * s

	// Clamp to screen / playfield bounds.
	pw, ph := float64(playerSprite.width()), float64(playerSprite.height())
	if p.player.x < 0 {
		p.player.x = 0
	}
	if p.player.x > float64(p.w)-pw {
		p.player.x = float64(p.w) - pw
	}
	if p.player.y < float64(p.pfTop) {
		p.player.y = float64(p.pfTop)
	}
	if p.player.y > float64(p.pfBot)-ph {
		p.player.y = float64(p.pfBot) - ph
	}

	// Terrain collision. Respawn blink doesn't grant terrain immunity —
	// crashing into a mountain is unambiguous.
	worldX := int(p.cameraX + p.player.x)
	if p.terrain.hits(worldX, int(p.player.y),
		worldX+int(pw), int(p.player.y+ph)) {
		p.killPlayer()
	}
}

// tickEnemies steps each enemy's animation and behaviour. Off-screen
// (left of camera) enemies are removed.
func (p *playScene) tickEnemies(s float64) {
	kept := p.enemies[:0]
	for _, e := range p.enemies {
		// Discard anything that's drifted off-screen behind us.
		_, _, x1, _ := e.bbox()
		if float64(x1) < p.cameraX-12 {
			continue
		}
		// Skip activity for entities far ahead of the camera; this keeps
		// fireballs/UFOs from oscillating uselessly.
		if e.x > p.cameraX+float64(p.w)+40 && e.kind != entReactor {
			kept = append(kept, e)
			continue
		}
		e.frameT += s
		if e.frameT >= 0.25 {
			e.frameT -= 0.25
			e.frame = 1 - e.frame
		}
		switch e.kind {
		case entRocket:
			if !e.launched {
				// Wake when the player is within trigger range to the
				// right of this rocket.
				playerWorldX := p.cameraX + p.player.x + float64(playerSprite.width()/2)
				if playerWorldX >= e.x-rocketTriggerDx && playerWorldX < e.x+8 {
					e.launched = true
				}
			} else {
				e.y -= rocketLaunchSpd * s
				if e.y < float64(p.pfTop)-8 {
					e.alive = false
				}
			}
		case entUFO:
			e.x += e.vx * s
			// Sinusoidal sway around the spawned y.
			e.cooldown += s
			e.y += math.Sin(e.cooldown*ufoHoverHz*2*math.Pi) * ufoHoverAmp * s
			// Don't let them slide off the top.
			if e.y < float64(p.pfTop)+1 {
				e.y = float64(p.pfTop) + 1
			}
		case entFireball:
			e.x += e.vx * s
			e.y += e.vy * s
			if e.y > float64(p.pfBot)+4 {
				e.alive = false
			}
		case entFuel:
			// Stationary.
		case entTower:
			e.cooldown += s
			// Fire when the tower is on screen and the cool-down has
			// expired. Cool-down resets each shot.
			if e.cooldown > towerFireGap &&
				e.x > p.cameraX-4 && e.x < p.cameraX+float64(p.w)+4 {
				e.cooldown = 0
				mx := e.x + float64(baseTower.width()/2) - float64(missile.width()/2)
				my := e.y - float64(missile.height())
				p.missiles = append(p.missiles, &entity{
					kind:  entMissile,
					x:     mx,
					y:     my,
					vy:    -missileSpd,
					alive: true,
				})
			}
		case entReactor:
			// Floats; small idle vertical bob.
			e.cooldown += s
			e.y += math.Sin(e.cooldown*0.4*2*math.Pi) * 4 * s
		}
		if e.alive {
			kept = append(kept, e)
		}
	}
	p.enemies = kept
}

// reactorAlive reports whether the reactor entity is still in the list.
// Destroyed reactors fall out of p.enemies on the next tick.
func (p *playScene) reactorAlive() bool {
	for _, e := range p.enemies {
		if e.kind == entReactor && e.alive {
			return true
		}
	}
	return false
}

// tickProjectiles advances all player bullets, bombs, and enemy missiles.
func (p *playScene) tickProjectiles(s float64) {
	// Player lasers.
	kb := p.bullets[:0]
	for _, b := range p.bullets {
		b.x += b.vx * s
		// Drop if off-right or behind the camera.
		if b.x > p.cameraX+float64(p.w)+8 || b.x < p.cameraX-8 {
			continue
		}
		// Terrain hits absorb the laser silently — no spark FX, matches
		// Scramble where the laser is line-of-sight only.
		if p.terrain.hits(int(b.x), int(b.y), int(b.x)+playerBullet.width(), int(b.y)+playerBullet.height()) {
			continue
		}
		kb = append(kb, b)
	}
	p.bullets = kb

	// Player bombs.
	kbb := p.bombs[:0]
	for _, b := range p.bombs {
		b.vy += bombGravity * s
		b.x += b.vx * s
		b.y += b.vy * s
		if b.y > float64(p.pfBot)+4 || b.x < p.cameraX-8 {
			continue
		}
		// Terrain impact — produce a small explosion at the impact site.
		if p.terrain.hits(int(b.x), int(b.y), int(b.x)+playerBomb.width(), int(b.y)+playerBomb.height()) {
			p.spawnBoom(b.x, b.y)
			continue
		}
		kbb = append(kbb, b)
	}
	p.bombs = kbb

	// Enemy missiles (tower → player).
	km := p.missiles[:0]
	for _, m := range p.missiles {
		m.y += m.vy * s
		// Bend slightly toward the player x for that anti-air feel.
		playerCX := p.cameraX + p.player.x + float64(playerSprite.width()/2)
		if m.x < playerCX {
			m.x += 6 * s
		} else if m.x > playerCX {
			m.x -= 6 * s
		}
		if m.y < float64(p.pfTop)-4 {
			continue
		}
		// Off-screen left → drop.
		_, _, x1, _ := m.bbox()
		if float64(x1) < p.cameraX-8 {
			continue
		}
		km = append(km, m)
	}
	p.missiles = km
}

// tickBooms drives explosion-animation timers and prunes expired ones.
func (p *playScene) tickBooms(s float64) {
	kept := p.booms[:0]
	for _, b := range p.booms {
		b.dieT += s
		if b.dieT < 0.45 {
			kept = append(kept, b)
		}
	}
	p.booms = kept
}

// tickStars walks the parallax field. Stars wrap around the screen; they
// don't have world positions because they're decorative.
func (p *playScene) tickStars(s float64) {
	for i := range p.stars {
		p.stars[i].x -= starSpeed * s
		p.stars[i].twink += s * 3
		if p.stars[i].x < 0 {
			p.stars[i].x = float64(p.w)
			p.stars[i].y = float64(p.pfTop) +
				p.rng.Float64()*float64(p.pfBot-p.pfTop)*0.55
			p.stars[i].c = p.rng.Intn(len(starPalette))
		}
	}
}

// spawnBoom creates an explosion at world coordinate (x, y).
func (p *playScene) spawnBoom(x, y float64) {
	p.booms = append(p.booms, &entity{
		kind:  entExplosion,
		x:     x - float64(explode0.width()/2),
		y:     y - float64(explode0.height()/2),
		alive: true,
	})
}

// -------- Collisions -----------------------------------------------------

func (p *playScene) playerWorldBBox() (x0, y0, x1, y1 int) {
	x0 = int(p.cameraX + p.player.x)
	y0 = int(p.player.y)
	x1 = x0 + playerSprite.width()
	y1 = y0 + playerSprite.height()
	return
}

func aabbOverlap(ax0, ay0, ax1, ay1, bx0, by0, bx1, by1 int) bool {
	return ax0 < bx1 && ax1 > bx0 && ay0 < by1 && ay1 > by0
}

func (p *playScene) resolveCollisions() {
	if p.player.explodeT > 0 {
		return
	}

	// Lasers vs enemies.
	for _, e := range p.enemies {
		if !e.alive {
			continue
		}
		ex0, ey0, ex1, ey1 := e.bbox()
		hitIdx := -1
		for i, b := range p.bullets {
			bx0 := int(b.x)
			by0 := int(b.y)
			bx1 := bx0 + playerBullet.width()
			by1 := by0 + playerBullet.height()
			if aabbOverlap(bx0, by0, bx1, by1, ex0, ey0, ex1, ey1) {
				hitIdx = i
				break
			}
		}
		if hitIdx >= 0 {
			p.bullets = append(p.bullets[:hitIdx], p.bullets[hitIdx+1:]...)
			p.applyHit(e, hitKindLaser)
		}
	}

	// Bombs vs enemies. Bombs can also hit airborne enemies, but are
	// mainly for ground-pounding (fuel tanks, rockets, towers).
	for _, e := range p.enemies {
		if !e.alive {
			continue
		}
		ex0, ey0, ex1, ey1 := e.bbox()
		hitIdx := -1
		for i, b := range p.bombs {
			bx0 := int(b.x)
			by0 := int(b.y)
			bx1 := bx0 + playerBomb.width()
			by1 := by0 + playerBomb.height()
			if aabbOverlap(bx0, by0, bx1, by1, ex0, ey0, ex1, ey1) {
				hitIdx = i
				break
			}
		}
		if hitIdx >= 0 {
			p.bombs = append(p.bombs[:hitIdx], p.bombs[hitIdx+1:]...)
			p.applyHit(e, hitKindBomb)
		}
	}

	// Player vs enemies (ramming) and player vs missiles (anti-air).
	px0, py0, px1, py1 := p.playerWorldBBox()
	for _, e := range p.enemies {
		if !e.alive {
			continue
		}
		ex0, ey0, ex1, ey1 := e.bbox()
		if aabbOverlap(px0, py0, px1, py1, ex0, ey0, ex1, ey1) {
			// Touching the reactor or fuel tank doesn't kill the player
			// from contact — fuel tanks just refuel on bomb hit; the
			// reactor takes damage from lasers. Other contacts kill.
			if e.kind == entFuel || e.kind == entReactor {
				continue
			}
			p.killPlayer()
			// Also kill the enemy involved in the collision.
			p.applyHit(e, hitKindRam)
			return
		}
	}
	for _, m := range p.missiles {
		mx0, my0, mx1, my1 := m.bbox()
		if aabbOverlap(px0, py0, px1, py1, mx0, my0, mx1, my1) {
			m.alive = false
			p.killPlayer()
			return
		}
	}
	// Prune dead missiles.
	kept := p.missiles[:0]
	for _, m := range p.missiles {
		if m.alive {
			kept = append(kept, m)
		}
	}
	p.missiles = kept
}

// hitKind disambiguates why applyHit was called — different weapons
// score differently on the same target (e.g. bombs refuel fuel tanks).
type hitKind int

const (
	hitKindLaser hitKind = iota
	hitKindBomb
	hitKindRam
)

// applyHit applies damage / scoring / FX to enemy e from a hit of the
// given kind. Most enemies die in one hit; the reactor takes multiple.
func (p *playScene) applyHit(e *entity, hk hitKind) {
	switch e.kind {
	case entRocket:
		p.score += rocketScore(e.launched)
		p.spawnBoom(e.x+float64(rocketIdle.width()/2), e.y+float64(rocketIdle.height()/2))
		e.alive = false
	case entUFO:
		p.score += 100
		p.spawnBoom(e.x+float64(ufoA.width()/2), e.y+float64(ufoA.height()/2))
		e.alive = false
	case entFireball:
		p.score += 50
		p.spawnBoom(e.x+float64(fireballA.width()/2), e.y+float64(fireballA.height()/2))
		e.alive = false
	case entFuel:
		// Bombs refuel; lasers just destroy without refuel.
		if hk == hitKindBomb {
			p.fuel += fuelRefill
			if p.fuel > fuelMax {
				p.fuel = fuelMax
			}
			p.score += 150
		} else if hk == hitKindLaser {
			p.score += 100
		}
		p.spawnBoom(e.x+float64(fuelTank.width()/2), e.y+float64(fuelTank.height()/2))
		e.alive = false
	case entTower:
		p.score += 200
		p.spawnBoom(e.x+float64(baseTower.width()/2), e.y+float64(baseTower.height()/2))
		e.alive = false
	case entReactor:
		// Reactor takes 3 hits.
		e.hits++
		// Splash explosion at the hit site.
		p.spawnBoom(e.x+float64(reactor.width()/2)+float64(p.rng.Intn(6)-3),
			e.y+float64(reactor.height()/2)+float64(p.rng.Intn(4)-2))
		if e.hits >= 3 {
			p.score += 800
			e.alive = false
			// Big finale boom on the reactor itself.
			for i := 0; i < 5; i++ {
				p.spawnBoom(e.x+float64(p.rng.Intn(reactor.width())),
					e.y+float64(p.rng.Intn(reactor.height())))
			}
		} else {
			p.score += 100
		}
	}
}

func rocketScore(launched bool) int {
	if launched {
		return 80
	}
	return 50
}

// killPlayer triggers the player explosion sequence. lives is decremented
// here so HUD updates immediately; respawn (or game-over transition)
// happens once the explosion timer expires.
func (p *playScene) killPlayer() {
	if p.player.explodeT > 0 || p.state != psPlaying {
		return
	}
	p.player.lives--
	p.player.explodeT = playerExplodeDur
	p.spawnBoom(p.cameraX+p.player.x+float64(playerSprite.width()/2),
		p.player.y+float64(playerSprite.height()/2))
	p.state = psPlayerHit
	p.stateT = 0
}
