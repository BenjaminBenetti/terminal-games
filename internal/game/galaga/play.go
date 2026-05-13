package galaga

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Speeds are in pixels per second; times in seconds.
const (
	playerSpeed         = 40.0
	playerBulletSpeed   = 90.0
	maxPlayerBullets    = 2 // dual-shot is the canonical Galaga feel
	playerFireGap       = 0.08
	playerExplodeDur    = 1.2
	playerRespawnDur    = 0.8

	enemyBombSpeed   = 32.0
	enemyBombMax     = 5    // total in-flight bombs cap
	enemyBombMinGap  = 0.55 // per-enemy fire cool-down (avoids spamming)
	enemyAnimPeriod  = 0.32 // wing flap interval

	enemyDiveSpd     = 55.0
	enemyReturnSpd   = 55.0

	// Dive scheduling. The first scheduled dive after formation-complete,
	// and the per-dive cool-down (jittered). These scale down per stage
	// to ramp difficulty.
	diveFirstDelay     = 1.4
	diveBaseInterval   = 2.2
	diveIntervalJitter = 1.5
	maxConcurrentDives = 3
	tractorChance      = 0.40 // probability a boss dive becomes a beam capture

	// Tractor beam timing.
	beamOpenDur     = 0.55 // beam expands open
	beamHoldDur     = 1.6  // beam fully open
	beamCloseDur    = 0.45 // beam shrinks closed
	beamTotalDur    = beamOpenDur + beamHoldDur + beamCloseDur
	beamWidth       = 22   // peak width of the beam at the player's row
	beamHoverYRatio = 0.30 // boss hover y as a fraction of canvas height

	stageBannerDur = 2.0

	// Stars in the parallax background.
	starCount = 40
	starSpeed = 6.0
)

// rect is a simple AABB in canvas pixel coordinates.
type rect struct{ x0, y0, x1, y1 int }

func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && r.x1 > o.x0 && r.y0 < o.y1 && r.y1 > o.y0
}

// playState is the play-scene sub-state machine. The top-level scene
// distinguishes only title / play / game-over; this exists inside play.
type playState int

const (
	psStageIntro   playState = iota // brief "STAGE n" banner
	psPlaying                       // gameplay
	psPlayerHit                     // explosion + brief delay
	psStageCleared                  // brief delay before next stage
	psGameOver
)

// captureAnimDur is how long the player is "captured" (invisible /
// uncontrollable) during the tractor-beam suck animation before the
// next life spawns.
const captureAnimDur = 0.6

// bullet is a single player projectile (going up).
type playerBulletEntity struct {
	x, y float64
}

// bomb is a single enemy projectile (going down).
type bombEntity struct {
	x, y   float64
	frame  int
	frameT float64
}

// star is a single twinkling/parallax pixel in the background.
type star struct {
	x, y  float64
	c     engine.Color
	twink float64
}

// playerEntity is the defender at the bottom.
type playerEntity struct {
	x        float64 // sprite-left pixel x of the primary ship
	y        int     // sprite-top pixel y (fixed)
	cooldown float64
	lives    int
	explodeT float64 // >0 means in explosion animation
	respawnT float64 // >0 means freshly respawned, blinking
	dual     bool    // true if the captured ship is rescued and attached
	// captured is true during the brief tractor-beam suck animation. The
	// ship is not drawn or controllable while this is set; once the
	// captureAnimT timer expires the next-life ship spawns. The boss
	// carrying the captured ship is tracked separately on
	// playScene.bossWithShip and persists until it's shot down.
	captured     bool
	captureAnimT float64
}

// playScene is the active gameplay state.
type playScene struct {
	e    *engine.Engine
	w, h int

	player  playerEntity
	bullets []*playerBulletEntity
	bombs   []*bombEntity
	enemies []*enemy
	form    formation

	stars []star

	plan          []waveDef
	stageT        float64 // seconds since the stage entry script began
	pendingSpawn  map[int]int // wave index -> next slot index to spawn
	diveTimer     float64
	bossWithShip  *enemy // boss currently carrying the player's captured ship, if any

	score   int
	hiScore int
	stage   int

	state  playState
	stateT float64

	rng *rand.Rand

	// Layout
	playerY int
	formTop int
	hudRows int

	wantQuit bool
}

// newPlayScene constructs a play scene sized to the engine's canvas.
func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		hiScore: hiScore,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		stage:   1,
	}
	p.player.lives = 3
	p.computeLayout()
	p.spawnStars()
	p.beginStage(1)
	return p
}

// computeLayout derives Y bands for HUD, formation top, and the player
// from canvas dimensions. The formation centers horizontally; vertical
// placement reserves space for diving below.
func (p *playScene) computeLayout() {
	p.hudRows = 2
	p.playerY = p.h - playerSprite.height() - 1

	// Formation top: just below the HUD with a tiny breathing-room gap.
	p.formTop = p.hudRows*2 + 1

	// Formation origin x centres the slot grid horizontally.
	formW := formationWidthPx(playerSprite.width())
	p.form.originX = float64((p.w - formW) / 2)
	p.form.originY = float64(p.formTop)
}

// beginStage resets formation/wave state for the given stage and queues
// the entry script. Bunkers don't exist in Galaga — the only persistent
// shielding effect is the dual fighter, which carries over between
// stages if you're holding it.
func (p *playScene) beginStage(stage int) {
	p.stage = stage
	p.state = psStageIntro
	p.stateT = 0
	p.stageT = 0
	p.enemies = nil
	p.bullets = nil
	p.bombs = nil
	p.bossWithShip = nil
	p.plan = stagePlan()
	p.pendingSpawn = map[int]int{}
	p.diveTimer = diveFirstDelay
	p.player.cooldown = 0
	p.player.explodeT = 0
	p.player.respawnT = playerRespawnDur
	p.player.captured = false
	if p.player.lives <= 0 {
		p.player.lives = 1
	}
	// Centre the player.
	w := playerSprite.width()
	if p.player.dual {
		w = dualPlayerSprite.width()
	}
	p.player.x = float64(p.w-w) / 2
	p.player.y = p.playerY
}

func (p *playScene) spawnStars() {
	p.stars = make([]star, starCount)
	for i := range p.stars {
		p.stars[i] = star{
			x:     p.rng.Float64() * float64(p.w),
			y:     p.rng.Float64() * float64(p.h),
			c:     starPalette[p.rng.Intn(len(starPalette))],
			twink: p.rng.Float64(),
		}
	}
}

// --- Update path --------------------------------------------------------

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s

	// Stars always tick. Sway pauses while any enemy is still flying in
	// along its entry path — otherwise the slot endpoint chosen when the
	// path was generated would drift away from where the enemy lands,
	// producing a visible "snap" once it transitions to esFormation.
	p.tickStars(s)
	anyEntering := false
	for _, e := range p.enemies {
		if e.state == esEntering {
			anyEntering = true
			break
		}
	}
	if !anyEntering {
		p.form.swayT += s
	}

	switch p.state {
	case psStageIntro:
		if p.stateT >= stageBannerDur {
			p.state = psPlaying
			p.stateT = 0
			p.stageT = 0
		}
	case psPlaying:
		p.updatePlaying(s)
	case psPlayerHit:
		p.tickEnemies(s)
		p.tickBombs(s)
		p.tickBullets(s)
		p.player.explodeT -= s
		if p.player.explodeT <= 0 {
			if p.player.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.player.explodeT = 0
				p.player.respawnT = playerRespawnDur
				w := playerSprite.width()
				if p.player.dual {
					w = dualPlayerSprite.width()
				}
				p.player.x = float64(p.w-w) / 2
				p.state = psPlaying
				p.stateT = 0
			}
		}
	case psStageCleared:
		if p.stateT >= stageBannerDur {
			p.beginStage(p.stage + 1)
		}
	case psGameOver:
		// Just wait for input.
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
		case psPlayerHit, psStageIntro, psStageCleared:
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
				p.player.dual = false
				p.hiScore = hi
				p.beginStage(1)
			} else if k.Code == engine.KeyEsc ||
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

func (p *playScene) tryFire() {
	if p.player.captured || p.player.explodeT > 0 {
		return
	}
	cap := maxPlayerBullets
	if p.player.dual {
		cap = maxPlayerBullets * 2
	}
	if len(p.bullets) >= cap {
		return
	}
	if p.player.cooldown > 0 {
		return
	}
	if p.player.dual {
		// Twin shot: one from each ship.
		dw := dualPlayerSprite.width()
		left := p.player.x + 3
		right := p.player.x + float64(dw) - 4
		by := float64(p.player.y) - float64(playerBullet.height())
		p.bullets = append(p.bullets,
			&playerBulletEntity{x: left, y: by},
			&playerBulletEntity{x: right, y: by})
	} else {
		bx := p.player.x + float64(playerSprite.width())/2
		by := float64(p.player.y) - float64(playerBullet.height())
		p.bullets = append(p.bullets, &playerBulletEntity{x: bx, y: by})
	}
	p.player.cooldown = playerFireGap
}

// updatePlaying drives the main gameplay tick.
func (p *playScene) updatePlaying(s float64) {
	p.stageT += s

	// Tick the capture animation. When it elapses the player respawns
	// at centre (with the usual blink invincibility) — unless the
	// capture also drained the last life, in which case it's game over.
	// The boss carrying the captured ship is unaffected and continues
	// independently; the player must shoot it later to free the ship
	// and earn the dual fighter.
	if p.player.captured {
		p.player.captureAnimT -= s
		if p.player.captureAnimT <= 0 {
			p.player.captured = false
			if p.player.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.player.respawnT = playerRespawnDur
				p.player.x = float64(p.w-playerSprite.width()) / 2
			}
		}
	}

	// Player horizontal motion via held-key polling.
	if p.player.respawnT > 0 {
		p.player.respawnT -= s
	}
	if !p.player.captured {
		left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
		right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
		var dir int
		switch {
		case left && !right:
			dir = -1
		case right && !left:
			dir = 1
		}
		if dir != 0 {
			p.player.x += float64(dir) * playerSpeed * s
		}
		pw := playerSprite.width()
		if p.player.dual {
			pw = dualPlayerSprite.width()
		}
		maxX := float64(p.w - pw)
		if p.player.x < 0 {
			p.player.x = 0
		}
		if p.player.x > maxX {
			p.player.x = maxX
		}
	}
	if p.player.cooldown > 0 {
		p.player.cooldown -= s
	}

	// Spawn enemies from the stage entry plan.
	p.tickEntryScript(s)

	// Enemies (movement, animation, decisions to dive).
	p.tickEnemies(s)

	// Bombs.
	p.tickBombs(s)

	// Bullets.
	p.tickBullets(s)

	// Maybe schedule a new dive.
	p.maybeStartDive(s)

	// Collisions.
	p.resolveBulletEnemyHits()
	p.resolveBombPlayer()
	p.resolveEnemyPlayerRam()
	p.resolveTractorBeam()

	// Stage clear? Only transition if a collision earlier in this tick
	// didn't already kick us into another state (death, capture).
	if p.state == psPlaying && p.formationCleared() {
		p.state = psStageCleared
		p.stateT = 0
	}
}

// formationCleared reports true if every enemy in the formation has
// been destroyed and there are no in-flight enemies left. Diving
// enemies and ones still entering count as alive.
func (p *playScene) formationCleared() bool {
	if p.stageT < 0.5 {
		return false // before script kicks in
	}
	// Count if any enemy is still alive in any state.
	for _, e := range p.enemies {
		if e.alive() {
			return false
		}
	}
	// Also make sure every wave has fully spawned, otherwise we'd race
	// the script and clear before bosses are born.
	for i, w := range p.plan {
		if p.pendingSpawn[i] < len(w.slots) {
			return false
		}
	}
	return true
}

// tickEntryScript spawns enemies according to the per-wave start time
// and per-enemy spacing.
func (p *playScene) tickEntryScript(s float64) {
	_ = s
	for i, w := range p.plan {
		idx := p.pendingSpawn[i]
		if idx >= len(w.slots) {
			continue
		}
		due := w.startT + float64(idx)*w.spacing
		if p.stageT < due {
			continue
		}
		p.spawnFormationEntry(w, w.slots[idx])
		p.pendingSpawn[i] = idx + 1
	}
}

func (p *playScene) spawnFormationEntry(w waveDef, sl slotRC) {
	target := p.form.slotPos(sl.row, sl.col)
	var path *path
	switch w.pathKind {
	case pathTopLoopLeft:
		path = entryTopLoopLeft(p.w, p.h, target.x, target.y)
	case pathTopLoopRight:
		path = entryTopLoopRight(p.w, p.h, target.x, target.y)
	case pathBottomSweepLeft:
		path = entryBottomSweepLeft(p.w, p.h, target.x, target.y)
	case pathBottomSweepRight:
		path = entryBottomSweepRight(p.w, p.h, target.x, target.y)
	case pathTopDirect:
		path = entryTopDirect(p.w, p.h, target.x, target.y)
	}
	start := path.at(0)
	e := &enemy{
		kind:    kindForSlot(sl.row, sl.col),
		slotRow: sl.row,
		slotCol: sl.col,
		state:   esEntering,
		x:       start.x,
		y:       start.y,
		path:    path,
		pathSpd: w.speed,
	}
	p.enemies = append(p.enemies, e)
}

// tickEnemies advances each enemy's per-frame state — path traversal,
// animation, and state transitions when they reach path ends.
func (p *playScene) tickEnemies(s float64) {
	kept := p.enemies[:0]
	for _, e := range p.enemies {
		// Animation.
		e.frameT += s
		if e.frameT >= enemyAnimPeriod {
			e.frameT -= enemyAnimPeriod
			e.frame = 1 - e.frame
		}

		switch e.state {
		case esEntering:
			e.pathDist += e.pathSpd * s
			pos := e.path.at(e.pathDist)
			e.x, e.y = pos.x, pos.y
			if e.pathDist >= e.path.total {
				e.state = esFormation
			}
		case esFormation:
			slot := p.form.slotPos(e.slotRow, e.slotCol)
			e.x, e.y = slot.x, slot.y
		case esDiving:
			e.pathDist += e.pathSpd * s
			pos := e.path.at(e.pathDist)
			e.x, e.y = pos.x, pos.y

			// Bomb dropping during dives.
			e.bombCooldown -= s
			if e.bombCooldown <= 0 && len(p.bombs) < enemyBombMax {
				// Don't spam: only fire when the enemy is in the upper
				// two-thirds of the canvas (i.e. before it gets too
				// close to the player to be dodgeable).
				if e.y < float64(p.h)*0.70 && e.y > 0 {
					p.bombs = append(p.bombs, &bombEntity{
						x: e.x + 3,
						y: e.y + 5,
					})
					e.bombCooldown = enemyBombMinGap + p.rng.Float64()*0.6
				} else {
					e.bombCooldown = 0.2
				}
			}

			if e.pathDist >= e.path.total {
				if e.tractorDive {
					// Reached the hover point for the tractor-beam
					// sequence. Stop here and let esHoverBeam play out
					// the beam open/hold/close animation.
					e.state = esHoverBeam
					e.beamT = 0
				} else {
					// Off-screen at bottom — switch to a return path that
					// reappears at the top and curves back to the slot.
					e.state = esReturning
					slot := p.form.slotPos(e.slotRow, e.slotCol)
					e.path = returnPath(p.w, p.h, slot.x, slot.y)
					e.pathDist = 0
					e.pathSpd = enemyReturnSpd
					start := e.path.at(0)
					e.x, e.y = start.x, start.y
				}
			}
		case esReturning:
			e.pathDist += e.pathSpd * s
			pos := e.path.at(e.pathDist)
			e.x, e.y = pos.x, pos.y
			if e.pathDist >= e.path.total {
				e.state = esFormation
			}
		case esHoverBeam:
			// Stationary while the beam plays. Beam progress advances
			// in resolveTractorBeam; here we just step the timer for
			// rendering. The transition out of the beam happens when
			// the beam ends (in resolveTractorBeam).
			e.beamT += s
			if e.beamT >= beamTotalDur {
				// Beam closed; this boss now returns to formation. If it
				// captured the player it carries the ship back via the
				// carry state; otherwise straight return.
				if e.carryHasShip {
					e.state = esCarryShip
				} else {
					e.state = esReturning
				}
				slot := p.form.slotPos(e.slotRow, e.slotCol)
				e.path = returnPath(p.w, p.h, slot.x, slot.y)
				e.pathDist = 0
				e.pathSpd = enemyReturnSpd
				start := e.path.at(0)
				e.x, e.y = start.x, start.y
			}
		case esCarryShip:
			// Same as esReturning, but render with the captured ship
			// inverted underneath.
			e.pathDist += e.pathSpd * s
			pos := e.path.at(e.pathDist)
			e.x, e.y = pos.x, pos.y
			if e.pathDist >= e.path.total {
				e.state = esFormation
				// Boss now lives in the formation with the captured ship
				// attached — visualised in drawEnemies.
			}
		case esDying:
			e.dyingT += s
			if e.dyingT >= 0.45 {
				e.state = esGone
				continue
			}
		}
		if e.state != esGone {
			kept = append(kept, e)
		}
	}
	p.enemies = kept
}

func (p *playScene) tickBombs(s float64) {
	kept := p.bombs[:0]
	for _, b := range p.bombs {
		b.y += enemyBombSpeed * s
		b.frameT += s
		if b.frameT >= 0.12 {
			b.frameT = 0
			b.frame = 1 - b.frame
		}
		if b.y < float64(p.h) {
			kept = append(kept, b)
		}
	}
	p.bombs = kept
}

func (p *playScene) tickBullets(s float64) {
	kept := p.bullets[:0]
	for _, b := range p.bullets {
		b.y -= playerBulletSpeed * s
		if b.y+float64(playerBullet.height()) >= 0 {
			kept = append(kept, b)
		}
	}
	p.bullets = kept
}

func (p *playScene) tickStars(s float64) {
	for i := range p.stars {
		p.stars[i].y += starSpeed * s
		p.stars[i].twink += s * 4
		if p.stars[i].y >= float64(p.h) {
			p.stars[i].y = 0
			p.stars[i].x = p.rng.Float64() * float64(p.w)
			p.stars[i].c = starPalette[p.rng.Intn(len(starPalette))]
		}
	}
}

// maybeStartDive schedules the next enemy dive once the formation has
// finished assembling. Only formation-resident enemies can be drafted,
// and we cap concurrent dives.
func (p *playScene) maybeStartDive(s float64) {
	if p.state != psPlaying {
		return
	}
	// Wait for formation to mostly assemble.
	if p.stageT < diveFirstDelay {
		return
	}
	// Count current dives.
	diving := 0
	for _, e := range p.enemies {
		if e.state == esDiving || e.state == esHoverBeam ||
			e.state == esCarryShip || e.state == esReturning {
			diving++
		}
	}
	if diving >= maxConcurrentDives {
		return
	}
	p.diveTimer -= s
	if p.diveTimer > 0 {
		return
	}
	// Stage-scaled interval — each stage shaves a bit off.
	scale := math.Pow(0.92, float64(p.stage-1))
	p.diveTimer = (diveBaseInterval + p.rng.Float64()*diveIntervalJitter) * scale
	// Pick a random formation enemy.
	var pool []int
	for i, e := range p.enemies {
		if e.state == esFormation {
			pool = append(pool, i)
		}
	}
	if len(pool) == 0 {
		return
	}
	idx := pool[p.rng.Intn(len(pool))]
	p.startDive(p.enemies[idx])
}

// startDive transitions an enemy from esFormation to esDiving with a
// freshly computed dive path. Bosses with luck go on a tractor-beam
// run instead.
func (p *playScene) startDive(e *enemy) {
	if e.kind == enemyBoss && !p.player.captured && p.bossWithShip == nil &&
		p.rng.Float64() < tractorChance {
		hoverY := float64(p.h) * beamHoverYRatio
		// Pull hoverX from canvas centre toward the current player x so
		// the beam has at least a chance of catching the player without
		// being trivially walked around.
		hoverX := (float64(p.w-7)/2 + p.player.x) / 2
		e.path = diveTractor(p.w, p.h, e.x, e.y, hoverX, hoverY)
		e.pathDist = 0
		e.pathSpd = enemyDiveSpd
		e.state = esDiving
		e.tractorDive = true
		// tickEnemies will transition to esHoverBeam once the path
		// completes (because tractorDive is set).
		return
	}
	e.tractorDive = false
	dir := -1
	if e.slotCol > formationCols/2 {
		dir = 1
	}
	// Three dive flavours weighted by enemy type.
	roll := p.rng.Float64()
	var path *path
	switch e.kind {
	case enemyBee:
		if roll < 0.5 {
			path = diveStraight(p.w, p.h, e.x, e.y, p.player.x)
		} else {
			path = diveSwoop(p.w, p.h, e.x, e.y, p.player.x, dir)
		}
	case enemyButterfly:
		if roll < 0.35 {
			path = diveStraight(p.w, p.h, e.x, e.y, p.player.x)
		} else if roll < 0.75 {
			path = diveSwoop(p.w, p.h, e.x, e.y, p.player.x, dir)
		} else {
			path = diveLoop(p.w, p.h, e.x, e.y, p.player.x, dir)
		}
	case enemyBoss:
		if roll < 0.5 {
			path = diveSwoop(p.w, p.h, e.x, e.y, p.player.x, dir)
		} else {
			path = diveLoop(p.w, p.h, e.x, e.y, p.player.x, dir)
		}
	}
	e.path = path
	e.pathDist = 0
	e.pathSpd = enemyDiveSpd
	e.state = esDiving
	e.bombCooldown = 0.4 + p.rng.Float64()*0.5
}

// --- Collisions --------------------------------------------------------

func (p *playScene) playerRect() rect {
	if p.player.captured {
		return rect{}
	}
	w := playerSprite.width()
	if p.player.dual {
		w = dualPlayerSprite.width()
	}
	return rect{
		x0: int(p.player.x),
		y0: p.player.y,
		x1: int(p.player.x) + w,
		y1: p.player.y + playerSprite.height(),
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
		// Hover/dying don't take hits ... actually hover does take hits
		// (that's how you save your captured ship). Dying does not.
		if e.state == esDying || e.state == esGone {
			continue
		}
		er := e.boundingBox()
		hitIdx := -1
		for i, b := range p.bullets {
			br := rect{
				x0: int(b.x),
				y0: int(b.y),
				x1: int(b.x) + playerBullet.width(),
				y1: int(b.y) + playerBullet.height(),
			}
			if br.overlaps(er) {
				hitIdx = i
				break
			}
		}
		if hitIdx < 0 {
			continue
		}
		// Remove the bullet.
		p.bullets = append(p.bullets[:hitIdx], p.bullets[hitIdx+1:]...)
		// Apply hit.
		diving := e.state == esDiving || e.state == esHoverBeam ||
			e.state == esCarryShip || e.state == esReturning
		killed := e.applyHit(diving)
		if !killed {
			// Boss took one hit, doesn't die. Continue.
			continue
		}
		// Score.
		if diving {
			p.score += e.kind.flightScore()
		} else {
			p.score += e.kind.formationScore()
		}
		// Special rescues: if the boss carrying the captured ship is killed
		// while in esHoverBeam or esCarryShip, the player gets the dual
		// fighter and is restored.
		if e == p.bossWithShip {
			// Rescue: the boss carrying our captured ship has been
			// destroyed. Award the dual fighter — the next bullet
			// volley fires from both ships.
			p.bossWithShip = nil
			p.player.dual = true
		}
		// Kill the enemy.
		e.state = esDying
		e.dyingT = 0
	}
}

func (p *playScene) resolveBombPlayer() {
	if p.player.captured || p.player.explodeT > 0 {
		return
	}
	pr := p.playerRect()
	kept := p.bombs[:0]
	hit := false
	for _, b := range p.bombs {
		if hit {
			kept = append(kept, b)
			continue
		}
		br := rect{
			x0: int(b.x),
			y0: int(b.y),
			x1: int(b.x) + enemyBombA.width(),
			y1: int(b.y) + enemyBombA.height(),
		}
		if br.overlaps(pr) {
			hit = true
			continue
		}
		kept = append(kept, b)
	}
	p.bombs = kept
	if hit {
		p.killPlayer()
	}
}

// resolveEnemyPlayerRam handles a diving enemy crashing into the player.
func (p *playScene) resolveEnemyPlayerRam() {
	if p.player.captured || p.player.explodeT > 0 {
		return
	}
	pr := p.playerRect()
	for _, e := range p.enemies {
		if !e.alive() {
			continue
		}
		if e.state != esDiving {
			continue
		}
		if pr.overlaps(e.boundingBox()) {
			// Score the ram-killed enemy.
			p.score += e.kind.flightScore()
			e.state = esDying
			e.dyingT = 0
			p.killPlayer()
			return
		}
	}
}

// resolveTractorBeam tests the player against every active beam each
// frame. The beam state machine (open / hold / close) is driven by
// tickEnemies's esHoverBeam handler; this function only checks for
// capture overlap.
func (p *playScene) resolveTractorBeam() {
	if p.player.captured {
		return
	}
	for _, e := range p.enemies {
		if e.state != esHoverBeam {
			continue
		}
		// Beam is "active" during open + hold; the player must be inside
		// the beam cone.
		if e.beamT < beamOpenDur*0.4 || e.beamT > beamOpenDur+beamHoldDur {
			continue
		}
		beamRect := p.beamRectFor(e)
		if p.playerRect().overlaps(beamRect) {
			p.capturePlayer(e)
			return
		}
	}
}

// beamRectFor returns the current beam footprint rectangle at the
// player's row. The beam is a tapering cone widest at the player's row.
func (p *playScene) beamRectFor(e *enemy) rect {
	// Beam centred under the boss, opening over time.
	progress := 1.0
	if e.beamT < beamOpenDur {
		progress = e.beamT / beamOpenDur
	} else if e.beamT > beamOpenDur+beamHoldDur {
		progress = 1.0 - (e.beamT-beamOpenDur-beamHoldDur)/beamCloseDur
		if progress < 0 {
			progress = 0
		}
	}
	halfWidth := int(float64(beamWidth) / 2 * progress)
	cx := int(e.x) + 3
	top := int(e.y) + 5
	bot := p.playerY + playerSprite.height()
	return rect{
		x0: cx - halfWidth,
		y0: top,
		x1: cx + halfWidth,
		y1: bot,
	}
}

func (p *playScene) capturePlayer(boss *enemy) {
	p.player.captured = true
	p.bossWithShip = boss
	boss.carryHasShip = true
	// The capture costs a life. While the suck-up animation plays the
	// player is uncontrollable and invisible (p.player.captured). After
	// captureAnimDur elapses the next ship spawns at centre with the
	// normal respawn-blink invincibility — gameplay continues, and the
	// player must shoot the boss carrying their captured ship to free
	// it and earn the dual fighter.
	p.player.lives--
	p.player.captured = true
	p.player.captureAnimT = captureAnimDur
	p.player.dual = false
	// Game-over transition is handled where the capture animation
	// expires — that way the suck-up animation still plays out even
	// when this was the final ship.
}

func (p *playScene) killPlayer() {
	p.player.lives--
	if p.player.dual {
		// Losing one of the two ships demotes back to single ship — in
		// the arcade you keep playing with the surviving half.
		p.player.dual = false
		p.player.explodeT = playerExplodeDur * 0.6
		p.state = psPlayerHit
		p.stateT = 0
		return
	}
	p.player.explodeT = playerExplodeDur
	p.state = psPlayerHit
	p.stateT = 0
}

// --- Rendering ---------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 0, G: 0, B: 8, A: 255})
	p.drawStars(c)
	p.drawHUD(c)
	p.drawTractorBeams(c)
	p.drawEnemies(c)
	p.drawPlayer(c)
	p.drawBullets(c)
	p.drawBombs(c)

	switch p.state {
	case psStageIntro:
		p.drawCentreText(c, fmt.Sprintf("STAGE %d", p.stage), engine.Color{R: 240, G: 240, B: 200, A: 255})
	case psStageCleared:
		p.drawCentreText(c, fmt.Sprintf("STAGE %d CLEARED", p.stage), engine.Color{R: 240, G: 240, B: 120, A: 255})
	case psGameOver:
		p.drawGameOver(c)
	}
}

func (p *playScene) drawStars(c *engine.Canvas) {
	for _, s := range p.stars {
		// Twinkle: dim every other beat.
		col := s.c
		if math.Sin(s.twink) < -0.5 {
			col = engine.Color{R: col.R / 3, G: col.G / 3, B: col.B / 3, A: 255}
		}
		c.Set(int(s.x), int(s.y), col)
	}
}

func (p *playScene) drawHUD(c *engine.Canvas) {
	cols := c.Cols()
	scoreText := fmt.Sprintf("SCORE %06d", p.score)
	hiText := fmt.Sprintf("HI %06d", p.hiScore)
	stageText := fmt.Sprintf("STAGE %d", p.stage)

	c.Print(1, 0, scoreText, engine.White)
	mid := (cols - len(hiText)) / 2
	if mid < len(scoreText)+2 {
		mid = len(scoreText) + 2
	}
	c.Print(mid, 0, hiText, engine.Yellow)
	rightCol := cols - len(stageText) - 1
	if rightCol < mid+len(hiText)+2 {
		rightCol = mid + len(hiText) + 2
	}
	c.Print(rightCol, 0, stageText, engine.Cyan)

	// Lives counter: one player-ship icon per *remaining* ship beyond
	// the one currently in play. lives goes to -1 on the final death.
	reserve := p.player.lives - 1
	if p.player.captured {
		// During capture animation the captured ship is gone; lives
		// already decremented, so reserve is the still-spawnable count.
		reserve = p.player.lives
	}
	for i := 0; i < reserve; i++ {
		x := 1 + i*(playerSprite.width()+1)
		drawSprite(c, x, 2, playerSprite,
			engine.Color{R: 100, G: 240, B: 180, A: 255})
	}
}

func (p *playScene) drawEnemies(c *engine.Canvas) {
	for _, e := range p.enemies {
		switch e.state {
		case esDying:
			p.drawEnemyExplosion(c, e)
			continue
		case esGone:
			continue
		}
		fa, fb := e.kind.frames()
		spr := fa
		if e.frame == 1 {
			spr = fb
		}
		col := e.kind.color()
		// Boss that's taken a hit (still alive) renders blue/dim to signal it.
		if e.kind == enemyBoss && e.hits > 0 {
			col = engine.Color{R: 80, G: 150, B: 255, A: 255}
		}
		drawSprite(c, int(e.x), int(e.y), spr, col)
		// Carried ship: while the boss is carrying the player's captured
		// ship, render it inverted beneath the boss in every visible
		// state (carrying back, sitting in formation, diving again with
		// the ship still attached).
		if e == p.bossWithShip && e.alive() && e.state != esHoverBeam {
			drawSpriteFlipY(c, int(e.x), int(e.y)+6, playerSprite,
				engine.Color{R: 100, G: 240, B: 180, A: 255})
		}
	}
}

func (p *playScene) drawEnemyExplosion(c *engine.Canvas, e *enemy) {
	frames := []sprite{enemyExplode0, enemyExplode1, enemyExplode2}
	step := int(e.dyingT / 0.15)
	if step < 0 {
		step = 0
	}
	if step >= len(frames) {
		step = len(frames) - 1
	}
	col := engine.Color{R: 255, G: 220, B: 120, A: 255}
	drawSprite(c, int(e.x), int(e.y), frames[step], col)
}

func (p *playScene) drawPlayer(c *engine.Canvas) {
	if p.player.captured {
		return
	}
	if p.state == psGameOver {
		return
	}
	if p.player.explodeT > 0 {
		frame := playerExplodeA
		if int((playerExplodeDur-p.player.explodeT)*10)%2 == 1 {
			frame = playerExplodeB
		}
		drawSprite(c, int(p.player.x), p.player.y, frame,
			engine.Color{R: 255, G: 200, B: 80, A: 255})
		return
	}
	// Blink during respawn invincibility.
	if p.player.respawnT > 0 {
		if int(p.player.respawnT*10)%2 == 0 {
			return
		}
	}
	spr := playerSprite
	if p.player.dual {
		spr = dualPlayerSprite
	}
	drawSprite(c, int(p.player.x), p.player.y, spr,
		engine.Color{R: 100, G: 240, B: 180, A: 255})
}

func (p *playScene) drawBullets(c *engine.Canvas) {
	for _, b := range p.bullets {
		drawSprite(c, int(b.x), int(b.y), playerBullet,
			engine.Color{R: 240, G: 240, B: 255, A: 255})
	}
}

func (p *playScene) drawBombs(c *engine.Canvas) {
	for _, b := range p.bombs {
		spr := enemyBombA
		if b.frame == 1 {
			spr = enemyBombB
		}
		drawSprite(c, int(b.x), int(b.y), spr,
			engine.Color{R: 255, G: 220, B: 100, A: 255})
	}
}

func (p *playScene) drawTractorBeams(c *engine.Canvas) {
	for _, e := range p.enemies {
		if e.state != esHoverBeam {
			continue
		}
		progress := 1.0
		if e.beamT < beamOpenDur {
			progress = e.beamT / beamOpenDur
		} else if e.beamT > beamOpenDur+beamHoldDur {
			progress = 1.0 - (e.beamT-beamOpenDur-beamHoldDur)/beamCloseDur
			if progress < 0 {
				progress = 0
			}
		}
		halfWidth := float64(beamWidth) / 2 * progress
		cx := e.x + 3
		top := e.y + 5
		bot := float64(p.playerY)
		// Beam shimmer colour alternates with time.
		colA := engine.Color{R: 240, G: 200, B: 80, A: 255}
		colB := engine.Color{R: 180, G: 240, B: 80, A: 255}
		// Draw the beam as a series of horizontal lines whose half-width
		// grows from 0 at the boss to halfWidth at the player row.
		rows := int(bot - top)
		if rows < 1 {
			continue
		}
		stripe := int(e.beamT*30) % 6
		for r := 0; r < rows; r++ {
			t := float64(r) / float64(rows)
			w := int(halfWidth * t)
			y := int(top) + r
			col := colA
			if (r+stripe)%6 < 3 {
				col = colB
			}
			x0 := int(cx) - w
			x1 := int(cx) + w
			for x := x0; x <= x1; x++ {
				c.Set(x, y, col)
			}
		}
	}
}

func (p *playScene) drawCentreText(c *engine.Canvas, text string, col engine.Color) {
	w := engine.TextWidth(text)
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2
	c.FillRect(x-3, y-2, w+6, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, text, col)
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	w := engine.TextWidth("GAME OVER")
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2 - 4
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, "GAME OVER", engine.Color{R: 255, G: 90, B: 90, A: 255})

	hint := "ENTER PLAY AGAIN   ESC QUIT"
	hw := len(hint)
	c.Print((c.Cols()-hw)/2, c.Rows()/2+2, hint, engine.White)
}
