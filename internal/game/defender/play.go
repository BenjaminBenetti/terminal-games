package defender

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// playState is the play-scene's internal state machine. The top-level
// scene only distinguishes title vs play; this exists inside play so
// the ship-death animation, wave-cleared banner, planet-explode
// transition, and game-over screen don't leak into the scene above.
type playState int

const (
	psWaveIntro playState = iota
	psPlaying
	psWaveCleared
	psPlanetExplode
	psGameOver
)

// playerBolt is a single horizontal laser projectile. Defender's
// laser is a long thin bar that moves quickly; we model it as a point
// with a separately-rendered "tail" for the bar.
type playerBolt struct {
	worldX float64
	y      float64
	vx     float64 // ± playerShotSpeed
	life   float64 // seconds until it expires
}

// enemyBoltEntity is one enemy-fired bolt. They move slowly and curve
// only slightly; we treat them as ballistic.
type enemyBoltEntity struct {
	worldX float64
	y      float64
	vx     float64
	vy     float64
}

// mine is a bomber-dropped cross that hangs in space until its life
// timer expires. Mines do not move.
type mine struct {
	worldX float64
	y      float64
	life   float64
}

// explosion is a generic short-lived particle burst rendered as
// expanding speckle rings. Used for the player's death and for the
// enemy explosions when something doesn't have its own renderer.
type explosion struct {
	worldX float64
	y      float64
	t      float64
	dur    float64
	col    engine.Color
}

// playScene is the active gameplay state.
type playScene struct {
	e    *engine.Engine
	w, h int

	world  *world
	player player

	enemies     []*enemy
	playerBolts []*playerBolt
	enemyBolts  []*enemyBoltEntity
	mines       []*mine
	humans      []*humanoid
	explosions  []*explosion

	rng *rand.Rand

	wave    int
	score   int
	hiScore int

	state  playState
	stateT float64
	gameT  float64

	// Wave director state.
	landersToSpawn int
	spawnGap       float64
	baiterTimer    float64
	plannedBombers int
	plannedPods    int
	bomberTimer    float64
	podTimer       float64

	// Status text — "+500" bonus indicator drawn by the renderer.
	statusMsg  string
	statusT    float64

	wantQuit bool
}

func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	w, h := c.Width(), c.Height()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	p := &playScene{
		e:       e,
		w:       w,
		h:       h,
		hiScore: hiScore,
		rng:     rng,
		wave:    1,
		world:   newWorld(w, h, rng),
	}
	p.initPlayer()
	p.humans = spawnHumans(p.world, 10, rng)
	p.beginWave(1)
	return p
}

// beginWave resets the per-wave director state and queues the wave's
// initial enemy population. Wave difficulty ramps the lander count,
// adds bombers around wave 3, pods around wave 4, and shortens the
// baiter timer as the wave drags.
//
// If the previous wave ended after a planet explosion, the planet is
// restored here: terrain reappears and 10 fresh humanoids respawn.
// Surviving humanoids on a non-exploded planet carry forward.
func (p *playScene) beginWave(n int) {
	if p.world.flattened {
		// New planet — re-roll terrain and respawn the 10 humanoids.
		p.world.buildTerrain(p.rng)
		p.world.flattened = false
		p.humans = spawnHumans(p.world, 10, p.rng)
	}
	p.wave = n
	p.state = psWaveIntro
	p.stateT = 0
	p.gameT = 0
	p.enemies = nil
	p.playerBolts = nil
	p.enemyBolts = nil
	p.mines = nil
	p.explosions = nil

	// Lander count grows but caps so the wave doesn't become a slog.
	p.landersToSpawn = 6 + n*2
	if p.landersToSpawn > 28 {
		p.landersToSpawn = 28
	}
	p.spawnGap = 0.6

	// Bombers and pods are added gradually as you progress.
	p.plannedBombers = 0
	p.plannedPods = 0
	if n >= 3 {
		p.plannedBombers = 1 + (n-3)/2
	}
	if n >= 4 {
		p.plannedPods = 1 + (n-4)/2
	}
	if p.plannedBombers > 4 {
		p.plannedBombers = 4
	}
	if p.plannedPods > 4 {
		p.plannedPods = 4
	}
	p.bomberTimer = 4.0
	p.podTimer = 6.0
	p.baiterTimer = baiterGraceTime
}

// Update is the engine.Scene tick.
func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s
	p.gameT += s

	if p.statusT > 0 {
		p.statusT -= s
	}

	// Camera follows the player every frame regardless of sub-state so
	// the world feels continuous even during banners.
	p.world.updateCamera(p.player.worldX, p.player.facing, s)

	switch p.state {
	case psWaveIntro:
		if p.stateT >= waveIntroDur {
			p.state = psPlaying
			p.stateT = 0
		}
		p.tickPassive(s)
	case psPlaying:
		p.tickPlaying(s)
	case psWaveCleared:
		// Brief moment of glory between waves.
		p.tickPassive(s)
		if p.stateT >= waveClearDur {
			p.beginWave(p.wave + 1)
		}
	case psPlanetExplode:
		p.tickPlanetExplode(s)
	case psGameOver:
		p.tickPassive(s)
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	return nil
}

// tickPassive runs only the cosmetic systems — useful during intros
// and game-over screens where gameplay is paused but the world should
// still drift.
func (p *playScene) tickPassive(s float64) {
	// Decay explosion particles.
	kept := p.explosions[:0]
	for _, ex := range p.explosions {
		ex.t += s
		if ex.t < ex.dur {
			kept = append(kept, ex)
		}
	}
	p.explosions = kept
}

// tickPlaying is the workhorse — runs everything that moves and
// resolves all collisions.
func (p *playScene) tickPlaying(s float64) {
	p.updatePlayer(s)
	p.updateEnemies(s)
	p.updateHumanoids(s)
	p.updatePlayerBolts(s)
	p.updateEnemyBolts(s)
	p.updateMines(s)
	p.tickPassive(s)

	p.runWaveDirector(s)

	p.resolveCollisions()

	// Wave clear?
	if p.state == psPlaying && p.waveCleared() {
		p.state = psWaveCleared
		p.stateT = 0
		// Big bonus per surviving humanoid.
		alive := 0
		for _, h := range p.humans {
			if !h.dead {
				alive++
			}
		}
		p.score += alive * waveClearHumanBonus
		p.statusMsg = fmt.Sprintf("WAVE %d CLEAR  +%d", p.wave, alive*waveClearHumanBonus)
		p.statusT = 2.0
	}

	// Game-over transitions are handled inside updatePlayer when the
	// final death animation completes; nothing more to do here.
}

// tickPlanetExplode plays the "all humans dead" disaster: terrain
// flattens, every remaining enemy becomes a mutant, baiters spawn
// faster, and the player has to grind out the wave with no humans
// left to defend.
func (p *playScene) tickPlanetExplode(s float64) {
	// Brief shockwave moment.
	p.tickPlaying(s)
	if p.stateT >= planetExplodeDur {
		p.world.flattened = true
		// Convert every alive lander to a mutant.
		for _, e := range p.enemies {
			if e.kind == kLander && e.alive() {
				e.kind = kMutant
			}
		}
		p.state = psPlaying
		p.stateT = 0
	}
}

// beginPlanetExplosion kicks off the disaster transition. Called from
// killHuman when the last surviving human dies.
func (p *playScene) beginPlanetExplosion() {
	if p.state == psPlanetExplode || p.world.flattened {
		return
	}
	p.state = psPlanetExplode
	p.stateT = 0
	p.statusMsg = "PLANET LOST"
	p.statusT = 3.0
	// Spawn a fat explosion at the world centre relative to player.
	p.explosions = append(p.explosions, &explosion{
		worldX: p.player.worldX,
		y:      float64(p.world.groundY),
		dur:    planetExplodeDur,
		col:    engine.Color{R: 255, G: 200, B: 100, A: 255},
	})
}

// updatePlayerBolts advances every player laser. Out-of-screen or
// expired bolts are reaped.
func (p *playScene) updatePlayerBolts(s float64) {
	kept := p.playerBolts[:0]
	for _, b := range p.playerBolts {
		b.worldX = p.world.wrapX(b.worldX + b.vx*s)
		b.life -= s
		if b.life > 0 {
			kept = append(kept, b)
		}
	}
	p.playerBolts = kept
}

// updateEnemyBolts moves each enemy bolt until it leaves the visible
// screen — wrap-around shouldn't carry a bolt indefinitely.
func (p *playScene) updateEnemyBolts(s float64) {
	kept := p.enemyBolts[:0]
	for _, b := range p.enemyBolts {
		b.worldX = p.world.wrapX(b.worldX + b.vx*s)
		b.y += b.vy * s
		sx := p.world.toScreen(b.worldX)
		if sx < -10 || sx > p.w+10 {
			continue
		}
		if b.y < float64(p.world.playZoneTop) || b.y > float64(p.h) {
			continue
		}
		kept = append(kept, b)
	}
	p.enemyBolts = kept
}

func (p *playScene) updateMines(s float64) {
	kept := p.mines[:0]
	for _, m := range p.mines {
		m.life -= s
		if m.life > 0 {
			kept = append(kept, m)
		}
	}
	p.mines = kept
}

// runWaveDirector drips in landers, bombers, pods, and (if the wave
// drags) baiters. Spawns are paced rather than dumped all at once so
// the player doesn't drown immediately.
func (p *playScene) runWaveDirector(s float64) {
	if p.state != psPlaying {
		return
	}
	// Landers.
	if p.landersToSpawn > 0 {
		p.spawnGap -= s
		if p.spawnGap <= 0 {
			p.enemies = append(p.enemies, spawnLander(p.world, p.rng))
			p.landersToSpawn--
			p.spawnGap = 0.6 + p.rng.Float64()*0.4
		}
	}
	// Bombers.
	if p.plannedBombers > 0 {
		p.bomberTimer -= s
		if p.bomberTimer <= 0 {
			p.enemies = append(p.enemies, spawnBomber(p.world, p.rng))
			p.plannedBombers--
			p.bomberTimer = 8.0 + p.rng.Float64()*4.0
		}
	}
	// Pods.
	if p.plannedPods > 0 {
		p.podTimer -= s
		if p.podTimer <= 0 {
			p.enemies = append(p.enemies, spawnPod(p.world, p.rng))
			p.plannedPods--
			p.podTimer = 9.0 + p.rng.Float64()*5.0
		}
	}
	// Baiters: spawn if the wave's been live for too long. The
	// trigger is *only* the elapsed gameT — original Defender behaved
	// the same way.
	if p.gameT > baiterGraceTime {
		p.baiterTimer -= s
		if p.baiterTimer <= 0 {
			p.enemies = append(p.enemies, spawnBaiter(p.world, p.rng, p.player.y))
			p.baiterTimer = 3.0 + p.rng.Float64()*2.0
		}
	}
}

// waveCleared checks the wave-end condition: no more landers to spawn,
// every spawned enemy that came from the lander pool is dead, and no
// dangerous mid-wave enemy is still on the field. Baiters, pods, and
// pod-burst swarmers must also be cleared. Note: humans being all
// dead is a separate condition that triggers planet explosion, not
// wave clear.
func (p *playScene) waveCleared() bool {
	if p.landersToSpawn > 0 {
		return false
	}
	if p.plannedBombers > 0 || p.plannedPods > 0 {
		return false
	}
	for _, e := range p.enemies {
		if e.alive() && (e.kind == kLander || e.kind == kMutant ||
			e.kind == kBomber || e.kind == kPod || e.kind == kSwarmer ||
			e.kind == kBaiter) {
			return false
		}
	}
	return true
}

// resolveCollisions handles every overlap-driven event. The order
// matters: shots before contact damage so a shot can save the player
// from a ramming enemy in the same frame.
func (p *playScene) resolveCollisions() {
	p.collidePlayerBolts()
	p.collideMineBolts()
	if p.player.invulnerable() {
		return
	}
	p.collidePlayerWithEnemies()
	p.collidePlayerWithBolts()
	p.collidePlayerWithMines()
	p.tryCatchFallingHumans()
}

// playerBoltRect returns the bullet's world-space hitbox. The visible
// bar trails BEHIND the head (the head is the leading edge in the
// direction of travel), so the rect on the world axis depends on
// vx's sign.
func playerBoltRect(b *playerBolt) (x0, y0, x1, y1 float64) {
	y0 = b.y - 1
	y1 = b.y + 2
	if b.vx >= 0 {
		// Rightward: head at worldX, tail trails LEFT.
		x0 = b.worldX - float64(playerShotLen)
		x1 = b.worldX + 1
	} else {
		// Leftward: head at worldX, tail trails RIGHT.
		x0 = b.worldX - 1
		x1 = b.worldX + float64(playerShotLen)
	}
	return
}

// collidePlayerBolts walks every active enemy against every active
// player bolt. A hit removes the bolt and either kills the enemy
// or, for pods, also bursts them into swarmers.
func (p *playScene) collidePlayerBolts() {
	if len(p.playerBolts) == 0 {
		return
	}
	for _, e := range p.enemies {
		if !e.alive() {
			continue
		}
		ex0, ey0, ex1, ey1 := e.boundingBox()
		hit := -1
		for i, b := range p.playerBolts {
			bx0, by0, bx1, by1 := playerBoltRect(b)
			if !p.world.rectsOverlap(ex0, ey0, ex1, ey1, bx0, by0, bx1, by1) {
				continue
			}
			hit = i
			break
		}
		if hit < 0 {
			continue
		}
		p.playerBolts = append(p.playerBolts[:hit], p.playerBolts[hit+1:]...)
		p.score += kindScore(e.kind)
		// Free any carried human into freefall (so the player can
		// catch them) before killing the lander.
		if e.carrying != nil {
			e.carrying.state = humanFalling
			e.carrying.fallV = 0
			e.carrying.carrier = nil
			e.carrying = nil
		}
		if e.kind == kPod {
			p.burstPod(e)
		}
		e.state = esDying
		e.dyingT = 0
	}
}

// collideMineBolts gives the player a way to clear bomber mines, even
// though the original Defender mines were destructible only by smart
// bomb. We keep this minor (player bolt cancels mine) because it's a
// readability win in the terminal where mines are easy to miss.
func (p *playScene) collideMineBolts() {
	if len(p.playerBolts) == 0 || len(p.mines) == 0 {
		return
	}
	kept := p.mines[:0]
	for _, m := range p.mines {
		hit := -1
		for i, b := range p.playerBolts {
			bx0, by0, bx1, by1 := playerBoltRect(b)
			if !p.world.rectsOverlap(m.worldX, m.y, m.worldX+5, m.y+5, bx0, by0, bx1, by1) {
				continue
			}
			hit = i
			break
		}
		if hit >= 0 {
			p.playerBolts = append(p.playerBolts[:hit], p.playerBolts[hit+1:]...)
			continue
		}
		kept = append(kept, m)
	}
	p.mines = kept
}

// collidePlayerWithEnemies — ramming death.
func (p *playScene) collidePlayerWithEnemies() {
	px0, py0, px1, py1 := p.playerBox()
	for _, e := range p.enemies {
		if !e.alive() {
			continue
		}
		ex0, ey0, ex1, ey1 := e.boundingBox()
		if !p.world.rectsOverlap(px0, py0, px1, py1, ex0, ey0, ex1, ey1) {
			continue
		}
		// Award score for the rammed enemy.
		p.score += kindScore(e.kind)
		if e.carrying != nil {
			e.carrying.state = humanFalling
			e.carrying.fallV = 0
			e.carrying.carrier = nil
			e.carrying = nil
		}
		e.state = esDying
		e.dyingT = 0
		p.killPlayer()
		return
	}
}

// collidePlayerWithBolts — enemy-fire death.
func (p *playScene) collidePlayerWithBolts() {
	px0, py0, px1, py1 := p.playerBox()
	for i, b := range p.enemyBolts {
		if !p.world.rectsOverlap(px0, py0, px1, py1,
			b.worldX-1, b.y-1, b.worldX+3, b.y+2) {
			continue
		}
		p.enemyBolts = append(p.enemyBolts[:i], p.enemyBolts[i+1:]...)
		p.killPlayer()
		return
	}
}

// collidePlayerWithMines — bomber mine death.
func (p *playScene) collidePlayerWithMines() {
	px0, py0, px1, py1 := p.playerBox()
	for i, m := range p.mines {
		if !p.world.rectsOverlap(px0, py0, px1, py1,
			m.worldX, m.y, m.worldX+5, m.y+5) {
			continue
		}
		// Remove the mine (mines explode on contact).
		p.mines = append(p.mines[:i], p.mines[i+1:]...)
		p.killPlayer()
		return
	}
}

// tryCatchFallingHumans tests the player AABB against falling humans
// — when the player drives under a freed humanoid, the humanoid
// attaches to the ship's underside.
func (p *playScene) tryCatchFallingHumans() {
	if p.player.carrying != nil {
		return
	}
	px0, py0, px1, py1 := p.playerBox()
	for _, h := range p.humans {
		if h.dead || h.state != humanFalling {
			continue
		}
		hw := float64(humanoidSprite.width())
		hh := float64(humanoidSprite.height())
		if !p.world.rectsOverlap(px0, py0, px1, py1+hh,
			h.worldX, h.y, h.worldX+hw, h.y+hh) {
			continue
		}
		// Catch — no immediate score (the points come at delivery,
		// matching the arcade). Mark rescued so a subsequent natural
		// fall doesn't double-pay the survival bonus.
		h.state = humanCarried
		h.fallV = 0
		p.player.carrying = h
		h.rescued = true
		return
	}
}

// handleInput drains the engine's key queue.
func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psPlaying, psWaveIntro, psWaveCleared, psPlanetExplode:
			p.handlePlayKey(k)
		case psGameOver:
			p.handleGameOverKey(k)
		}
	}
}

func (p *playScene) handlePlayKey(k engine.Key) {
	switch k.Code {
	case engine.KeyEsc:
		p.wantQuit = true
	case engine.KeyChar:
		switch k.Rune {
		case 'q', 'Q':
			p.wantQuit = true
		case ' ':
			p.firePlayer()
		case 'b', 'B':
			p.triggerSmartBomb()
		case 'h', 'H':
			p.triggerHyperspace()
		case 'r', 'R':
			// Flip facing without moving — same as the arcade reverse
			// button.
			if p.player.facing > 0 {
				p.player.facing = -1
			} else {
				p.player.facing = 1
			}
		}
	}
}

func (p *playScene) handleGameOverKey(k engine.Key) {
	switch k.Code {
	case engine.KeyEsc:
		p.wantQuit = true
	case engine.KeyEnter:
		// Restart from wave 1.
		hi := p.hiScore
		*p = *newPlayScene(p.e, hi)
	case engine.KeyChar:
		switch k.Rune {
		case 'q', 'Q':
			p.wantQuit = true
		case 'r', 'R':
			hi := p.hiScore
			*p = *newPlayScene(p.e, hi)
		}
	}
}

// --- Tuning -----------------------------------------------------------

const (
	waveIntroDur          = 1.6
	waveClearDur          = 1.8
	planetExplodeDur      = 1.2
	waveClearHumanBonus   = 100
	baiterGraceTime       = 40.0
)

// --- Rendering --------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 0, G: 0, B: 10, A: 255})
	p.world.drawStarfield(c, p.gameT)
	p.world.drawTerrain(c)
	p.drawHumans(c)
	p.drawEnemies(c)
	p.drawMines(c)
	p.drawEnemyBolts(c)
	p.drawPlayerBolts(c)
	p.drawPlayer(c)
	p.drawExplosions(c)
	p.drawSmartBombShockwave(c)
	p.drawHUD(c)
	p.drawScanner(c)
	p.drawOverlay(c)
}

// drawHumans renders every alive (or just-rescued) humanoid plus any
// "+points" floating bonus indicator above recently rescued ones.
func (p *playScene) drawHumans(c *engine.Canvas) {
	for _, h := range p.humans {
		if h.dead {
			continue
		}
		sx := p.world.toScreen(h.worldX)
		if sx+humanoidSprite.width() < 0 || sx >= p.w {
			continue
		}
		col := colHumanoid
		if h.state == humanLifted {
			// Dim while abducted — readability cue.
			col = engine.Color{R: 60, G: 200, B: 100, A: 255}
		}
		drawSprite(c, sx, int(h.y), humanoidSprite, col)
		if h.bonusT > 0 {
			c.Print(sx-1, int(h.y)/2-1, "+", engine.Yellow)
		}
	}
}

// drawEnemies renders every alive (and visible) enemy plus the
// brief explosion sprite for dying ones.
func (p *playScene) drawEnemies(c *engine.Canvas) {
	for _, e := range p.enemies {
		sx := p.world.toScreen(e.worldX)
		spA, spB := e.sprite()
		w := spA.width()
		if sx+w < 0 || sx >= p.w {
			continue
		}
		if e.state == esDying {
			p.drawEnemyExplosion(c, sx, int(e.y), e.dyingT)
			continue
		}
		spr := spA
		if e.frame == 1 {
			spr = spB
		}
		var col engine.Color
		switch e.kind {
		case kLander:
			col = colLander
		case kMutant:
			col = colMutant
		case kBomber:
			col = colBomber
		case kPod:
			col = colPod
		case kSwarmer:
			col = colSwarmer
		case kBaiter:
			col = colBaiter
		}
		drawSprite(c, sx, int(e.y), spr, col)
	}
}

func (p *playScene) drawEnemyExplosion(c *engine.Canvas, sx, sy int, t float64) {
	r := int(t * 24)
	if r < 1 {
		r = 1
	}
	col := engine.Color{R: 255, G: 220, B: 120, A: 255}
	if t > enemyExplodeDur*0.5 {
		col = engine.Color{R: 255, G: 100, B: 60, A: 255}
	}
	// Draw a quick burst of pixels around the center.
	cx := sx + 3
	cy := sy + 2
	for ang := 0; ang < 8; ang++ {
		theta := float64(ang) * math.Pi / 4
		x := cx + int(math.Cos(theta)*float64(r))
		y := cy + int(math.Sin(theta)*float64(r))
		c.Set(x, y, col)
	}
}

func (p *playScene) drawMines(c *engine.Canvas) {
	for _, m := range p.mines {
		sx := p.world.toScreen(m.worldX)
		if sx < -6 || sx >= p.w {
			continue
		}
		// Flash when about to expire.
		col := colMine
		if m.life < 0.6 && int(m.life*10)%2 == 0 {
			col = engine.Color{R: 255, G: 200, B: 200, A: 255}
		}
		drawSprite(c, sx, int(m.y), bomberMine, col)
	}
}

func (p *playScene) drawEnemyBolts(c *engine.Canvas) {
	for _, b := range p.enemyBolts {
		sx := p.world.toScreen(b.worldX)
		drawSprite(c, sx, int(b.y), enemyBolt, colEnemyShot)
	}
}

func (p *playScene) drawPlayerBolts(c *engine.Canvas) {
	for _, b := range p.playerBolts {
		sx := p.world.toScreen(b.worldX)
		dir := 1
		if b.vx < 0 {
			dir = -1
		}
		// Draw a bar of length playerShotLen extending in the
		// direction of travel.
		for i := 0; i < playerShotLen; i++ {
			x := sx
			if dir > 0 {
				x = sx - i
			} else {
				x = sx + i
			}
			// Tail fades.
			col := colPlayerLas
			if i > playerShotLen-3 {
				col = engine.Color{R: 200, G: 80, B: 80, A: 255}
			}
			c.Set(x, int(b.y), col)
		}
	}
}

// drawPlayer renders the ship with thrust flame, death animation, or
// invulnerability blink as appropriate.
func (p *playScene) drawPlayer(c *engine.Canvas) {
	pl := &p.player
	if pl.hyperT > 0 {
		// Hyperspace flicker — a few random pixels at the destination
		// for a couple frames.
		sx := p.world.toScreen(pl.worldX)
		for i := 0; i < 10; i++ {
			rx := sx + p.rng.Intn(playerShip.width()*2) - 4
			ry := int(pl.y) + p.rng.Intn(playerShip.height()*2) - 2
			c.Set(rx, ry, colPlayer)
		}
		return
	}
	if pl.dead {
		// Death animation — expanding speckle starburst.
		sx := p.world.toScreen(pl.worldX)
		r := int(pl.deadT * 30)
		if r < 1 {
			r = 1
		}
		for ang := 0; ang < 16; ang++ {
			theta := float64(ang) * math.Pi / 8
			x := sx + int(math.Cos(theta)*float64(r)) + 4
			y := int(pl.y) + int(math.Sin(theta)*float64(r)) + 2
			col := engine.Color{R: 255, G: 180, B: 80, A: 255}
			if pl.deadT > playerDeathDur*0.5 {
				col = engine.Color{R: 200, G: 60, B: 60, A: 255}
			}
			c.Set(x, y, col)
		}
		return
	}
	// Respawn blink.
	if pl.deadT < 0 {
		if int(-pl.deadT*12)%2 == 0 {
			return
		}
	}
	sx := p.world.toScreen(pl.worldX)
	flip := pl.facing < 0
	if pl.thrusting {
		// Draw thrust flame behind the tail. Tail is at the "back"
		// (opposite of facing).
		spr := thrustFlameA
		if int(pl.thrustT*30)%2 == 1 {
			spr = thrustFlameB
		}
		flameX := sx
		if flip {
			// Flame off the right side (facing left).
			flameX = sx + playerShip.width()
		} else {
			flameX = sx - spr.width()
		}
		drawSprite(c, flameX, int(pl.y)+1, spr, colThrust)
	}
	if flip {
		drawSpriteFlipX(c, sx, int(pl.y), playerShip, colPlayer)
	} else {
		drawSprite(c, sx, int(pl.y), playerShip, colPlayer)
	}
	// Draw carried human (if any) below the ship.
	if pl.carrying != nil {
		drawSprite(c, sx+3, int(pl.y)+playerShip.height(), humanoidSprite, colHumanoid)
	}
}

// drawExplosions paints generic explosions (currently just the planet
// kaboom).
func (p *playScene) drawExplosions(c *engine.Canvas) {
	for _, ex := range p.explosions {
		sx := p.world.toScreen(ex.worldX)
		r := int((ex.t/ex.dur)*40) + 1
		for ang := 0; ang < 24; ang++ {
			theta := float64(ang) * math.Pi / 12
			x := sx + int(math.Cos(theta)*float64(r))
			y := int(ex.y) + int(math.Sin(theta)*float64(r))
			c.Set(x, y, ex.col)
		}
	}
}

// drawSmartBombShockwave renders the screen-clearing ring expanding
// from the player's location at the moment the bomb was triggered.
func (p *playScene) drawSmartBombShockwave(c *engine.Canvas) {
	if p.player.bombT <= 0 {
		return
	}
	progress := 1 - p.player.bombT/bombShockwaveDur
	if progress < 0 {
		progress = 0
	}
	maxR := float64(p.w) * 0.6
	r := int(progress * maxR)
	sx := p.world.toScreen(p.player.bombX)
	cy := int(p.player.bombY) + 2
	// Two concentric rings, slightly different radii, for a tube look.
	colA := engine.Color{R: 255, G: 240, B: 120, A: 255}
	colB := engine.Color{R: 240, G: 100, B: 60, A: 255}
	drawRingOutline(c, sx, cy, r, colA)
	drawRingOutline(c, sx, cy, r-2, colB)
}

func drawRingOutline(c *engine.Canvas, cx, cy, r int, col engine.Color) {
	if r <= 0 {
		return
	}
	c.DrawCircle(cx, cy, r, col)
}

// drawOverlay paints the wave-cleared banner, planet-explode banner,
// game-over screen, and floating status text.
func (p *playScene) drawOverlay(c *engine.Canvas) {
	switch p.state {
	case psWaveIntro:
		txt := fmt.Sprintf("WAVE %d", p.wave)
		drawBanner(c, p.w, p.h, txt, engine.Color{R: 240, G: 240, B: 120, A: 255})
	case psWaveCleared:
		drawBanner(c, p.w, p.h, "WAVE CLEAR", engine.Color{R: 120, G: 240, B: 120, A: 255})
	case psPlanetExplode:
		drawBanner(c, p.w, p.h, "PLANET LOST", engine.Color{R: 255, G: 80, B: 80, A: 255})
	case psGameOver:
		drawBanner(c, p.w, p.h, "GAME OVER", engine.Color{R: 255, G: 90, B: 90, A: 255})
		hint := "ENTER PLAY AGAIN   ESC QUIT"
		c.Print((c.Cols()-len(hint))/2, c.Rows()/2+3, hint, engine.White)
	}
	if p.statusT > 0 && p.state == psPlaying {
		c.Print((c.Cols()-len(p.statusMsg))/2, p.world.playZoneBot/2+2, p.statusMsg, engine.Yellow)
	}
}

func drawBanner(c *engine.Canvas, w, h int, txt string, col engine.Color) {
	tw := engine.TextWidth(txt)
	x := (w - tw) / 2
	y := (h - engine.FontHeight) / 2
	c.FillRect(x-4, y-2, tw+8, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, txt, col)
}

// zeroPad formats n as a fixed-width zero-padded decimal. Same idiom
// the other games use; defender owns its own copy to avoid coupling.
func zeroPad(n, width int) string {
	if n < 0 {
		n = -n
	}
	digits := []byte{}
	if n == 0 {
		digits = []byte{'0'}
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	for len(digits) < width {
		digits = append([]byte{'0'}, digits...)
	}
	return string(digits)
}
