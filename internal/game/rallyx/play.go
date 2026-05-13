package rallyx

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Speeds are in TILES per second. The original
// arcade ran cars at roughly 8 tiles/s with a slight edge for the
// player at the start of every stage; the AI's effective speed
// ramped up over time to apply pressure.
const (
	basePlayerSpeed = 7.0
	baseEnemySpeed  = 5.4

	// Fuel: a full tank lasts ~70 seconds of driving, dropping faster
	// while smoke is being deployed (smoke costs fuel, just like in
	// the arcade). The fuel display 0..100 maps to fullTank seconds.
	fullTank         = 70.0
	smokeFuelDrain   = 5.0 // additional drain per second of smoke held
	emptyTankSpeedMul = 0.55 // car slows when out of fuel

	// Smoke trail: each puff lasts smokePuffTTL, the player drops
	// them at smokeDropPeriod intervals while the smoke button is
	// held, and an enemy that touches a live puff is stunned for
	// smokeStunDuration seconds.
	smokePuffTTL      = 2.0
	smokeDropPeriod   = 0.10
	smokeStunDuration = 1.4
	smokeHitRadiusSq  = 0.45 * 0.45

	// Scoring (lifted from the cabinet manual).
	scoreFlag         = 100
	scoreLuckyFlag    = 1000 // arcade pays out 1000 for the lucky one
	scoreCrashEnemy   = 200  // enemy stunned then driven into / hits a rock
	extraLifeScore    = 20000

	// Round / stage timing.
	readyHold     = 2.0
	dyingHold     = 1.6
	stageClearGap = 2.2
	startLives    = 3
	maxStages     = 99
)

// playState is the per-round state machine.
type playState int

const (
	psReady playState = iota
	psPlaying
	psDying
	psStageClear
	psGameOver
)

// playScene is the active match state. The top-level scene flips
// between this and the title screen on quit / game-over.
type playScene struct {
	e   *engine.Engine
	maze *maze
	rng *rand.Rand

	player  car
	enemies []*enemy
	smoke   []smokePuff

	state  playState
	stateT float64

	score     int
	hiScore   int
	lives     int
	stage     int
	fuel      float64
	flagMult  int // 1 by default, 2 after collecting the special flag
	extraLifeAwarded bool

	wantQuit bool

	// challenging is true on the bonus rounds (stages 3, 6, 9, …)
	// where no enemies spawn and the player races the clock for a
	// completion bonus, just like the arcade's CHALLENGING STAGE.
	challenging bool

	// smokeQueueT counts down to the next smoke puff drop while the
	// player holds the smoke button.
	smokeQueueT float64

	// stageClearFlash drives the "you cleared it!" flicker.
	stageClearFlash float64

	// popups is the live list of floating score numbers. Identical in
	// purpose to the Pac-Man port's popup list.
	popups []scorePopup
}

// scorePopup is a short-lived label drawn above the world when the
// player scores something visible (flag pickup, enemy crash).
type scorePopup struct {
	x, y float64
	text string
	age  float64
	ttl  float64
	col  engine.Color
}

func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	p := &playScene{
		e:       e,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		hiScore: hiScore,
		lives:   startLives,
		stage:   1,
	}
	p.startStage()
	return p
}

// startStage rebuilds the maze, repositions everyone at their spawn
// points, refills the tank, and primes the READY pause. Stages that
// are multiples of 3 are CHALLENGING STAGES — no enemies appear, the
// player just rushes for flags and a clear bonus.
func (p *playScene) startStage() {
	p.maze = newMaze(p.stage)
	p.assignLuckyFlag()
	p.challenging = p.stage%3 == 0
	p.player = car{
		x:     p.maze.playerSpawn[0],
		y:     p.maze.playerSpawn[1],
		dir:   dirRight,
		desired: dirRight,
		speed: basePlayerSpeed,
		alive: true,
	}
	p.enemies = nil
	if !p.challenging {
		for i, sp := range p.maze.enemySpawns {
			en := newEnemy(sp[0], sp[1], int64(p.stage*131+i*7))
			en.speed = baseEnemySpeed + 0.18*float64(p.stage-1)
			p.enemies = append(p.enemies, en)
		}
	}
	p.smoke = nil
	p.fuel = fullTank
	p.flagMult = 1
	p.smokeQueueT = 0
	p.popups = nil
	p.state = psReady
	p.stateT = 0
}

// assignLuckyFlag picks a random remaining flag and upgrades it to
// the lucky flag for this round. In the arcade, every stage has one
// random flag marked "L" — when the player grabs it, they're paid
// out a big bonus.
func (p *playScene) assignLuckyFlag() {
	if p.maze.luckyFlag != nil {
		// An explicit lucky-flag marker in the map takes precedence.
		return
	}
	if len(p.maze.flags) == 0 {
		return
	}
	idx := p.rng.Intn(len(p.maze.flags))
	// Promote the chosen flag to lucky by storing a separate pickup
	// that aliases the same tile; the normal flag at that tile becomes
	// inert so it isn't double-counted.
	src := p.maze.flags[idx]
	p.maze.flags[idx].taken = true
	p.maze.luckyFlag = &pickup{
		col:  src.col,
		row:  src.row,
		kind: itemLuckyFlag,
	}
}

// --- Update ---------------------------------------------------------

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s

	switch p.state {
	case psReady:
		if p.stateT >= readyHold {
			p.state = psPlaying
			p.stateT = 0
		}
	case psPlaying:
		p.updatePlaying(s)
	case psDying:
		if p.stateT >= dyingHold {
			if p.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.respawnAfterDeath()
			}
		}
	case psStageClear:
		p.stageClearFlash += s
		if p.stateT >= stageClearGap {
			p.advanceStage()
		}
	case psGameOver:
		// Wait for Enter / Q in handleInput.
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	return nil
}

// handleInput drains discrete key events for turn buffering, smoke,
// and quit, then samples the held smoke key separately.
func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			break
		}
		switch k.Code {
		case engine.KeyUp:
			p.player.desired = dirUp
		case engine.KeyDown:
			p.player.desired = dirDown
		case engine.KeyLeft:
			p.player.desired = dirLeft
		case engine.KeyRight:
			p.player.desired = dirRight
		case engine.KeyEsc:
			p.wantQuit = true
		case engine.KeyEnter:
			if p.state == psGameOver {
				p.wantQuit = true
			}
		case engine.KeyChar:
			switch k.Rune {
			case 'w', 'W':
				p.player.desired = dirUp
			case 's', 'S':
				p.player.desired = dirDown
			case 'a', 'A':
				p.player.desired = dirLeft
			case 'd', 'D':
				p.player.desired = dirRight
			case 'q', 'Q':
				p.wantQuit = true
			case ' ':
				if p.state == psGameOver {
					p.wantQuit = true
				}
			}
		}
	}
}

// smokeHeld reports whether the smoke key is currently held. Sampled
// every frame inside updatePlaying so the player can tap-tap-tap or
// hold continuously.
func (p *playScene) smokeHeld() bool {
	return p.e.IsCharDown(' ') || p.e.IsCharDown('z') || p.e.IsCharDown('Z') ||
		p.e.IsKeyDown(engine.KeyEnter)
}

func (p *playScene) updatePlaying(s float64) {
	p.tickFuel(s)
	p.movePlayer(s)
	if p.state != psPlaying {
		return
	}
	p.moveEnemies(s)
	p.updateSmoke(s)
	p.handleSmokeCollisions()
	p.handlePlayerCollisions()
	p.updatePopups(s)

	if p.maze.remainingFlags() == 0 {
		if p.challenging {
			// Challenging-stage clear bonus scales with the fuel left
			// over: the arcade rewards perfect runs.
			bonus := 2000 + int(p.fuel*30)
			p.score += bonus
			p.spawnPopup(p.player.x, p.player.y,
				fmt.Sprintf("BONUS %d", bonus),
				engine.Color{R: 80, G: 220, B: 255, A: 255})
			p.checkExtraLife()
		}
		p.state = psStageClear
		p.stateT = 0
		p.stageClearFlash = 0
	}
}

func (p *playScene) tickFuel(s float64) {
	drain := 1.0
	if p.smokeHeld() && p.fuel > 0 {
		drain += smokeFuelDrain * 0.2 // smoke costs fuel even before a puff drops
	}
	p.fuel -= drain * s
	if p.fuel < 0 {
		p.fuel = 0
	}
}

func (p *playScene) movePlayer(s float64) {
	speed := basePlayerSpeed
	if p.fuel <= 0 {
		speed *= emptyTankSpeedMul
	}
	p.player.speed = speed

	prevTX, prevTY := p.player.tileX(), p.player.tileY()
	// canPass allows the player to drive into rock tiles — entering a
	// rock counts as a deliberate-or-careless crash, which is checked
	// just below. Walls remain off-limits.
	p.player.advance(s, func(col, row int) bool {
		return p.maze.drivable(col, row)
	})

	tx, ty := p.player.tileX(), p.player.tileY()
	if tx != prevTX || ty != prevTY {
		// Rock entry = wreck. The rock is consumed (matches the
		// arcade's "boulder absorbs a car" behaviour).
		if p.maze.at(tx, ty) == tileRock {
			p.maze.removeRock(tx, ty)
			p.spawnPopup(p.player.x, p.player.y, "CRASH!",
				engine.Color{R: 255, G: 120, B: 80, A: 255})
			p.startDying()
			return
		}
		if pk := p.maze.pickupAt(tx, ty); pk != nil {
			p.collectPickup(pk)
		}
	}

	// Maybe drop a smoke puff this frame.
	if p.smokeHeld() && p.fuel > 0 && p.player.dir != dirNone {
		p.smokeQueueT -= s
		if p.smokeQueueT <= 0 {
			p.smokeQueueT = smokeDropPeriod
			p.dropSmokePuff()
		}
	} else {
		p.smokeQueueT = 0
	}
}

func (p *playScene) dropSmokePuff() {
	// Puff appears one tile behind the player's current heading,
	// clamped to a road tile.
	bx := p.player.x - float64(p.player.dir.dx())*0.9
	by := p.player.y - float64(p.player.dir.dy())*0.9
	col := int(math.Floor(bx))
	row := int(math.Floor(by))
	if !p.maze.drivable(col, row) {
		bx, by = p.player.x, p.player.y
	}
	p.smoke = append(p.smoke, smokePuff{x: bx, y: by, ttl: smokePuffTTL})
	// Smoke costs an extra burst of fuel per puff so the player can
	// run out by spamming it.
	p.fuel -= 0.4
	if p.fuel < 0 {
		p.fuel = 0
	}
}

func (p *playScene) moveEnemies(s float64) {
	canPass := func(col, row int) bool {
		return p.maze.passable(col, row)
	}
	for _, en := range p.enemies {
		if !en.alive || en.crashed {
			continue
		}
		en.tickSmoke(s)
		// Speed creeps up as fewer flags remain — the chase tightens
		// when the player is one flag from clearing.
		remaining := p.maze.remainingFlags()
		urgency := 1.0
		if remaining <= 3 {
			urgency = 1.15
		}
		if remaining <= 1 {
			urgency = 1.3
		}
		en.speed = (baseEnemySpeed + 0.18*float64(p.stage-1)) * urgency
		en.drive(p.player.x, p.player.y, p.maze)
		en.advance(s, canPass)

		// Enemy crashes if it drives onto a rock tile.
		tx, ty := en.tileX(), en.tileY()
		if p.maze.at(tx, ty) == tileRock {
			p.crashEnemy(en, true)
		}
	}
}

func (p *playScene) updateSmoke(s float64) {
	if len(p.smoke) == 0 {
		return
	}
	kept := p.smoke[:0]
	for _, puff := range p.smoke {
		puff.age += s
		if puff.alive() {
			kept = append(kept, puff)
		}
	}
	p.smoke = kept
}

// handleSmokeCollisions stuns any enemy intersecting a live puff.
func (p *playScene) handleSmokeCollisions() {
	for _, puff := range p.smoke {
		if !puff.alive() {
			continue
		}
		for _, en := range p.enemies {
			if !en.alive || en.crashed {
				continue
			}
			dx := en.x - puff.x
			dy := en.y - puff.y
			if dx*dx+dy*dy <= smokeHitRadiusSq {
				en.stunBySmoke(smokeStunDuration)
			}
		}
	}
}

// handlePlayerCollisions tests the player against every living, non-
// stunned enemy. Stunned enemies are harmless — the player can
// actually ram them off the road for bonus points.
func (p *playScene) handlePlayerCollisions() {
	if !p.player.alive {
		return
	}
	for _, en := range p.enemies {
		if !en.alive || en.crashed {
			continue
		}
		const hit = 0.7
		if p.player.distSq(&en.car) > hit*hit {
			continue
		}
		if en.smokeT > 0 {
			// Stunned — ramming counts as a wreck.
			p.crashEnemy(en, false)
		} else {
			p.startDying()
			return
		}
	}
}

func (p *playScene) crashEnemy(en *enemy, fromRock bool) {
	en.crash()
	p.score += scoreCrashEnemy
	p.checkExtraLife()
	col := engine.Color{R: 255, G: 180, B: 90, A: 255}
	p.spawnPopup(en.x, en.y, fmt.Sprintf("%d", scoreCrashEnemy), col)
	if fromRock {
		// Rock is consumed in the crash, matching the arcade.
		p.maze.removeRock(en.tileX(), en.tileY())
	}
}

func (p *playScene) collectPickup(pk *pickup) {
	pk.taken = true
	switch pk.kind {
	case itemFlag:
		val := scoreFlag * p.flagMult
		p.score += val
		p.spawnPopup(float64(pk.col)+0.5, float64(pk.row)+0.5,
			fmt.Sprintf("%d", val),
			engine.Color{R: 255, G: 240, B: 0, A: 255})
	case itemSpecialFlag:
		p.flagMult = 2
		p.spawnPopup(float64(pk.col)+0.5, float64(pk.row)+0.5,
			"SPECIAL X2",
			engine.Color{R: 255, G: 180, B: 255, A: 255})
	case itemLuckyFlag:
		val := scoreLuckyFlag * p.flagMult
		p.score += val
		p.spawnPopup(float64(pk.col)+0.5, float64(pk.row)+0.5,
			fmt.Sprintf("%d LUCKY!", val),
			engine.Color{R: 80, G: 220, B: 255, A: 255})
	}
	p.checkExtraLife()
}

func (p *playScene) checkExtraLife() {
	if !p.extraLifeAwarded && p.score >= extraLifeScore {
		p.lives++
		p.extraLifeAwarded = true
	}
}

func (p *playScene) startDying() {
	p.lives--
	p.player.alive = false
	p.state = psDying
	p.stateT = 0
}

// respawnAfterDeath rebuilds the player and enemies at their spawn
// points without resetting collected flags. The maze (and any rocks
// already cleared) carries over.
func (p *playScene) respawnAfterDeath() {
	p.player = car{
		x:       p.maze.playerSpawn[0],
		y:       p.maze.playerSpawn[1],
		dir:     dirRight,
		desired: dirRight,
		speed:   basePlayerSpeed,
		alive:   true,
	}
	// Reset enemies to their spawn positions (alive and unstunned),
	// matching the arcade's behaviour where pursuers retreat after
	// taking a life so the player has a moment of breathing room.
	for i, sp := range p.maze.enemySpawns {
		if i >= len(p.enemies) {
			break
		}
		en := p.enemies[i]
		en.x = sp[0]
		en.y = sp[1]
		en.dir = dirLeft
		en.desired = dirLeft
		en.alive = true
		en.crashed = false
		en.smokeT = 0
		en.lastTile = [2]int{-1, -1}
	}
	p.fuel = fullTank
	p.smoke = nil
	p.state = psReady
	p.stateT = 0
}

func (p *playScene) advanceStage() {
	p.stage++
	if p.stage > maxStages {
		p.stage = maxStages
	}
	p.startStage()
}

func (p *playScene) updatePopups(s float64) {
	if len(p.popups) == 0 {
		return
	}
	kept := p.popups[:0]
	for _, pop := range p.popups {
		pop.age += s
		pop.y -= 1.2 * s
		if pop.age < pop.ttl {
			kept = append(kept, pop)
		}
	}
	p.popups = kept
}

func (p *playScene) spawnPopup(x, y float64, text string, col engine.Color) {
	p.popups = append(p.popups, scorePopup{
		x: x, y: y, text: text, ttl: 1.1, col: col,
	})
}
