package donkeykong

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Speeds are in canvas pixels per second; intervals in
// seconds. The numbers are picked so the action reads as "Donkey Kong":
// Mario walks slowly and deliberately, barrels keep pace with him, and
// the bonus countdown puts a clock on every climb.
const (
	// Mario.
	marioWidth         = 5
	marioHeight        = 6
	marioWalkSpeed     = 22.0
	marioWalkAccel     = 120.0 // ramp on ground: reach top speed in ~0.18 s
	marioWalkFriction  = 90.0  // decay when no input: stops in ~0.25 s
	marioAirAccel      = 70.0  // less control mid-jump than on the girder
	marioClimbSpeed    = 18.0
	marioJumpVy        = -42.0 // higher initial velocity → ~9 px apex, ~0.84 s airtime
	gravity            = 100.0
	marioWalkFrame     = 0.14

	// Barrels.
	barrelW            = 5
	barrelH            = 3
	barrelFallW        = 3
	barrelFallH        = 5
	barrelRollSpeed    = 14.0
	barrelLadderChance = 0.22 // per-encounter probability to descend
	barrelGravity      = 100.0
	barrelRollFrame    = 0.16

	// DK throwing cadence.
	dkThrowMin = 2.4
	dkThrowMax = 4.6
	dkAnimDur  = 0.55 // hold the throw frame for this long

	// Fireball.
	fireballW         = 5
	fireballH         = 5
	fireballSpeed     = 10.0
	fireballSpawnT    = 16.0 // first fireball appears after this much play time
	fireballRespawnT  = 24.0 // gap between subsequent fireball spawns
	fireballFrameRate = 0.18
	fireballMax       = 2

	// Hammer.
	hammerActiveDur  = 8.5
	hammerSwingFrame = 0.10
	hammerKillScore  = 300

	// Bonus countdown.
	bonusStart    = 5000
	bonusTickRate = 0.45 // every Nth second, lose 100 points
	bonusTickAmt  = 100

	// Death + clear.
	deathDuration      = 1.7
	stageClearDuration = 2.6

	// Scoring.
	pointsBarrelJump = 100

	// Visual.
	flameFrameRate = 0.20

	// Wave difficulty scale: each wave throws barrels faster.
	waveSpeedup = 0.90
)

// -- Sub-state machines -------------------------------------------------

type marioState int

const (
	msWalking marioState = iota
	msClimbing
	msJumping
	msDying
)

type playState int

const (
	psPreStage playState = iota // brief "HOW HIGH CAN YOU GET?" banner
	psPlaying
	psStageClear
	psGameOver
)

// -- Entities -----------------------------------------------------------

type marioEntity struct {
	x, y      float64 // x = sprite-left; y = feet row (one above the supporting girder pixel)
	vx        float64 // horizontal momentum (px/s)
	vy        float64
	facing    int // -1 / +1
	state     marioState
	girderIdx int // valid while walking or jumping/landing predictions

	// Climb state.
	climbing       ladder
	climbGap       int     // gap index in lvl.ladders
	climbBoundsTop float64 // highest feet-y reachable (climbTopY)
	climbDescend   bool    // true if we entered the ladder from the top

	// Animation.
	walkAnimT float64
	walkFrame int

	// Death animation timer.
	dyingT float64

	// Jump bookkeeping for "points for jumping over barrels".
	jumpedBarrels map[int]bool
	jumpStartFeet float64

	hammerSwingT float64
	hammerHigh   bool // alternates each swing frame
}

type barrel struct {
	id        int
	x, y      float64
	vx, vy    float64
	state     int     // 0=rolling, 1=falling, 2=descending ladder
	girderIdx int     // valid when state==0
	descLad   ladder  // valid when state==2
	descGap   int     // valid when state==2
	frameT    float64
	frame     int
	// Tracks which ladder x-ranges this barrel has already "rolled" over —
	// resets each time it lands on a new girder.
	ladderSeen map[int]bool
}

type fireballEntity struct {
	x, y      float64
	vx        float64
	state     int // 0=walking, 1=climbing
	girderIdx int
	climbing  ladder
	climbGap  int
	climbDir  int // -1 (up) / +1 (down)
	frameT    float64
	frame     int
}

type hammerPickupEntity struct {
	x, y      int
	picked    bool
	girderIdx int // which girder it sits on (for drawing)
}

type scorePopup struct {
	x, y float64
	text string
	col  engine.Color
	age  float64
}

// -- playScene ----------------------------------------------------------

type playScene struct {
	e    *engine.Engine
	w, h int
	lvl  *level

	mario     marioEntity
	barrels   []*barrel
	fireballs []*fireballEntity
	hammer    *hammerPickupEntity

	// Hammer wield state — set when Mario picks up the hammer.
	hammerActive bool
	hammerEndT   float64

	state  playState
	stateT float64

	timeT          float64 // total play-state time
	nextBarrelT    float64
	dkThrowAnimT   float64 // counts down while DK is mid-throw
	nextFireballT  float64
	fireballNumOut int

	score   int
	hiScore int
	lives   int
	bonus   int
	bonusT  float64

	wave        int
	barrelIDSeq int

	popups []scorePopup

	rng *rand.Rand

	// Quit signal.
	wantQuit bool

	// Input edge tracking — jump fires on the space-press event, not on hold.
	jumpQueued bool
}

func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		hiScore: hiScore,
		lives:   3,
		wave:    1,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.lvl = buildLevel(p.w, p.h)
	p.startStage()
	return p
}

// startStage (re)initialises everything that resets between lives or
// between waves: Mario position, barrels, fireballs, hammer, bonus timer.
// Does NOT reset score / lives / hi-score / wave.
func (p *playScene) startStage() {
	p.mario = marioEntity{
		girderIdx: p.lvl.marioStartGIdx,
		facing:    +1,
		state:     msWalking,
	}
	g := p.lvl.girders[p.mario.girderIdx]
	p.mario.x = float64(p.lvl.marioStartX)
	centerX := int(p.mario.x) + marioWidth/2
	p.mario.y = float64(g.yAt(centerX) - 1)

	p.barrels = nil
	p.fireballs = nil
	p.popups = nil
	p.hammer = &hammerPickupEntity{
		// Place hammer on G2 (slanted, middle of stage) near a column the
		// player can reach by climbing to G3, then up to G2 via the right
		// ladder. Sits ON the girder.
		girderIdx: 2,
	}
	hg := p.lvl.girders[p.hammer.girderIdx]
	hx := p.w/2 - 2
	hy := hg.yAt(hx + 1)
	p.hammer.x = hx
	p.hammer.y = hy - hammerPickup.height()

	p.hammerActive = false
	p.hammerEndT = 0

	p.bonus = bonusStart
	p.bonusT = 0
	p.timeT = 0
	p.dkThrowAnimT = 0
	p.nextBarrelT = 2.0 // grace period before first barrel
	p.nextFireballT = fireballSpawnT
	p.fireballNumOut = 0
	p.barrelIDSeq = 0

	p.state = psPreStage
	p.stateT = 0
}

// waveScale shrinks barrel spawn interval as waves advance.
func (p *playScene) waveScale() float64 {
	s := 1.0
	for i := 1; i < p.wave; i++ {
		s *= waveSpeedup
	}
	return s
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
	case psPreStage:
		// Hold the banner for ~2 s, then start the action.
		if p.stateT >= 2.0 {
			p.state = psPlaying
			p.stateT = 0
		}
	case psPlaying:
		p.timeT += s
		p.updateBonus(s)
		p.updateDK(s)
		p.updateMario(s)
		p.updateBarrels(s)
		p.updateFireballs(s)
		p.updateHammer(s)
		p.updatePopups(s)
		p.resolveCollisions()

		// Win condition: Mario's feet are on or above the top (DK) girder.
		if p.mario.state == msWalking && p.mario.girderIdx == p.lvl.marioGoalGIdx {
			p.score += p.bonus
			p.bonus = 0
			p.state = psStageClear
			p.stateT = 0
		}
	case psStageClear:
		p.updateDK(s) // keep DK animated even on the clear screen
		if p.stateT >= stageClearDuration {
			p.wave++
			p.startStage()
		}
	case psGameOver:
		// Wait for keypress (handled in handleInput).
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
		case engine.KeyEsc:
			p.wantQuit = true
		case engine.KeyEnter:
			if p.state == psGameOver {
				// Player acknowledges — fall back to the title via wantQuit.
				p.wantQuit = true
			}
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				p.wantQuit = true
			case ' ':
				// Only fire one jump per press, and only while playing.
				if p.state == psPlaying {
					p.jumpQueued = true
				}
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

// updateBonus ticks the on-stage bonus counter down.
func (p *playScene) updateBonus(s float64) {
	if p.bonus <= 0 {
		return
	}
	p.bonusT += s
	if p.bonusT >= bonusTickRate {
		p.bonusT -= bonusTickRate
		p.bonus -= bonusTickAmt
		if p.bonus < 0 {
			p.bonus = 0
		}
	}
}

// updateDK ticks DK's throw cooldown and emits new barrels when due.
func (p *playScene) updateDK(s float64) {
	if p.dkThrowAnimT > 0 {
		p.dkThrowAnimT -= s
	}
	if p.state != psPlaying {
		return
	}
	p.nextBarrelT -= s
	if p.nextBarrelT <= 0 {
		p.throwBarrel()
		// Schedule next throw, accelerated by wave.
		iv := dkThrowMin + p.rng.Float64()*(dkThrowMax-dkThrowMin)
		p.nextBarrelT = iv * p.waveScale()
	}
}

func (p *playScene) throwBarrel() {
	p.dkThrowAnimT = dkAnimDur
	// DK throws barrels off the left edge of his platform — they bypass
	// the top girder and start falling toward girder 1. Spawn just below
	// g0 and to the left of DK so the visual is "rolled out of his hand
	// and tipped over the edge", and so the falling-collision logic doesn't
	// immediately snap the barrel back onto DK's platform.
	g0 := p.lvl.girders[0]
	bx := float64(p.lvl.dkX) - 2
	if bx < 0 {
		bx = 0
	}
	startY := float64(g0.yAt(int(bx)+barrelFallW/2) + 2)
	b := &barrel{
		id:         p.nextBarrelID(),
		x:          bx,
		y:          startY,
		vx:         3.0, // small leftward bias (negative would also work)
		vy:         8.0, // initial downward push so gravity has somewhere to start
		state:      1,
		ladderSeen: map[int]bool{},
	}
	p.barrels = append(p.barrels, b)
}

func (p *playScene) nextBarrelID() int {
	p.barrelIDSeq++
	return p.barrelIDSeq
}

// -- Mario -------------------------------------------------------------

func (p *playScene) updateMario(s float64) {
	switch p.mario.state {
	case msWalking:
		p.marioWalk(s)
	case msClimbing:
		p.marioClimb(s)
	case msJumping:
		p.marioJump(s)
	case msDying:
		p.mario.dyingT -= s
		if p.mario.dyingT <= 0 {
			p.respawnOrGameOver()
		}
	}
}

func (p *playScene) marioWalk(s float64) {
	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	up := p.e.IsKeyDown(engine.KeyUp) || p.e.IsCharDown('w') || p.e.IsCharDown('W')
	down := p.e.IsKeyDown(engine.KeyDown) || p.e.IsCharDown('s') || p.e.IsCharDown('S')

	// Accelerate toward target velocity when a direction is held; apply
	// friction when neither is held. This gives Mario a brief slide on
	// release instead of stopping dead, which makes positioning for a
	// jump much more forgiving on a terminal where key-up timing is
	// imprecise.
	target := 0.0
	switch {
	case left && !right:
		target = -marioWalkSpeed
		p.mario.facing = -1
	case right && !left:
		target = marioWalkSpeed
		p.mario.facing = +1
	}
	p.mario.vx = approach(p.mario.vx, target, marioWalkAccel*s, marioWalkFriction*s)
	p.mario.x += p.mario.vx * s
	moving := p.mario.vx != 0

	// Clamp horizontally to canvas and oil-drum collision on the Mario
	// girder (the drum blocks the leftmost columns there).
	p.clampMarioWalk()

	// Snap y to the supporting girder's slope.
	g := p.lvl.girders[p.mario.girderIdx]
	cx := int(p.mario.x) + marioWidth/2
	p.mario.y = float64(g.yAt(cx) - 1)

	// Animation.
	if moving {
		p.mario.walkAnimT += s
		if p.mario.walkAnimT >= marioWalkFrame {
			p.mario.walkAnimT -= marioWalkFrame
			p.mario.walkFrame = 1 - p.mario.walkFrame
		}
	} else {
		p.mario.walkAnimT = 0
		p.mario.walkFrame = 0
	}

	// Update hammer swing if active.
	if p.hammerActive {
		p.mario.hammerSwingT += s
		if p.mario.hammerSwingT >= hammerSwingFrame {
			p.mario.hammerSwingT -= hammerSwingFrame
			p.mario.hammerHigh = !p.mario.hammerHigh
		}
	}

	// Climbing transitions (only when not wielding the hammer).
	if !p.hammerActive {
		if up && p.tryClimbUp(cx) {
			return
		}
		if down && p.tryClimbDown(cx) {
			return
		}
	}

	// Jump (only without hammer).
	if p.jumpQueued && !p.hammerActive {
		p.mario.state = msJumping
		p.mario.vy = marioJumpVy
		p.mario.jumpedBarrels = map[int]bool{}
		p.mario.jumpStartFeet = p.mario.y
	}
	p.jumpQueued = false

	// Pick up the hammer if Mario walks under it.
	p.checkHammerPickup()
}

func (p *playScene) clampMarioWalk() {
	// Clamp to the supporting girder's x range so Mario can't walk off
	// the cropped end of a slanted girder. Sprite-left at g.leftX puts
	// the leftmost pixel column flush with the girder's start; sprite-
	// right at g.rightX puts the rightmost column flush with its end.
	g := p.lvl.girders[p.mario.girderIdx]
	minX := float64(g.leftX)
	maxX := float64(g.rightX - marioWidth + 1)
	if p.mario.x < minX {
		p.mario.x = minX
	}
	if p.mario.x > maxX {
		p.mario.x = maxX
	}
	// On Mario's bottom girder, the oil drum + flame block the leftmost
	// area. Don't let Mario walk into the drum.
	if p.mario.girderIdx == p.lvl.marioStartGIdx {
		drumX := float64(p.lvl.oilDrumX + oilDrum.width() + 1)
		if p.mario.x < drumX {
			p.mario.x = drumX
		}
	}
}

func (p *playScene) tryClimbUp(cx int) bool {
	// Mario climbs from the girder he's on (gap = girderIdx) upward.
	gapIdx := p.mario.girderIdx - 1
	if gapIdx < 0 {
		return false
	}
	for _, ld := range p.lvl.ladders[gapIdx] {
		if !ld.containsX(cx) {
			continue
		}
		// Only step onto a ladder if our feet line up with its bottomY (the
		// girder we're on). Broken ladders are still climbable from below.
		if int(p.mario.y)+1 == ld.bottomY {
			p.startClimb(gapIdx, ld, false)
			return true
		}
	}
	return false
}

func (p *playScene) tryClimbDown(cx int) bool {
	// Mario climbs from the upper girder of a gap = girderIdx, gapIdx==girderIdx.
	gapIdx := p.mario.girderIdx
	if gapIdx >= len(p.lvl.ladders) {
		return false
	}
	for _, ld := range p.lvl.ladders[gapIdx] {
		if ld.broken {
			// Broken ladders don't reach the upper girder; can't descend.
			continue
		}
		if !ld.containsX(cx) {
			continue
		}
		if int(p.mario.y)+1 == ld.topY {
			p.startClimb(gapIdx, ld, true)
			return true
		}
	}
	return false
}

func (p *playScene) startClimb(gapIdx int, ld ladder, fromTop bool) {
	p.mario.state = msClimbing
	p.mario.climbing = ld
	p.mario.climbGap = gapIdx
	p.mario.climbDescend = fromTop
	p.mario.climbBoundsTop = float64(ld.climbTopY() - 1)
	// Snap Mario's x to the ladder centre; kill any leftover walking
	// momentum so he doesn't drift off the ladder.
	p.mario.x = float64(ld.x + ladderWidth/2 - marioWidth/2)
	p.mario.vx = 0
	// Bump feet 1 px in the direction of climb so we don't immediately
	// re-detect the source girder.
	if fromTop {
		p.mario.y++ // start descending: feet move 1 px down
	} else {
		p.mario.y-- // start ascending: feet move 1 px up
	}
}

func (p *playScene) marioClimb(s float64) {
	up := p.e.IsKeyDown(engine.KeyUp) || p.e.IsCharDown('w') || p.e.IsCharDown('W')
	down := p.e.IsKeyDown(engine.KeyDown) || p.e.IsCharDown('s') || p.e.IsCharDown('S')
	switch {
	case up && !down:
		p.mario.y -= marioClimbSpeed * s
		p.mario.walkAnimT += s
	case down && !up:
		p.mario.y += marioClimbSpeed * s
		p.mario.walkAnimT += s
	}
	if p.mario.walkAnimT >= marioWalkFrame {
		p.mario.walkAnimT -= marioWalkFrame
		p.mario.walkFrame = 1 - p.mario.walkFrame
	}

	// Clamp to ladder vertical range. The top of climb depends on whether
	// the ladder is broken.
	if p.mario.y < p.mario.climbBoundsTop {
		p.mario.y = p.mario.climbBoundsTop
	}
	if p.mario.y > float64(p.mario.climbing.bottomY-1) {
		p.mario.y = float64(p.mario.climbing.bottomY - 1)
	}

	// Dismount transitions: when Mario's feet line up with a girder
	// directly, snap onto it.
	feetY := int(p.mario.y) + 1
	if feetY == p.mario.climbing.topY && !p.mario.climbing.broken {
		// Mounted onto upper girder (gapIdx).
		p.mario.girderIdx = p.mario.climbGap
		p.mario.state = msWalking
		// Re-snap y to upper girder y minus 1.
		ug := p.lvl.girders[p.mario.girderIdx]
		cx := int(p.mario.x) + marioWidth/2
		p.mario.y = float64(ug.yAt(cx) - 1)
		return
	}
	if feetY == p.mario.climbing.bottomY {
		// Mounted onto lower girder (gapIdx+1).
		p.mario.girderIdx = p.mario.climbGap + 1
		p.mario.state = msWalking
		lg := p.lvl.girders[p.mario.girderIdx]
		cx := int(p.mario.x) + marioWidth/2
		p.mario.y = float64(lg.yAt(cx) - 1)
		return
	}
}

func (p *playScene) marioJump(s float64) {
	// Air control: keep the horizontal momentum Mario had at takeoff and
	// add limited steering. Air friction is zero, so a running jump keeps
	// full speed across the arc.
	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	target := p.mario.vx // no input → preserve momentum (no air drag)
	switch {
	case left && !right:
		target = -marioWalkSpeed
		p.mario.facing = -1
	case right && !left:
		target = marioWalkSpeed
		p.mario.facing = +1
	}
	p.mario.vx = approach(p.mario.vx, target, marioAirAccel*s, marioAirAccel*s)
	p.mario.x += p.mario.vx * s
	if p.mario.x < 0 {
		p.mario.x = 0
		p.mario.vx = 0
	}
	if int(p.mario.x)+marioWidth > p.w {
		p.mario.x = float64(p.w - marioWidth)
		p.mario.vx = 0
	}

	// Apply gravity. Remember the feet row before this frame's movement so
	// landing can be detected as a CROSSING (prev above, now at/below the
	// girder's standing row), not just "girder somewhere above feet".
	prevY := p.mario.y
	p.mario.vy += gravity * s
	p.mario.y += p.mario.vy * s

	// Only check for landing while descending — going up obviously can't
	// land on anything. We also forbid landing on a girder ABOVE the one
	// the jump started from: jumps are for clearing barrels, not for
	// ascending floors. p.mario.girderIdx stays pinned to the start girder
	// for the whole airborne phase, so any girder with a smaller index
	// (higher on screen) is filtered out.
	if p.mario.vy >= 0 {
		cx := int(p.mario.x) + marioWidth/2
		bestIdx := -1
		bestGy := 1 << 30
		for i, g := range p.lvl.girders {
			if i < p.mario.girderIdx {
				continue
			}
			if !g.contains(cx) {
				continue
			}
			gy := g.yAt(cx)
			standY := float64(gy - 1) // mario.y row when standing on this girder
			// Crossed downward this frame: was above the standing row,
			// now at or below. Pick the topmost (smallest gy) such girder
			// in case a single frame crosses two (rare but possible).
			if prevY < standY && p.mario.y >= standY && gy < bestGy {
				bestGy = gy
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			g := p.lvl.girders[bestIdx]
			cx2 := int(p.mario.x) + marioWidth/2
			p.mario.y = float64(g.yAt(cx2) - 1)
			p.mario.girderIdx = bestIdx
			p.mario.state = msWalking
			p.mario.vy = 0
			// Score the jumped-over barrels (set during the jump arc).
			if jumped := len(p.mario.jumpedBarrels); jumped > 0 {
				gain := jumped * pointsBarrelJump
				p.score += gain
				p.spawnPopup(p.mario.x+float64(marioWidth)/2,
					p.mario.y-float64(marioHeight), fmt.Sprintf("+%d", gain), exclaimColor)
			}
			p.mario.jumpedBarrels = nil
		}
	}

	// Fell off the bottom of the world — count as a death.
	if int(p.mario.y) >= p.h {
		p.killMario()
	}
}

// -- Hammer ------------------------------------------------------------

func (p *playScene) checkHammerPickup() {
	if p.hammer == nil || p.hammer.picked {
		return
	}
	mr := p.marioRect()
	hr := rect{
		x0: p.hammer.x, y0: p.hammer.y,
		x1: p.hammer.x + hammerPickup.width(),
		y1: p.hammer.y + hammerPickup.height(),
	}
	if mr.overlaps(hr) {
		p.hammer.picked = true
		p.hammerActive = true
		p.hammerEndT = p.timeT + hammerActiveDur
		p.mario.hammerHigh = true
		p.mario.hammerSwingT = 0
	}
}

func (p *playScene) updateHammer(_ float64) {
	if !p.hammerActive {
		return
	}
	if p.timeT >= p.hammerEndT {
		p.hammerActive = false
	}
}

// hammerSwingRect returns the bounding box of the hammer head this frame.
// Used to clobber barrels and fireballs while wielding.
func (p *playScene) hammerSwingRect() rect {
	// 3-wide head, varies in position based on swing frame and facing.
	hx := int(p.mario.x)
	hy := int(p.mario.y) - marioHeight + 1
	if p.mario.hammerHigh {
		// Head sits 3px above Mario's hat.
		hx += 1
		hy -= 3
		return rect{x0: hx, y0: hy, x1: hx + 3, y1: hy + 2}
	}
	// Low swing — head out in front at chest height.
	if p.mario.facing >= 0 {
		hx += marioWidth
	} else {
		hx -= 2
	}
	hy += marioHeight - 4
	return rect{x0: hx, y0: hy, x1: hx + 2, y1: hy + 2}
}

// -- Barrels -----------------------------------------------------------

func (p *playScene) updateBarrels(s float64) {
	kept := p.barrels[:0]
	for _, b := range p.barrels {
		if p.tickBarrel(b, s) {
			kept = append(kept, b)
		}
	}
	p.barrels = kept
}

// tickBarrel advances one barrel one frame. Returns false if the barrel
// should be removed (rolled off screen or destroyed).
func (p *playScene) tickBarrel(b *barrel, s float64) bool {
	b.frameT += s
	if b.frameT >= barrelRollFrame {
		b.frameT -= barrelRollFrame
		b.frame = 1 - b.frame
	}

	switch b.state {
	case 0:
		return p.barrelRoll(b, s)
	case 1:
		return p.barrelFall(b, s)
	case 2:
		return p.barrelDescend(b, s)
	}
	return false
}

func (p *playScene) barrelRoll(b *barrel, s float64) bool {
	g := p.lvl.girders[b.girderIdx]

	// Adopt this girder's roll direction (flat girders keep current sign).
	if g.rollDir != dirNone {
		b.vx = float64(g.rollDir) * barrelRollSpeed
	} else if b.vx == 0 {
		b.vx = barrelRollSpeed
	} else {
		// Normalise to roll speed on the flat girder.
		if b.vx > 0 {
			b.vx = barrelRollSpeed
		} else {
			b.vx = -barrelRollSpeed
		}
	}
	b.x += b.vx * s

	// Follow girder slope (place barrel sitting on top).
	cx := int(b.x) + barrelW/2
	if cx < g.leftX {
		cx = g.leftX
	}
	if cx > g.rightX {
		cx = g.rightX
	}
	b.y = float64(g.yAt(cx) - barrelH)

	// Off the end of the girder → fall.
	if int(b.x)+barrelW < g.leftX || int(b.x) > g.rightX {
		// Off the very bottom flat girder — despawn.
		if b.girderIdx == len(p.lvl.girders)-1 {
			return false
		}
		// Otherwise transition to falling, retaining a small horizontal velocity.
		b.state = 1
		b.vy = 0
		return true
	}

	// Ladder descent check — barrel rolls over the centre of a ladder
	// belonging to the gap below this girder, with a chance to take it.
	if b.girderIdx < len(p.lvl.ladders) {
		for li, ld := range p.lvl.ladders[b.girderIdx] {
			if b.ladderSeen[li] {
				continue
			}
			if cx < ld.x || cx > ld.x+ladderWidth-1 {
				continue
			}
			b.ladderSeen[li] = true
			if p.rng.Float64() < barrelLadderChance {
				b.state = 2
				b.descLad = ld
				b.descGap = b.girderIdx
				// Snap barrel to ladder centre for the descent.
				b.x = float64(ld.x + ladderWidth/2 - barrelFallW/2)
				return true
			}
		}
	}
	return true
}

func (p *playScene) barrelFall(b *barrel, s float64) bool {
	b.vy += barrelGravity * s
	b.x += b.vx * s
	b.y += b.vy * s

	// Off the bottom of the world.
	if int(b.y) >= p.h {
		return false
	}
	// Off horizontally — keep falling but stop sideways drift once past edge.
	if b.x < 0 {
		b.x = 0
		b.vx = 0
	}
	if int(b.x)+barrelFallW > p.w {
		b.x = float64(p.w - barrelFallW)
		b.vx = 0
	}

	// Look for a girder beneath that the barrel's bottom edge is about to
	// cross. If found, snap onto it and start rolling.
	cx := int(b.x) + barrelFallW/2
	bottomY := int(b.y) + barrelFallH
	for i, g := range p.lvl.girders {
		if !g.contains(cx) {
			continue
		}
		gy := g.yAt(cx)
		if bottomY >= gy && bottomY <= gy+3 && b.vy > 0 {
			b.state = 0
			b.girderIdx = i
			b.vy = 0
			b.ladderSeen = map[int]bool{}
			// Lock barrel onto the girder.
			b.y = float64(gy - barrelH)
			return true
		}
	}
	return true
}

func (p *playScene) barrelDescend(b *barrel, s float64) bool {
	// Falling speed down a ladder is a bit slower than gravity-fall, so
	// the player can read the threat earlier.
	b.y += (barrelGravity * 0.6) * s
	bottomY := int(b.y) + barrelFallH
	if bottomY >= b.descLad.bottomY {
		// Snap onto the lower girder.
		lowerIdx := b.descGap + 1
		g := p.lvl.girders[lowerIdx]
		cx := int(b.x) + barrelFallW/2
		b.state = 0
		b.girderIdx = lowerIdx
		b.y = float64(g.yAt(cx) - barrelH)
		// Re-centre x on the barrel sprite when rolling (the rolling sprite
		// is wider than the falling sprite, so shift left by the diff).
		b.x -= float64(barrelW-barrelFallW) / 2
		b.ladderSeen = map[int]bool{}
		// Mark this ladder as already-taken so the barrel doesn't try to
		// descend it again immediately after landing.
		b.ladderSeen[-1] = true
		return true
	}
	return true
}

// -- Fireballs --------------------------------------------------------

func (p *playScene) updateFireballs(s float64) {
	if p.state == psPlaying {
		p.nextFireballT -= s
		if p.nextFireballT <= 0 && p.fireballNumOut < fireballMax {
			p.spawnFireball()
			p.nextFireballT = fireballRespawnT
		}
	}
	kept := p.fireballs[:0]
	for _, f := range p.fireballs {
		if p.tickFireball(f, s) {
			kept = append(kept, f)
		} else {
			p.fireballNumOut--
		}
	}
	p.fireballs = kept
}

func (p *playScene) spawnFireball() {
	g := p.lvl.girders[p.lvl.marioStartGIdx]
	x := float64(p.lvl.oilDrumX + oilDrum.width() + 2)
	cx := int(x) + fireballW/2
	y := float64(g.yAt(cx) - fireballH)
	f := &fireballEntity{
		x:         x,
		y:         y,
		vx:        fireballSpeed,
		state:     0,
		girderIdx: p.lvl.marioStartGIdx,
	}
	p.fireballs = append(p.fireballs, f)
	p.fireballNumOut++
}

func (p *playScene) tickFireball(f *fireballEntity, s float64) bool {
	f.frameT += s
	if f.frameT >= fireballFrameRate {
		f.frameT -= fireballFrameRate
		f.frame = 1 - f.frame
	}

	switch f.state {
	case 0:
		return p.fireballWalk(f, s)
	case 1:
		return p.fireballClimb(f, s)
	}
	return false
}

func (p *playScene) fireballWalk(f *fireballEntity, s float64) bool {
	g := p.lvl.girders[f.girderIdx]
	f.x += f.vx * s

	cx := int(f.x) + fireballW/2

	// Reverse at girder ends.
	if cx <= g.leftX {
		f.x = float64(g.leftX)
		f.vx = math.Abs(f.vx)
	}
	if cx >= g.rightX {
		f.x = float64(g.rightX - fireballW)
		f.vx = -math.Abs(f.vx)
	}
	cx = int(f.x) + fireballW/2
	f.y = float64(g.yAt(cx) - fireballH)

	// Decide to climb. Each frame, with a small probability when over a
	// ladder, decide to ascend or descend.
	if p.rng.Float64() < 0.01 {
		// Look for ladders touching cx that lead to a higher girder.
		if f.girderIdx > 0 {
			for _, ld := range p.lvl.ladders[f.girderIdx-1] {
				if ld.broken {
					continue
				}
				if ld.containsX(cx) {
					f.state = 1
					f.climbing = ld
					f.climbGap = f.girderIdx - 1
					f.climbDir = -1
					f.x = float64(ld.x + ladderWidth/2 - fireballW/2)
					return true
				}
			}
		}
		if f.girderIdx < len(p.lvl.ladders) {
			for _, ld := range p.lvl.ladders[f.girderIdx] {
				if ld.broken {
					continue
				}
				if ld.containsX(cx) {
					f.state = 1
					f.climbing = ld
					f.climbGap = f.girderIdx
					f.climbDir = +1
					f.x = float64(ld.x + ladderWidth/2 - fireballW/2)
					return true
				}
			}
		}
	}
	return true
}

func (p *playScene) fireballClimb(f *fireballEntity, s float64) bool {
	f.y += float64(f.climbDir) * fireballSpeed * s
	feetY := int(f.y) + fireballH
	if f.climbDir < 0 && feetY <= f.climbing.topY {
		f.state = 0
		f.girderIdx = f.climbGap
		g := p.lvl.girders[f.girderIdx]
		cx := int(f.x) + fireballW/2
		f.y = float64(g.yAt(cx) - fireballH)
		// Random new direction.
		if p.rng.Intn(2) == 0 {
			f.vx = fireballSpeed
		} else {
			f.vx = -fireballSpeed
		}
		return true
	}
	if f.climbDir > 0 && feetY >= f.climbing.bottomY {
		f.state = 0
		f.girderIdx = f.climbGap + 1
		g := p.lvl.girders[f.girderIdx]
		cx := int(f.x) + fireballW/2
		f.y = float64(g.yAt(cx) - fireballH)
		if p.rng.Intn(2) == 0 {
			f.vx = fireballSpeed
		} else {
			f.vx = -fireballSpeed
		}
		return true
	}
	return true
}

// -- Collisions -------------------------------------------------------

type rect struct {
	x0, y0, x1, y1 int
}

func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && r.x1 > o.x0 && r.y0 < o.y1 && r.y1 > o.y0
}

// approach moves cur toward target by at most one of accel (when |cur - target|
// pushes cur OUTWARD from zero or holds it past zero) or friction (when
// decelerating toward zero). Used for Mario's horizontal velocity:
// accel ramps him up to walk speed, friction brings him to a halt.
func approach(cur, target, accel, friction float64) float64 {
	diff := target - cur
	// Pick the step size: if we're slowing down (target=0 or moving away
	// from current sign), use friction; otherwise use accel.
	step := accel
	if target == 0 || (cur > 0 && target < cur) || (cur < 0 && target > cur) {
		// Only treat as "slowing" if the move shrinks |cur|.
		if (cur > 0 && diff < 0) || (cur < 0 && diff > 0) {
			step = friction
		}
	}
	if diff > step {
		return cur + step
	}
	if diff < -step {
		return cur - step
	}
	return target
}

func (p *playScene) marioRect() rect {
	x := int(p.mario.x)
	yBottom := int(p.mario.y) + 1
	return rect{
		x0: x, y0: yBottom - marioHeight,
		x1: x + marioWidth, y1: yBottom,
	}
}

func (p *playScene) barrelRect(b *barrel) rect {
	switch b.state {
	case 0:
		return rect{x0: int(b.x), y0: int(b.y), x1: int(b.x) + barrelW, y1: int(b.y) + barrelH}
	default:
		return rect{x0: int(b.x), y0: int(b.y), x1: int(b.x) + barrelFallW, y1: int(b.y) + barrelFallH}
	}
}

func (p *playScene) fireballRect(f *fireballEntity) rect {
	return rect{x0: int(f.x), y0: int(f.y), x1: int(f.x) + fireballW, y1: int(f.y) + fireballH}
}

func (p *playScene) resolveCollisions() {
	if p.mario.state == msDying {
		return
	}
	mr := p.marioRect()
	var hr rect
	if p.hammerActive {
		hr = p.hammerSwingRect()
	}

	// Barrels.
	kept := p.barrels[:0]
	for _, b := range p.barrels {
		br := p.barrelRect(b)
		if p.hammerActive && hr.overlaps(br) {
			p.score += hammerKillScore
			p.spawnPopup(b.x+float64(barrelW)/2, b.y-2,
				fmt.Sprintf("+%d", hammerKillScore), exclaimColor)
			continue // barrel destroyed
		}
		// Jump-over scoring: while Mario is airborne, mark barrels whose
		// top is below his feet AND within a small horizontal slop.
		if p.mario.state == msJumping {
			cx := int(p.mario.x) + marioWidth/2
			bcx := int(b.x) + barrelW/2
			if math.Abs(float64(cx-bcx)) < 8 && br.y0 > mr.y1-2 && br.y0 < mr.y1+5 {
				p.mario.jumpedBarrels[b.id] = true
			}
		}
		if br.overlaps(mr) {
			p.killMario()
			kept = append(kept, b)
			continue
		}
		kept = append(kept, b)
	}
	p.barrels = kept

	// Fireballs.
	fKept := p.fireballs[:0]
	for _, f := range p.fireballs {
		fr := p.fireballRect(f)
		if p.hammerActive && hr.overlaps(fr) {
			p.score += hammerKillScore * 2
			p.spawnPopup(f.x+float64(fireballW)/2, f.y-2,
				fmt.Sprintf("+%d", hammerKillScore*2), exclaimColor)
			continue
		}
		if fr.overlaps(mr) {
			p.killMario()
		}
		fKept = append(fKept, f)
	}
	p.fireballs = fKept

	// Mario walked into the flame.
	flameRect := rect{
		x0: p.lvl.flameX, y0: p.lvl.flameY,
		x1: p.lvl.flameX + flameA.width(),
		y1: p.lvl.flameY + flameA.height(),
	}
	if mr.overlaps(flameRect) {
		p.killMario()
	}
}

func (p *playScene) killMario() {
	if p.mario.state == msDying {
		return
	}
	p.mario.state = msDying
	p.mario.dyingT = deathDuration
	p.mario.vy = 0
	p.lives--
	// Drop the hammer if held.
	p.hammerActive = false
}

func (p *playScene) respawnOrGameOver() {
	if p.lives <= 0 {
		p.state = psGameOver
		p.stateT = 0
		return
	}
	// Reset Mario position; keep barrels and fireballs in play (more
	// punishing, but classic).
	g := p.lvl.girders[p.lvl.marioStartGIdx]
	p.mario = marioEntity{
		girderIdx: p.lvl.marioStartGIdx,
		facing:    +1,
		state:     msWalking,
	}
	p.mario.x = float64(p.lvl.marioStartX)
	cx := int(p.mario.x) + marioWidth/2
	p.mario.y = float64(g.yAt(cx) - 1)
	p.bonus = bonusStart
	p.bonusT = 0
}

// -- Popups -----------------------------------------------------------

func (p *playScene) spawnPopup(x, y float64, text string, col engine.Color) {
	p.popups = append(p.popups, scorePopup{x: x, y: y, text: text, col: col})
}

func (p *playScene) updatePopups(s float64) {
	kept := p.popups[:0]
	for _, pop := range p.popups {
		pop.age += s
		pop.y -= 8 * s
		if pop.age < 1.1 {
			kept = append(kept, pop)
		}
	}
	p.popups = kept
}

// -- Rendering --------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(bgColor)
	p.drawStage(c)
	p.drawDK(c)
	p.drawPauline(c)
	p.drawOilDrum(c)
	p.drawHammerPickup(c)
	p.drawBarrels(c)
	p.drawFireballs(c)
	p.drawMario(c)
	p.drawPopups(c)
	p.drawHUD(c)

	switch p.state {
	case psPreStage:
		p.drawPreStageBanner(c)
	case psStageClear:
		p.drawStageClear(c)
	case psGameOver:
		p.drawGameOver(c)
	}
}

func (p *playScene) drawStage(c *engine.Canvas) {
	// Pauline's pedestal — thin 2-pixel block (red + shadow) right under her.
	c.FillRect(p.lvl.paulinePedestalX, p.lvl.paulinePedestalY,
		p.lvl.paulinePedestalW, 1, girderRed)
	c.FillRect(p.lvl.paulinePedestalX, p.lvl.paulinePedestalY+1,
		p.lvl.paulinePedestalW, 1, girderDark)

	// Girders.
	for _, g := range p.lvl.girders {
		drawGirder(c, g)
	}

	// Ladders.
	for _, group := range p.lvl.ladders {
		for _, ld := range group {
			drawLadder(c, ld)
		}
	}
}

// drawGirder paints one pixel-thick girder plus a shadow row underneath.
func drawGirder(c *engine.Canvas, g girder) {
	span := g.rightX - g.leftX
	if span <= 0 {
		return
	}
	for x := g.leftX; x <= g.rightX; x++ {
		y := g.yAt(x)
		c.Set(x, y, girderRed)
		c.Set(x, y+1, girderDark)
	}
	// Bracket the ends with chunky support pixels so each girder reads as a
	// platform, not a thin stripe.
	c.Set(g.leftX, g.leftY-1, girderRed)
	c.Set(g.rightX, g.rightY-1, girderRed)
}

func drawLadder(c *engine.Canvas, ld ladder) {
	top := ld.topY
	if ld.broken {
		top = ld.climbTopY()
	}
	// Two vertical rails.
	for y := top; y < ld.bottomY; y++ {
		c.Set(ld.x, y, ladderYellow)
		c.Set(ld.x+ladderWidth-1, y, ladderYellow)
	}
	// Horizontal rungs every 2 rows.
	for y := top + 1; y < ld.bottomY; y += 2 {
		for x := ld.x; x < ld.x+ladderWidth; x++ {
			c.Set(x, y, ladderDark)
		}
	}
}

func (p *playScene) drawDK(c *engine.Canvas) {
	spr := dkIdle
	if p.dkThrowAnimT > 0 {
		spr = dkThrow
	}
	drawColorSprite(c, p.lvl.dkX, p.lvl.dkY, spr, false)
}

func (p *playScene) drawPauline(c *engine.Canvas) {
	drawColorSprite(c, p.lvl.paulineX, p.lvl.paulineY, paulineSprite, false)
}

func (p *playScene) drawOilDrum(c *engine.Canvas) {
	drawColorSprite(c, p.lvl.oilDrumX, p.lvl.oilDrumY, oilDrum, false)
	frame := flameA
	if int(p.timeT/flameFrameRate)%2 == 1 {
		frame = flameB
	}
	drawColorSprite(c, p.lvl.flameX, p.lvl.flameY, frame, false)
}

func (p *playScene) drawHammerPickup(c *engine.Canvas) {
	if p.hammer == nil || p.hammer.picked {
		return
	}
	drawColorSprite(c, p.hammer.x, p.hammer.y, hammerPickup, false)
}

func (p *playScene) drawBarrels(c *engine.Canvas) {
	for _, b := range p.barrels {
		switch b.state {
		case 0:
			spr := barrelA
			if b.frame == 1 {
				spr = barrelB
			}
			drawColorSprite(c, int(b.x), int(b.y), spr, false)
		case 1, 2:
			drawColorSprite(c, int(b.x), int(b.y), barrelFall, false)
		}
	}
}

func (p *playScene) drawFireballs(c *engine.Canvas) {
	for _, f := range p.fireballs {
		spr := fireballA
		if f.frame == 1 {
			spr = fireballB
		}
		drawColorSprite(c, int(f.x), int(f.y), spr, false)
	}
}

func (p *playScene) drawMario(c *engine.Canvas) {
	x := int(p.mario.x)
	y := int(p.mario.y) - marioHeight + 1
	flip := p.mario.facing < 0

	switch p.mario.state {
	case msDying:
		// Tumble: alternate between dead pose and upside-down based on phase.
		drawColorSprite(c, x, y, marioDead, flip)
		return
	case msClimbing:
		spr := marioClimbA
		if p.mario.walkFrame == 1 {
			spr = marioClimbB
		}
		drawColorSprite(c, x, y, spr, false)
		return
	case msJumping:
		drawColorSprite(c, x, y, marioJump, flip)
		return
	}

	// Walking / standing.
	if p.hammerActive {
		spr := marioHammerHigh
		yOff := -2
		if !p.mario.hammerHigh {
			spr = marioHammerLow
			yOff = 0
		}
		drawColorSprite(c, x, y+yOff, spr, flip)
		return
	}

	if p.mario.walkAnimT > 0 || (p.e.IsKeyDown(engine.KeyLeft) ||
		p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('a') ||
		p.e.IsCharDown('d') || p.e.IsCharDown('A') || p.e.IsCharDown('D')) {
		spr := marioWalkA
		if p.mario.walkFrame == 1 {
			spr = marioWalkB
		}
		drawColorSprite(c, x, y, spr, flip)
		return
	}
	drawColorSprite(c, x, y, marioStand, flip)
}

func (p *playScene) drawPopups(c *engine.Canvas) {
	for _, pop := range p.popups {
		frac := pop.age / 1.1
		if frac > 1 {
			frac = 1
		}
		dim := 1.0 - frac*0.6
		col := engine.Color{
			R: uint8(float64(pop.col.R) * dim),
			G: uint8(float64(pop.col.G) * dim),
			B: uint8(float64(pop.col.B) * dim),
			A: 255,
		}
		cellCol := int(pop.x) - len(pop.text)/2
		if cellCol < 0 {
			cellCol = 0
		}
		row := int(pop.y) / 2
		if row < 0 {
			row = 0
		}
		c.Print(cellCol, row, pop.text, col)
	}
}

func (p *playScene) drawHUD(c *engine.Canvas) {
	// HUD is a single text row at the very top so the playfield (including
	// DK's head) doesn't have to fight it for pixels. Layout:
	//
	//   [SCORE 000000]  [HI 000000]  [BONUS 5000]  [Lx3] [L=1]
	//
	cols := c.Cols()
	scoreText := fmt.Sprintf("SCORE %06d", p.score)
	hiText := fmt.Sprintf("HI %06d", p.hiScore)
	bonusText := fmt.Sprintf("BONUS %04d", p.bonus)
	livesText := fmt.Sprintf("Lx%d", p.lives)
	waveText := fmt.Sprintf("L=%d", p.wave)

	c.Print(1, 0, scoreText, scoreColor)
	hiCol := (cols - len(hiText)) / 2
	if hiCol < len(scoreText)+3 {
		hiCol = len(scoreText) + 3
	}
	c.Print(hiCol, 0, hiText, bonusColor)
	right := cols - len(waveText) - 1
	c.Print(right, 0, waveText, livesColor)
	right -= len(livesText) + 2
	if right > hiCol+len(hiText)+1 {
		c.Print(right, 0, livesText, heartColor)
	}
	right -= len(bonusText) + 2
	if right > hiCol+len(hiText)+1 {
		c.Print(right, 0, bonusText, exclaimColor)
	}
}

func (p *playScene) drawPreStageBanner(c *engine.Canvas) {
	msg := "HOW HIGH CAN YOU GET?"
	w := engine.TextWidth(msg)
	x := (p.w - w) / 2
	y := p.h/2 - engine.FontHeight/2
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4,
		engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, msg, exclaimColor)
}

func (p *playScene) drawStageClear(c *engine.Canvas) {
	msg := fmt.Sprintf("YOU RESCUED PAULINE  L=%d", p.wave)
	w := engine.TextWidth(msg)
	x := (p.w - w) / 2
	y := p.h/2 - engine.FontHeight/2
	c.FillRect(x-4, y-3, w+8, engine.FontHeight+6,
		engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, msg, exclaimColor)
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	msg := "GAME OVER"
	w := engine.TextWidth(msg)
	x := (p.w - w) / 2
	y := p.h/2 - engine.FontHeight/2 - 3
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4,
		engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, msg, engine.Color{R: 255, G: 80, B: 80, A: 255})

	hint := "ENTER / SPACE: RETURN TO TITLE   ESC: QUIT"
	c.Print((c.Cols()-len(hint))/2, p.h/2/2+2, hint, hintColor)
}
