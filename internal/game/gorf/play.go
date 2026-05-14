package gorf

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// =====================================================================
// Tuning constants.
// =====================================================================

const (
	// Player.
	playerSpeedX     = 38.0
	playerSpeedY     = 28.0
	playerFireGap    = 0.18
	playerLaserSpeed = 110.0
	playerExplodeDur = 1.3
	playerStartLives = 4

	// Common enemy bombs.
	bombSpeed         = 30.0
	bombMaxAlive      = 8
	bombFrameInterval = 0.10

	// Pacing.
	missionIntroDur = 1.8
	missionClearDur = 1.6
	tauntDefaultDur = 1.6

	// Starfield.
	playStarsCount = 60
)

// =====================================================================
// State machines — top-level + per-life status.
// =====================================================================

// playState is the gameplay sub-state machine. Mission progression is a
// separate axis: a mission is always one of the five (astro, laser, …)
// and `state` distinguishes whether we're showing the mission intro
// card, actively playing, the player just died, the mission was cleared,
// or the game is over.
type playState int

const (
	psMissionIntro playState = iota
	psPlaying
	psPlayerHit
	psMissionCleared
	psGameOver
)

// missionID enumerates the five sub-games. The cycle order is fixed in
// the original arcade; defeating mission 5 advances the cycle counter
// (which multiplies score) and loops back to mission 1.
type missionID int

const (
	missionAstro missionID = iota
	missionLaser
	missionGalaxians
	missionWarp
	missionFlag
)

func (m missionID) name() string {
	switch m {
	case missionAstro:
		return "ASTRO BATTLES"
	case missionLaser:
		return "LASER ATTACK"
	case missionGalaxians:
		return "GALAXIANS"
	case missionWarp:
		return "SPACE WARP"
	case missionFlag:
		return "FLAG SHIP"
	}
	return ""
}

// next returns the mission that follows m within a cycle. Flag wraps
// back to Astro; the cycle counter is bumped elsewhere.
func (m missionID) next() missionID {
	if m == missionFlag {
		return missionAstro
	}
	return m + 1
}

// =====================================================================
// Common entity types.
// =====================================================================

// playerEntity is the defender ship — 2D movement, quad-laser, shields.
type playerEntity struct {
	x, y     float64 // sprite top-left in canvas pixel coords
	cooldown float64
	laser    *laserBolt // single quad-laser shot in flight
	lives    int        // remaining "shields" — Gorf calls them shields
	explodeT float64
}

// laserBolt is the player's quad-laser projectile. Wider than a normal
// bullet and only one can be in flight at a time.
type laserBolt struct {
	x, y float64
	vy   float64
}

// bomb is a falling projectile dropped by any enemy class.
type bomb struct {
	x, y   float64
	vy     float64
	frame  int
	frameT float64
	kind   int // 0 normal, 1 boss
}

// explosion is a short-lived debris cloud at (x, y). When t reaches dur
// the explosion is removed.
type explosion struct {
	x, y int
	t    float64
	dur  float64
	kind int // 0 small (alien), 1 large (player)
}

// playStar is a single twinkling background star in the play scene.
type playStar struct {
	x, y  float64
	speed float64
	phase float64
	tint  int
}

// =====================================================================
// playScene — the active game.
// =====================================================================

// playScene contains the full gameplay state: player, common projectiles,
// the active mission's bespoke state, and the top-level state machine.
type playScene struct {
	e    *engine.Engine
	w, h int

	player     playerEntity
	bombs      []*bomb
	explosions []*explosion
	stars      []playStar

	// Per-mission state. Only one of these is non-nil at any moment.
	astro *astroState
	laser *laserState
	galax *galaxState
	warp  *warpState
	flag  *flagState

	mission missionID
	cycle   int // 1-based; doubles all scores from cycle 2 onward.

	state  playState
	stateT float64

	// Overlay taunt — short Gorfian one-liner that fades in/out. Triggered
	// on player hit, mission intro, etc.
	taunt    string
	tauntT   float64
	tauntDur float64

	score   int
	hiScore int
	hiCycle int

	// Layout — derived once at construction.
	playTop    int // first y the player area starts (below HUD)
	playerYMin int
	playerYMax int

	wantQuit bool
	rng      *rand.Rand
}

// newPlayScene constructs a play scene sized to the engine's canvas.
func newPlayScene(e *engine.Engine, hiScore, hiCycle int) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		hiScore: hiScore,
		hiCycle: hiCycle,
		cycle:   1,
		mission: missionAstro,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.player.lives = playerStartLives
	p.computeLayout()
	p.spawnStars()
	p.beginMission()
	return p
}

// computeLayout derives the play-area dimensions from the canvas size.
// HUD takes the top two rows (4 pixels); the player can roam in the
// lower ~40% of the play area.
func (p *playScene) computeLayout() {
	hudRows := 2
	p.playTop = hudRows * 2 // y in pixels where the play area starts

	playH := p.h - p.playTop - 1
	p.playerYMax = p.h - playerSprite.height() - 1
	// Allow the player to roam upward to about 55% down the play area.
	p.playerYMin = p.playTop + playH*55/100
	if p.playerYMin > p.playerYMax-4 {
		p.playerYMin = p.playerYMax - 4
	}
	// Centre the player horizontally and place them at the bottom.
	p.player.x = float64(p.w-playerSprite.width()) / 2
	p.player.y = float64(p.playerYMax)
}

func (p *playScene) spawnStars() {
	count := p.w * p.h / 60
	if count > playStarsCount*2 {
		count = playStarsCount * 2
	}
	if count < 30 {
		count = 30
	}
	p.stars = make([]playStar, count)
	for i := range p.stars {
		p.stars[i] = playStar{
			x:     p.rng.Float64() * float64(p.w),
			y:     p.rng.Float64() * float64(p.h),
			speed: 2 + p.rng.Float64()*9,
			phase: p.rng.Float64(),
			tint:  p.rng.Intn(4),
		}
	}
}

// beginMission resets per-mission state and enters the intro card phase.
func (p *playScene) beginMission() {
	p.bombs = nil
	p.explosions = nil
	p.player.laser = nil
	p.player.cooldown = 0
	p.player.x = float64(p.w-playerSprite.width()) / 2
	p.player.y = float64(p.playerYMax)
	p.state = psMissionIntro
	p.stateT = 0
	p.astro = nil
	p.laser = nil
	p.galax = nil
	p.warp = nil
	p.flag = nil

	switch p.mission {
	case missionAstro:
		p.astro = newAstroState(p)
	case missionLaser:
		p.laser = newLaserState(p)
	case missionGalaxians:
		p.galax = newGalaxState(p)
	case missionWarp:
		p.warp = newWarpState(p)
	case missionFlag:
		p.flag = newFlagState(p)
	}

	p.setTaunt(missionIntroTaunt(p.mission), missionIntroDur)
}

// missionIntroTaunt picks a Gorfian one-liner appropriate to the next
// mission.
func missionIntroTaunt(m missionID) string {
	switch m {
	case missionAstro:
		return "MY ASTRO ARMY ATTACKS"
	case missionLaser:
		return "ALL LASERS FIRE"
	case missionGalaxians:
		return "INVADERS ON THE WING"
	case missionWarp:
		return "THROUGH THE WARP"
	case missionFlag:
		return "THE FLAG SHIP ARRIVES"
	}
	return ""
}

// setTaunt activates a brief on-screen taunt for dur seconds.
func (p *playScene) setTaunt(text string, dur float64) {
	if dur <= 0 {
		dur = tauntDefaultDur
	}
	p.taunt = text
	p.tauntT = 0
	p.tauntDur = dur
}

// =====================================================================
// Update — top-level state machine + dispatch to active mission.
// =====================================================================

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s
	p.tauntT += s
	p.tickStars(s)

	switch p.state {
	case psMissionIntro:
		// The intro card holds for missionIntroDur; the active mission
		// still ticks (formation sway etc.) so the field looks alive.
		p.tickMission(s, false)
		if p.stateT >= missionIntroDur {
			p.state = psPlaying
			p.stateT = 0
		}
	case psPlaying:
		p.tickMission(s, true)
		p.tickPlayer(s)
		p.tickBullets(s)
		p.tickExplosions(s)
		p.resolveCollisions()
		// Mission's own clear condition signals via the mission state's
		// `cleared` flag set during its tick.
		if p.missionCleared() {
			p.state = psMissionCleared
			p.stateT = 0
			p.setTaunt(missionClearedTaunt(p.mission), missionClearDur)
		}
	case psPlayerHit:
		p.player.explodeT -= s
		p.tickMission(s, false)
		p.tickBullets(s)
		p.tickExplosions(s)
		if p.player.explodeT <= 0 {
			if p.player.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.player.x = float64(p.w-playerSprite.width()) / 2
				p.player.y = float64(p.playerYMax)
				p.player.explodeT = 0
				p.state = psPlaying
				p.stateT = 0
			}
		}
	case psMissionCleared:
		// Hold the banner, then advance.
		p.tickMission(s, false)
		p.tickExplosions(s)
		if p.stateT >= missionClearDur {
			p.advanceMission()
		}
	case psGameOver:
		p.tickMission(s, false)
		p.tickExplosions(s)
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	if p.cycle > p.hiCycle {
		p.hiCycle = p.cycle
	}
	return nil
}

// tickMission dispatches to the active mission's per-frame update.
// `active` is true only during psPlaying — other states keep cosmetic
// animation running but suspend gameplay logic (firing, dive starts).
func (p *playScene) tickMission(s float64, active bool) {
	switch p.mission {
	case missionAstro:
		p.astro.tick(p, s, active)
	case missionLaser:
		p.laser.tick(p, s, active)
	case missionGalaxians:
		p.galax.tick(p, s, active)
	case missionWarp:
		p.warp.tick(p, s, active)
	case missionFlag:
		p.flag.tick(p, s, active)
	}
}

// missionCleared returns true when the active mission's state reports
// completion.
func (p *playScene) missionCleared() bool {
	switch p.mission {
	case missionAstro:
		return p.astro.cleared
	case missionLaser:
		return p.laser.cleared
	case missionGalaxians:
		return p.galax.cleared
	case missionWarp:
		return p.warp.cleared
	case missionFlag:
		return p.flag.cleared
	}
	return false
}

func missionClearedTaunt(m missionID) string {
	switch m {
	case missionAstro:
		return "ASTRO BATTLE WON"
	case missionLaser:
		return "LASER ATTACK CLEARED"
	case missionGalaxians:
		return "GALAXIAN SQUADRON DOWN"
	case missionWarp:
		return "WARP HOLE COLLAPSED"
	case missionFlag:
		return "THE FLAG SHIP FALLS"
	}
	return ""
}

// advanceMission moves to the next mission, or starts a new cycle if
// the Flag Ship was just defeated.
func (p *playScene) advanceMission() {
	if p.mission == missionFlag {
		p.cycle++
		if p.cycle > p.hiCycle {
			p.hiCycle = p.cycle
		}
	}
	p.mission = p.mission.next()
	p.beginMission()
}

// =====================================================================
// Input.
// =====================================================================

func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psPlaying:
			p.handlePlayKey(k)
		case psPlayerHit, psMissionIntro, psMissionCleared:
			if k.Code == engine.KeyEsc {
				p.wantQuit = true
			}
		case psGameOver:
			if k.Code == engine.KeyEnter ||
				(k.Code == engine.KeyChar && (k.Rune == 'r' || k.Rune == 'R')) {
				p.restartGame()
			}
			if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
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

// restartGame rebuilds the playScene state for a fresh run, preserving
// hi-score across the reset.
func (p *playScene) restartGame() {
	hi := p.hiScore
	hc := p.hiCycle
	p.score = 0
	p.player.lives = playerStartLives
	p.player.explodeT = 0
	p.cycle = 1
	p.mission = missionAstro
	p.hiScore = hi
	p.hiCycle = hc
	p.beginMission()
}

// tryFire spawns the quad-laser if no shot is currently in flight and
// the cooldown has elapsed.
func (p *playScene) tryFire() {
	if p.player.laser != nil || p.player.cooldown > 0 {
		return
	}
	bx := p.player.x + float64(playerSprite.width()-playerLaserSprite.width())/2
	by := p.player.y - float64(playerLaserSprite.height())
	p.player.laser = &laserBolt{
		x:  bx,
		y:  by,
		vy: -playerLaserSpeed,
	}
	p.player.cooldown = playerFireGap
}

// =====================================================================
// Per-frame entity ticks (player, bullets, common bombs, explosions, stars).
// =====================================================================

func (p *playScene) tickPlayer(s float64) {
	// Held-key state — read every frame so move-and-fire work together.
	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	up := p.e.IsKeyDown(engine.KeyUp) || p.e.IsCharDown('w') || p.e.IsCharDown('W')
	down := p.e.IsKeyDown(engine.KeyDown) || p.e.IsCharDown('s') || p.e.IsCharDown('S')

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
	if dx != 0 && dy != 0 {
		// Diagonal movement — normalize so total speed isn't √2 faster.
		dx *= 0.7071
		dy *= 0.7071
	}
	p.player.x += dx * playerSpeedX * s
	p.player.y += dy * playerSpeedY * s

	// Clamp to play area.
	minX := 1.0
	maxX := float64(p.w-playerSprite.width()) - 1
	if p.player.x < minX {
		p.player.x = minX
	}
	if p.player.x > maxX {
		p.player.x = maxX
	}
	if p.player.y < float64(p.playerYMin) {
		p.player.y = float64(p.playerYMin)
	}
	if p.player.y > float64(p.playerYMax) {
		p.player.y = float64(p.playerYMax)
	}

	if p.player.cooldown > 0 {
		p.player.cooldown -= s
	}
}

func (p *playScene) tickBullets(s float64) {
	// Player quad-laser.
	if b := p.player.laser; b != nil {
		b.y += b.vy * s
		if b.y+float64(playerLaserSprite.height()) < 0 {
			p.player.laser = nil
		}
	}
	// Enemy bombs.
	kept := p.bombs[:0]
	for _, bm := range p.bombs {
		bm.y += bm.vy * s
		bm.frameT += s
		if bm.frameT >= bombFrameInterval {
			bm.frameT = 0
			bm.frame = 1 - bm.frame
		}
		if bm.y < float64(p.h)+2 {
			kept = append(kept, bm)
		}
	}
	p.bombs = kept
}

func (p *playScene) tickExplosions(s float64) {
	kept := p.explosions[:0]
	for _, e := range p.explosions {
		e.t += s
		if e.t < e.dur {
			kept = append(kept, e)
		}
	}
	p.explosions = kept
}

func (p *playScene) tickStars(s float64) {
	for i := range p.stars {
		st := &p.stars[i]
		st.y += st.speed * s
		st.phase += s * 0.7
		if st.y >= float64(p.h) {
			st.y = -1
			st.x = p.rng.Float64() * float64(p.w)
			st.speed = 2 + p.rng.Float64()*9
			st.tint = p.rng.Intn(4)
		}
	}
}

// spawnBomb adds a bomb falling from (x, y). The kind determines its
// sprite (boss bombs are larger/violet).
func (p *playScene) spawnBomb(x, y float64, speed float64, kind int) {
	if len(p.bombs) >= bombMaxAlive {
		return
	}
	p.bombs = append(p.bombs, &bomb{
		x:    x,
		y:    y,
		vy:   speed,
		kind: kind,
	})
}

func (p *playScene) spawnExplosion(x, y, w, h int, kind int) {
	dur := 0.38
	if kind == 1 {
		dur = playerExplodeDur
	}
	p.explosions = append(p.explosions, &explosion{
		x:    x + w/2 - astroExplode.width()/2,
		y:    y + h/2 - astroExplode.height()/2,
		dur:  dur,
		kind: kind,
	})
}

// =====================================================================
// Collision (player-laser-vs-enemy, bomb-vs-player). Enemy-vs-player
// contact is handled inside each mission's tick because the geometry
// differs (warp ships are radial, flagship is gigantic, etc.).
// =====================================================================

// rect is a simple AABB in canvas pixel coordinates (x0, y0, x1, y1).
type rect struct{ x0, y0, x1, y1 int }

func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && r.x1 > o.x0 && r.y0 < o.y1 && r.y1 > o.y0
}

// playerRect returns the player's current AABB. Used by mission code
// (e.g. warp-ship collision) and by bomb-vs-player checks here.
func (p *playScene) playerRect() rect {
	return rect{
		x0: int(p.player.x),
		y0: int(p.player.y),
		x1: int(p.player.x) + playerSprite.width(),
		y1: int(p.player.y) + playerSprite.height(),
	}
}

// laserRect returns the quad-laser's AABB, or (false) if no shot exists.
func (p *playScene) laserRect() (rect, bool) {
	b := p.player.laser
	if b == nil {
		return rect{}, false
	}
	return rect{
		x0: int(b.x),
		y0: int(b.y),
		x1: int(b.x) + playerLaserSprite.width(),
		y1: int(b.y) + playerLaserSprite.height(),
	}, true
}

// resolveCollisions runs the cross-cutting collision checks that aren't
// mission-specific. The player-laser-vs-enemy resolution lives inside
// each mission tick because the enemy hit logic is different per mission.
func (p *playScene) resolveCollisions() {
	p.collideBombsVsPlayer()
}

func (p *playScene) collideBombsVsPlayer() {
	if p.player.explodeT > 0 {
		return
	}
	pr := p.playerRect()
	kept := p.bombs[:0]
	hit := false
	for _, bm := range p.bombs {
		w, h := bombA.width(), bombA.height()
		if bm.kind == 1 {
			w, h = bossBomb.width(), bossBomb.height()
		}
		br := rect{x0: int(bm.x), y0: int(bm.y), x1: int(bm.x) + w, y1: int(bm.y) + h}
		if !hit && br.overlaps(pr) {
			hit = true
			continue // bomb consumed
		}
		kept = append(kept, bm)
	}
	p.bombs = kept
	if hit {
		p.playerHit()
	}
}

// playerHit consumes one of the player's shields and starts the death
// animation. The mission-specific tick may already have set state to
// psPlayerHit if it killed the player by collision; we guard against
// double-decrement.
func (p *playScene) playerHit() {
	if p.player.explodeT > 0 {
		return
	}
	p.player.lives--
	p.player.explodeT = playerExplodeDur
	p.state = psPlayerHit
	p.stateT = 0
	p.setTaunt(playerHitTaunt(p.rng), playerExplodeDur)
}

var playerHitTaunts = []string{
	"BAD MOVE SPACE CADET",
	"YOU ARE NO MATCH",
	"GORF WINS",
	"PATHETIC",
	"GOOD",
}

func playerHitTaunt(r *rand.Rand) string {
	return playerHitTaunts[r.Intn(len(playerHitTaunts))]
}

// addScore adds points to the player's score, applying the cycle
// multiplier (cycle 1 = ×1, cycle 2 = ×2, …).
func (p *playScene) addScore(pts int) {
	p.score += pts * p.cycle
}

// =====================================================================
// Draw — composes HUD, starfield, mission, common entities, overlays.
// =====================================================================

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 16, A: 255})
	p.drawStars(c)

	// Mission paints its own enemies / terrain.
	switch p.mission {
	case missionAstro:
		p.astro.draw(p, c)
	case missionLaser:
		p.laser.draw(p, c)
	case missionGalaxians:
		p.galax.draw(p, c)
	case missionWarp:
		p.warp.draw(p, c)
	case missionFlag:
		p.flag.draw(p, c)
	}

	p.drawBombs(c)
	p.drawPlayer(c)
	p.drawLaser(c)
	p.drawExplosions(c)
	p.drawHUD(c)
	p.drawTauntOverlay(c)

	switch p.state {
	case psMissionIntro:
		p.drawMissionIntro(c)
	case psMissionCleared:
		p.drawCentreBanner(c,
			fmt.Sprintf("%s CLEARED", p.mission.name()),
			engine.Yellow)
	case psGameOver:
		p.drawGameOver(c)
	}
}

func (p *playScene) drawStars(c *engine.Canvas) {
	for _, s := range p.stars {
		bri := 0.5 + 0.5*math.Sin(s.phase*2*math.Pi)
		var base engine.Color
		switch s.tint {
		case 0:
			base = engine.Color{R: 230, G: 230, B: 240, A: 255}
		case 1:
			base = engine.Color{R: 130, G: 220, B: 240, A: 255}
		case 2:
			base = engine.Color{R: 240, G: 230, B: 150, A: 255}
		default:
			base = engine.Color{R: 240, G: 180, B: 220, A: 255}
		}
		c.Set(int(s.x), int(s.y), engine.Color{
			R: uint8(float64(base.R) * bri),
			G: uint8(float64(base.G) * bri),
			B: uint8(float64(base.B) * bri),
			A: 255,
		})
	}
}

func (p *playScene) drawPlayer(c *engine.Canvas) {
	if p.player.lives <= 0 && p.state == psGameOver {
		return
	}
	if p.player.explodeT > 0 {
		t := playerExplodeDur - p.player.explodeT
		spr := playerExplodeA
		if int(t*10)%2 == 1 {
			spr = playerExplodeB
		}
		drawSprite(c, int(p.player.x), int(p.player.y), spr, playerExplodePalette)
		return
	}
	drawSprite(c, int(p.player.x), int(p.player.y), playerSprite, playerPalette)
}

func (p *playScene) drawLaser(c *engine.Canvas) {
	if b := p.player.laser; b != nil {
		drawSprite(c, int(b.x), int(b.y), playerLaserSprite, playerLaserPalette)
	}
}

func (p *playScene) drawBombs(c *engine.Canvas) {
	for _, bm := range p.bombs {
		var spr sprite
		var pal map[byte]engine.Color
		switch bm.kind {
		case 1:
			spr = bossBomb
			pal = bossBombPalette
		default:
			spr = bombA
			if bm.frame == 1 {
				spr = bombB
			}
			pal = bombPalette
		}
		drawSprite(c, int(bm.x), int(bm.y), spr, pal)
	}
}

func (p *playScene) drawExplosions(c *engine.Canvas) {
	for _, e := range p.explosions {
		switch e.kind {
		case 1:
			// Player explosion already drawn by drawPlayer while alive;
			// after death we'd show debris here, but the simple version
			// just fades the same sprite.
			t := e.t / e.dur
			frame := playerExplodeA
			if int(t*10)%2 == 1 {
				frame = playerExplodeB
			}
			drawSprite(c, e.x, e.y, frame, playerExplodePalette)
		default:
			drawSprite(c, e.x, e.y, astroExplode, astroExplodePalette)
		}
	}
}

func (p *playScene) drawHUD(c *engine.Canvas) {
	cols := c.Cols()
	scoreText := fmt.Sprintf("SCORE %06d", p.score)
	hiText := fmt.Sprintf("HI %06d", p.hiScore)
	missionText := fmt.Sprintf("MISSION %d", int(p.mission)+1)
	cycleText := fmt.Sprintf("CYCLE %d", p.cycle)
	shieldsText := fmt.Sprintf("SHIELDS %d", p.player.lives)
	if p.player.lives < 0 {
		shieldsText = "SHIELDS 0"
	}

	c.Print(1, 0, scoreText, engine.White)
	hiCol := (cols - len(hiText)) / 2
	if hiCol < len(scoreText)+2 {
		hiCol = len(scoreText) + 2
	}
	c.Print(hiCol, 0, hiText, engine.Yellow)
	rightCol := cols - len(missionText) - 1
	if rightCol < hiCol+len(hiText)+2 {
		rightCol = hiCol + len(hiText) + 2
	}
	c.Print(rightCol, 0, missionText, engine.Color{R: 120, G: 220, B: 255, A: 255})

	c.Print(1, 1, shieldsText, engine.Color{R: 130, G: 240, B: 160, A: 255})
	c.Print(cols-len(cycleText)-1, 1, cycleText, engine.Color{R: 250, G: 180, B: 240, A: 255})
}

func (p *playScene) drawMissionIntro(c *engine.Canvas) {
	// Big centred MISSION N banner with the mission name below in
	// terminal-font.
	header := fmt.Sprintf("MISSION %d", int(p.mission)+1)
	hw := engine.TextWidth(header)
	x := (p.w - hw) / 2
	y := (p.h - engine.FontHeight) / 2 - 4
	c.FillRect(x-4, y-2, hw+8, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, header, engine.Color{R: 250, G: 230, B: 110, A: 255})

	name := p.mission.name()
	row := (y+engine.FontHeight)/2 + 1
	c.Print((c.Cols()-len(name))/2, row, name, engine.White)
}

func (p *playScene) drawCentreBanner(c *engine.Canvas, text string, col engine.Color) {
	w := engine.TextWidth(text)
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2
	c.FillRect(x-3, y-2, w+6, engine.FontHeight+4, engine.Color{R: 6, G: 6, B: 22, A: 255})
	c.DrawText(x, y, text, col)
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	w := engine.TextWidth("GAME OVER")
	x := (p.w - w) / 2
	y := (p.h-engine.FontHeight)/2 - 4
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, "GAME OVER", engine.Color{R: 250, G: 80, B: 80, A: 255})

	gloat := "GORF IS VICTORIOUS"
	c.Print((c.Cols()-len(gloat))/2, c.Rows()/2+2, gloat, engine.Color{R: 250, G: 90, B: 240, A: 255})

	hint := "ENTER PLAY AGAIN   ESC QUIT"
	c.Print((c.Cols()-len(hint))/2, c.Rows()/2+3, hint, engine.White)
}

func (p *playScene) drawTauntOverlay(c *engine.Canvas) {
	if p.taunt == "" || p.tauntT >= p.tauntDur {
		return
	}
	u := p.tauntT / p.tauntDur
	dim := 1.0
	if u < 0.15 {
		dim = u / 0.15
	} else if u > 0.85 {
		dim = (1 - u) / 0.15
	}
	if dim < 0 {
		dim = 0
	}
	base := engine.Color{R: 250, G: 100, B: 240, A: 255}
	col := engine.Color{
		R: uint8(float64(base.R) * dim),
		G: uint8(float64(base.G) * dim),
		B: uint8(float64(base.B) * dim),
		A: 255,
	}
	// Place taunt just above the player at the bottom of the play area.
	row := c.Rows() - 4
	c.Print((c.Cols()-len(p.taunt))/2, row, p.taunt, col)
}
