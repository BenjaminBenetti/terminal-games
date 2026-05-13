package pacman

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Speeds are in TILES per second (the maze is 28×31
// tiles regardless of the rendered tile-size, so this is the natural
// per-frame unit). Mode-timer values come from the arcade ROM's
// level-1 cadence; higher levels shorten the scatter periods and the
// frightened duration (see levelTuning).
const (
	basePacSpeed   = 8.0
	baseGhostSpeed = 7.4
	frightSpeed    = 4.4
	tunnelSpeed    = 4.0
	eatenSpeed     = 14.0
	houseLeaveSpd  = 4.5

	ghostHitRadius = 0.55 // collision threshold (in tiles) between Pac-Man and a ghost

	readyHold     = 2.2 // seconds of "READY!" before the first round
	dyingHold     = 1.5 // seconds of Pac-Man death animation
	levelClearGap = 1.6 // seconds between clearing the last dot and next level

	// Mouth open/close rate. The actual visible open/close cadence is
	// 2× this (|sin| period is half of sin's period), so 2.0 here
	// gives roughly four chomps per second — close to the arcade.
	mouthCyclePerSec = 2.0

	scoreDot       = 10
	scorePellet    = 50
	scoreFruit     = 100 // simplified — original increases by level
	extraLifeScore = 10000
	startLives     = 3
)

// playState is the gameplay sub-state machine.
type playState int

const (
	psReady playState = iota
	psPlaying
	psDying
	psLevelClear
	psGameOver
)

// modePhase records the current global ghost mode (scatter vs chase).
// The frightened state is tracked separately because it sits on top
// of the underlying phase — when frightened ends, ghosts revert to
// whatever the phase timer says is current.
type modePhase struct {
	current   ghostMode // modeScatter or modeChase
	t         float64   // elapsed in current phase
	idx       int       // index into modeSchedule
	pacDot    int       // count of dots eaten since last reverse (drives Elroy etc; here unused)
	frightenT float64   // > 0 while frightened mode is active
	ghostsEat int       // how many ghosts eaten during the current frightened
}

// modeSchedule is the level-1 mode pattern: alternating scatter and
// chase phases, ending in permanent chase. Higher levels shrink the
// scatter periods (level 2-4 ≈ original, level 5+ shrinks more).
var modeSchedule = []struct {
	mode ghostMode
	dur  float64 // seconds; <0 means "infinite, stay here"
}{
	{modeScatter, 7},
	{modeChase, 20},
	{modeScatter, 7},
	{modeChase, 20},
	{modeScatter, 5},
	{modeChase, 20},
	{modeScatter, 5},
	{modeChase, -1},
}

// levelTuning is the per-level difficulty knob. Speeds scale and the
// frightened-power duration shrinks as the level number grows. Higher
// levels cap at the level-5 values (the original game eventually
// removes frightened entirely; we keep a token 1-second window so the
// energizer still chains ghosts visually).
type levelTuning struct {
	pacSpeedMul     float64
	ghostSpeedMul   float64
	frightSpeedMul  float64
	frightDuration  float64
	frightFlashes   int
}

func tuningForLevel(level int) levelTuning {
	// Speeds increase by ~5% per level for the first 4, then hold.
	mul := 1.0 + 0.05*math.Min(float64(level-1), 4)
	fright := 6.0
	switch {
	case level >= 19:
		fright = 0
	case level >= 9:
		fright = 1
	case level >= 5:
		fright = 2
	case level >= 4:
		fright = 3
	case level >= 3:
		fright = 4
	case level >= 2:
		fright = 5
	}
	return levelTuning{
		pacSpeedMul:    mul,
		ghostSpeedMul:  mul,
		frightSpeedMul: 1,
		frightDuration: fright,
		frightFlashes:  5,
	}
}

// frightChain is the score for the n-th ghost in a single frightened
// streak. Anything past 4 stays at 1600 (matches arcade).
var frightChain = [...]int{200, 400, 800, 1600}

// scorePopup is a short-lived floating number rendered over the maze
// when Pac-Man scores a notable reward (eating a ghost, picking up a
// fruit, …). It drifts upward and fades to transparent over its TTL.
type scorePopup struct {
	x, y float64 // tile-space position
	text string
	age  float64
	ttl  float64
	col  engine.Color
}

// playScene contains the full match state.
type playScene struct {
	e    *engine.Engine
	maze *maze
	rng  *rand.Rand

	pac    entity
	pacAngle float64 // animation phase for mouth open/close

	ghosts [4]*ghost

	phase modePhase
	state playState
	stateT float64

	score   int
	hiScore int
	lives   int
	level   int
	dotsEaten int

	tuning levelTuning

	extraLifeAwarded bool
	wantQuit         bool

	// pendingFruit and fruitScore are placeholders for the bonus fruit
	// system. The fruit appears once per level after 70 dots eaten, in
	// the corridor in front of the ghost house, for ~9 seconds.
	fruitActive bool
	fruitTimer  float64
	fruitTile   [2]int
	fruitScore  int

	// flashing-maze state at level clear (the maze flashes between
	// dark blue and white as the level-cleared celebration).
	clearFlashT float64

	// popups is the live list of floating score numbers. Aged in
	// updatePlaying and rendered on top of the maze in Draw.
	popups []scorePopup
}

// newPlayScene constructs a play scene ready to begin level 1 with
// the given carry-over high score.
func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	p := &playScene{
		e:       e,
		maze:    newMaze(),
		rng:     rng,
		hiScore: hiScore,
		lives:   startLives,
		level:   1,
	}
	p.tuning = tuningForLevel(p.level)
	p.resetRound()
	return p
}

// resetRound puts Pac-Man and the ghosts back at their starting
// positions and primes the READY pause. Called on round start, after
// a death (without level reset), and at level transitions.
func (p *playScene) resetRound() {
	p.pac = entity{
		x:       13.5,
		y:       23.5,
		dir:     dirLeft,
		desired: dirLeft,
		speed:   basePacSpeed * p.tuning.pacSpeedMul,
	}
	p.pacAngle = 0

	p.ghosts[blinky] = newGhost(blinky, p.rng, 0)
	p.ghosts[pinky] = newGhost(pinky, p.rng, 0)
	p.ghosts[inky] = newGhost(inky, p.rng, 30)
	p.ghosts[clyde] = newGhost(clyde, p.rng, 60)
	for _, g := range p.ghosts {
		g.speed = baseGhostSpeed * p.tuning.ghostSpeedMul
	}

	p.phase = modePhase{current: modeSchedule[0].mode}
	p.popups = nil
	p.state = psReady
	p.stateT = 0
}

// resetForNewLevel keeps the score and lives but rebuilds the maze
// (dots restored) and bumps the level number / tuning.
func (p *playScene) advanceLevel() {
	p.level++
	p.tuning = tuningForLevel(p.level)
	p.maze = newMaze()
	p.dotsEaten = 0
	p.fruitActive = false
	p.fruitTimer = 0
	p.resetRound()
}

// --- Update ----------------------------------------------------------

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
				p.resetRound()
			}
		}
	case psLevelClear:
		p.clearFlashT += s
		if p.stateT >= levelClearGap {
			p.advanceLevel()
		}
	case psGameOver:
		// Wait for player input (handled in handleInput).
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
		// Direction keys (also from WASD) buffer the next Pac-Man turn.
		switch k.Code {
		case engine.KeyUp:
			p.pac.desired = dirUp
		case engine.KeyDown:
			p.pac.desired = dirDown
		case engine.KeyLeft:
			p.pac.desired = dirLeft
		case engine.KeyRight:
			p.pac.desired = dirRight
		case engine.KeyEsc:
			p.wantQuit = true
		case engine.KeyEnter:
			if p.state == psGameOver {
				p.wantQuit = true
			}
		case engine.KeyChar:
			switch k.Rune {
			case 'w', 'W':
				p.pac.desired = dirUp
			case 's', 'S':
				p.pac.desired = dirDown
			case 'a', 'A':
				p.pac.desired = dirLeft
			case 'd', 'D':
				p.pac.desired = dirRight
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

// updatePlaying drives the main gameplay frame.
func (p *playScene) updatePlaying(s float64) {
	p.advanceModePhase(s)

	// Pac-Man moves first so ghosts can react to his fresh position
	// for the AI decisions later this frame (matches arcade ordering).
	p.movePacMan(s)
	if p.state != psPlaying {
		return
	}

	// Ghosts pick their next tile based on Pac-Man's *post-move*
	// position. Each ghost handles its own state machine internally,
	// only requesting the global phase from p.phase.current.
	for _, g := range p.ghosts {
		p.driveGhost(g, s)
	}

	p.checkGhostCollisions()
	p.updateFruit(s)
	p.updatePopups(s)
}

// updatePopups ages every active popup and drops the ones that have
// outlived their TTL.
func (p *playScene) updatePopups(s float64) {
	if len(p.popups) == 0 {
		return
	}
	kept := p.popups[:0]
	for _, pop := range p.popups {
		pop.age += s
		pop.y -= 1.2 * s // drift up ~1.2 tiles/sec
		if pop.age < pop.ttl {
			kept = append(kept, pop)
		}
	}
	p.popups = kept
}

// spawnPopup queues a floating score label at the given tile-space
// position.
func (p *playScene) spawnPopup(x, y float64, text string, col engine.Color) {
	p.popups = append(p.popups, scorePopup{
		x: x, y: y,
		text: text,
		ttl:  1.0,
		col:  col,
	})
}

// advanceModePhase ticks the global scatter/chase timer and the
// frightened timer, reversing all hunting ghosts when the phase
// switches.
func (p *playScene) advanceModePhase(s float64) {
	if p.phase.frightenT > 0 {
		p.phase.frightenT -= s
		if p.phase.frightenT <= 0 {
			p.phase.frightenT = 0
			p.endFrightened()
		}
	} else {
		// Phase only advances when NOT frightened — frightened pauses
		// the timer in the arcade.
		sched := modeSchedule[p.phase.idx]
		if sched.dur > 0 {
			p.phase.t += s
			if p.phase.t >= sched.dur {
				p.phase.idx++
				if p.phase.idx >= len(modeSchedule) {
					p.phase.idx = len(modeSchedule) - 1
				}
				p.phase.t = 0
				newMode := modeSchedule[p.phase.idx].mode
				if newMode != p.phase.current {
					p.phase.current = newMode
					p.reverseHuntingGhosts()
				}
			}
		}
	}
}

// reverseHuntingGhosts flips the heading of every ghost that's
// currently in chase or scatter (the "outside, hunting" condition).
// This is the visible cue at a scatter↔chase boundary and at the
// moment a power pellet is eaten.
func (p *playScene) reverseHuntingGhosts() {
	for _, g := range p.ghosts {
		if g.mode == modeChase || g.mode == modeScatter || g.mode == modeFrightened {
			g.dir = g.dir.opposite()
			g.desired = g.dir
			// Force the AI to re-pick at the next tile centre by
			// clearing the last-decision marker.
			g.lastDecisionTile = [2]int{-1, -1}
		}
	}
}

// movePacMan advances Pac-Man, eats any pellet at his arrival tile,
// and folds in the side effects of those pellets (score, fright
// trigger, extra life threshold, level-clear).
func (p *playScene) movePacMan(s float64) {
	p.pac.speed = basePacSpeed * p.tuning.pacSpeedMul
	prevTileX, prevTileY := p.pac.tileX(), p.pac.tileY()
	p.pac.advance(s, p.maze.walkableForPac)

	// Mouth animation only progresses while actually moving.
	if p.pac.dir != dirNone {
		p.pacAngle += s * mouthCyclePerSec * 2 * math.Pi
	}

	tx, ty := p.pac.tileX(), p.pac.tileY()
	if tx != prevTileX || ty != prevTileY {
		// Crossed into a new tile — check for pellet collection.
		switch p.maze.eatPellet(tx, ty) {
		case tileDot:
			p.score += scoreDot
			p.dotsEaten++
			p.maybeSpawnFruit()
			p.checkExtraLife()
		case tilePellet:
			p.score += scorePellet
			p.dotsEaten++
			p.maybeSpawnFruit()
			p.checkExtraLife()
			p.triggerFrightened()
		}
		if p.maze.remainingDots() == 0 {
			p.state = psLevelClear
			p.stateT = 0
			p.clearFlashT = 0
			return
		}
	}

	// Fruit pickup — checked by tile match.
	if p.fruitActive && tx == p.fruitTile[0] && ty == p.fruitTile[1] {
		p.score += p.fruitScore
		p.fruitActive = false
		p.checkExtraLife()
	}
}

// driveGhost advances one ghost for this frame, applying mode-specific
// movement rules.
func (p *playScene) driveGhost(g *ghost, s float64) {
	switch g.mode {
	case modeInHouse:
		p.updateHouseGhost(g, s)
		return
	case modeLeavingHouse:
		p.updateLeavingGhost(g, s)
		return
	case modeEaten:
		p.updateEatenGhost(g, s)
		return
	case modeEntering:
		p.updateEnteringGhost(g, s)
		return
	}

	// Choose speed based on conditions.
	speed := baseGhostSpeed * p.tuning.ghostSpeedMul
	if g.mode == modeFrightened {
		speed = frightSpeed * p.tuning.ghostSpeedMul
	}
	// Slow down in the tunnel row at the side mouths.
	if g.tileY() == tunnelRow && (g.tileX() < 6 || g.tileX() >= mazeCols-6) {
		if speed > tunnelSpeed {
			speed = tunnelSpeed
		}
	}
	g.speed = speed

	// Hunting ghosts can't re-enter the house door, so canPass forbids it.
	canPass := func(col, row int) bool {
		return p.maze.walkableForGhost(col, row, false)
	}

	// AI decision at each tile centre. We detect "newly arrived at a
	// tile" by comparing the current tile to lastDecisionTile.
	cur := [2]int{g.tileX(), g.tileY()}
	if cur != g.lastDecisionTile {
		g.lastDecisionTile = cur

		// Sync per-ghost mode with the global phase unless frightened.
		if g.mode != modeFrightened {
			g.mode = p.phase.current
		}

		in := aiInputs{
			pacTileX:    p.pac.tileX(),
			pacTileY:    p.pac.tileY(),
			pacDir:      p.pac.dir,
			blinkyTileX: p.ghosts[blinky].tileX(),
			blinkyTileY: p.ghosts[blinky].tileY(),
			phase:       p.phase.current,
		}
		next := g.pickNextDirection(in, canPass)
		if next != dirNone {
			g.desired = next
		}
	}

	g.entity.advance(s, canPass)
}

// updateHouseGhost handles the bobbing motion of a ghost still waiting
// inside the house, and checks whether the dot count permits release.
func (p *playScene) updateHouseGhost(g *ghost, s float64) {
	g.bobT += s
	slot := ghostHouseSlot[g.kind]
	g.x = slot[0]
	g.y = slot[1] + 0.35*math.Sin(g.bobT*3.5)

	if p.dotsEaten >= g.dotsRequired {
		g.mode = modeLeavingHouse
		g.dir = dirUp
		g.desired = dirUp
	}
}

// updateLeavingGhost steers the ghost up the centre column through
// the door and out into the corridor. Movement here is hand-scripted
// (not AI-driven) so the ghost emerges cleanly regardless of the
// usual non-walkable-by-ghost-AI rules.
func (p *playScene) updateLeavingGhost(g *ghost, s float64) {
	g.speed = houseLeaveSpd
	const exitX = 13.5
	const exitY = 11.5 // tile above the door
	// First slide horizontally toward exitX, then rise to exitY.
	dx := exitX - g.x
	dy := exitY - g.y
	step := g.speed * s
	if math.Abs(dx) > 0.01 {
		if math.Abs(dx) <= step {
			g.x = exitX
		} else if dx > 0 {
			g.x += step
		} else {
			g.x -= step
		}
		// Face the direction we're sliding.
		if dx > 0 {
			g.dir = dirRight
		} else {
			g.dir = dirLeft
		}
		return
	}
	g.dir = dirUp
	if math.Abs(dy) <= step {
		g.y = exitY
		// Now outside — switch into the global hunting mode.
		g.mode = p.phase.current
		g.dir = dirLeft
		g.desired = dirLeft
		g.lastDecisionTile = [2]int{-1, -1}
		return
	}
	g.y -= step
}

// updateEatenGhost steers eyes back toward the ghost-house door using
// the standard targeting algorithm. The target tile is the corridor
// cell directly above the door — once the eyes arrive there, control
// hands off to updateEnteringGhost for the scripted dive through the
// door into the house centre.
func (p *playScene) updateEatenGhost(g *ghost, s float64) {
	// Arrival at the entry tile: switch to the scripted dive. We
	// intercept this BEFORE the AI runs because the AI's distance-
	// minimisation breaks down when the ghost is sitting on top of
	// its target tile (all neighbours are equidistant and the
	// tie-break can pick a direction *away* from the door).
	if g.tileX() == 13 && g.tileY() == 11 {
		g.mode = modeEntering
		g.x = 13.5 // line up with the door so the dive doesn't clip
		g.dir = dirDown
		g.desired = dirDown
		g.lastDecisionTile = [2]int{-1, -1}
		return
	}

	g.speed = eatenSpeed
	canPass := func(col, row int) bool {
		return p.maze.walkableForGhost(col, row, true)
	}

	cur := [2]int{g.tileX(), g.tileY()}
	if cur != g.lastDecisionTile {
		g.lastDecisionTile = cur
		next := g.pickNextDirection(aiInputs{}, canPass)
		if next != dirNone {
			g.desired = next
		}
	}
	g.entity.advance(s, canPass)
}

// updateEnteringGhost runs the scripted dive: from the entry tile
// (13.5, 11.5), slide down to the house centre (13.5, 14.5), then
// flip into modeLeavingHouse so the ghost climbs back out and rejoins
// the hunt. No collision with Pac-Man is possible during this phase
// because the ghost is rendered eyes-only and checkGhostCollisions
// skips it.
func (p *playScene) updateEnteringGhost(g *ghost, s float64) {
	g.speed = eatenSpeed
	const targetX = 13.5
	const targetY = 14.5
	step := g.speed * s

	// First snap the x-axis to the door column. Eyes that arrived from
	// a side approach will be slightly off-centre.
	if math.Abs(g.x-targetX) > 1e-3 {
		if math.Abs(g.x-targetX) <= step {
			g.x = targetX
		} else if g.x < targetX {
			g.x += step
			return
		} else {
			g.x -= step
			return
		}
	}

	g.dir = dirDown
	g.y += step
	if g.y >= targetY {
		g.y = targetY
		g.dotsRequired = 0
		g.mode = modeLeavingHouse
		g.dir = dirUp
		g.desired = dirUp
		g.lastDecisionTile = [2]int{-1, -1}
	}
}

// triggerFrightened starts (or extends) the frightened state and
// resets the per-streak ghost-eaten counter. All hunting ghosts
// reverse direction immediately.
func (p *playScene) triggerFrightened() {
	if p.tuning.frightDuration <= 0 {
		// On the very late levels, frightened is effectively skipped —
		// don't even flip ghost direction in that case.
		return
	}
	p.phase.frightenT = p.tuning.frightDuration
	p.phase.ghostsEat = 0
	for _, g := range p.ghosts {
		if g.mode == modeChase || g.mode == modeScatter {
			g.mode = modeFrightened
			g.dir = g.dir.opposite()
			g.desired = g.dir
			g.speed = frightSpeed * p.tuning.frightSpeedMul
			g.lastDecisionTile = [2]int{-1, -1}
		}
	}
}

// endFrightened returns every still-frightened ghost to the global
// chase/scatter phase.
func (p *playScene) endFrightened() {
	for _, g := range p.ghosts {
		if g.mode == modeFrightened {
			g.mode = p.phase.current
			g.lastDecisionTile = [2]int{-1, -1}
		}
	}
}

// checkGhostCollisions tests Pac-Man against each ghost. Hunting
// ghosts kill Pac-Man on contact; frightened ghosts get eaten for an
// escalating score; eaten/in-house/leaving ghosts are non-lethal.
func (p *playScene) checkGhostCollisions() {
	for _, g := range p.ghosts {
		dx := g.x - p.pac.x
		dy := g.y - p.pac.y
		if dx*dx+dy*dy > ghostHitRadius*ghostHitRadius {
			continue
		}
		switch g.mode {
		case modeFrightened:
			idx := p.phase.ghostsEat
			if idx >= len(frightChain) {
				idx = len(frightChain) - 1
			}
			points := frightChain[idx]
			p.score += points
			p.phase.ghostsEat++
			p.spawnPopup(g.x, g.y, fmt.Sprintf("%d", points),
				engine.Color{R: 80, G: 220, B: 255, A: 255})
			g.mode = modeEaten
			g.lastDecisionTile = [2]int{-1, -1}
			p.checkExtraLife()
		case modeChase, modeScatter:
			p.startDying()
			return
		}
	}
}

func (p *playScene) startDying() {
	p.lives--
	p.state = psDying
	p.stateT = 0
}

func (p *playScene) checkExtraLife() {
	if !p.extraLifeAwarded && p.score >= extraLifeScore {
		p.lives++
		p.extraLifeAwarded = true
	}
}

// maybeSpawnFruit drops the bonus fruit into the corridor in front of
// the ghost house after the canonical dot-count thresholds (70 and
// 170). The fruit lingers for ~9 seconds.
func (p *playScene) maybeSpawnFruit() {
	if p.fruitActive {
		return
	}
	if p.dotsEaten == 70 || p.dotsEaten == 170 {
		p.fruitActive = true
		p.fruitTimer = 9
		p.fruitTile = [2]int{13, 17}
		p.fruitScore = scoreFruit + (p.level-1)*100
		if p.fruitScore > 5000 {
			p.fruitScore = 5000
		}
	}
}

func (p *playScene) updateFruit(s float64) {
	if !p.fruitActive {
		return
	}
	p.fruitTimer -= s
	if p.fruitTimer <= 0 {
		p.fruitActive = false
	}
}

// --- Drawing ---------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 0, G: 0, B: 0, A: 255})

	geo := computeGeometry(c)
	p.drawMaze(c, geo)
	p.drawPellets(c, geo)
	p.drawFruit(c, geo)
	if p.state != psLevelClear {
		p.drawGhosts(c, geo)
	}
	if p.state != psLevelClear && p.state != psGameOver {
		p.drawPacMan(c, geo)
	}
	p.drawPopups(c, geo)
	p.drawHUD(c, geo)
	p.drawOverlay(c, geo)
}

// drawPopups paints the floating-score labels. Each one is rendered
// in the terminal's native font (one rune per cell), centred on its
// tracked tile-space position and dimmed as its age approaches the
// TTL so it fades out instead of vanishing abruptly.
func (p *playScene) drawPopups(c *engine.Canvas, geo mazeGeo) {
	for _, pop := range p.popups {
		px := geo.originX + int(math.Round(pop.x*float64(geo.tile)))
		py := geo.originY + int(math.Round(pop.y*float64(geo.tile)))
		frac := pop.age / pop.ttl
		if frac > 1 {
			frac = 1
		}
		dim := 1 - frac*0.5
		col := engine.Color{
			R: uint8(float64(pop.col.R) * dim),
			G: uint8(float64(pop.col.G) * dim),
			B: uint8(float64(pop.col.B) * dim),
			A: 255,
		}
		cellX := px - len(pop.text)/2
		cellY := py / 2
		c.Print(cellX, cellY, pop.text, col)
	}
}

// mazeGeo holds the on-screen layout: tile size in pixels, the pixel
// origin of (col 0, row 0), and a handful of derived constants used
// by the drawing functions.
type mazeGeo struct {
	tile    int
	originX int
	originY int
	hudTop  int
	hudBot  int
}

// computeGeometry sizes the maze to the available canvas. The HUD
// takes a few cell rows at the top and bottom; the rest splits
// between width and height to find the largest integer tile size
// that fits. On very small terminals the HUD reservation shrinks
// or the tile falls back to 1 pixel rather than clipping the maze.
func computeGeometry(c *engine.Canvas) mazeGeo {
	w := c.Width()
	h := c.Height()
	hudTop := 4
	hudBot := 4

	// Pick the largest integer tile size such that both width and the
	// (HUD-reserved) height fit. Walk down from a sensible max so the
	// maze stays compact on big terminals.
	tile := 0
	for t := 6; t >= 1; t-- {
		if t*mazeCols <= w && t*mazeRows+hudTop+hudBot <= h {
			tile = t
			break
		}
	}
	// If even tile=1 doesn't fit, try shrinking the HUD margins.
	if tile == 0 {
		hudTop = 2
		hudBot = 2
		for t := 6; t >= 1; t-- {
			if t*mazeCols <= w && t*mazeRows+hudTop+hudBot <= h {
				tile = t
				break
			}
		}
	}
	if tile == 0 {
		// Worst case: render whatever fits, clipping if necessary.
		tile = 1
		hudTop = 2
		hudBot = 2
	}

	mazePixW := tile * mazeCols
	mazePixH := tile * mazeRows
	originX := (w - mazePixW) / 2
	if originX < 0 {
		originX = 0
	}
	originY := hudTop + ((h-hudTop-hudBot)-mazePixH)/2
	if originY < hudTop {
		originY = hudTop
	}
	return mazeGeo{
		tile:    tile,
		originX: originX,
		originY: originY,
		hudTop:  hudTop,
		hudBot:  hudBot,
	}
}

// tileToPixel converts a tile-space float position to the centre pixel.
func (g mazeGeo) tileToPixel(tx, ty float64) (int, int) {
	x := g.originX + int(math.Round(tx*float64(g.tile)))
	y := g.originY + int(math.Round(ty*float64(g.tile)))
	return x, y
}

// drawMaze paints the wall structure. Each wall tile is filled with
// the maze blue, except during the level-clear flash where the maze
// strobes between blue and white.
func (p *playScene) drawMaze(c *engine.Canvas, geo mazeGeo) {
	col := engine.Color{R: 33, G: 33, B: 222, A: 255}
	if p.state == psLevelClear {
		// Flash blue ↔ white. Cycle ~6 Hz so the celebration reads.
		if int(p.clearFlashT*6)%2 == 0 {
			col = engine.Color{R: 240, G: 240, B: 255, A: 255}
		}
	}
	for r := 0; r < mazeRows; r++ {
		for cc := 0; cc < mazeCols; cc++ {
			t := p.maze.wallAt(cc, r)
			if t != tileWall {
				continue
			}
			x := geo.originX + cc*geo.tile
			y := geo.originY + r*geo.tile
			c.FillRect(x, y, geo.tile, geo.tile, col)
		}
	}

	// Ghost-house door: thin pink strip in the centre of its two cells.
	doorCol := engine.Color{R: 255, G: 184, B: 222, A: 255}
	for cc := 0; cc < mazeCols; cc++ {
		if p.maze.wallAt(cc, 12) != tileDoor {
			continue
		}
		x := geo.originX + cc*geo.tile
		y := geo.originY + 12*geo.tile + geo.tile/2
		c.FillRect(x, y, geo.tile, 1, doorCol)
	}
}

// drawPellets paints the small dots and the pulsing energizers. The
// dot colour matches the arcade's salmon-pink "pellet" hue; energizers
// blink on a 4 Hz cycle, exactly as in the original.
func (p *playScene) drawPellets(c *engine.Canvas, geo mazeGeo) {
	dotCol := engine.Color{R: 250, G: 200, B: 168, A: 255}
	pelletCol := engine.Color{R: 255, G: 218, B: 172, A: 255}
	energizerVisible := int(p.stateT*4)%2 == 0
	for r := 0; r < mazeRows; r++ {
		for cc := 0; cc < mazeCols; cc++ {
			t := p.maze.pelletAt(cc, r)
			x := geo.originX + cc*geo.tile + geo.tile/2
			y := geo.originY + r*geo.tile + geo.tile/2
			switch t {
			case tileDot:
				c.Set(x, y, dotCol)
				// At tile ≥ 3 paint a small 2-pixel dot so it reads
				// from a distance; at tile=2 the maze is dense enough
				// that a single pixel is the right size.
				if geo.tile >= 3 {
					c.Set(x+1, y, dotCol)
				}
			case tilePellet:
				if !energizerVisible {
					continue
				}
				rad := geo.tile / 2
				if rad < 1 {
					rad = 1
				}
				c.FillCircle(x, y, rad, pelletCol)
			}
		}
	}
}

// drawPacMan paints Pac-Man as a yellow disc with a wedge mouth
// pointing in his current direction. The mouth opens and closes by
// shrinking and growing a wedge that is "cut" from the disc by
// repainting it with the background colour.
func (p *playScene) drawPacMan(c *engine.Canvas, geo mazeGeo) {
	cx, cy := geo.tileToPixel(p.pac.x, p.pac.y)
	radius := geo.tile - 1
	if radius < 1 {
		radius = 1
	}

	yellow := engine.Color{R: 255, G: 240, B: 0, A: 255}

	// Death animation: shrink to nothing.
	if p.state == psDying {
		frac := p.stateT / dyingHold
		if frac > 1 {
			frac = 1
		}
		r := int(float64(radius) * (1 - frac))
		if r < 1 {
			return
		}
		c.FillCircle(cx, cy, r, yellow)
		return
	}

	c.FillCircle(cx, cy, radius, yellow)
	// A radius-1 circle is a 5-pixel plus shape — fill in the corners
	// so Pac-Man reads as a solid blob at the smallest tile size.
	if radius == 1 {
		c.Set(cx-1, cy-1, yellow)
		c.Set(cx+1, cy-1, yellow)
		c.Set(cx-1, cy+1, yellow)
		c.Set(cx+1, cy+1, yellow)
	}

	if p.pac.dir == dirNone {
		return
	}

	// Both sprite sizes share the same open/closed phase so the chomp
	// stays in lock-step regardless of tile size.
	open := math.Abs(math.Sin(p.pacAngle))

	// Tiny-Pac-Man fallback: at radius 1 the wedge is impossible, so
	// stage three discrete frames — closed, half-open (tip pixel
	// punched), and wide-open (tip + a bite into the body) — based on
	// the open phase. The result is a recognisable chomp even at 3×3
	// pixels.
	if radius < 2 {
		if open < 0.2 {
			return // closed mouth: leave the full disc as drawn
		}
		bg := engine.Color{R: 0, G: 0, B: 0, A: 255}
		var dx, dy int
		switch p.pac.dir {
		case dirRight:
			dx, dy = 1, 0
		case dirLeft:
			dx, dy = -1, 0
		case dirUp:
			dx, dy = 0, -1
		case dirDown:
			dx, dy = 0, 1
		}
		c.Set(cx+dx, cy+dy, bg) // tip
		if open >= 0.6 {
			c.Set(cx, cy, bg) // bite into the body for the wide-open frame
		}
		return
	}

	// Mouth wedge for radius ≥ 2: a black triangle cut from the disc
	// on the side of the direction of motion. The opening angle
	// oscillates 0..maxOpen following pacAngle.
	maxOpen := math.Pi / 2 // 90° at full open
	half := maxOpen * open / 2
	if half < 0.01 {
		return
	}

	mouthDir := p.pac.dir
	// Pac-Man faces the direction of motion. Compute the wedge axis.
	var ax, ay float64
	switch mouthDir {
	case dirRight:
		ax, ay = 1, 0
	case dirLeft:
		ax, ay = -1, 0
	case dirUp:
		ax, ay = 0, -1
	case dirDown:
		ax, ay = 0, 1
	default:
		ax, ay = 1, 0
	}

	// Build the wedge as a fan of lines from the centre out to two
	// boundary points on the disc edge, then sweep the angle between.
	bg := engine.Color{R: 0, G: 0, B: 0, A: 255}
	steps := radius * 6
	if steps < 8 {
		steps = 8
	}
	for i := 0; i <= steps; i++ {
		t := -half + 2*half*float64(i)/float64(steps)
		// Rotate (ax, ay) by t.
		cs := math.Cos(t)
		sn := math.Sin(t)
		rx := ax*cs - ay*sn
		ry := ax*sn + ay*cs
		ex := cx + int(math.Round(rx*float64(radius+1)))
		ey := cy + int(math.Round(ry*float64(radius+1)))
		c.DrawLine(cx, cy, ex, ey, bg)
	}
}

// drawGhosts paints the four ghosts. The body colour reflects the
// ghost's current effective mode (frightened ghosts are blue, eaten
// ghosts are just eyes). Each ghost has two small eye-pupils that
// shift toward its direction of travel.
func (p *playScene) drawGhosts(c *engine.Canvas, geo mazeGeo) {
	for _, g := range p.ghosts {
		p.drawGhost(c, geo, g)
	}
}

func (p *playScene) drawGhost(c *engine.Canvas, geo mazeGeo, g *ghost) {
	cx, cy := geo.tileToPixel(g.x, g.y)
	radius := geo.tile - 1
	if radius < 1 {
		radius = 1
	}

	body := ghostBodyColor(g.kind)
	switch g.mode {
	case modeFrightened:
		// Flash white near the end of frightened.
		body = engine.Color{R: 33, G: 33, B: 222, A: 255}
		if p.phase.frightenT < 2 && int(p.phase.frightenT*8)%2 == 0 {
			body = engine.Color{R: 240, G: 240, B: 240, A: 255}
		}
	case modeEaten:
		// Eyes only — don't draw the body.
		p.drawGhostEyes(c, cx, cy, radius, g.dir)
		return
	}

	// Round top half of the ghost body.
	c.FillCircle(cx, cy, radius, body)
	// Rectangular bottom half so the silhouette is the classic dome.
	c.FillRect(cx-radius, cy, 2*radius+1, radius+1, body)

	// Frilled bottom: paint two notches of the background under the
	// "skirt". Only attempted at tile ≥ 3 since we need horizontal
	// resolution to spell out the curves.
	if radius >= 2 {
		notch := engine.Color{R: 0, G: 0, B: 0, A: 255}
		notchY := cy + radius
		notchW := radius / 2
		if notchW < 1 {
			notchW = 1
		}
		c.FillRect(cx-radius, notchY, notchW, 1, notch)
		c.FillRect(cx+radius-notchW+1, notchY, notchW, 1, notch)
		// Middle notch shifts a tiny bit horizontally over time to
		// suggest the wavy bottom edge.
		mid := cx - notchW/2
		mid += int(2*math.Sin(p.stateT*8+float64(g.kind)))
		c.FillRect(mid, notchY, notchW, 1, notch)
	}

	p.drawGhostEyes(c, cx, cy, radius, g.dir)
}

func (p *playScene) drawGhostEyes(c *engine.Canvas, cx, cy, radius int, dir direction) {
	if radius < 1 {
		return
	}
	white := engine.Color{R: 255, G: 255, B: 255, A: 255}
	pupil := engine.Color{R: 33, G: 33, B: 222, A: 255}

	eyeR := radius / 3
	if eyeR < 1 {
		eyeR = 1
	}
	offX := radius / 2
	if offX < 1 {
		offX = 1 // ensure the two eyes don't overlap on tiny sprites
	}
	offY := radius / 3
	if offY < 1 {
		offY = 1
	}

	lx, ly := cx-offX, cy-offY
	rx, ry := cx+offX, cy-offY
	c.FillCircle(lx, ly, eyeR, white)
	c.FillCircle(rx, ry, eyeR, white)

	// Pupil shift in direction of travel. At the smallest sprite size
	// the pupil overwrites the entire eye, which still reads as a
	// directional eye thanks to the contrast.
	var px, py int
	switch dir {
	case dirLeft:
		px = -1
	case dirRight:
		px = 1
	case dirUp:
		py = -1
	case dirDown:
		py = 1
	}
	c.Set(lx+px, ly+py, pupil)
	c.Set(rx+px, ry+py, pupil)
}

func ghostBodyColor(k ghostKind) engine.Color {
	switch k {
	case blinky:
		return engine.Color{R: 255, G: 0, B: 0, A: 255}
	case pinky:
		return engine.Color{R: 255, G: 184, B: 222, A: 255}
	case inky:
		return engine.Color{R: 0, G: 222, B: 222, A: 255}
	case clyde:
		return engine.Color{R: 255, G: 184, B: 71, A: 255}
	}
	return engine.Color{R: 255, G: 255, B: 255, A: 255}
}

// drawFruit paints the bonus fruit, if active.
func (p *playScene) drawFruit(c *engine.Canvas, geo mazeGeo) {
	if !p.fruitActive {
		return
	}
	x := geo.originX + p.fruitTile[0]*geo.tile + geo.tile/2
	y := geo.originY + p.fruitTile[1]*geo.tile + geo.tile/2
	radius := geo.tile - 1
	if radius < 1 {
		radius = 1
	}
	// Simple cherry-coloured blob.
	c.FillCircle(x, y, radius, engine.Color{R: 255, G: 50, B: 50, A: 255})
	c.Set(x, y-radius-1, engine.Color{R: 0, G: 255, B: 0, A: 255})
}

// drawHUD paints the score / hi-score line at the top and the lives /
// level indicator at the bottom.
func (p *playScene) drawHUD(c *engine.Canvas, geo mazeGeo) {
	score := fmt.Sprintf("SCORE  %06d", p.score)
	hi := fmt.Sprintf("HI  %06d", p.hiScore)
	level := fmt.Sprintf("LEVEL %d", p.level)
	c.Print(1, 0, score, engine.Yellow)
	c.Print((c.Cols()-len(hi))/2, 0, hi, engine.White)
	c.Print(c.Cols()-len(level)-1, 0, level, engine.Cyan)

	// Lives row — small Pac-Man icons.
	rowY := c.Height() - geo.hudBot + 1
	for i := 0; i < p.lives-1; i++ {
		x := 2 + i*(geo.tile*2+1) + geo.tile
		c.FillCircle(x, rowY, geo.tile-1, engine.Color{R: 255, G: 240, B: 0, A: 255})
	}

	// Right-side hint.
	hint := "ESC QUIT"
	c.Print(c.Cols()-len(hint)-1, c.Rows()-1, hint, engine.Gray)
}

// drawOverlay paints state-specific banners (READY, GAME OVER, …).
func (p *playScene) drawOverlay(c *engine.Canvas, geo mazeGeo) {
	switch p.state {
	case psReady:
		drawCentreText(c, geo, "READY!", engine.Yellow)
	case psDying:
		// Pac-Man's death animation is the visible feedback; no banner.
	case psLevelClear:
		// The flashing maze itself is the celebration. Keep score row
		// clean above it.
	case psGameOver:
		drawCentreText(c, geo, "GAME OVER", engine.Color{R: 255, G: 50, B: 50, A: 255})
		hint := "ENTER QUIT"
		c.Print((c.Cols()-len(hint))/2, c.Rows()/2+4, hint, engine.White)
	}
}

func drawCentreText(c *engine.Canvas, _ mazeGeo, text string, col engine.Color) {
	tw := engine.TextWidth(text)
	tx := (c.Width() - tw) / 2
	ty := c.Height()/2 - engine.FontHeight/2
	c.DrawText(tx, ty, text, col)
}
