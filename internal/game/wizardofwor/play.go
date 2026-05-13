package wizardofwor

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// -- Tuning constants -------------------------------------------------

const (
	// Speeds — tile units per second. Player speed is fixed; monster
	// speeds are tuned per-kind in monster.go and scaled per-dungeon.
	playerSpeed = 2.6

	// Per-state hold timers (seconds).
	readyHold           = 2.2
	dyingHold           = 1.4
	betweenDungeonsHold = 1.8
	worlukHold          = 0.9
	worlukActive        = 6.0
	wizardHold          = 0.9
	wizardActive        = 9.0

	// Player respawn invulnerability — gives the player a moment to
	// step out of the cage before being legitimately killable.
	respawnInvuln = 1.5

	// Bookkeeping.
	playerStartLives = 3
	extraLifeScore   = 20000

	// Cage emerge cadence.
	cageFirstEmerge  = 0.9
	cageEmergeRate   = 1.7
	cageLastBoost    = 1.3 // last few monsters emerge faster

	// Bullet caps.
	maxPlayerBullets   = 1
	maxMonsterBullets  = 3
	maxWizardBullets   = 2

	// Hit radii (in tile units) — sloppy enough to feel fair, tight
	// enough that bullets don't seem to phase through. 0.45 is just
	// shy of "anywhere in the same tile".
	bulletHitRadius = 0.45
	touchHitRadius  = 0.55
)

// playState is the gameplay sub-state machine.
type playState int

const (
	psReady           playState = iota // "READY!" pre-round pause
	psPlaying                          // regular monster hunt
	psDying                            // player explosion before respawn / game over
	psDungeonClear                     // post-dungeon pause, score totalled
	psWorlukReady                      // brief pause before Worluk drops
	psWorluk                           // Worluk escape sequence
	psWizardReady                      // brief pause before Wizard arrives
	psWizard                           // Wizard battle
	psGameOver                         // end of run; wait for input
)

// playScene contains the full match state.
type playScene struct {
	e    *engine.Engine
	rng  *rand.Rand
	maze *maze

	state  playState
	stateT float64

	player           entity
	playerAlive      bool
	playerInvulnT    float64
	playerStep       int  // animation step counter (used for sprite walk cycle)
	playerWalkPhase  float64
	lastPlayerDir    direction

	monsters    []*monster
	spawnQueue  []monsterKind // monsters still waiting to emerge from cage
	spawnTimer  float64
	transformT  float64 // counts down to next burwor → garwor / garwor → thorwor

	worluk *monster
	wizard *monster
	wizardTeleportT float64

	bullets    []bullet
	explosions []explosion

	score            int
	hiScore          int
	lives            int
	dungeon          int
	doubleScore      bool // active when the previous Worluk was killed
	extraLifeAwarded bool

	wantQuit bool

	currentWave wave
}

// explosion is a short-lived visual at a tile-space position.
type explosion struct {
	x, y float64
	age  float64
	ttl  float64
}

// newPlayScene constructs a play scene ready to begin dungeon 1.
func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	p := &playScene{
		e:       e,
		rng:     rng,
		hiScore: hiScore,
		lives:   playerStartLives,
	}
	p.beginDungeon(1)
	return p
}

// beginDungeon resets per-dungeon state and queues up the wave.
func (p *playScene) beginDungeon(num int) {
	p.dungeon = num
	p.maze = newMaze(num)
	p.currentWave = waveFor(num)
	p.monsters = nil
	p.worluk = nil
	p.wizard = nil
	p.bullets = nil
	p.explosions = nil

	// Build the cage spawn queue: Burwors first, then Garwors, then
	// Thorwors — matching the original's spawn order roughly.
	p.spawnQueue = nil
	for i := 0; i < p.currentWave.burwors; i++ {
		p.spawnQueue = append(p.spawnQueue, mkBurwor)
	}
	for i := 0; i < p.currentWave.garwors; i++ {
		p.spawnQueue = append(p.spawnQueue, mkGarwor)
	}
	for i := 0; i < p.currentWave.thorwors; i++ {
		p.spawnQueue = append(p.spawnQueue, mkThorwor)
	}
	p.spawnTimer = cageFirstEmerge
	p.transformT = p.currentWave.transformAfter

	p.placePlayerAtSpawn()
	p.state = psReady
	p.stateT = 0
}

// placePlayerAtSpawn drops the Worrior at the bottom-left spawn cell
// with brief invuln. The cage at the centre is monster territory —
// players never spawn there in the arcade. Used on first dungeon
// start, between dungeons, and after death.
func (p *playScene) placePlayerAtSpawn() {
	p.player = entity{
		x:       float64(playerSpawnCol) + 0.5,
		y:       float64(playerSpawnRow) + 0.5,
		dir:     dirRight,
		desired: dirRight,
		speed:   playerSpeed,
	}
	p.playerAlive = true
	p.playerInvulnT = respawnInvuln
	p.lastPlayerDir = dirRight
}

// -- Update -----------------------------------------------------------

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s

	switch p.state {
	case psReady:
		// Drain input but don't advance simulation.
		if p.stateT >= readyHold {
			p.state = psPlaying
			p.stateT = 0
		}
	case psPlaying:
		p.updatePlaying(s)
	case psDying:
		p.advanceExplosions(s)
		p.advanceBullets(s) // bullets still travel during the pause
		if p.stateT >= dyingHold {
			if p.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.placePlayerAtSpawn()
				p.state = psReady
				p.stateT = 0
			}
		}
	case psDungeonClear:
		p.advanceExplosions(s)
		if p.stateT >= betweenDungeonsHold {
			p.beginDungeon(p.dungeon + 1)
		}
	case psWorlukReady:
		p.advanceExplosions(s)
		if p.stateT >= worlukHold {
			p.spawnWorluk()
			p.state = psWorluk
			p.stateT = 0
		}
	case psWorluk:
		p.updateWorluk(s)
	case psWizardReady:
		p.advanceExplosions(s)
		if p.stateT >= wizardHold {
			p.spawnWizard()
			p.state = psWizard
			p.stateT = 0
		}
	case psWizard:
		p.updateWizard(s)
	case psGameOver:
		// Wait for input.
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
				} else {
					p.firePlayerBullet()
				}
			}
		}
	}
}

// updatePlaying drives the main gameplay frame.
func (p *playScene) updatePlaying(s float64) {
	p.advancePlayer(s)
	p.advanceSpawns(s)
	p.advanceTransforms(s)
	p.advanceMonsters(s)
	p.advanceBullets(s)
	p.advanceExplosions(s)
	p.resolveCollisions()
	if p.state != psPlaying {
		return
	}

	// All monsters cleared? Trigger Worluk phase.
	huntCount := 0
	for _, m := range p.monsters {
		if m.state == msHunting || m.state == msInCage || m.state == msEmerging {
			huntCount++
		}
	}
	if huntCount == 0 && len(p.spawnQueue) == 0 {
		p.state = psWorlukReady
		p.stateT = 0
	}
}

func (p *playScene) advancePlayer(s float64) {
	if !p.playerAlive {
		return
	}
	if p.playerInvulnT > 0 {
		p.playerInvulnT -= s
	}

	canPass := func(c, r int, d direction) bool {
		return p.maze.canMove(c, r, d)
	}

	prevDir := p.player.dir
	p.player.advance(s, canPass)

	// Update animation step on tile transitions or steady walking.
	if p.player.dir != dirNone {
		p.playerWalkPhase += s
		if p.playerWalkPhase >= 0.18 {
			p.playerWalkPhase = 0
			p.playerStep++
		}
	}

	// Remember the last non-none direction so the sprite keeps its
	// facing when the Worrior is stopped against a wall.
	if p.player.dir != dirNone {
		p.lastPlayerDir = p.player.dir
	} else if prevDir != dirNone {
		p.lastPlayerDir = prevDir
	}
}

// advanceSpawns ticks the cage emerge timer and pushes a monster onto
// the playfield when ready.
func (p *playScene) advanceSpawns(s float64) {
	if len(p.spawnQueue) == 0 {
		return
	}
	p.spawnTimer -= s
	if p.spawnTimer > 0 {
		return
	}
	kind := p.spawnQueue[0]
	p.spawnQueue = p.spawnQueue[1:]
	m := newMonster(kind, p.currentWave.speedMul, p.rng)
	m.state = msHunting
	p.monsters = append(p.monsters, m)

	// Subsequent monsters emerge a bit faster as the cage empties.
	rate := cageEmergeRate
	if len(p.spawnQueue) <= 2 {
		rate = cageLastBoost
	}
	p.spawnTimer = rate
}

// advanceTransforms upgrades a surviving Burwor (or Garwor) to the
// next tier on a per-dungeon timer. This is the original arcade's
// answer to a stalling player.
func (p *playScene) advanceTransforms(s float64) {
	if p.currentWave.transformAfter <= 0 {
		return
	}
	p.transformT -= s
	if p.transformT > 0 {
		return
	}
	// Pick a candidate: prefer Burwor over Garwor (climb the tier ladder).
	var pick *monster
	for _, m := range p.monsters {
		if m.state != msHunting {
			continue
		}
		if m.kind == mkBurwor {
			pick = m
			break
		}
	}
	if pick == nil {
		for _, m := range p.monsters {
			if m.state == msHunting && m.kind == mkGarwor {
				pick = m
				break
			}
		}
	}
	if pick != nil {
		var to monsterKind
		if pick.kind == mkBurwor {
			to = mkGarwor
		} else {
			to = mkThorwor
		}
		pick.transform(to, p.currentWave.speedMul, p.rng)
	}
	p.transformT = p.currentWave.transformAfter
}

// advanceMonsters runs each monster's AI + movement + visibility +
// shooting for the frame.
func (p *playScene) advanceMonsters(s float64) {
	pc, pr := p.player.tileX(), p.player.tileY()
	canPass := func(c, r int, d direction) bool {
		return p.maze.canMove(c, r, d)
	}

	monsterBulletCount := 0
	for _, b := range p.bullets {
		if b.alive && b.shooter == shooterMonster {
			monsterBulletCount++
		}
	}

	for i, m := range p.monsters {
		if m.state == msDying {
			m.dieT += s
			continue
		}

		// Tile-centre arrival decision.
		if m.atCentre() {
			randomness := 0.25
			switch m.kind {
			case mkBurwor:
				randomness = 0.45
			case mkGarwor:
				randomness = 0.25
			case mkThorwor:
				randomness = 0.10
			}
			next := m.pickAtJunction(p.maze, pc, pr, p.rng, randomness)
			if next != dirNone {
				m.desired = next
			}
		}
		m.advance(s, canPass)

		// Sprite leg animation.
		m.walkPhase += s
		if m.walkPhase >= 0.2 {
			m.walkPhase = 0
		}

		// Visibility cycle.
		m.updateVisibility(s, p.rng)

		// Shooting.
		if monsterBulletCount < maxMonsterBullets {
			if dir, ok := m.shouldShoot(p.maze, pc, pr, s, p.rng); ok {
				p.bullets = append(p.bullets,
					newMonsterBullet(m.x, m.y, dir, i))
				monsterBulletCount++
			}
		}
	}
}

func (p *playScene) advanceBullets(s float64) {
	for i := range p.bullets {
		if p.bullets[i].alive {
			p.bullets[i].advance(s, p.maze)
		}
	}
	// Sweep dead bullets — but keep them for one extra frame so the
	// wall-hit position is visible at least briefly.
	kept := p.bullets[:0]
	for _, b := range p.bullets {
		if b.alive {
			kept = append(kept, b)
		}
	}
	p.bullets = kept
}

func (p *playScene) advanceExplosions(s float64) {
	kept := p.explosions[:0]
	for _, ex := range p.explosions {
		ex.age += s
		if ex.age < ex.ttl {
			kept = append(kept, ex)
		}
	}
	p.explosions = kept
}

// resolveCollisions handles all entity-vs-bullet and entity-vs-entity
// interactions for the frame.
func (p *playScene) resolveCollisions() {
	// Bullet vs bullet — opposite-shooter pairs annihilate.
	for i := range p.bullets {
		if !p.bullets[i].alive {
			continue
		}
		for j := i + 1; j < len(p.bullets); j++ {
			if !p.bullets[j].alive {
				continue
			}
			bi := p.bullets[i]
			bj := p.bullets[j]
			if bi.shooter == bj.shooter {
				continue
			}
			dx := bi.x - bj.x
			dy := bi.y - bj.y
			if dx*dx+dy*dy <= bulletHitRadius*bulletHitRadius {
				p.bullets[i].alive = false
				p.bullets[j].alive = false
				p.spawnExplosion(bi.x, bi.y, 0.25)
				break
			}
		}
	}

	// Bullet vs monsters.
	for bi := range p.bullets {
		b := &p.bullets[bi]
		if !b.alive {
			continue
		}
		if b.shooter != shooterPlayer {
			continue
		}
		for _, m := range p.monsters {
			if m.state == msDying {
				continue
			}
			if b.hitsEntity(m.x, m.y, bulletHitRadius+0.05) {
				b.alive = false
				p.killMonster(m)
				break
			}
		}
		// Hits worluk / wizard separately.
		if b.alive && p.worluk != nil && p.worluk.state != msDying {
			if b.hitsEntity(p.worluk.x, p.worluk.y, bulletHitRadius+0.1) {
				b.alive = false
				p.killWorluk()
			}
		}
		if b.alive && p.wizard != nil && p.wizard.state != msDying {
			if b.hitsEntity(p.wizard.x, p.wizard.y, bulletHitRadius+0.1) {
				b.alive = false
				p.killWizard()
			}
		}
	}

	// Bullet vs player.
	if p.playerAlive && p.playerInvulnT <= 0 {
		for bi := range p.bullets {
			b := &p.bullets[bi]
			if !b.alive {
				continue
			}
			if b.shooter == shooterPlayer {
				continue
			}
			if b.hitsEntity(p.player.x, p.player.y, bulletHitRadius+0.05) {
				b.alive = false
				p.startDying()
				return
			}
		}
	}

	// Touch: monster vs player.
	if p.playerAlive && p.playerInvulnT <= 0 {
		for _, m := range p.monsters {
			if m.state != msHunting {
				continue
			}
			dx := m.x - p.player.x
			dy := m.y - p.player.y
			if dx*dx+dy*dy <= touchHitRadius*touchHitRadius {
				p.killMonster(m)
				p.startDying()
				return
			}
		}
		if p.wizard != nil && p.wizard.state == msHunting {
			dx := p.wizard.x - p.player.x
			dy := p.wizard.y - p.player.y
			if dx*dx+dy*dy <= touchHitRadius*touchHitRadius {
				p.startDying()
				return
			}
		}
	}
}

// firePlayerBullet attempts to spawn a new player bullet. No-op if one
// is already in flight or the player isn't in a state where they can
// shoot (dying, ready, game over).
func (p *playScene) firePlayerBullet() {
	if p.state != psPlaying && p.state != psWorluk && p.state != psWizard {
		return
	}
	if !p.playerAlive {
		return
	}
	live := 0
	for _, b := range p.bullets {
		if b.alive && b.shooter == shooterPlayer {
			live++
		}
	}
	if live >= maxPlayerBullets {
		return
	}
	dir := p.lastPlayerDir
	if dir == dirNone {
		dir = dirUp
	}
	// Offset bullet origin slightly in the firing direction so it
	// doesn't immediately register a self-hit (and looks like it
	// emerges from the gun barrel, not the player's centre).
	const muzzle = 0.35
	x := p.player.x + float64(dir.dx())*muzzle
	y := p.player.y + float64(dir.dy())*muzzle
	p.bullets = append(p.bullets, newPlayerBullet(x, y, dir))
}

// killMonster transitions a monster into dying and awards score.
func (p *playScene) killMonster(m *monster) {
	if m.state == msDying {
		return
	}
	m.state = msDying
	m.dieT = 0
	mult := 1
	if p.doubleScore {
		mult = 2
	}
	p.score += monsterScore[m.kind] * mult
	p.spawnExplosion(m.x, m.y, dyingHold)
	p.checkExtraLife()
}

func (p *playScene) startDying() {
	p.playerAlive = false
	p.lives--
	p.spawnExplosion(p.player.x, p.player.y, dyingHold)
	// Clear all bullets so they don't keep hitting things mid-pause.
	for i := range p.bullets {
		p.bullets[i].alive = false
	}
	p.state = psDying
	p.stateT = 0
}

func (p *playScene) checkExtraLife() {
	if !p.extraLifeAwarded && p.score >= extraLifeScore {
		p.lives++
		p.extraLifeAwarded = true
	}
}

func (p *playScene) spawnExplosion(x, y, ttl float64) {
	p.explosions = append(p.explosions, explosion{
		x: x, y: y, ttl: ttl,
	})
}

// -- Worluk -----------------------------------------------------------

// spawnWorluk drops the Worluk into the maze. The Worluk's job is to
// reach a side warp; if it does, the dungeon ends without the bonus.
// Killing it before it escapes awards 1000 points AND the doubled
// score modifier for the next dungeon.
func (p *playScene) spawnWorluk() {
	w := newMonster(mkWorluk, p.currentWave.speedMul, p.rng)
	w.state = msHunting
	// Drop into the tunnel row near one of the side mouths so it has
	// somewhere to head. Pick the side opposite the player to give
	// the player a fighting chance.
	if p.player.tileX() > mazeCols/2 {
		w.x = 0.5
	} else {
		w.x = float64(mazeCols) - 0.5
	}
	w.y = float64(tunnelRow) + 0.5
	// Aim toward the opposite tunnel.
	if w.x < float64(mazeCols)/2 {
		w.dir = dirRight
	} else {
		w.dir = dirLeft
	}
	w.desired = w.dir
	p.worluk = w
}

// updateWorluk advances the Worluk and checks whether it escaped via
// the side tunnels or the time limit ran out.
func (p *playScene) updateWorluk(s float64) {
	p.advancePlayer(s)
	canPass := func(c, r int, d direction) bool {
		// Worluk strongly prefers staying on the tunnel row, so we
		// only allow vertical moves into the tunnel row.
		if d == dirUp || d == dirDown {
			ny := c
			_ = ny
		}
		return p.maze.canMove(c, r, d)
	}
	if p.worluk != nil && p.worluk.state == msHunting {
		// Simple AI: head straight along the tunnel row toward the
		// nearest exit. If blocked (a wall in the way), turn around.
		w := p.worluk
		if w.atCentre() {
			// If we somehow drifted off the tunnel row, pick a vertical
			// direction toward it.
			if w.tileY() != tunnelRow {
				if w.tileY() < tunnelRow && p.maze.canMove(w.tileX(), w.tileY(), dirDown) {
					w.desired = dirDown
				} else if w.tileY() > tunnelRow && p.maze.canMove(w.tileX(), w.tileY(), dirUp) {
					w.desired = dirUp
				}
			} else {
				// On tunnel row — keep going.
				if !p.maze.canMove(w.tileX(), tunnelRow, w.dir) {
					w.dir = w.dir.opposite()
					w.desired = w.dir
				}
			}
		}
		w.advance(s, canPass)
	}

	p.advanceBullets(s)
	p.advanceExplosions(s)
	p.resolveCollisions()

	// Escape check: if the Worluk has wrapped through a tunnel mouth,
	// the bonus round ends without reward.
	if p.worluk != nil && p.worluk.state == msHunting {
		if p.worluk.x < -0.5 || p.worluk.x > float64(mazeCols)+0.5 {
			p.endWorluk(false)
			return
		}
	}
	// Time expires.
	if p.stateT >= worlukActive {
		p.endWorluk(false)
		return
	}
	// Killed?
	if p.worluk == nil {
		p.endWorluk(true)
		return
	}
}

func (p *playScene) killWorluk() {
	if p.worluk == nil {
		return
	}
	p.score += monsterScore[mkWorluk]
	p.spawnExplosion(p.worluk.x, p.worluk.y, 0.6)
	p.worluk = nil
	p.checkExtraLife()
}

func (p *playScene) endWorluk(killed bool) {
	// Bonus persists into the NEXT dungeon if the Worluk was killed.
	p.doubleScore = killed
	p.worluk = nil
	// Roll for Wizard appearance — ~30% chance.
	if p.rng.Float64() < 0.3 {
		p.state = psWizardReady
		p.stateT = 0
		return
	}
	p.state = psDungeonClear
	p.stateT = 0
}

// -- Wizard -----------------------------------------------------------

func (p *playScene) spawnWizard() {
	w := newMonster(mkWizard, p.currentWave.speedMul, p.rng)
	w.state = msHunting
	// Teleport in at a random walkable cell that isn't where the
	// player is standing.
	p.placeAtRandomCell(&w.entity)
	p.wizard = w
	p.wizardTeleportT = 1.5 + p.rng.Float64()*1.5
}

func (p *playScene) updateWizard(s float64) {
	p.advancePlayer(s)
	if p.wizard != nil && p.wizard.state == msHunting {
		p.wizardTeleportT -= s
		if p.wizardTeleportT <= 0 {
			p.placeAtRandomCell(&p.wizard.entity)
			p.wizardTeleportT = 1.5 + p.rng.Float64()*2.0
		}
		// Wizard fires when in line.
		if p.bulletCount(shooterWizard) < maxWizardBullets {
			if dir, ok := hasLineOfFire(p.maze, p.wizard.tileX(), p.wizard.tileY(),
				p.player.tileX(), p.player.tileY()); ok {
				// Throttle wizard fire with shootT.
				p.wizard.shootT -= s
				if p.wizard.shootT <= 0 {
					p.bullets = append(p.bullets,
						newWizardBullet(p.wizard.x, p.wizard.y, dir))
					p.wizard.shootT = 0.9
				}
			}
		}
	}

	p.advanceBullets(s)
	p.advanceExplosions(s)
	p.resolveCollisions()

	if p.wizard == nil {
		p.state = psDungeonClear
		p.stateT = 0
		return
	}
	if p.stateT >= wizardActive {
		// Wizard escapes — no penalty.
		p.wizard = nil
		p.state = psDungeonClear
		p.stateT = 0
	}
}

func (p *playScene) killWizard() {
	if p.wizard == nil {
		return
	}
	p.score += monsterScore[mkWizard]
	p.spawnExplosion(p.wizard.x, p.wizard.y, 0.7)
	p.wizard = nil
	p.checkExtraLife()
}

// placeAtRandomCell drops the entity onto a random cell that isn't
// the player's current cell. Used for Wizard teleports.
func (p *playScene) placeAtRandomCell(e *entity) {
	for tries := 0; tries < 32; tries++ {
		c := p.rng.Intn(mazeCols)
		r := p.rng.Intn(mazeRows)
		if c == p.player.tileX() && r == p.player.tileY() {
			continue
		}
		if c == cageCol && r == cageRow {
			continue
		}
		// Avoid landing on a cell adjacent to the player (too cheap).
		dx := c - p.player.tileX()
		dy := r - p.player.tileY()
		if dx*dx+dy*dy < 4 {
			continue
		}
		e.x = float64(c) + 0.5
		e.y = float64(r) + 0.5
		e.dir = dirNone
		e.desired = dirNone
		return
	}
	// Worst case: dump into the corner.
	e.x = 0.5
	e.y = 0.5
}

func (p *playScene) bulletCount(s shooterID) int {
	n := 0
	for _, b := range p.bullets {
		if b.alive && b.shooter == s {
			n++
		}
	}
	return n
}

// -- Drawing ----------------------------------------------------------

// geometry holds the on-screen layout: cell size in pixels, the
// pixel origin of (col 0, row 0), and the radar's mini-map placement.
type geometry struct {
	cell     int
	scale    int
	originX  int
	originY  int
	hudTopH  int
	hudBotH  int
	radarX   int
	radarY   int
	radarCell int
}

// computeGeometry sizes the maze to the available canvas. The HUD
// takes a couple of rows at top and bottom; the rest splits between
// width and height to find the largest integer cell size that fits.
func computeGeometry(c *engine.Canvas) geometry {
	w := c.Width()
	h := c.Height()
	hudTop := 4   // pixels above maze (~2 cell rows)
	hudBot := 14  // pixels below maze for radar + status

	// Cell needs to be large enough for the 5×5 sprite plus 1 px wall
	// margin. Find the largest integer cell that fits in both axes.
	cell := 0
	for tryCell := 12; tryCell >= 5; tryCell-- {
		mw := tryCell*mazeCols + 1
		mh := tryCell*mazeRows + 1
		if mw <= w && mh+hudTop+hudBot <= h {
			cell = tryCell
			break
		}
	}
	if cell == 0 {
		// Fall back to whatever fits, possibly clipping the HUD.
		cell = 5
		hudTop = 2
		hudBot = 6
	}

	// Scale factor for sprites: integer divisor of (cell - 1) / 5.
	scale := (cell - 1) / 5
	if scale < 1 {
		scale = 1
	}

	mazeW := cell*mazeCols + 1
	mazeH := cell*mazeRows + 1
	originX := (w - mazeW) / 2
	originY := hudTop + (h-hudTop-hudBot-mazeH)/2
	if originX < 0 {
		originX = 0
	}
	if originY < hudTop {
		originY = hudTop
	}

	// Radar: ~2px per cell, centred below the maze. Falls back to 1
	// px / cell on tight canvases; if even that doesn't fit beneath
	// the maze, slide the radar up to overlap the bottom HUD margin
	// rather than clip off the screen.
	rcell := 2
	rw := rcell * mazeCols
	rh := rcell * mazeRows
	radarX := (w - rw) / 2
	radarY := originY + mazeH + 2
	for radarY+rh+1 > h && rcell > 1 {
		rcell--
		rw = rcell * mazeCols
		rh = rcell * mazeRows
		radarX = (w - rw) / 2
	}
	if radarY+rh+1 > h {
		radarY = h - rh - 1
	}

	return geometry{
		cell:      cell,
		scale:     scale,
		originX:   originX,
		originY:   originY,
		hudTopH:   hudTop,
		hudBotH:   hudBot,
		radarX:    radarX,
		radarY:    radarY,
		radarCell: rcell,
	}
}

// tileToPixel converts a tile-space float position to the pixel of the
// cell-centre, given the geometry.
func (g geometry) tileToPixel(tx, ty float64) (int, int) {
	x := g.originX + int(math.Round(tx*float64(g.cell)))
	y := g.originY + int(math.Round(ty*float64(g.cell)))
	return x, y
}

// spriteOrigin returns the top-left pixel for a sprite whose centre
// should be at the given tile-space position.
func (g geometry) spriteOrigin(tx, ty float64, spriteW, spriteH int) (int, int) {
	cx, cy := g.tileToPixel(tx, ty)
	return cx - (spriteW*g.scale)/2, cy - (spriteH*g.scale)/2
}

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 18, A: 255})
	geo := computeGeometry(c)
	p.drawMaze(c, geo)
	p.drawCage(c, geo)
	p.drawMonsters(c, geo)
	p.drawWorluk(c, geo)
	p.drawWizard(c, geo)
	p.drawPlayer(c, geo)
	p.drawBullets(c, geo)
	p.drawExplosions(c, geo)
	p.drawRadar(c, geo)
	p.drawHUD(c, geo)
	p.drawOverlay(c, geo)
}

// drawMaze paints the wall lattice. Walls are 1-pixel lines on the
// edges between cells; corners are drawn naturally because the
// vertical and horizontal segments include their shared endpoints.
func (p *playScene) drawMaze(c *engine.Canvas, geo geometry) {
	cell := geo.cell
	col := wallColor

	// Vertical walls. r ranges over cells; col ranges 0..mazeCols
	// inclusive for outer + inner walls.
	for r := 0; r < mazeRows; r++ {
		for cc := 0; cc <= mazeCols; cc++ {
			if !p.maze.vwalls[r][cc] {
				continue
			}
			x := geo.originX + cc*cell
			y := geo.originY + r*cell
			c.FillRect(x, y, 1, cell+1, col)
		}
	}
	// Horizontal walls.
	for r := 0; r <= mazeRows; r++ {
		for cc := 0; cc < mazeCols; cc++ {
			if !p.maze.hwalls[r][cc] {
				continue
			}
			x := geo.originX + cc*cell
			y := geo.originY + r*cell
			c.FillRect(x, y, cell+1, 1, col)
		}
	}

	// Highlight the tunnel mouths so the player can see the warp.
	tunY := geo.originY + tunnelRow*cell
	c.FillRect(geo.originX-2, tunY+cell/3, 2, cell/3, wallHighlight)
	c.FillRect(geo.originX+mazeCols*cell+1, tunY+cell/3, 2, cell/3, wallHighlight)
}

// drawCage repaints the cage walls in the cage-bar colour so the
// central cell reads as the WORRIOR CAGE. The door (top) is left
// unpainted so the gap is visible.
func (p *playScene) drawCage(c *engine.Canvas, geo geometry) {
	cell := geo.cell
	cx := geo.originX + cageCol*cell
	cy := geo.originY + cageRow*cell

	// Side walls, floor — overpaint the previous wall blue.
	c.FillRect(cx, cy, 1, cell+1, cageBarColor)
	c.FillRect(cx+cell, cy, 1, cell+1, cageBarColor)
	c.FillRect(cx, cy+cell, cell+1, 1, cageBarColor)

	// Decorative vertical bars inside the cage at finer spacings (only
	// at cell >= 7, otherwise they crowd the cage interior).
	if cell >= 7 {
		bar := engine.Color{
			R: cageBarColor.R, G: cageBarColor.G, B: cageBarColor.B / 2, A: 255,
		}
		for i := 1; i < cell; i += 2 {
			c.Set(cx+i, cy+1, bar)
			c.Set(cx+i, cy+cell-1, bar)
		}
	}
}

func (p *playScene) drawPlayer(c *engine.Canvas, geo geometry) {
	if !p.playerAlive {
		return
	}
	// Flicker during invulnerability so the state reads at a glance.
	if p.playerInvulnT > 0 && int(p.playerInvulnT*10)%2 == 0 {
		return
	}
	dir := p.lastPlayerDir
	spr := worriorSprite(dir)
	w, h := spr.width(), spr.height()
	x, y := geo.spriteOrigin(p.player.x, p.player.y, w, h)
	drawSpriteScaled(c, x, y, spr, geo.scale)
}

func (p *playScene) drawMonsters(c *engine.Canvas, geo geometry) {
	for _, m := range p.monsters {
		if m.state == msDying {
			continue
		}
		col := monsterColor(m.kind)
		w, h := monsterA.width(), monsterA.height()
		x, y := geo.spriteOrigin(m.x, m.y, w, h)
		step := int(m.walkPhase*10) + m.tileX() + m.tileY()
		if !m.visible {
			drawMonsterGhost(c, x, y, col, geo.scale)
			continue
		}
		if m.isAiming() {
			p.drawMonsterAiming(c, geo, m, x, y, col, step)
			continue
		}
		drawMonster(c, x, y, col, step, geo.scale)
	}
}

// drawMonsterAiming paints a monster in its "windup" pose: body
// brightened, eyes flashed to red so the player can see the shot
// coming. A tiny muzzle dot in the aim direction makes the trajectory
// unambiguous at a glance.
func (p *playScene) drawMonsterAiming(c *engine.Canvas, geo geometry, m *monster, x, y int, body engine.Color, step int) {
	src := monsterFrame(step)
	bright := brightenColor(body, 0.35)
	red := engine.Color{R: 255, G: 70, B: 70, A: 255}
	hot := engine.Color{R: 255, G: 200, B: 90, A: 255}
	pal := map[byte]engine.Color{
		'B': bright, 'W': red, 'P': hot,
	}
	drawSpriteScaled(c, x, y, sprite{rows: src.rows, palette: pal}, geo.scale)

	// Muzzle telegraph dot just outside the body in the aim direction.
	cx, cy := geo.tileToPixel(m.x, m.y)
	off := geo.cell/2 + 1
	mx := cx + m.aimDir.dx()*off
	my := cy + m.aimDir.dy()*off
	if geo.scale >= 2 {
		c.FillRect(mx-1, my-1, 3, 3, hot)
	} else {
		c.Set(mx, my, hot)
	}
}

// brightenColor lifts c by amt of the way toward white (amt in 0..1).
func brightenColor(c engine.Color, amt float64) engine.Color {
	if amt <= 0 {
		return c
	}
	if amt >= 1 {
		return engine.White
	}
	lift := func(v uint8) uint8 {
		nv := float64(v) + (255-float64(v))*amt
		if nv > 255 {
			nv = 255
		}
		return uint8(nv)
	}
	return engine.Color{R: lift(c.R), G: lift(c.G), B: lift(c.B), A: 255}
}

func (p *playScene) drawWorluk(c *engine.Canvas, geo geometry) {
	if p.worluk == nil {
		return
	}
	w, h := worlukSprite.width(), worlukSprite.height()
	x, y := geo.spriteOrigin(p.worluk.x, p.worluk.y, w, h)
	// Worluk flashes between two tones to advertise its bonus value.
	pal := worlukSprite.palette
	if int(p.stateT*8)%2 == 0 {
		pal = map[byte]engine.Color{
			'W': engine.Color{R: 255, G: 220, B: 90, A: 255},
			'B': worlukWing,
		}
	}
	s := sprite{rows: worlukSprite.rows, palette: pal}
	drawSpriteScaled(c, x, y, s, geo.scale)
}

func (p *playScene) drawWizard(c *engine.Canvas, geo geometry) {
	if p.wizard == nil {
		return
	}
	w, h := wizardSprite.width(), wizardSprite.height()
	x, y := geo.spriteOrigin(p.wizard.x, p.wizard.y, w, h)
	drawSpriteScaled(c, x, y, wizardSprite, geo.scale)
	// Aura — a quick spark ring around the wizard so they read as
	// magical and dangerous.
	if int(p.stateT*10)%2 == 0 {
		cx, cy := geo.tileToPixel(p.wizard.x, p.wizard.y)
		c.DrawCircle(cx, cy, geo.cell/2+1,
			engine.Color{R: 200, G: 120, B: 240, A: 255})
	}
}

func (p *playScene) drawBullets(c *engine.Canvas, geo geometry) {
	for _, b := range p.bullets {
		if !b.alive {
			continue
		}
		col := bulletColor
		if b.shooter == shooterMonster {
			col = engine.Color{R: 255, G: 120, B: 120, A: 255}
		} else if b.shooter == shooterWizard {
			col = fireballHot
		}
		cx, cy := geo.tileToPixel(b.x, b.y)
		if geo.scale >= 2 {
			c.FillRect(cx-1, cy-1, 3, 3, col)
		} else {
			c.Set(cx, cy, col)
			c.Set(cx-1, cy, col)
			c.Set(cx+1, cy, col)
			c.Set(cx, cy-1, col)
			c.Set(cx, cy+1, col)
		}

		// Smear: draw the dimmed trail.
		dim := engine.Color{R: col.R / 2, G: col.G / 2, B: col.B / 2, A: 255}
		for _, tp := range b.trail {
			tx, ty := geo.tileToPixel(tp.x, tp.y)
			c.Set(tx, ty, dim)
		}
	}
}

func (p *playScene) drawExplosions(c *engine.Canvas, geo geometry) {
	for _, ex := range p.explosions {
		frac := ex.age / ex.ttl
		spr := explosionSprite(frac)
		if spr == nil {
			continue
		}
		w, h := spr.width(), spr.height()
		x, y := geo.spriteOrigin(ex.x, ex.y, w, h)
		drawSpriteScaled(c, x, y, *spr, geo.scale)
	}
}

// drawRadar paints the bottom-of-screen mini-map showing every
// entity's tile position, including invisible Garwors and Thorwors.
// This is the player's only way to track invisible enemies — exactly
// as it works in the arcade.
func (p *playScene) drawRadar(c *engine.Canvas, geo geometry) {
	rcell := geo.radarCell
	if rcell < 1 {
		return
	}
	rx, ry := geo.radarX, geo.radarY

	rw := mazeCols * rcell
	rh := mazeRows * rcell

	// Frame.
	c.DrawRect(rx-1, ry-1, rw+2, rh+2, radarFrame)
	c.FillRect(rx, ry, rw, rh, engine.Color{R: 4, G: 4, B: 24, A: 255})

	// Plot player.
	if p.playerAlive {
		px := rx + int(p.player.x*float64(rcell))
		py := ry + int(p.player.y*float64(rcell))
		c.FillRect(px, py, rcell, rcell, radarPlayer)
	}
	// Plot monsters — radar always shows them, regardless of
	// invisibility on the main canvas.
	for _, m := range p.monsters {
		if m.state == msDying {
			continue
		}
		mx := rx + int(m.x*float64(rcell))
		my := ry + int(m.y*float64(rcell))
		col := monsterColor(m.kind)
		c.FillRect(mx, my, rcell, rcell, col)
	}
	if p.worluk != nil {
		mx := rx + int(p.worluk.x*float64(rcell))
		my := ry + int(p.worluk.y*float64(rcell))
		c.FillRect(mx, my, rcell, rcell, radarWorluk)
	}
	if p.wizard != nil {
		mx := rx + int(p.wizard.x*float64(rcell))
		my := ry + int(p.wizard.y*float64(rcell))
		c.FillRect(mx, my, rcell, rcell, radarWizard)
	}
}

// drawHUD paints the top score line, the lives indicator, and a
// dungeon counter / bonus label.
func (p *playScene) drawHUD(c *engine.Canvas, _ geometry) {
	scoreText := fmt.Sprintf("SCORE %s", pad6(p.score))
	hiText := fmt.Sprintf("HI %s", pad6(p.hiScore))
	dunText := fmt.Sprintf("DUNGEON %d", p.dungeon)
	if p.doubleScore {
		dunText = "DOUBLE  " + dunText
	}

	c.Print(1, 0, scoreText, worriorBody)
	c.Print((c.Cols()-len(hiText))/2, 0, hiText, engine.White)
	c.Print(c.Cols()-len(dunText)-1, 0, dunText,
		engine.Color{R: 255, G: 150, B: 80, A: 255})

	// Lives — small Worrior icons on the second row.
	lifeY := 2
	for i := 0; i < p.lives-1; i++ {
		x := 1 + i*7
		drawSpriteScaled(c, x, lifeY, worriorUp, 1)
	}

	hint := "ESC QUIT"
	c.Print(c.Cols()-len(hint)-1, c.Rows()-1, hint, engine.Gray)
}

// drawOverlay paints state-specific banners.
func (p *playScene) drawOverlay(c *engine.Canvas, _ geometry) {
	switch p.state {
	case psReady:
		text := "READY!"
		if p.dungeon > 1 {
			text = fmt.Sprintf("DUNGEON %d", p.dungeon)
		}
		drawCentreText(c, text, engine.Color{R: 255, G: 230, B: 60, A: 255})
	case psDungeonClear:
		drawCentreText(c, "DUNGEON CLEAR", engine.Color{R: 120, G: 240, B: 120, A: 255})
		if p.doubleScore {
			sub := "DOUBLE SCORE NEXT"
			c.Print((c.Cols()-len(sub))/2, c.Rows()/2+3, sub, worlukBody)
		}
	case psWorlukReady:
		drawCentreText(c, "WORLUK!", worlukBody)
	case psWizardReady:
		drawCentreText(c, "THE WIZARD!", wizardRobe)
	case psGameOver:
		drawCentreText(c, "GAME OVER", engine.Color{R: 255, G: 80, B: 80, A: 255})
		hint := "ENTER  QUIT"
		c.Print((c.Cols()-len(hint))/2, c.Rows()/2+4, hint, engine.White)
	}
}

func drawCentreText(c *engine.Canvas, text string, col engine.Color) {
	tw := engine.TextWidth(text)
	tx := (c.Width() - tw) / 2
	ty := c.Height()/2 - engine.FontHeight/2
	c.DrawText(tx, ty, text, col)
}
