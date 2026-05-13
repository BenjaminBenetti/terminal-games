package vanguard

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tunable constants. Speeds in pixels/sec, durations in seconds, etc.
const (
	playerSpeed       = 36.0
	playerBulletSpeed = 110.0
	playerFireGap     = 0.20 // per-direction cool-down
	maxBulletsPerDir  = 3

	enemyBulletSpeed = 36.0
	maxEnemyBullets  = 16

	playerExplodeDur = 1.4
	playerRespawnDur = 1.2
	playerInvulDur   = 1.6 // post-respawn / post-pod blink invuln

	// Energy meter — drains constantly. Hits 0 = lose a life.
	energyDrainRate = 0.045 // fraction per second
	podRefillAmt    = 1.0   // pod tops up to full
	podPoweredDur   = 6.0   // seconds of invuln + auto-fire after pod

	zoneIntroDur = 2.0
	zoneClearDur = 1.6
	gondDeathDur = 2.4

	// Spawn pacing.
	podSpawnGap = 12.0

	starCount = 50
	starSpeed = 8.0
)

// rect is an inclusive-exclusive AABB in canvas pixel coordinates.
type rect struct{ x0, y0, x1, y1 int }

func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && r.x1 > o.x0 && r.y0 < o.y1 && r.y1 > o.y0
}

// playState is the play-scene sub-state machine.
type playState int

const (
	psZoneIntro playState = iota // 2 s "MOUNTAIN ZONE" banner
	psPlaying                    // normal scrolling gameplay
	psZoneClear                  // brief banner before next zone
	psBossEntry                  // Gond rolling on screen
	psBossFight                  // Gond combat
	psBossDying                  // Gond exploding
	psStageWon                   // brief banner before loop
	psPlayerHit                  // player exploded
	psGameOver
)

// fireDir indices match how cool-downs and the WASD keymap line up.
const (
	fireUp    = 0
	fireDown  = 1
	fireLeft  = 2
	fireRight = 3
)

// playerEntity holds all per-frame mutable state for the ship.
type playerEntity struct {
	x, y     float64
	energy   float64    // 0..1
	lives    int
	powered  float64    // pod-active seconds remaining
	invul    float64    // generic invuln (post-respawn)
	fireCD   [4]float64 // per-direction cool-down
	explodeT float64    // >0 mid-explosion
	respawnT float64    // >0 freshly respawned
}

// bullet is a single projectile travelling at constant velocity.
// fromPlayer flips collision categories — player bullets hit enemies,
// enemy bullets hit the player.
type bullet struct {
	x, y       float64
	vx, vy     float64
	fromPlayer bool
}

// energyPod is a collectible drifting with the world scroll. The pod's
// position is in canvas pixels and is advanced each frame by the world
// scroll velocity (so the pod looks "stuck" to the world).
type energyPod struct {
	x, y      float64
	bob       float64
	collected bool
}

// star is a single parallax pixel — purely decorative.
type star struct {
	x, y, twink float64
	c           engine.Color
}

// playScene owns the gameplay loop. It implements engine.Scene's
// methods through its parent scene.
type playScene struct {
	e    *engine.Engine
	w, h int

	// Layout (pixel coords).
	hudH    int
	playTop int
	playH   int

	// Zone progression / scrolling.
	zoneIdx   int
	zone      zoneConfig
	zoneT     float64
	worldOff  float64 // sub-pixel scroll accumulator
	worldOffI int     // integer scroll position used for terrainAt
	loop      int

	// Entities.
	player       playerEntity
	bullets      []*bullet
	enemyBullets []*bullet
	enemies      []*enemy
	pods         []*energyPod
	stars        []star
	gond         *gondBoss

	// Spawn timers.
	enemySpawnT float64
	podSpawnT   float64

	// HUD/score.
	score, hiScore int

	state    playState
	stateT   float64
	rng      *rand.Rand
	wantQuit bool

	// Game-over banner timer.
	gameOverT float64
}

// newPlayScene constructs a play scene sized to the engine canvas.
func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		hiScore: hiScore,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.hudH = 4 // 2 cells × 2 pixels
	p.playTop = p.hudH
	p.playH = p.h - p.hudH - 2 // leave 1 cell at the bottom for the energy bar
	p.player.lives = 3
	p.player.energy = 1.0
	p.spawnStars()
	p.beginZone(0)
	return p
}

// --- World setup -------------------------------------------------------

// beginZone resets per-zone state (timers, scroll offset, enemy lists)
// and queues the intro banner.
func (p *playScene) beginZone(idx int) {
	p.zoneIdx = idx
	p.zone = zoneOrder[idx]
	p.zoneT = 0
	p.worldOff = 0
	p.worldOffI = 0
	p.enemySpawnT = 0
	p.podSpawnT = 0
	p.enemies = nil
	p.bullets = nil
	p.enemyBullets = nil
	p.pods = nil
	p.gond = nil
	p.state = psZoneIntro
	p.stateT = 0

	// Place the player at a reasonable starting position relative to
	// the scroll axis — middle-left for horizontal zones, bottom-centre
	// for vertical zones.
	pw := playerShip.width()
	ph := playerShip.height()
	switch p.zone.axis {
	case scrollHoriz:
		p.player.x = float64(p.w / 6)
		p.player.y = float64(p.playTop + (p.playH-ph)/2)
	case scrollVert:
		p.player.x = float64((p.w - pw) / 2)
		p.player.y = float64(p.playTop + p.playH - ph - 2)
	}
	p.player.respawnT = playerRespawnDur
	p.player.invul = playerInvulDur
	for i := range p.player.fireCD {
		p.player.fireCD[i] = 0
	}
	p.player.explodeT = 0
}

// spawnStars seeds the parallax stars. Stars are not zone-specific —
// they keep a sense of motion through every zone (subtle in the lit
// zones, more visible in Bleak).
func (p *playScene) spawnStars() {
	p.stars = make([]star, starCount)
	for i := range p.stars {
		p.stars[i] = star{
			x:     p.rng.Float64() * float64(p.w),
			y:     p.rng.Float64() * float64(p.h),
			c:     starPalette[p.rng.Intn(len(starPalette))],
			twink: p.rng.Float64() * math.Pi * 2,
		}
	}
}

// --- Update path -------------------------------------------------------

// Update is called by the parent engine.Scene.
func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s
	p.tickStars(s)

	switch p.state {
	case psZoneIntro:
		if p.stateT >= zoneIntroDur {
			p.state = psPlaying
			p.stateT = 0
		}
	case psPlaying:
		p.updatePlaying(s)
	case psZoneClear:
		if p.stateT >= zoneClearDur {
			p.beginZone(p.zoneIdx + 1)
		}
	case psBossEntry:
		p.tickPlayer(s)
		p.tickBullets(s)
		if p.gond == nil {
			p.gond = newGond(p)
		}
		p.gond.entryY += 12 * s
		if p.gond.entryY >= p.gond.targetY {
			p.gond.entryY = p.gond.targetY
			p.state = psBossFight
			p.stateT = 0
		}
	case psBossFight:
		p.updateBossFight(s)
	case psBossDying:
		p.tickPlayer(s)
		p.tickBullets(s)
		if p.stateT >= gondDeathDur {
			p.state = psStageWon
			p.stateT = 0
		}
	case psStageWon:
		if p.stateT >= zoneClearDur*1.5 {
			p.loop++
			p.beginZone(0)
		}
	case psPlayerHit:
		p.tickEnemies(s)
		p.tickBullets(s)
		p.player.explodeT -= s
		if p.player.explodeT <= 0 {
			if p.player.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.respawnPlayer()
				if p.gond != nil && p.gond.alive() {
					p.state = psBossFight
				} else {
					p.state = psPlaying
				}
				p.stateT = 0
			}
		}
	case psGameOver:
		p.gameOverT += s
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	return nil
}

// updatePlaying drives the standard scrolling-gameplay tick.
func (p *playScene) updatePlaying(s float64) {
	p.zoneT += s

	// Scroll the world. worldOff is real-valued; worldOffI is the
	// integer position queries pass to terrainAt.
	p.worldOff += p.zone.scrollSpd * s * p.difficultyScale()
	p.worldOffI = int(p.worldOff)

	p.tickPlayer(s)
	p.tickEnemies(s)
	p.tickBullets(s)
	p.tickPods(s)
	p.tickSpawns(s)
	p.resolveBulletEnemyHits()
	p.resolveEnemyPlayerHits()
	p.resolveBulletPlayerHits()
	p.resolvePodPickups()
	p.resolveTerrainCrush()

	// Energy drain — a small-but-steady pressure to keep moving.
	p.player.energy -= energyDrainRate * s * p.difficultyScale()
	if p.player.energy <= 0 {
		p.player.energy = 0
		// Dead by starvation — same as a normal death (without the bomb).
		p.killPlayer()
		return
	}

	// Powered timer.
	if p.player.powered > 0 {
		p.player.powered -= s
		if p.player.powered < 0 {
			p.player.powered = 0
		}
	}

	// Zone transitions. Styx ends in a Gond fight; the others just chain.
	if p.zoneT >= p.zone.duration {
		if p.zone.kind == zoneStyx {
			p.state = psBossEntry
			p.stateT = 0
		} else {
			p.state = psZoneClear
			p.stateT = 0
		}
	}
}

// difficultyScale returns the speed/spawn multiplier for the current
// loop. Each completed loop bumps the world ~10% faster, capped to keep
// the late game playable.
func (p *playScene) difficultyScale() float64 {
	scale := 1.0 + 0.10*float64(p.loop)
	if scale > 1.6 {
		scale = 1.6
	}
	return scale
}

// --- Input -------------------------------------------------------------

func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psPlaying, psBossFight, psBossEntry:
			p.handlePlayKey(k)
		case psPlayerHit, psZoneIntro, psZoneClear, psBossDying, psStageWon:
			if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
			}
		case psGameOver:
			if k.Code == engine.KeyEnter ||
				(k.Code == engine.KeyChar && (k.Rune == 'r' || k.Rune == 'R')) {
				hi := p.hiScore
				p.score = 0
				p.player.lives = 3
				p.player.energy = 1.0
				p.player.powered = 0
				p.loop = 0
				p.hiScore = hi
				p.gameOverT = 0
				p.beginZone(0)
			} else if k.Code == engine.KeyEsc ||
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
		if k.Rune == 'q' || k.Rune == 'Q' {
			p.wantQuit = true
		}
	}
}

// tickPlayer handles 8-way movement (arrow keys), 4-direction firing
// (WASD), and movement clamping against terrain + bounds.
func (p *playScene) tickPlayer(s float64) {
	if p.player.explodeT > 0 {
		return
	}
	// Tick respawn / invuln blink-down.
	if p.player.respawnT > 0 {
		p.player.respawnT -= s
	}
	if p.player.invul > 0 {
		p.player.invul -= s
	}
	for i := range p.player.fireCD {
		if p.player.fireCD[i] > 0 {
			p.player.fireCD[i] -= s
		}
	}

	// Movement: 8-way via held arrow keys.
	left := p.e.IsKeyDown(engine.KeyLeft)
	right := p.e.IsKeyDown(engine.KeyRight)
	up := p.e.IsKeyDown(engine.KeyUp)
	down := p.e.IsKeyDown(engine.KeyDown)
	dx, dy := 0.0, 0.0
	if left && !right {
		dx = -1
	} else if right && !left {
		dx = 1
	}
	if up && !down {
		dy = -1
	} else if down && !up {
		dy = 1
	}
	// Normalise diagonals.
	if dx != 0 && dy != 0 {
		inv := 1.0 / math.Sqrt2
		dx *= inv
		dy *= inv
	}
	speed := playerSpeed
	if p.player.powered > 0 {
		speed *= 1.15
	}
	newX := p.player.x + dx*speed*s
	newY := p.player.y + dy*speed*s
	pw := float64(playerShip.width())
	ph := float64(playerShip.height())
	if newX < 0 {
		newX = 0
	}
	if newX > float64(p.w)-pw {
		newX = float64(p.w) - pw
	}
	topMin := float64(p.playTop)
	if newY < topMin {
		newY = topMin
	}
	if newY > float64(p.playTop+p.playH)-ph {
		newY = float64(p.playTop+p.playH) - ph
	}
	// Terrain collision: walls block movement. We test the would-be
	// new bounding box against the terrain mask. If blocked on one
	// axis, we keep the un-blocked axis movement (so you slide along
	// walls rather than getting stuck).
	if p.terrainBlocks(newX, p.player.y, pw, ph) {
		newX = p.player.x
	}
	if p.terrainBlocks(newX, newY, pw, ph) {
		newY = p.player.y
	}
	p.player.x = newX
	p.player.y = newY

	// Firing — WASD presses fire bullets in those directions. Powered
	// state auto-fires every direction continuously.
	w := p.e.IsCharDown('w') || p.e.IsCharDown('W')
	a := p.e.IsCharDown('a') || p.e.IsCharDown('A')
	sd := p.e.IsCharDown('s') || p.e.IsCharDown('S')
	d := p.e.IsCharDown('d') || p.e.IsCharDown('D')
	if p.player.powered > 0 {
		w, a, sd, d = true, true, true, true
	}
	if w {
		p.tryFire(fireUp)
	}
	if sd {
		p.tryFire(fireDown)
	}
	if a {
		p.tryFire(fireLeft)
	}
	if d {
		p.tryFire(fireRight)
	}
}

// tryFire spawns a bullet in the requested direction if the per-
// direction cool-down and global per-direction bullet cap allow.
func (p *playScene) tryFire(dir int) {
	if p.player.fireCD[dir] > 0 {
		return
	}
	count := 0
	for _, b := range p.bullets {
		if !b.fromPlayer {
			continue
		}
		switch dir {
		case fireUp:
			if b.vy < 0 {
				count++
			}
		case fireDown:
			if b.vy > 0 {
				count++
			}
		case fireLeft:
			if b.vx < 0 {
				count++
			}
		case fireRight:
			if b.vx > 0 {
				count++
			}
		}
	}
	if count >= maxBulletsPerDir {
		return
	}
	pw := float64(playerShip.width())
	ph := float64(playerShip.height())
	cx := p.player.x + pw/2
	cy := p.player.y + ph/2
	var bx, by, vx, vy float64
	switch dir {
	case fireUp:
		bx, by = cx, p.player.y - float64(playerBulletV.height())
		vy = -playerBulletSpeed
	case fireDown:
		bx, by = cx, p.player.y+ph
		vy = playerBulletSpeed
	case fireLeft:
		bx, by = p.player.x - float64(playerBulletH.width()), cy
		vx = -playerBulletSpeed
	case fireRight:
		bx, by = p.player.x+pw, cy
		vx = playerBulletSpeed
	}
	p.bullets = append(p.bullets, &bullet{x: bx, y: by, vx: vx, vy: vy, fromPlayer: true})
	p.player.fireCD[dir] = playerFireGap
}

// tickBullets advances every bullet (player + enemy) and culls anything
// off-screen or into terrain.
func (p *playScene) tickBullets(s float64) {
	kept := p.bullets[:0]
	for _, b := range p.bullets {
		b.x += b.vx * s
		b.y += b.vy * s
		if b.x < -2 || b.y < -2 ||
			b.x > float64(p.w)+2 || b.y > float64(p.h)+2 {
			continue
		}
		// Player bullets stop on terrain (so you can't shoot through walls
		// in Mountain). Enemy bullets ignore terrain — they're traveling
		// toward the player so terrain protects you naturally if you tuck
		// behind it.
		if b.fromPlayer && p.terrainBlocks(b.x, b.y, 1, 1) {
			continue
		}
		kept = append(kept, b)
	}
	p.bullets = kept

	keptE := p.enemyBullets[:0]
	for _, b := range p.enemyBullets {
		b.x += b.vx * s
		b.y += b.vy * s
		if b.x < -2 || b.y < -2 ||
			b.x > float64(p.w)+2 || b.y > float64(p.h)+2 {
			continue
		}
		keptE = append(keptE, b)
	}
	p.enemyBullets = keptE
}

// tickEnemies advances each enemy AI, prunes off-screen ones, and
// queues any bombs they fired into enemyBullets.
func (p *playScene) tickEnemies(s float64) {
	kept := p.enemies[:0]
	for _, e := range p.enemies {
		switch e.state {
		case esActive:
			fired, fx, fy := p.tickEnemy(e, s)
			if fired && len(p.enemyBullets) < maxEnemyBullets {
				p.fireEnemyBullet(e, fx, fy)
			}
			// Wall-locked Styx turrets ride the world scroll DOWN the
			// screen (toward the player) along with the wall behind them.
			if e.kind == ekStyxTurret && p.zone.axis == scrollVert {
				e.y += p.zone.scrollSpd * s * p.difficultyScale()
			}
			// Cull when they wander far off-screen on the leaving side.
			if e.x < -20 || e.x > float64(p.w)+20 ||
				e.y < float64(p.playTop)-20 ||
				e.y > float64(p.playTop+p.playH)+20 {
				e.state = esGone
			}
		case esDying:
			e.dyingT += s
			if e.dyingT >= 0.45 {
				e.state = esGone
			}
		}
		if e.state != esGone {
			kept = append(kept, e)
		}
	}
	p.enemies = kept
}

// fireEnemyBullet emits a bomb travelling toward the player from the
// enemy's reported origin point. Direction is biased toward the player
// for hover-shooters; straight-down for fast movers.
func (p *playScene) fireEnemyBullet(e *enemy, fx, fy float64) {
	pw := float64(playerShip.width())
	ph := float64(playerShip.height())
	dx := (p.player.x + pw/2) - fx
	dy := (p.player.y + ph/2) - fy
	dist := math.Hypot(dx, dy) + 0.001
	speed := enemyBulletSpeed
	switch e.kind {
	case ekKemleyL, ekKemleyR:
		// Doesn't fire (handled at AI level — just in case).
		return
	case ekHelm, ekFloater, ekBringer:
		// Aimed shots.
	case ekStyxTurret:
		// Aimed.
	case ekBear:
		// Sprayed straight down for a "swipe".
		dx, dy = 0, 1
		dist = 1
	}
	vx := dx / dist * speed
	vy := dy / dist * speed
	p.enemyBullets = append(p.enemyBullets, &bullet{
		x: fx, y: fy, vx: vx, vy: vy, fromPlayer: false,
	})
}

// tickPods slides each energy pod with the world scroll and culls ones
// that have left the play area. Horizontal zones scroll right-to-left
// (pods drift left, off-screen). Vertical zones scroll the world DOWN
// the screen toward the player at the bottom (pods drift down).
func (p *playScene) tickPods(s float64) {
	scrollX := 0.0
	scrollY := 0.0
	switch p.zone.axis {
	case scrollHoriz:
		scrollX = -p.zone.scrollSpd * s * p.difficultyScale()
	case scrollVert:
		scrollY = p.zone.scrollSpd * s * p.difficultyScale()
	}
	kept := p.pods[:0]
	for _, pod := range p.pods {
		if pod.collected {
			continue
		}
		pod.x += scrollX
		pod.y += scrollY
		pod.bob += s
		if pod.x < -10 || pod.x > float64(p.w)+10 ||
			pod.y < float64(p.playTop)-10 ||
			pod.y > float64(p.playTop+p.playH)+10 {
			continue
		}
		kept = append(kept, pod)
	}
	p.pods = kept
}

// tickStars updates parallax star positions; stars wrap on either axis.
func (p *playScene) tickStars(s float64) {
	axis := scrollHoriz
	if p.state == psBossFight || p.state == psBossEntry || p.state == psBossDying {
		axis = scrollVert
	} else {
		axis = p.zone.axis
	}
	for i := range p.stars {
		switch axis {
		case scrollHoriz:
			p.stars[i].x -= starSpeed * s
			if p.stars[i].x < 0 {
				p.stars[i].x = float64(p.w)
				p.stars[i].y = p.rng.Float64() * float64(p.h)
			}
		case scrollVert:
			p.stars[i].y += starSpeed * s
			if p.stars[i].y >= float64(p.h) {
				p.stars[i].y = 0
				p.stars[i].x = p.rng.Float64() * float64(p.w)
			}
		}
		p.stars[i].twink += s * 4
	}
}

// tickSpawns runs the per-zone spawn rules. Each zone has its own
// pacing — Mountain trickles Kemleys in from the edges, Stripe sends
// Bears, Bleak sends Floaters, Rainbow drops Dancers, Styx sets up
// turrets along the gates.
func (p *playScene) tickSpawns(s float64) {
	p.enemySpawnT += s
	p.podSpawnT += s

	// Energy pods on a schedule across all zones.
	if p.podSpawnT >= podSpawnGap {
		p.podSpawnT = 0
		p.spawnPod()
	}

	switch p.zone.kind {
	case zoneMountain:
		if p.enemySpawnT >= 1.4 {
			p.enemySpawnT = 0
			r := p.rng.Float64()
			switch {
			case r < 0.55:
				p.spawnKemley()
			case r < 0.80:
				p.spawnHelm()
			default:
				p.spawnBringer()
			}
		}
	case zoneStripe:
		if p.enemySpawnT >= 1.6 {
			p.enemySpawnT = 0
			if p.rng.Float64() < 0.65 {
				p.spawnBear()
			} else {
				p.spawnHelm()
			}
		}
	case zoneBleak:
		if p.enemySpawnT >= 1.5 {
			p.enemySpawnT = 0
			if p.rng.Float64() < 0.7 {
				p.spawnFloater()
			} else {
				p.spawnBringer()
			}
		}
	case zoneRainbow:
		if p.enemySpawnT >= 0.9 {
			p.enemySpawnT = 0
			p.spawnDancer()
		}
	case zoneStyx:
		if p.enemySpawnT >= 2.2 {
			p.enemySpawnT = 0
			p.spawnStyxTurret()
		}
	}
}

// --- Spawn helpers -----------------------------------------------------

// edgeY picks a y inside the play area, biased away from the very top
// so the new enemy doesn't immediately clip into the HUD line.
func (p *playScene) edgeY() float64 {
	margin := 4.0
	span := float64(p.playH) - 2*margin
	return float64(p.playTop) + margin + p.rng.Float64()*span
}

// edgeX picks an x inside the play area, biased away from the edges.
func (p *playScene) edgeX() float64 {
	margin := 6.0
	span := float64(p.w) - 2*margin
	return margin + p.rng.Float64()*span
}

func (p *playScene) spawnKemley() {
	if p.rng.Intn(2) == 0 {
		// From left.
		e := spawnEnemy(ekKemleyL, -float64(kemleyA.width()), p.edgeY(), p.zoneT)
		p.enemies = append(p.enemies, e)
	} else {
		e := spawnEnemy(ekKemleyR, float64(p.w), p.edgeY(), p.zoneT)
		p.enemies = append(p.enemies, e)
	}
}

func (p *playScene) spawnHelm() {
	x := p.edgeX()
	y := float64(p.playTop) + 2
	p.enemies = append(p.enemies, spawnEnemy(ekHelm, x, y, p.zoneT))
}

func (p *playScene) spawnBringer() {
	x := float64(p.w) // enter from right
	y := p.edgeY()
	p.enemies = append(p.enemies, spawnEnemy(ekBringer, x, y, p.zoneT))
}

func (p *playScene) spawnBear() {
	x := float64(p.w)
	y := p.edgeY()
	p.enemies = append(p.enemies, spawnEnemy(ekBear, x, y, p.zoneT))
}

func (p *playScene) spawnFloater() {
	x := float64(p.w)
	y := p.edgeY()
	p.enemies = append(p.enemies, spawnEnemy(ekFloater, x, y, p.zoneT))
}

func (p *playScene) spawnDancer() {
	x := p.edgeX()
	y := float64(p.playTop) - 4
	p.enemies = append(p.enemies, spawnEnemy(ekDancer, x, y, p.zoneT))
}

func (p *playScene) spawnStyxTurret() {
	// Anchor against either left or right wall, near the top of the
	// visible area so it has time to fire before scrolling off.
	side := p.rng.Intn(2)
	margin := 4.0
	var x float64
	if side == 0 {
		x = margin
	} else {
		x = float64(p.w) - margin - float64(helmA.width())
	}
	y := float64(p.playTop) + 4
	e := spawnEnemy(ekStyxTurret, x, y, p.zoneT)
	p.enemies = append(p.enemies, e)
}

func (p *playScene) spawnPod() {
	pod := &energyPod{}
	switch p.zone.axis {
	case scrollHoriz:
		pod.x = float64(p.w) + 4
		pod.y = p.edgeY()
	case scrollVert:
		pod.x = p.edgeX()
		pod.y = float64(p.playTop) - 4
	}
	p.pods = append(p.pods, pod)
}

// --- Collisions --------------------------------------------------------

func (p *playScene) playerRect() rect {
	if p.player.explodeT > 0 {
		return rect{}
	}
	pw := playerShip.width()
	ph := playerShip.height()
	return rect{
		x0: int(p.player.x),
		y0: int(p.player.y),
		x1: int(p.player.x) + pw,
		y1: int(p.player.y) + ph,
	}
}

func (p *playScene) resolveBulletEnemyHits() {
	if len(p.bullets) == 0 {
		return
	}
	for _, e := range p.enemies {
		if !e.alive() {
			continue
		}
		eb := e.boundingBox()
		hitIdx := -1
		for i, b := range p.bullets {
			if !b.fromPlayer {
				continue
			}
			br := bulletRect(b)
			if br.overlaps(eb) {
				hitIdx = i
				break
			}
		}
		if hitIdx < 0 {
			continue
		}
		// Remove the bullet that hit.
		p.bullets = append(p.bullets[:hitIdx], p.bullets[hitIdx+1:]...)
		e.hp--
		if e.hp <= 0 {
			p.score += e.kind.score()
			e.state = esDying
			e.dyingT = 0
		}
	}
}

func (p *playScene) resolveEnemyPlayerHits() {
	if p.player.explodeT > 0 || p.player.invul > 0 || p.player.powered > 0 {
		return
	}
	pr := p.playerRect()
	for _, e := range p.enemies {
		if !e.alive() {
			continue
		}
		if pr.overlaps(e.boundingBox()) {
			// Score the enemy as a kill (it crashed into us).
			p.score += e.kind.score()
			e.state = esDying
			e.dyingT = 0
			p.killPlayer()
			return
		}
	}
}

func (p *playScene) resolveBulletPlayerHits() {
	if p.player.explodeT > 0 || p.player.invul > 0 || p.player.powered > 0 {
		return
	}
	pr := p.playerRect()
	keep := p.enemyBullets[:0]
	hit := false
	for _, b := range p.enemyBullets {
		if hit {
			keep = append(keep, b)
			continue
		}
		br := bulletRect(b)
		if br.overlaps(pr) {
			hit = true
			continue
		}
		keep = append(keep, b)
	}
	p.enemyBullets = keep
	if hit {
		p.killPlayer()
	}
}

// resolveTerrainCrush kills the player if they're currently embedded
// in a wall — happens when the wall sweeps into them faster than they
// dodge out (Styx gates, Bleak pillars, narrow Mountain passages). The
// invuln window suppresses this so players don't insta-die on respawn.
func (p *playScene) resolveTerrainCrush() {
	if p.player.explodeT > 0 || p.player.invul > 0 || p.player.powered > 0 {
		return
	}
	pw := float64(playerShip.width())
	ph := float64(playerShip.height())
	if p.terrainBlocks(p.player.x, p.player.y, pw, ph) {
		p.killPlayer()
	}
}

func (p *playScene) resolvePodPickups() {
	pr := p.playerRect()
	for _, pod := range p.pods {
		if pod.collected {
			continue
		}
		pr2 := rect{
			x0: int(pod.x),
			y0: int(pod.y),
			x1: int(pod.x) + energyPodA.width(),
			y1: int(pod.y) + energyPodA.height(),
		}
		if pr.overlaps(pr2) {
			pod.collected = true
			p.player.energy = podRefillAmt
			p.player.powered = podPoweredDur
			p.score += 1000
		}
	}
}

func bulletRect(b *bullet) rect {
	w, h := 1, 1
	if b.fromPlayer {
		if b.vx != 0 {
			w = playerBulletH.width()
			h = playerBulletH.height()
		} else {
			w = playerBulletV.width()
			h = playerBulletV.height()
		}
	} else {
		w = enemyBulletA.width()
		h = enemyBulletA.height()
	}
	return rect{
		x0: int(b.x),
		y0: int(b.y),
		x1: int(b.x) + w,
		y1: int(b.y) + h,
	}
}

// killPlayer advances the death state machine. We don't immediately
// transition to game-over; the explosion plays out for ~1.4 s and then
// either respawns or transitions to game over.
func (p *playScene) killPlayer() {
	if p.player.explodeT > 0 {
		return
	}
	p.player.lives--
	p.player.powered = 0
	p.player.explodeT = playerExplodeDur
	p.state = psPlayerHit
	p.stateT = 0
}

// respawnPlayer re-centres the ship and grants invuln blink.
func (p *playScene) respawnPlayer() {
	pw := playerShip.width()
	ph := playerShip.height()
	switch p.zone.axis {
	case scrollHoriz:
		p.player.x = float64(p.w / 6)
		p.player.y = float64(p.playTop + (p.playH-ph)/2)
	case scrollVert:
		p.player.x = float64((p.w - pw) / 2)
		p.player.y = float64(p.playTop + p.playH - ph - 2)
	}
	p.player.energy = 0.6
	p.player.respawnT = playerRespawnDur
	p.player.invul = playerInvulDur
	p.player.explodeT = 0
}

// terrainBlocks returns true if any pixel of the supplied AABB sits in
// terrain wall. The check rasterises the rect into the terrain mask
// for the current zone.
func (p *playScene) terrainBlocks(x, y, w, h float64) bool {
	x0 := int(x)
	y0 := int(y)
	x1 := int(x + w - 1)
	y1 := int(y + h - 1)
	if y1 < p.playTop {
		y0 = p.playTop
	}
	if y0 < p.playTop {
		y0 = p.playTop
	}
	if y1 >= p.playTop+p.playH {
		y1 = p.playTop + p.playH - 1
	}
	switch p.zone.axis {
	case scrollHoriz:
		for cx := x0; cx <= x1; cx++ {
			world := cx + p.worldOffI
			ts := terrainAt(p.zone, world, p.w, p.playH)
			topPx := p.playTop + ts.nearH
			botPx := p.playTop + p.playH - ts.farH
			if y0 < topPx || y1 >= botPx {
				return true
			}
		}
	case scrollVert:
		for cy := y0; cy <= y1; cy++ {
			world := p.vertWorldRow(cy)
			ts := terrainAt(p.zone, world, p.w, p.playH)
			leftPx := ts.nearH
			rightPx := p.w - ts.farH
			if x0 < leftPx || x1 >= rightPx {
				return true
			}
		}
	}
	return false
}

// --- Boss fight --------------------------------------------------------

func (p *playScene) updateBossFight(s float64) {
	p.tickPlayer(s)
	p.tickBullets(s)
	if p.gond != nil {
		p.gond.update(p, s)
	}
	p.resolveBulletGondHits()
	p.resolveBulletPlayerHits()
	if p.gond != nil && !p.gond.alive() {
		p.score += 5000
		p.state = psBossDying
		p.stateT = 0
	}
}

func (p *playScene) resolveBulletGondHits() {
	if p.gond == nil || !p.gond.alive() {
		return
	}
	gr := p.gond.coreRect()
	hitIdx := -1
	for i, b := range p.bullets {
		if !b.fromPlayer {
			continue
		}
		br := bulletRect(b)
		if br.overlaps(gr) {
			hitIdx = i
			break
		}
	}
	if hitIdx >= 0 {
		p.bullets = append(p.bullets[:hitIdx], p.bullets[hitIdx+1:]...)
		p.gond.hp--
		p.gond.hitFlashT = 0.18
	}
}

// --- Rendering ---------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(p.zone.bg)
	p.drawBackground(c)
	p.drawTerrain(c)
	p.drawPods(c)
	p.drawEnemies(c)
	p.drawBullets(c)
	p.drawPlayer(c)
	if p.gond != nil {
		p.gond.draw(c, p)
	}
	p.drawHUD(c)
	p.drawEnergyBar(c)

	switch p.state {
	case psZoneIntro:
		p.drawCentreText(c, p.zone.name, p.zone.accent)
	case psZoneClear:
		p.drawCentreText(c, "ZONE CLEAR", p.zone.accent)
	case psStageWon:
		p.drawCentreText(c, "GOND DESTROYED", engine.Color{R: 240, G: 240, B: 90, A: 255})
	case psGameOver:
		p.drawGameOver(c)
	}
}

// drawBackground paints the per-zone backdrop. Most zones use the
// flat bg from zoneConfig (already cleared), then add stripes / stars.
func (p *playScene) drawBackground(c *engine.Canvas) {
	// Stars first — universal subtle layer.
	for _, st := range p.stars {
		col := st.c
		if math.Sin(st.twink) < -0.5 {
			col = engine.Color{R: col.R / 3, G: col.G / 3, B: col.B / 3, A: 255}
		}
		c.Set(int(st.x), int(st.y), col)
	}
	switch p.zone.kind {
	case zoneStripe:
		// Vertical bands of colour scrolling by.
		for x := 0; x < p.w; x++ {
			world := x + p.worldOffI
			col := stripeStripeColor(world)
			// Dim the bands so the gameplay reads on top.
			col = engine.Color{R: col.R / 4, G: col.G / 4, B: col.B / 4, A: 255}
			for y := p.playTop; y < p.playTop+p.playH; y++ {
				c.Set(x, y, col)
			}
		}
	case zoneRainbow:
		// Horizontal bands scrolling toward the player (down the screen).
		for y := p.playTop; y < p.playTop+p.playH; y++ {
			world := p.vertWorldRow(y)
			col := rainbowStripeColor(world)
			// Dim the bands so foreground gameplay reads on top.
			col = engine.Color{R: col.R / 4, G: col.G / 4, B: col.B / 4, A: 255}
			for x := 0; x < p.w; x++ {
				c.Set(x, y, col)
			}
		}
	case zoneBleak:
		// Sparse coloured dots scattered through the play area.
		for x := 0; x < p.w; x++ {
			world := x + p.worldOffI
			if col, ok := bleakDot(world); ok {
				y := p.playTop + ((world*7+11)%(p.playH-2) + 1)
				c.Set(x, y, col)
			}
		}
	}
}

// drawTerrain paints the per-zone wall mask. Mountain caves get an
// accent-coloured surface stripe; Styx walls get a striped warning look.
func (p *playScene) drawTerrain(c *engine.Canvas) {
	wallCol := p.zone.wallCol
	accent := p.zone.accent
	switch p.zone.axis {
	case scrollHoriz:
		for x := 0; x < p.w; x++ {
			world := x + p.worldOffI
			ts := terrainAt(p.zone, world, p.w, p.playH)
			if ts.nearH > 0 {
				for y := p.playTop; y < p.playTop+ts.nearH; y++ {
					col := wallCol
					if y == p.playTop+ts.nearH-1 {
						col = accent
					}
					c.Set(x, y, col)
				}
			}
			if ts.farH > 0 {
				bot := p.playTop + p.playH
				for y := bot - ts.farH; y < bot; y++ {
					col := wallCol
					if y == bot-ts.farH {
						col = accent
					}
					c.Set(x, y, col)
				}
			}
		}
	case scrollVert:
		for y := p.playTop; y < p.playTop+p.playH; y++ {
			world := p.vertWorldRow(y)
			ts := terrainAt(p.zone, world, p.w, p.playH)
			if ts.nearH > 0 || ts.farH > 0 {
				// Styx walls span side-to-side with a gap. Render filled.
				for x := 0; x < ts.nearH; x++ {
					col := wallCol
					if x == ts.nearH-1 {
						col = accent
					}
					c.Set(x, y, col)
				}
				right := p.w - ts.farH
				for x := right; x < p.w; x++ {
					col := wallCol
					if x == right {
						col = accent
					}
					c.Set(x, y, col)
				}
			}
		}
	}
}

// vertWorldRow maps a screen row (only valid in the play area) to its
// world row for vertical-scroll zones. The player rides the bottom of
// the screen, so increasing worldOffI must make features flow DOWN the
// screen (i.e. toward the player) — that means the screen-bottom row
// is the "lowest" world row and the screen-top row is the highest.
func (p *playScene) vertWorldRow(y int) int {
	return p.worldOffI + (p.playH - 1 - (y - p.playTop))
}

func (p *playScene) drawPods(c *engine.Canvas) {
	for _, pod := range p.pods {
		if pod.collected {
			continue
		}
		spr := energyPodA
		if int(pod.bob*4)%2 == 0 {
			spr = energyPodB
		}
		col := engine.Color{R: 80, G: 240, B: 240, A: 255}
		drawSprite(c, int(pod.x), int(pod.y), spr, col)
	}
}

func (p *playScene) drawEnemies(c *engine.Canvas) {
	for _, e := range p.enemies {
		if e.state == esDying {
			frames := []sprite{enemyExplode0, enemyExplode1, enemyExplode2}
			step := int(e.dyingT / 0.15)
			if step >= len(frames) {
				step = len(frames) - 1
			}
			drawSprite(c, int(e.x), int(e.y), frames[step],
				engine.Color{R: 255, G: 220, B: 120, A: 255})
			continue
		}
		fa, fb := e.kind.frames()
		spr := fa
		if e.frame == 1 {
			spr = fb
		}
		col := e.kind.color()
		switch e.kind {
		case ekKemleyR:
			drawSpriteFlipX(c, int(e.x), int(e.y), spr, col)
		default:
			drawSprite(c, int(e.x), int(e.y), spr, col)
		}
	}
}

func (p *playScene) drawBullets(c *engine.Canvas) {
	for _, b := range p.bullets {
		if !b.fromPlayer {
			continue
		}
		spr := playerBulletV
		if b.vx != 0 {
			spr = playerBulletH
		}
		drawSprite(c, int(b.x), int(b.y), spr,
			engine.Color{R: 240, G: 240, B: 255, A: 255})
	}
	for _, b := range p.enemyBullets {
		spr := enemyBulletA
		// Cheap animation flicker keyed off bullet position so different
		// bullets blink out of phase.
		if (int(b.x)+int(b.y)+int(p.stateT*8))%2 == 0 {
			spr = enemyBulletB
		}
		drawSprite(c, int(b.x), int(b.y), spr,
			engine.Color{R: 255, G: 200, B: 100, A: 255})
	}
}

func (p *playScene) drawPlayer(c *engine.Canvas) {
	if p.state == psGameOver {
		return
	}
	if p.player.explodeT > 0 {
		frame := playerExplodeA
		if int((playerExplodeDur-p.player.explodeT)*10)%2 == 1 {
			frame = playerExplodeB
		}
		drawSprite(c, int(p.player.x), int(p.player.y), frame,
			engine.Color{R: 255, G: 200, B: 80, A: 255})
		return
	}
	// Blink while invuln (post-respawn or under a pod's tail end).
	if p.player.invul > 0 && int(p.player.invul*10)%2 == 0 {
		return
	}
	spr := playerShip
	col := engine.Color{R: 100, G: 240, B: 180, A: 255}
	if p.player.powered > 0 {
		spr = playerShipPowered
		// Pulse the powered colour through gold/orange.
		t := math.Sin(p.stateT*16) * 0.5
		col = engine.Color{
			R: uint8(220 + int(30*t)),
			G: uint8(200 + int(40*t)),
			B: uint8(80 + int(60*t)),
			A: 255,
		}
	}
	drawSprite(c, int(p.player.x), int(p.player.y), spr, col)
}

func (p *playScene) drawHUD(c *engine.Canvas) {
	cols := c.Cols()
	scoreText := fmt.Sprintf("SCORE %06d", p.score)
	hiText := fmt.Sprintf("HI %06d", p.hiScore)
	zoneText := p.zone.name
	if p.state == psBossEntry || p.state == psBossFight || p.state == psBossDying {
		zoneText = "GOND"
	}

	c.Print(1, 0, scoreText, engine.White)
	mid := (cols - len(hiText)) / 2
	if mid < len(scoreText)+2 {
		mid = len(scoreText) + 2
	}
	c.Print(mid, 0, hiText, engine.Yellow)
	rightCol := cols - len(zoneText) - 1
	if rightCol < mid+len(hiText)+2 {
		rightCol = mid + len(hiText) + 2
	}
	c.Print(rightCol, 0, zoneText, p.zone.accent)

	// Lives counter on row 2 — written as cell text to stay clear of
	// the play area (the 5-px life sprite would punch into the top row
	// of terrain otherwise).
	reserve := p.player.lives - 1
	if reserve < 0 {
		reserve = 0
	}
	livesText := "SHIPS " + zeroPad(reserve, 2)
	c.Print(1, 1, livesText, engine.Color{R: 100, G: 240, B: 180, A: 255})

	// Loop counter (after first loop) on the right of row 2.
	if p.loop > 0 {
		txt := fmt.Sprintf("LOOP %d", p.loop+1)
		c.Print(cols-len(txt)-1, 1, txt, p.zone.accent)
	}
}

// drawEnergyBar renders the bottom-row energy bar. The bar is a full-
// width strip in the very bottom cell; colour fades from green to red
// as energy depletes.
func (p *playScene) drawEnergyBar(c *engine.Canvas) {
	yTop := p.h - 2
	if yTop < 0 {
		return
	}
	// Left label.
	c.Print(1, c.Rows()-1, "ENERGY", engine.White)
	barLeft := 9
	barRight := c.Cols() - 2
	barWidthCells := barRight - barLeft
	if barWidthCells < 4 {
		return
	}
	// Convert cells to pixels.
	pxLeft := barLeft
	pxWidth := barWidthCells
	fill := int(p.player.energy * float64(pxWidth))
	if fill < 0 {
		fill = 0
	}
	if fill > pxWidth {
		fill = pxWidth
	}
	emptyCol := engine.Color{R: 40, G: 40, B: 40, A: 255}
	// Colour by remaining energy.
	var fillCol engine.Color
	switch {
	case p.player.energy > 0.5:
		fillCol = engine.Color{R: 80, G: 220, B: 90, A: 255}
	case p.player.energy > 0.25:
		fillCol = engine.Color{R: 240, G: 220, B: 80, A: 255}
	default:
		fillCol = engine.Color{R: 240, G: 80, B: 80, A: 255}
	}
	for x := 0; x < pxWidth; x++ {
		col := emptyCol
		if x < fill {
			col = fillCol
		}
		c.Set(pxLeft+x, yTop, col)
		c.Set(pxLeft+x, yTop+1, col)
	}
}

func (p *playScene) drawCentreText(c *engine.Canvas, text string, col engine.Color) {
	w := engine.TextWidth(text)
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2
	c.FillRect(x-3, y-2, w+6, engine.FontHeight+4,
		engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, text, col)
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	w := engine.TextWidth("GAME OVER")
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2 - 4
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4,
		engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, "GAME OVER", engine.Color{R: 255, G: 90, B: 90, A: 255})
	hint := "ENTER PLAY AGAIN   ESC QUIT"
	c.Print((c.Cols()-len(hint))/2, c.Rows()/2+2, hint, engine.White)
}
