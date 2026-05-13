package frogger

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Speeds are in pixels per second; intervals in seconds.
// Numbers come from the 1981 cabinet's "feel" — hops are short, the time
// bar lasts ~30s per attempt, road lanes get progressively faster, the
// river has long passages that demand log-hopping. The level scaler boosts
// every speed by +10 % per wave (capped) so the difficulty curves up.
const (
	// Frog hop animation.
	hopDuration  = 0.135 // seconds per hop; faster than walking but readable
	hopArcHeight = 2.0   // peak parabolic lift mid-hop (px)

	// Time bar — full attempt is 30 seconds of real time.
	timeBarDuration = 30.0

	// Scoring (matches arcade).
	pointsPerRow    = 10   // for each new highest row reached this life
	pointsPerHome   = 50   // for delivering a frog to a home slot
	pointsLady      = 200  // for delivering the lady frog
	pointsFly       = 200  // for landing on a slot containing a fly
	pointsAllHomes  = 1000 // for completing all five homes (clearing the wave)
	timeBonusPer05s = 10   // per remaining 0.5 s of the time bar
	extraLifeAt     = 20000

	// State durations.
	deathDuration     = 1.4
	waveClearDuration = 3.0
	preStageDuration  = 1.6
	popupLifetime     = 1.0

	// Bonus entity timing.
	flyMinDelay  = 6.0
	flyMaxDelay  = 14.0
	flyLifetime  = 7.0
	ladyMinDelay = 8.0
	ladyMaxDelay = 18.0
	crocMinDelay = 11.0
	crocMaxDelay = 25.0
	crocLifetime = 6.5

	// Turtle dive phases (fractions of diveCycle). The remaining
	// fraction (1 - surface - warn) is the fully-submerged phase, where
	// the frog drowns if standing on a turtle in this lane.
	diveSurfaceFrac = 0.55
	diveWarnFrac    = 0.15

	// Wave speed scaling.
	waveSpeedup = 0.10 // +10 % per wave above level 1
	maxWaveMult = 2.4
)

// -- High-level state machine ------------------------------------------

type playState int

const (
	psPreStage  playState = iota // brief "GET READY" banner
	psPlaying                    // active gameplay
	psDying                      // showing death animation
	psWaveClear                  // showing wave-clear flash
	psGameOver                   // post-loss, waiting for input
)

// -- Frog --------------------------------------------------------------

type frogState int

const (
	fsAlive frogState = iota
	fsHopping
	fsSplat  // road kill
	fsSplash // drowned in river
	fsHome   // briefly locked in home before respawn
)

type hopDir int

const (
	hopNone hopDir = iota
	hopUp
	hopDown
	hopLeft
	hopRight
)

type frog struct {
	// Logical row index (0=home, 12=start). Always the row the frog is
	// currently AT (or hopping TO if state==fsHopping).
	row frogRow

	// Pixel position of the sprite's top-left corner. While idle, y is
	// pinned to rowCenterY(row). While riding a log/turtle, x advances
	// with the lane.
	x, y float64

	state  frogState
	facing hopDir

	// Hop interpolation state.
	hopT             float64
	hopSrcX, hopSrcY float64
	hopDstX, hopDstY float64

	// Death animation timer (seconds remaining).
	dieT float64

	// Highest row (smallest frogRow index) reached this trip. Drives the
	// +10 per row scoring rule.
	highestRow frogRow

	// True if the lady frog has been picked up on the current trip. Reset
	// to false on every spawn.
	carryingLady bool
}

// -- Lane state --------------------------------------------------------
//
// Lane entities are not stored individually. Instead, each lane has a
// scalar `base` offset, and entity i lives at x = base + i*spec.entitySpan
// (modulo the cycle period). Each frame `base` advances by speed*dt*dir.
// Render iterates over the small range of i's that produce a visible x.
// This keeps state tiny and the wrap math trivial.

type laneState struct {
	spec laneSpec
	base float64

	// Turtle-dive state for turtle lanes (otherwise unused).
	diveT float64
}

// laneCyclePeriod is the wrap distance for a lane in pixels.
func (l laneState) period() float64 {
	return float64(l.spec.entitySpan * l.spec.count)
}

// visibleEntityIndices returns the range of entity indices whose visible
// position overlaps the playfield [0, playfieldW). Indices are unbounded
// (they can be negative or huge), but each maps to a unique on-belt slot
// via i*entitySpan + base. Takes playfieldW because lane state itself
// doesn't carry it — the same lane can be queried against any width.
func (l laneState) visibleEntityIndices(playfieldW int) (lo, hi int) {
	period := l.period()
	if period <= 0 {
		return 0, -1
	}
	// Normalise base into [0, period).
	b := math.Mod(l.base, period)
	if b < 0 {
		b += period
	}
	span := float64(l.spec.entitySpan)
	w := float64(l.spec.entityW)
	loF := (-w - b) / span
	hiF := (float64(playfieldW) - b) / span
	return int(math.Ceil(loF)), int(math.Floor(hiF))
}

// entityX returns the visible pixel-x of entity index i. The returned
// value is the entity's leftmost pixel column in playfield-local
// coordinates and may be off-screen on either side (the renderer clips).
func (l laneState) entityX(i int) float64 {
	period := l.period()
	b := math.Mod(l.base, period)
	if b < 0 {
		b += period
	}
	return b + float64(i*l.spec.entitySpan)
}

// turtleDivePhase reports a turtle lane's current visibility phase based
// on its diveT cursor.
//
//	0 = surfaced (carries frog, sprite=turtleSurface)
//	1 = warning blink (carries frog, sprite alternates with shell-only)
//	2 = submerged (frog drowns, sprite invisible)
func (l laneState) turtleDivePhase() int {
	if l.spec.diveCycle <= 0 {
		return 0
	}
	cycle := l.spec.diveCycle
	t := math.Mod(l.diveT, cycle)
	if t < diveSurfaceFrac*cycle {
		return 0
	}
	if t < (diveSurfaceFrac+diveWarnFrac)*cycle {
		return 1
	}
	return 2
}

// -- Home state --------------------------------------------------------

type homeState struct {
	occupied bool
	hadLady  bool // true if the delivering frog was carrying the lady
}

// -- Popup -------------------------------------------------------------

type scorePopup struct {
	x, y float64
	text string
	col  engine.Color
	age  float64
}

// -- playScene ---------------------------------------------------------

type playScene struct {
	e          *engine.Engine
	w, h       int
	offX, offY int

	// Adaptive playfield width — sized to the canvas at construction so
	// large terminals get more horizontal room to weave (more entities
	// per lane, more hop columns) without scaling the sprites.
	playfieldW int

	frog  frog
	lanes []laneState
	homes [numHomes]homeState

	// Lady frog: rides a specific entity on lane index `ladyLaneIdx`,
	// at slot index `ladyEntityIdx`. Set both to -1 when she's absent.
	ladyLaneIdx   int
	ladyEntityIdx int
	ladyTimer     float64 // seconds until next spawn attempt (or until current lady wraps off)

	// Fly bonus in a home slot.
	flySlot  int     // -1 if none
	flyTimer float64 // seconds until next spawn / until current fly leaves

	// Crocodile in a home slot.
	crocSlot  int
	crocTimer float64

	state  playState
	stateT float64

	timeLeft      float64
	score         int
	hiScore       int
	lives         int
	level         int
	nextExtraLife int

	popups []scorePopup
	rng    *rand.Rand

	wantQuit bool

	// One-frame queued hop direction from input.
	hopQueued hopDir
}

func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:             e,
		w:             c.Width(),
		h:             c.Height(),
		hiScore:       hiScore,
		lives:         3,
		level:         1,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
		ladyLaneIdx:   -1,
		ladyEntityIdx: -1,
		flySlot:       -1,
		crocSlot:      -1,
		nextExtraLife: extraLifeAt,
	}
	p.playfieldW = computePlayfieldW(p.w)
	p.offX = (p.w - p.playfieldW) / 2
	p.offY = (p.h - playfieldH) / 2
	if p.offX < 0 {
		p.offX = 0
	}
	if p.offY < 0 {
		p.offY = 0
	}
	p.startWave(false)
	return p
}

// startWave (re)initialises lanes and bonus timers for the current level.
// Pass clearHomes=true to also wipe the home slots (a new wave); pass
// false to keep them (used after a life is lost mid-wave).
func (p *playScene) startWave(clearHomes bool) {
	waveScale := 1.0 + waveSpeedup*float64(p.level-1)
	if waveScale > maxWaveMult {
		waveScale = maxWaveMult
	}
	specs := buildLaneSpecs(p.playfieldW, waveScale)
	p.lanes = make([]laneState, len(specs))
	for i, s := range specs {
		// Stagger each lane's initial offset so adjacent lanes don't line
		// up — gives the playfield more visual interest at t=0.
		p.lanes[i] = laneState{
			spec: s,
			base: float64((i*17 + p.level*5) % (s.entitySpan * s.count)),
			// Stagger dive cycles so turtle lanes don't pulse in lock-step.
			diveT: float64(i) * 1.7,
		}
	}
	if clearHomes {
		for i := range p.homes {
			p.homes[i] = homeState{}
		}
	}
	p.resetBonusTimers()
	p.spawnFrog()
	p.state = psPreStage
	p.stateT = 0
}

func (p *playScene) resetBonusTimers() {
	p.flySlot = -1
	p.flyTimer = randRange(p.rng, flyMinDelay, flyMaxDelay)
	p.crocSlot = -1
	p.crocTimer = randRange(p.rng, crocMinDelay, crocMaxDelay)
	p.ladyLaneIdx = -1
	p.ladyEntityIdx = -1
	p.ladyTimer = randRange(p.rng, ladyMinDelay, ladyMaxDelay)
}

func (p *playScene) spawnFrog() {
	p.frog = frog{
		row:        rowStart,
		x:          float64(p.playfieldW/2 - frogW/2),
		y:          float64(rowCenterY(rowStart)),
		state:      fsAlive,
		facing:     hopUp,
		highestRow: rowStart,
	}
	p.timeLeft = timeBarDuration
	// A key press that landed in the same frame as the death (or during
	// the death animation, even though queueHop drops it) could leave
	// hopQueued set. Wipe it so the respawning frog doesn't immediately
	// take a hop the player isn't currently asking for.
	p.hopQueued = hopNone
}

// -- Update ------------------------------------------------------------

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s

	// Lanes advance whether the player is alive or watching a death;
	// it's odd if traffic freezes mid-cutscene.
	p.advanceLanes(s)
	p.updateBonusEntities(s)
	p.updatePopups(s)

	switch p.state {
	case psPreStage:
		if p.stateT >= preStageDuration {
			p.state = psPlaying
			p.stateT = 0
		}
	case psPlaying:
		p.advanceTimeBar(s)
		p.updateFrog(s)
		// Resolve car/frog collisions every tick — both move during the
		// frame, so the order has to be "after both have advanced".
		p.resolveCollisions()
	case psDying:
		p.frog.dieT -= s
		if p.frog.dieT <= 0 {
			p.respawnOrGameOver()
		}
	case psWaveClear:
		if p.stateT >= waveClearDuration {
			p.level++
			p.startWave(true)
		}
	case psGameOver:
		// Wait for key (handled in handleInput).
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	if p.score >= p.nextExtraLife {
		p.lives++
		p.nextExtraLife += extraLifeAt
		p.spawnPopup(float64(p.playfieldW/2), float64(rowCenterY(rowMedian)),
			"EXTRA FROG!", flashColor)
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
		case engine.KeyEsc:
			p.wantQuit = true
		case engine.KeyEnter:
			if p.state == psGameOver {
				p.wantQuit = true
			}
		case engine.KeyUp:
			p.queueHop(hopUp)
		case engine.KeyDown:
			p.queueHop(hopDown)
		case engine.KeyLeft:
			p.queueHop(hopLeft)
		case engine.KeyRight:
			p.queueHop(hopRight)
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				p.wantQuit = true
			case 'w', 'W':
				p.queueHop(hopUp)
			case 's', 'S':
				p.queueHop(hopDown)
			case 'a', 'A':
				p.queueHop(hopLeft)
			case 'd', 'D':
				p.queueHop(hopRight)
			case ' ':
				if p.state == psGameOver {
					p.wantQuit = true
				}
			case 'r', 'R':
				if p.state == psGameOver {
					p.wantQuit = true
				}
			}
		}
	}
}

func (p *playScene) queueHop(d hopDir) {
	if p.state != psPlaying {
		return
	}
	if p.frog.state != fsAlive {
		return
	}
	// Latest direction wins if multiple keys arrive in the same frame.
	p.hopQueued = d
}

// -- Time bar ----------------------------------------------------------

func (p *playScene) advanceTimeBar(s float64) {
	p.timeLeft -= s
	if p.timeLeft <= 0 {
		p.timeLeft = 0
		p.killFrog(false)
	}
}

// -- Lanes ------------------------------------------------------------

func (p *playScene) advanceLanes(s float64) {
	for i := range p.lanes {
		ln := &p.lanes[i]
		ln.base += float64(ln.spec.dir) * ln.spec.speed * s
		period := ln.period()
		// Normalise base into [0, period) — keeps the maths bounded.
		if period > 0 {
			ln.base = math.Mod(ln.base, period)
			if ln.base < 0 {
				ln.base += period
			}
		}
		if ln.spec.diveCycle > 0 {
			ln.diveT += s
		}
	}
}

// -- Bonus entities ----------------------------------------------------

func (p *playScene) updateBonusEntities(s float64) {
	// FLY.
	p.flyTimer -= s
	if p.flySlot < 0 {
		if p.flyTimer <= 0 {
			// Spawn in a random unoccupied, croc-free home.
			candidates := p.openHomeSlots()
			if len(candidates) > 0 {
				p.flySlot = candidates[p.rng.Intn(len(candidates))]
				p.flyTimer = flyLifetime
			} else {
				p.flyTimer = randRange(p.rng, flyMinDelay, flyMaxDelay)
			}
		}
	} else if p.flyTimer <= 0 {
		p.flySlot = -1
		p.flyTimer = randRange(p.rng, flyMinDelay, flyMaxDelay)
	}

	// CROC.
	p.crocTimer -= s
	if p.crocSlot < 0 {
		if p.crocTimer <= 0 {
			candidates := p.openHomeSlots()
			// Don't put a croc on top of a fly.
			pruned := candidates[:0]
			for _, c := range candidates {
				if c != p.flySlot {
					pruned = append(pruned, c)
				}
			}
			if len(pruned) > 0 {
				p.crocSlot = pruned[p.rng.Intn(len(pruned))]
				p.crocTimer = crocLifetime
			} else {
				p.crocTimer = randRange(p.rng, crocMinDelay, crocMaxDelay)
			}
		}
	} else if p.crocTimer <= 0 {
		p.crocSlot = -1
		p.crocTimer = randRange(p.rng, crocMinDelay, crocMaxDelay)
	}

	// LADY FROG.
	p.ladyTimer -= s
	if p.ladyLaneIdx < 0 {
		if p.ladyTimer <= 0 {
			// Find rowRiverL2 lane (the mid-length log lane) and pick a log.
			laneIdx := laneSpecForRow(rowRiverL2)
			if laneIdx >= 0 {
				ln := p.lanes[laneIdx]
				// Pick a visible entity index to ride; clamp to a sensible
				// range so she always starts within view.
				lo, hi := ln.visibleEntityIndices(p.playfieldW)
				if hi >= lo {
					choice := lo + p.rng.Intn(hi-lo+1)
					p.ladyLaneIdx = laneIdx
					p.ladyEntityIdx = choice
				}
			}
			p.ladyTimer = randRange(p.rng, ladyMinDelay, ladyMaxDelay)
		}
	} else {
		// Despawn the lady when her log scrolls fully off-screen.
		ln := p.lanes[p.ladyLaneIdx]
		ex := ln.entityX(p.ladyEntityIdx)
		if ex > float64(p.playfieldW)+10 || ex < -float64(ln.spec.entityW)-10 {
			p.ladyLaneIdx = -1
			p.ladyEntityIdx = -1
		}
	}
}

func (p *playScene) openHomeSlots() []int {
	out := make([]int, 0, numHomes)
	for i := 0; i < numHomes; i++ {
		if !p.homes[i].occupied {
			out = append(out, i)
		}
	}
	return out
}

// -- Frog logic --------------------------------------------------------

func (p *playScene) updateFrog(s float64) {
	switch p.frog.state {
	case fsAlive:
		p.frogIdle(s)
	case fsHopping:
		p.frogHop(s)
	}
}

func (p *playScene) frogIdle(s float64) {
	// While idle on a river row, the frog drifts with whatever it's
	// riding. Check ride attachment every frame; drop into water (death)
	// if the platform vanished or the frog drifted off-screen.
	if rowKind(p.frog.row) == laneRiverWater {
		laneIdx := laneSpecForRow(p.frog.row)
		ln := p.lanes[laneIdx]
		ride, _, alive := p.frogRideStatus(ln)
		if !alive {
			// Sit still — already handled (drowned earlier).
			return
		}
		if ride {
			delta := float64(ln.spec.dir) * ln.spec.speed * s
			p.frog.x += delta
		} else {
			// Lost the platform (turtle dived, or hopped between platforms
			// onto open water).
			p.killFrog(true)
			return
		}
		// Drifted off-screen edge?
		if p.frog.x < 0 || p.frog.x+float64(frogW) > float64(p.playfieldW) {
			p.killFrog(true)
			return
		}
	}

	// Initiate queued hop.
	if p.hopQueued != hopNone {
		p.startHop(p.hopQueued)
		p.hopQueued = hopNone
	}
}

// frogRideStatus inspects lane ln and decides whether the frog (assumed
// to be idle on the corresponding row) is on a valid rider. The second
// return value is the entity index ridden; the third is "false" only if
// the frog was already declared dead this frame and shouldn't be acted
// on further (currently always true — reserved).
func (p *playScene) frogRideStatus(ln laneState) (bool, int, bool) {
	// Only logs and surfaced turtles carry the frog. Submerged turtles
	// do not.
	if !(ln.spec.isLog || ln.spec.isTurtle) {
		return false, 0, true
	}
	if ln.spec.isTurtle && ln.turtleDivePhase() == 2 {
		return false, 0, true
	}
	frogX0 := p.frog.x
	frogX1 := p.frog.x + float64(frogW)
	lo, hi := ln.visibleEntityIndices(p.playfieldW)
	for i := lo; i <= hi; i++ {
		ex := ln.entityX(i)
		ex1 := ex + float64(ln.spec.entityW)
		// Frog must be fully on the platform (some tolerance for partial
		// overlap from a hop landing).
		if frogX0+2 < ex1 && frogX1-2 > ex {
			return true, i, true
		}
	}
	return false, 0, true
}

func (p *playScene) startHop(d hopDir) {
	srcX := p.frog.x
	srcY := p.frog.y
	dstX := srcX
	dstY := srcY
	targetRow := p.frog.row

	switch d {
	case hopUp:
		if p.frog.row == rowHome {
			return // already delivered (shouldn't happen, defensive)
		}
		targetRow = p.frog.row - 1
		dstY = float64(rowCenterY(targetRow))
	case hopDown:
		if p.frog.row >= rowStart {
			return // can't hop below start
		}
		targetRow = p.frog.row + 1
		dstY = float64(rowCenterY(targetRow))
	case hopLeft:
		dstX = srcX - float64(colStep)
		if dstX < 0 {
			dstX = 0
		}
	case hopRight:
		dstX = srcX + float64(colStep)
		if dstX > float64(p.playfieldW-frogW) {
			dstX = float64(p.playfieldW - frogW)
		}
	default:
		return
	}
	// No-op hop (already at edge)?
	if dstX == srcX && dstY == srcY {
		return
	}

	p.frog.state = fsHopping
	p.frog.facing = d
	p.frog.hopT = 0
	p.frog.hopSrcX, p.frog.hopSrcY = srcX, srcY
	p.frog.hopDstX, p.frog.hopDstY = dstX, dstY
	// Update row eagerly: target row is what we're heading toward, and
	// frogIdle's ride logic only re-engages on landing. row stays as the
	// PREVIOUS row until landing — re-set to targetRow on landing.
	p.frog.row = targetRow
}

func (p *playScene) frogHop(s float64) {
	p.frog.hopT += s
	t := p.frog.hopT / hopDuration
	if t >= 1 {
		// Land.
		p.frog.x = p.frog.hopDstX
		p.frog.y = p.frog.hopDstY
		p.frog.state = fsAlive
		p.landed()
		return
	}
	// Linear interpolation with a small parabolic vertical lift to give
	// the hop visible loft.
	p.frog.x = lerp(p.frog.hopSrcX, p.frog.hopDstX, t)
	yLin := lerp(p.frog.hopSrcY, p.frog.hopDstY, t)
	arc := hopArcHeight * math.Sin(math.Pi*t)
	p.frog.y = yLin - arc
}

// landed runs all the "what did I just hop onto?" logic. Distinguished
// from frogIdle because landing has one-shot consequences (scoring a new
// row, delivering home, dying on a hedge/croc) whereas idle just sustains
// the current row's invariants frame-to-frame.
func (p *playScene) landed() {
	// New row scoring.
	if p.frog.row < p.frog.highestRow {
		gained := int(p.frog.highestRow-p.frog.row) * pointsPerRow
		p.frog.highestRow = p.frog.row
		p.score += gained
		p.spawnPopup(p.frog.x+float64(frogW)/2, p.frog.y, fmt.Sprintf("+%d", gained),
			scoreColor)
	}

	switch rowKind(p.frog.row) {
	case laneMedian, laneStart:
		// Safe — no further checks.

	case laneRoad:
		// Road collision is resolved by resolveCollisions after the
		// hop completes; we don't pre-check here because the frog might
		// land slightly before/after a car arrives.

	case laneRiverWater:
		// Must be on a rider, AND if on lady-frog log, pick her up.
		laneIdx := laneSpecForRow(p.frog.row)
		ln := p.lanes[laneIdx]
		ride, entIdx, _ := p.frogRideStatus(ln)
		if !ride {
			p.killFrog(true)
			return
		}
		// Lady frog pickup.
		if !p.frog.carryingLady && laneIdx == p.ladyLaneIdx && entIdx == p.ladyEntityIdx {
			p.frog.carryingLady = true
			p.ladyLaneIdx = -1
			p.ladyEntityIdx = -1
			p.ladyTimer = randRange(p.rng, ladyMinDelay, ladyMaxDelay)
		}

	case laneHomeRow:
		p.tryEnterHome()
	}
}

// tryEnterHome resolves a hop into the home strip. The frog can:
//
//   - Land cleanly in an open slot (50 + time bonus, +200 if lady frog, +200
//     if fly). Frog respawns at start.
//   - Land on a slot containing a crocodile. Death.
//   - Miss every slot (lands on a hedge/divider). Death.
//   - Land on an already-occupied slot. Death.
func (p *playScene) tryEnterHome() {
	cx := p.frog.x + float64(frogW)/2
	slot := -1
	for i := 0; i < numHomes; i++ {
		x0, x1 := homeSlotX(i, p.playfieldW)
		if cx >= float64(x0) && cx < float64(x1) {
			slot = i
			break
		}
	}
	if slot < 0 {
		// Missed the opening, hit the hedge.
		p.killFrog(false)
		return
	}
	if p.homes[slot].occupied {
		// Already claimed.
		p.killFrog(false)
		return
	}
	if p.crocSlot == slot {
		// Death by croc.
		p.killFrog(false)
		return
	}
	// Successful delivery.
	p.homes[slot].occupied = true
	p.homes[slot].hadLady = p.frog.carryingLady
	p.score += pointsPerHome
	bonus := int(p.timeLeft*2) * timeBonusPer05s
	p.score += bonus
	if p.frog.carryingLady {
		p.score += pointsLady
	}
	if p.flySlot == slot {
		p.score += pointsFly
		p.flySlot = -1
		p.flyTimer = randRange(p.rng, flyMinDelay, flyMaxDelay)
	}
	// Popup.
	msg := fmt.Sprintf("+%d", pointsPerHome+bonus)
	if p.frog.carryingLady {
		msg += " LADY!"
	}
	p.spawnPopup(p.frog.x+float64(frogW)/2, p.frog.y, msg, flashColor)

	if p.allHomesFilled() {
		p.score += pointsAllHomes
		p.state = psWaveClear
		p.stateT = 0
		// Hide the just-delivered frog — drawHomes will draw the occupant
		// from the home slot instead, otherwise we'd double-render.
		p.frog.state = fsHome
		return
	}
	// Otherwise — respawn frog at start to continue the wave.
	p.spawnFrog()
}

func (p *playScene) allHomesFilled() bool {
	for _, h := range p.homes {
		if !h.occupied {
			return false
		}
	}
	return true
}

// -- Per-frame collisions (road) --------------------------------------
//
// The frog's "active" collision check runs every frame while idle or
// hopping on the road — a car can run the frog over while the hop is
// still in progress. River-collision is handled in frogRideStatus + the
// drift-off-screen check in frogIdle; home-collision in tryEnterHome.

func (p *playScene) resolveCollisions() {
	if p.frog.state != fsAlive && p.frog.state != fsHopping {
		return
	}
	if rowKind(p.frog.row) != laneRoad {
		return
	}
	laneIdx := laneSpecForRow(p.frog.row)
	if laneIdx < 0 {
		return
	}
	ln := p.lanes[laneIdx]
	fr := p.frogRect()
	lo, hi := ln.visibleEntityIndices(p.playfieldW)
	for j := lo; j <= hi; j++ {
		ex := ln.entityX(j)
		er := rect{
			x0: int(ex),
			y0: ln.spec.yTop,
			x1: int(ex) + ln.spec.entityW,
			y1: ln.spec.yTop + laneH,
		}
		if er.overlaps(fr) {
			p.killFrog(false)
			return
		}
	}
}

// frogRect returns the frog's collision rectangle pinned to its LOGICAL
// row's y, not its interpolated visual y. The hop animation lifts the
// frog up to hopArcHeight px above its row centre for visual loft; if we
// used p.frog.y directly, that lift would spill the collision rect into
// the lane above and let cars there hit the frog mid-sideways-hop.
// startHop updates p.frog.row to the destination eagerly, so during an
// up/down hop the frog is logically "already in" the destination lane.
func (p *playScene) frogRect() rect {
	rowY := rowCenterY(p.frog.row)
	return rect{
		x0: int(p.frog.x),
		y0: rowY,
		x1: int(p.frog.x) + frogW,
		y1: rowY + frogH,
	}
}

type rect struct{ x0, y0, x1, y1 int }

func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && r.x1 > o.x0 && r.y0 < o.y1 && r.y1 > o.y0
}

// -- Death + respawn ---------------------------------------------------

func (p *playScene) killFrog(drowned bool) {
	if p.frog.state == fsSplat || p.frog.state == fsSplash {
		return
	}
	if drowned {
		p.frog.state = fsSplash
	} else {
		p.frog.state = fsSplat
	}
	p.frog.dieT = deathDuration
	p.lives--
	p.state = psDying
	p.stateT = 0
}

func (p *playScene) respawnOrGameOver() {
	if p.lives <= 0 {
		p.state = psGameOver
		p.stateT = 0
		return
	}
	p.spawnFrog()
	p.state = psPlaying
	p.stateT = 0
}

// -- Popups ------------------------------------------------------------

func (p *playScene) spawnPopup(x, y float64, text string, col engine.Color) {
	p.popups = append(p.popups, scorePopup{x: x, y: y, text: text, col: col})
}

func (p *playScene) updatePopups(s float64) {
	kept := p.popups[:0]
	for _, pop := range p.popups {
		pop.age += s
		pop.y -= 6 * s
		if pop.age < popupLifetime {
			kept = append(kept, pop)
		}
	}
	p.popups = kept
}

// -- Helpers -----------------------------------------------------------

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func randRange(rng *rand.Rand, lo, hi float64) float64 {
	return lo + rng.Float64()*(hi-lo)
}

