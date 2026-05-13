package galaxian

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// ---------------------------------------------------------------------
// Tuning. Speeds are in pixels per second; intervals in seconds.
// ---------------------------------------------------------------------

const (
	playerSpeed       = 38.0
	playerFireGap     = 0.32 // single-bullet limit + min cadence
	playerBulletSpeed = 90.0
	playerExplodeDur  = 1.3

	// Formation layout. The row count and row pitch adapt to the canvas
	// height at construction time — see computeLayout. Column pitch is
	// fixed because alien sprites are uniform width.
	formationColPitch = 10 // horizontal centre-to-centre between columns
	formationSwayAmp  = 4  // pixels left/right of centre at peak sway
	formationSwayHz   = 0.25
	formationAnimHz   = 2.2 // wing-flap frequency

	// Dive timing & shapes. The dive is a 360° loop with a downward
	// translation that drops the alien ~14 px below the formation by
	// the time the loop ends. The descent phase then takes over with a
	// wavy fall toward the player.
	divePullDur       = 0.42 // initial pull-off (alien banks outward)
	diveLoopDur       = 1.10 // full revolution loop
	diveLoopRadius    = 9.5  // pixels — loop circle radius
	diveLoopFallPx    = 8.0 // px the loop translates downward over its dur
	diveDescentVy     = 32.0 // initial descent velocity, pix/s
	diveDescentAccel  = 22.0 // descent gravity, pix/s²
	diveWaveAmp       = 7.0
	diveWaveHz        = 1.1

	// Return-to-slot phase: alien wraps from the bottom of the screen back
	// to the top and curves toward its formation slot.
	returnDur     = 1.9
	returnEntryY  = -8.0 // start y when reappearing at top

	// Alien fire — diving aliens drop bullets occasionally.
	alienBulletSpeed   = 26.0
	alienBulletAccelY  = 8.0  // bullets accelerate downward over time
	alienFireGap       = 0.7  // min seconds between fires (per alien)
	alienFireChancePerSec = 0.55

	// Attack scheduling.
	attackLaunchMin   = 1.6 // wave-1 cadence (seconds between launches)
	attackLaunchMax   = 3.2
	attackMaxOnScreen = 4 // wave 1; +1 per wave, capped
	attackMaxCap      = 8

	// Wave progression — each wave shrinks intervals and bumps difficulty.
	waveSpeedup = 0.88

	// Star field.
	numStars      = 70
	starScrollVy  = 6.0
	starTwinkleHz = 0.4

	// Bonus life threshold (Galaxian classic: extra life at 7000 pts).
	bonusLifeAt = 7000

	waveClearDelay = 1.8
)

// flagshipBonusForEscorts is the convoy payout table — index by the
// number of escorts that are still alive in the convoy at the moment
// the flagship is shot. The 800-point reward for taking out a
// flagship with both escorts intact is the iconic Galaxian payoff.
var flagshipBonusForEscorts = [3]int{150, 200, 800}

// ---------------------------------------------------------------------
// State machine.
// ---------------------------------------------------------------------

type playState int

const (
	psPlaying playState = iota
	psPlayerHit
	psWaveCleared
	psGameOver
)

// alienState — where this alien is in its life cycle.
type alienState int

const (
	asFormation alienState = iota
	asPullout                  // initial 0.4s peel-off
	asLoop                     // 270° arc
	asDescend                  // sinusoidal fall
	asExited                   // off-screen at bottom, awaiting respawn
	asReturning                // re-entering from top, curving to slot
)

// ---------------------------------------------------------------------
// Alien data model.
// ---------------------------------------------------------------------

// alien is a single ship in (or off of) the formation. Formation slots
// are addressed by (row, col); the slot's pixel position is recomputed
// every frame from the current formation sway offset.
type alien struct {
	alive bool
	kind  alienKind

	row, col int // formation slot

	state  alienState
	phaseT float64 // seconds elapsed in current state

	// Pulled-state position (used when state != asFormation). When the
	// alien is in formation these are stale and ignored.
	x, y float64

	// Loop-phase parameters — set when the alien transitions into asLoop.
	loopCx float64
	loopCy float64
	loopTh0 float64 // start angle
	loopThSweep float64 // total sweep (signed, ±3π/2)

	// Descent-phase parameters.
	descentX0 float64 // x at moment of entering descent (centre of sine wave)
	descentY0 float64
	descentVy float64
	descentPhase0 float64 // phase offset so sine starts continuous

	// Return-phase parameters — set when re-entering from the top.
	retX0, retY0 float64 // start of return curve (top of screen)
	retCxOff     float64 // bezier control-point x offset

	// Side of the formation this alien dove off of: -1 left, +1 right.
	side int

	// Convoy bookkeeping — for flagship + escort runs.
	convoyID  int    // 0 means "not in a convoy"
	convoyRole int   // 0 leader, 1 left wing, 2 right wing

	// Bullet cooldown (per-alien) — populated while diving so the alien
	// doesn't fire instantly on dive start.
	fireCD float64

}

// formationAlive reports whether this alien is still in its formation slot.
func (a *alien) formationAlive() bool { return a.alive && a.state == asFormation }

// diving reports whether the alien is in any non-formation, non-exited
// state — i.e. somewhere on screen as a free agent.
func (a *alien) diving() bool {
	if !a.alive {
		return false
	}
	switch a.state {
	case asPullout, asLoop, asDescend, asReturning:
		return true
	}
	return false
}

// ---------------------------------------------------------------------
// Player, bullets, stars, explosions.
// ---------------------------------------------------------------------

type playerEntity struct {
	x        float64
	y        int
	cooldown float64
	bullet   *bullet
	lives    int
	explodeT float64
	// Bonus life tracking — score threshold last crossed.
	nextBonusAt int
}

type bullet struct {
	x, y       float64
	vx, vy     float64
	fromPlayer bool
	frame      int
	frameT     float64
	// Bullets accumulate downward velocity over their lifetime so they
	// don't look like they're floating mid-air.
	age float64
}

type star struct {
	x, y    float64
	phase   float64 // phase of twinkle
	depth   int     // 0 dim, 1 mid, 2 bright
	tint    int     // 0 white, 1 cyan, 2 yellow, 3 pink
}

type explosion struct {
	x, y   int
	t      float64
	dur    float64
	kind   int // 0 alien (small 5x5), 1 player (uses player explode sprites)
}

// ---------------------------------------------------------------------
// playScene — the active game.
// ---------------------------------------------------------------------

type playScene struct {
	e    *engine.Engine
	w, h int

	player       playerEntity
	aliens       []*alien
	alienBullets []*bullet
	explosions   []*explosion
	stars        []star

	score   int
	hiScore int
	wave    int

	state  playState
	stateT float64

	rng *rand.Rand

	// Formation layout. Rows/cols/pitch are picked by computeLayout to
	// fit the current canvas — small terminals get a shorter formation
	// and tighter row spacing.
	formationCols     int     // columns per row
	formationRowPitch int     // px between row tops
	formationRows     int     // total formation rows (4–6)
	formationCenterX  float64 // x of the centre of the formation (no sway)
	formationY0       int     // y of row 0
	formationT        float64 // time accumulator driving sway + wing-flap
	rowKinds          []alienKind

	// Attack scheduling.
	attackLaunchT     float64 // counts down to the next launch attempt
	nextConvoyID      int

	// Layout.
	playerY int

	wantQuit bool
}

// ---------------------------------------------------------------------
// Construction & layout.
// ---------------------------------------------------------------------

func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		hiScore: hiScore,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.player.lives = 3
	p.player.nextBonusAt = bonusLifeAt
	p.computeLayout()
	p.spawnStars()
	p.startWave(1)
	return p
}

// computeLayout picks formation cols / rows / row pitch to fit the
// current canvas. The formation always sits in the upper half of the
// canvas with dive space between its bottom row and the player.
func (p *playScene) computeLayout() {
	p.playerY = p.h - playerSprite.height() - 1

	// Column count: derive from canvas width, keeping it even so the
	// flagship pair flanks the centre symmetrically.
	innerW := p.w - 2*(formationSwayAmp+6)
	cols := innerW / formationColPitch
	if cols < 6 {
		cols = 6
	}
	if cols%2 == 1 {
		cols--
	}
	if cols > 10 {
		cols = 10
	}
	p.formationCols = cols
	p.formationCenterX = float64(p.w) / 2

	// Vertical budget. We want the bottom of the formation's deepest
	// sprite to leave clear space between formation and player so the
	// dive loops have somewhere to play out.
	p.formationY0 = 4
	maxSpriteH := flagshipA.height() // tallest alien sprite
	// 18 px of clearance between the deepest formation alien and the
	// player gives the dive loops room to breathe.
	available := p.playerY - 18 - p.formationY0

	// Candidate (rows, pitch) configurations in order of preference —
	// pick the first one that fits in the available vertical budget.
	type cfg struct{ rows, pitch int }
	candidates := []cfg{
		{6, 8}, {6, 7}, {5, 8}, {5, 7}, {4, 7}, {4, 6}, {3, 6},
	}
	for _, c := range candidates {
		if (c.rows-1)*c.pitch+maxSpriteH <= available {
			p.formationRows = c.rows
			p.formationRowPitch = c.pitch
			break
		}
	}
	if p.formationRows == 0 {
		p.formationRows = 3
		p.formationRowPitch = 6
	}

	// Row composition adapts to the row count. Top row is always
	// flagships; bottom rows are always drones; bees and bosses fill
	// in between.
	switch p.formationRows {
	case 6:
		p.rowKinds = []alienKind{kindFlagship, kindBoss, kindBee, kindBee, kindDrone, kindDrone}
	case 5:
		p.rowKinds = []alienKind{kindFlagship, kindBoss, kindBee, kindDrone, kindDrone}
	case 4:
		p.rowKinds = []alienKind{kindFlagship, kindBoss, kindBee, kindDrone}
	default:
		p.rowKinds = []alienKind{kindFlagship, kindBoss, kindDrone}
	}
}

func (p *playScene) spawnStars() {
	p.stars = make([]star, numStars)
	for i := range p.stars {
		p.stars[i] = star{
			x:     p.rng.Float64() * float64(p.w),
			y:     p.rng.Float64() * float64(p.h),
			phase: p.rng.Float64(),
			depth: p.rng.Intn(3),
			tint:  p.rng.Intn(4),
		}
	}
}

// startWave (re)spawns the formation for wave n.
func (p *playScene) startWave(wave int) {
	p.wave = wave
	p.state = psPlaying
	p.stateT = 0
	p.alienBullets = nil
	p.explosions = nil
	p.player.bullet = nil
	p.player.cooldown = 0
	p.player.x = float64(p.w-playerSprite.width()) / 2
	p.player.y = p.playerY
	p.player.explodeT = 0
	p.attackLaunchT = attackLaunchMin
	p.formationT = 0
	p.nextConvoyID = 0

	cols := p.formationCols
	p.aliens = nil

	// Build the formation from the row composition computed in
	// computeLayout. Flagship row is sparse (just two flagships
	// flanking the centre); every other row fills its columns.
	for row, kind := range p.rowKinds {
		if kind == kindFlagship {
			flagCols := []int{cols/2 - 2, cols/2 + 1}
			for _, col := range flagCols {
				if col < 0 || col >= cols {
					continue
				}
				p.aliens = append(p.aliens, &alien{
					alive: true,
					kind:  kind,
					row:   row,
					col:   col,
					state: asFormation,
				})
			}
			continue
		}
		for col := 0; col < cols; col++ {
			p.aliens = append(p.aliens, &alien{
				alive: true,
				kind:  kind,
				row:   row,
				col:   col,
				state: asFormation,
			})
		}
	}
}

// ---------------------------------------------------------------------
// Frame update.
// ---------------------------------------------------------------------

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s
	p.formationT += s
	p.tickStars(s)

	switch p.state {
	case psPlaying:
		p.updatePlaying(s)
	case psPlayerHit:
		p.player.explodeT -= s
		p.tickAliensExplosionMode(s)
		p.tickBullets(s)
		p.tickExplosions(s)
		if p.player.explodeT <= 0 {
			if p.player.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.player.x = float64(p.w-playerSprite.width()) / 2
				p.player.explodeT = 0
				p.state = psPlaying
				p.stateT = 0
			}
		}
	case psWaveCleared:
		// Keep the death-explosion of the last alien animating while the
		// banner is up; tickStars already ran at the top of Update.
		p.tickExplosions(s)
		if p.stateT >= waveClearDelay {
			p.startWave(p.wave + 1)
		}
	case psGameOver:
		// idle — wait for user input
	}

	if p.score >= p.player.nextBonusAt {
		p.player.lives++
		p.player.nextBonusAt += bonusLifeAt
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
		case psPlayerHit:
			if k.Code == engine.KeyEsc {
				p.wantQuit = true
			}
		case psWaveCleared:
			if k.Code == engine.KeyEsc {
				p.wantQuit = true
			}
		case psGameOver:
			if k.Code == engine.KeyEnter ||
				(k.Code == engine.KeyChar && (k.Rune == 'r' || k.Rune == 'R')) {
				hi := p.hiScore
				p.score = 0
				p.player.lives = 3
				p.player.nextBonusAt = bonusLifeAt
				p.hiScore = hi
				p.startWave(1)
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

func (p *playScene) tryFire() {
	if p.player.bullet != nil || p.player.cooldown > 0 {
		return
	}
	bx := p.player.x + float64(playerSprite.width())/2
	by := float64(p.player.y) - float64(playerBulletSprite.height())
	p.player.bullet = &bullet{
		x:          bx,
		y:          by,
		vy:         -playerBulletSpeed,
		fromPlayer: true,
	}
	p.player.cooldown = playerFireGap
}

func (p *playScene) updatePlaying(s float64) {
	// Player movement — held key state, so move and shoot work together.
	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	dir := 0
	if left && !right {
		dir = -1
	} else if right && !left {
		dir = 1
	}
	if dir != 0 {
		p.player.x += float64(dir) * playerSpeed * s
	}
	maxX := float64(p.w - playerSprite.width())
	if p.player.x < 0 {
		p.player.x = 0
	}
	if p.player.x > maxX {
		p.player.x = maxX
	}
	if p.player.cooldown > 0 {
		p.player.cooldown -= s
	}

	p.tickAliens(s)
	p.tickBullets(s)
	p.tickExplosions(s)
	p.scheduleAttacks(s)
	p.resolveCollisions()

	// Wave clear: no aliens left at all (all destroyed).
	stillAny := false
	for _, a := range p.aliens {
		if a.alive {
			stillAny = true
			break
		}
	}
	if !stillAny {
		p.state = psWaveCleared
		p.stateT = 0
	}
}

// tickAliensExplosionMode is the slimmed-down alien tick that runs while
// the player is exploding. Aliens keep their dive animations going but
// no new attacks launch.
func (p *playScene) tickAliensExplosionMode(s float64) {
	for _, a := range p.aliens {
		if a.diving() {
			p.tickDivingAlien(a, s)
		}
	}
}

// tickAliens updates every alien for one frame.
func (p *playScene) tickAliens(s float64) {
	for _, a := range p.aliens {
		if !a.alive {
			continue
		}
		if a.fireCD > 0 {
			a.fireCD -= s
		}
		switch a.state {
		case asFormation:
			// nothing to do — pos is derived from slot in draw
		case asExited:
			// Wait a short beat, then re-enter from the top to return
			// to formation slot.
			a.phaseT += s
			if a.phaseT >= 0.55 {
				p.startReturn(a)
			}
		default:
			p.tickDivingAlien(a, s)
		}
	}
}

// tickDivingAlien runs the per-state dive integration for one alien.
func (p *playScene) tickDivingAlien(a *alien, s float64) {
	a.phaseT += s

	switch a.state {
	case asPullout:
		// Brief arc: ease alien outward from slot to the entry of the loop.
		// Loop entry is on the slot's loop circle at angle θ0.
		dur := divePullDur
		u := clamp01(a.phaseT / dur)
		// Slot position now.
		sx, sy := p.aliensSlotPos(a)
		// Tilt the alien out so the loop starts from a clean entry. We
		// approximate the entry as a point slightly to the side and below.
		entryX := float64(sx) + float64(a.side)*4.0
		entryY := float64(sy) + 3.0
		a.x = float64(sx) + (entryX-float64(sx))*easeOutCubic(u)
		a.y = float64(sy) + (entryY-float64(sy))*easeOutCubic(u)
		if a.phaseT >= dur {
			p.startLoop(a)
		}
		// Maybe fire (a small chance during the pullout too).
		if a.fireCD <= 0 && p.rng.Float64() < alienFireChancePerSec*s {
			p.alienFire(a)
		}

	case asLoop:
		// Full 360° loop with a steady downward translation, so the
		// alien spirals outward and ends up below the formation ready
		// for descent — never floating back up to formation height.
		u := clamp01(a.phaseT / diveLoopDur)
		th := a.loopTh0 + a.loopThSweep*easeInOutCubic(u)
		fall := u * diveLoopFallPx
		a.x = a.loopCx + diveLoopRadius*math.Cos(th)
		a.y = a.loopCy + diveLoopRadius*math.Sin(th) + fall
		if a.phaseT >= diveLoopDur {
			p.startDescent(a)
		}
		if a.fireCD <= 0 && p.rng.Float64() < alienFireChancePerSec*s {
			p.alienFire(a)
		}

	case asDescend:
		// Vertical fall with sinusoidal x and slight downward acceleration.
		a.descentVy += diveDescentAccel * s
		a.y += a.descentVy * s
		wave := math.Sin(a.descentPhase0+a.phaseT*2*math.Pi*diveWaveHz) * diveWaveAmp
		// Drift slightly toward the player to make dives feel deliberate.
		targetDrift := (p.player.x + float64(playerSprite.width())/2 - a.descentX0) * 0.06 * s
		a.descentX0 += targetDrift
		a.x = a.descentX0 + wave
		if a.fireCD <= 0 && p.rng.Float64() < alienFireChancePerSec*s {
			p.alienFire(a)
		}
		if a.y > float64(p.h)+4 {
			a.state = asExited
			a.phaseT = 0
		}

	case asReturning:
		// Quadratic Bezier from (retX0, retY0) → control → (slotX, slotY).
		sx, sy := p.aliensSlotPos(a)
		u := clamp01(a.phaseT / returnDur)
		// Control point pulls the curve outward so it looks like a curve
		// rather than a straight line.
		ctrlX := (a.retX0 + float64(sx)) / 2 + a.retCxOff
		ctrlY := (a.retY0 + float64(sy)) / 2
		a.x = bezier(u, a.retX0, ctrlX, float64(sx))
		a.y = bezier(u, a.retY0, ctrlY, float64(sy))
		if a.phaseT >= returnDur {
			a.state = asFormation
			a.phaseT = 0
			a.convoyID = 0
		}
	}
}

func (p *playScene) startLoop(a *alien) {
	a.state = asLoop
	a.phaseT = 0
	// Loop centre is on the OUTWARD side of the alien (relative to the
	// formation's centre line), so the circle hangs outboard and the
	// alien arcs out, around, and back. For side=+1 (right-half dive)
	// the centre is to the right of the alien; for side=-1 it's left.
	a.loopCx = a.x + float64(a.side)*diveLoopRadius
	a.loopCy = a.y
	a.loopTh0 = math.Atan2(a.y-a.loopCy, a.x-a.loopCx)
	// Sweep direction is chosen so the alien starts heading SOUTH (down)
	// from its current position. The tangent at angle θ when sweeping in
	// the positive θ direction is (−sin θ, cos θ); we want (0, +1).
	//
	//   side=+1 → alien is WEST of centre, θ_start = π, positive sweep
	//             tangent is (0, −1) (north). We need NEGATIVE sweep.
	//   side=−1 → alien is EAST of centre, θ_start = 0, positive sweep
	//             tangent is (0, +1) (south). We need POSITIVE sweep.
	// Both cases reduce to sweep = −side · 2π for a full revolution.
	a.loopThSweep = -float64(a.side) * 2 * math.Pi
}

func (p *playScene) startDescent(a *alien) {
	a.state = asDescend
	a.phaseT = 0
	a.descentX0 = a.x
	a.descentY0 = a.y
	a.descentVy = diveDescentVy
	a.descentPhase0 = p.rng.Float64() * 2 * math.Pi
}

func (p *playScene) startReturn(a *alien) {
	a.state = asReturning
	a.phaseT = 0
	// Enter from the top, on the same horizontal side the alien left from.
	sx, _ := p.aliensSlotPos(a)
	side := 1
	if float64(sx) < float64(p.w)/2 {
		side = -1
	}
	margin := float64(p.w) / 5
	a.retX0 = float64(p.w)/2 - float64(side)*(margin+p.rng.Float64()*margin)
	if a.retX0 < 2 {
		a.retX0 = 2
	}
	if a.retX0 > float64(p.w-2) {
		a.retX0 = float64(p.w - 2)
	}
	a.retY0 = returnEntryY
	// Control offset bows the bezier outward.
	a.retCxOff = float64(side) * (10 + p.rng.Float64()*8)
}

func (p *playScene) aliensSlotPos(a *alien) (int, int) {
	return p.slotPos(a.row, a.col)
}

// slotPos returns the pixel top-left of (row, col) in the current sway state.
func (p *playScene) slotPos(row, col int) (int, int) {
	sway := math.Sin(p.formationT*2*math.Pi*formationSwayHz) * formationSwayAmp
	cols := p.formationCols
	// Centre the row around formationCenterX.
	rowWidth := float64((cols-1)*formationColPitch + droneA.width())
	leftX := p.formationCenterX - rowWidth/2 + sway
	x := leftX + float64(col*formationColPitch)
	y := float64(p.formationY0 + row*p.formationRowPitch)
	return int(x), int(y)
}

// ---------------------------------------------------------------------
// Attack scheduling.
// ---------------------------------------------------------------------

// scheduleAttacks decides whether to peel a new alien (or convoy) off
// the formation this frame.
func (p *playScene) scheduleAttacks(s float64) {
	p.attackLaunchT -= s
	if p.attackLaunchT > 0 {
		return
	}
	// Reset timer with wave-adjusted cadence.
	scale := p.waveScale()
	gap := attackLaunchMin*scale + p.rng.Float64()*(attackLaunchMax-attackLaunchMin)*scale
	p.attackLaunchT = gap

	// Count current divers so we respect the on-screen cap.
	diving := 0
	for _, a := range p.aliens {
		if a.diving() || a.state == asExited {
			diving++
		}
	}
	limit := attackMaxOnScreen + p.wave - 1
	if limit > attackMaxCap {
		limit = attackMaxCap
	}
	if diving >= limit {
		return
	}

	// 1-in-4 chance to launch a flagship convoy if a flagship is
	// available (and we haven't yet this cycle).
	if p.rng.Intn(4) == 0 {
		if p.tryLaunchConvoy() {
			return
		}
	}
	p.launchSolo()
}

func (p *playScene) launchSolo() {
	// Prefer to launch aliens from the outer columns; pick a live
	// formation alien at random with bias toward the edges.
	candidates := []*alien{}
	for _, a := range p.aliens {
		if a.formationAlive() {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return
	}
	a := candidates[p.rng.Intn(len(candidates))]
	p.beginDive(a)
}

func (p *playScene) tryLaunchConvoy() bool {
	// Find a flagship in formation; convoy needs the flagship + 1 or 2
	// adjacent live bosses.
	var leader *alien
	for _, a := range p.aliens {
		if a.kind == kindFlagship && a.formationAlive() {
			leader = a
			break
		}
	}
	if leader == nil {
		return false
	}
	// Find adjacent bosses to act as escorts.
	var left, right *alien
	for _, a := range p.aliens {
		if a.kind != kindBoss || !a.formationAlive() {
			continue
		}
		// "Adjacent" means row 1 with column close to leader's column.
		if a.row != 1 {
			continue
		}
		dc := a.col - leader.col
		if dc == -1 || dc == 0 {
			if left == nil {
				left = a
			}
		}
		if dc == 1 || dc == 0 {
			if right == nil && a != left {
				right = a
			}
		}
	}
	p.nextConvoyID++
	cid := p.nextConvoyID
	leader.convoyID = cid
	leader.convoyRole = 0
	p.beginDive(leader)
	if left != nil {
		left.convoyID = cid
		left.convoyRole = 1
		// Escorts dive in lock-step with the leader on the same side so
		// the convoy reads as a single attacking flight.
		p.beginEscortDive(left, leader.side)
	}
	if right != nil {
		right.convoyID = cid
		right.convoyRole = 2
		p.beginEscortDive(right, leader.side)
	}
	return true
}

// beginDive transitions a formation alien into asPullout. Side is set
// from which half of the formation the alien occupies.
func (p *playScene) beginDive(a *alien) {
	sx, sy := p.aliensSlotPos(a)
	a.x = float64(sx)
	a.y = float64(sy)
	if float64(sx) < p.formationCenterX {
		a.side = -1
	} else {
		a.side = +1
	}
	a.state = asPullout
	a.phaseT = 0
	a.fireCD = 0.5 + p.rng.Float64()*0.6
}

// beginEscortDive launches an escort with side matched to the convoy
// leader. The escort runs the same dive math from its own formation
// slot — since the escort's slot is adjacent to the leader's, the
// resulting flight paths trace a loose V.
func (p *playScene) beginEscortDive(escort *alien, side int) {
	sx, sy := p.aliensSlotPos(escort)
	escort.x = float64(sx)
	escort.y = float64(sy)
	escort.side = side
	escort.state = asPullout
	// Stagger the escort's start by a small delay so they trail the
	// leader through the loop instead of overlapping it perfectly.
	escort.phaseT = -0.10 - 0.05*float64(escort.convoyRole)
	escort.fireCD = 0.7 + p.rng.Float64()*0.6
}

// ---------------------------------------------------------------------
// Bullets, fire, and stars.
// ---------------------------------------------------------------------

func (p *playScene) alienFire(a *alien) {
	if len(p.alienBullets) >= 12 {
		return
	}
	// Bullet origin: bottom-centre of the alien sprite.
	w := a.kind.spriteWidth()
	bx := a.x + float64(w)/2
	by := a.y + float64(a.kind.spriteHeight())
	// Aim slightly toward the player so the bullets feel directed.
	target := p.player.x + float64(playerSprite.width())/2
	dx := target - bx
	dy := float64(p.h) - by
	if dy < 1 {
		dy = 1
	}
	// Normalize aim vector and scale to bullet speed.
	mag := math.Sqrt(dx*dx + dy*dy)
	if mag < 0.01 {
		mag = 1
	}
	vx := dx / mag * alienBulletSpeed * 0.6 // keep mostly-down trajectory
	vy := alienBulletSpeed
	p.alienBullets = append(p.alienBullets, &bullet{
		x:  bx - 1, // centre 3-px bullet sprite
		y:  by,
		vx: vx,
		vy: vy,
	})
	a.fireCD = alienFireGap + p.rng.Float64()*0.8
}

func (p *playScene) tickBullets(s float64) {
	if b := p.player.bullet; b != nil {
		b.y += b.vy * s
		if b.y+float64(playerBulletSprite.height()) < 0 {
			p.player.bullet = nil
		}
	}
	kept := p.alienBullets[:0]
	for _, b := range p.alienBullets {
		b.age += s
		b.x += b.vx * s
		b.vy += alienBulletAccelY * s
		b.y += b.vy * s
		b.frameT += s
		if b.frameT >= 0.10 {
			b.frameT = 0
			b.frame = 1 - b.frame
		}
		if b.y < float64(p.h)+2 && b.x > -4 && b.x < float64(p.w)+4 {
			kept = append(kept, b)
		}
	}
	p.alienBullets = kept
}

func (p *playScene) tickStars(s float64) {
	for i := range p.stars {
		st := &p.stars[i]
		st.y += starScrollVy * s
		st.phase += s * starTwinkleHz
		if st.y >= float64(p.h) {
			st.y = -1
			st.x = p.rng.Float64() * float64(p.w)
			st.depth = p.rng.Intn(3)
			st.tint = p.rng.Intn(4)
		}
	}
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

// ---------------------------------------------------------------------
// Collision.
// ---------------------------------------------------------------------

type rect struct{ x0, y0, x1, y1 int }

func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && r.x1 > o.x0 && r.y0 < o.y1 && r.y1 > o.y0
}

func (k alienKind) spriteWidth() int {
	a, _ := k.frames()
	return a.width()
}

func (k alienKind) spriteHeight() int {
	a, _ := k.frames()
	return a.height()
}

func (p *playScene) resolveCollisions() {
	p.collidePlayerBullet()
	p.collideAlienBullets()
}

func (p *playScene) collidePlayerBullet() {
	b := p.player.bullet
	if b == nil {
		return
	}
	br := rect{
		x0: int(b.x),
		y0: int(b.y),
		x1: int(b.x) + playerBulletSprite.width(),
		y1: int(b.y) + playerBulletSprite.height(),
	}
	for _, a := range p.aliens {
		if !a.alive {
			continue
		}
		var ax, ay int
		if a.state == asFormation {
			ax, ay = p.slotPos(a.row, a.col)
		} else {
			ax, ay = int(a.x), int(a.y)
		}
		w := a.kind.spriteWidth()
		h := a.kind.spriteHeight()
		ar := rect{x0: ax, y0: ay, x1: ax + w, y1: ay + h}
		if !br.overlaps(ar) {
			continue
		}
		p.killAlien(a)
		p.player.bullet = nil
		return
	}
}

// killAlien handles scoring + convoy bonus + explosion spawn.
func (p *playScene) killAlien(a *alien) {
	diving := a.diving()
	switch a.kind {
	case kindFlagship:
		// Count live escorts in the convoy still on screen.
		if diving && a.convoyID != 0 {
			escorts := 0
			for _, e := range p.aliens {
				if e == a || !e.alive {
					continue
				}
				if e.convoyID == a.convoyID && e.diving() {
					escorts++
				}
			}
			if escorts > 2 {
				escorts = 2
			}
			p.score += flagshipBonusForEscorts[escorts]
		} else if diving {
			p.score += a.kind.divingScore()
		} else {
			p.score += a.kind.stationaryScore()
		}
	default:
		if diving {
			p.score += a.kind.divingScore()
		} else {
			p.score += a.kind.stationaryScore()
		}
	}
	a.alive = false
	// Spawn a small explosion at the alien's current position.
	var ex, ey int
	if a.state == asFormation {
		ex, ey = p.slotPos(a.row, a.col)
	} else {
		ex, ey = int(a.x), int(a.y)
	}
	// Centre the 5×5 explosion sprite on the alien.
	w := a.kind.spriteWidth()
	h := a.kind.spriteHeight()
	p.explosions = append(p.explosions, &explosion{
		x:   ex + w/2 - 2,
		y:   ey + h/2 - 2,
		dur: 0.36,
	})
}

func (p *playScene) collideAlienBullets() {
	kept := p.alienBullets[:0]
	for _, b := range p.alienBullets {
		br := rect{
			x0: int(b.x),
			y0: int(b.y),
			x1: int(b.x) + alienBulletA.width(),
			y1: int(b.y) + alienBulletA.height(),
		}
		if p.player.explodeT <= 0 {
			pr := rect{
				x0: int(p.player.x),
				y0: p.player.y,
				x1: int(p.player.x) + playerSprite.width(),
				y1: p.player.y + playerSprite.height(),
			}
			if br.overlaps(pr) {
				p.playerHit()
				continue
			}
		}
		kept = append(kept, b)
	}
	p.alienBullets = kept

	// Also: a diving alien colliding with the player kills both.
	if p.player.explodeT <= 0 {
		pr := rect{
			x0: int(p.player.x),
			y0: p.player.y,
			x1: int(p.player.x) + playerSprite.width(),
			y1: p.player.y + playerSprite.height(),
		}
		for _, a := range p.aliens {
			if !a.alive || !a.diving() {
				continue
			}
			ar := rect{
				x0: int(a.x),
				y0: int(a.y),
				x1: int(a.x) + a.kind.spriteWidth(),
				y1: int(a.y) + a.kind.spriteHeight(),
			}
			if ar.overlaps(pr) {
				p.killAlien(a)
				p.playerHit()
				return
			}
		}
	}
}

func (p *playScene) playerHit() {
	p.player.lives--
	p.player.explodeT = playerExplodeDur
	p.state = psPlayerHit
	p.stateT = 0
}

func (p *playScene) waveScale() float64 {
	scale := 1.0
	for i := 1; i < p.wave; i++ {
		scale *= waveSpeedup
	}
	return scale
}

// ---------------------------------------------------------------------
// Drawing.
// ---------------------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 2, G: 2, B: 10, A: 255})
	p.drawStars(c)
	p.drawHUD(c)
	p.drawAliens(c)
	p.drawPlayer(c)
	p.drawBullets(c)
	p.drawExplosions(c)
	switch p.state {
	case psWaveCleared:
		p.drawCentreBanner(c, fmt.Sprintf("STAGE %d CLEAR", p.wave), engine.Yellow)
	case psGameOver:
		p.drawGameOver(c)
	}
}

func (p *playScene) drawStars(c *engine.Canvas) {
	for _, s := range p.stars {
		brightness := 0.6 + 0.4*math.Sin(s.phase*2*math.Pi)
		switch s.depth {
		case 0:
			brightness *= 0.35
		case 1:
			brightness *= 0.7
		default:
			// already bright
		}
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
		col := engine.Color{
			R: uint8(float64(base.R) * brightness),
			G: uint8(float64(base.G) * brightness),
			B: uint8(float64(base.B) * brightness),
			A: 255,
		}
		c.Set(int(s.x), int(s.y), col)
	}
}

func (p *playScene) drawHUD(c *engine.Canvas) {
	cols := c.Cols()
	scoreText := fmt.Sprintf("SCORE %06d", p.score)
	hiText := fmt.Sprintf("HI %06d", p.hiScore)
	stageText := fmt.Sprintf("STAGE %d", p.wave)
	livesText := fmt.Sprintf("LIVES %d", p.player.lives)

	c.Print(1, 0, scoreText, engine.White)
	hiCol := (cols - len(hiText)) / 2
	if hiCol < len(scoreText)+2 {
		hiCol = len(scoreText) + 2
	}
	c.Print(hiCol, 0, hiText, engine.Yellow)
	rightCol := cols - len(stageText) - 1
	if rightCol < hiCol+len(hiText)+2 {
		rightCol = hiCol + len(hiText) + 2
	}
	c.Print(rightCol, 0, stageText, engine.Color{R: 120, G: 220, B: 255, A: 255})
	c.Print(1, 1, livesText, engine.Color{R: 140, G: 240, B: 160, A: 255})
}

func (p *playScene) drawAliens(c *engine.Canvas) {
	frameFlip := int(p.formationT*formationAnimHz) % 2
	for _, a := range p.aliens {
		if !a.alive {
			continue
		}
		if a.state == asExited {
			continue
		}
		var x, y int
		if a.state == asFormation {
			x, y = p.slotPos(a.row, a.col)
		} else {
			x, y = int(a.x), int(a.y)
		}
		fA, fB := a.kind.frames()
		frame := fA
		if frameFlip == 1 {
			frame = fB
		}
		drawColorSprite(c, x, y, frame, a.kind.palette())
	}
}

func (p *playScene) drawPlayer(c *engine.Canvas) {
	if p.player.lives <= 0 && p.state == psGameOver {
		return
	}
	if p.player.explodeT > 0 {
		t := playerExplodeDur - p.player.explodeT
		frame := playerExplodeA
		if int(t*10)%2 == 1 {
			frame = playerExplodeB
		}
		drawColorSprite(c, int(p.player.x), p.player.y, frame, playerExplodePalette)
		return
	}
	drawColorSprite(c, int(p.player.x), p.player.y, playerSprite, playerPalette)
}

func (p *playScene) drawBullets(c *engine.Canvas) {
	if b := p.player.bullet; b != nil {
		drawColorSprite(c, int(b.x), int(b.y), playerBulletSprite, playerBulletPalette)
	}
	for _, b := range p.alienBullets {
		spr := alienBulletA
		if b.frame == 1 {
			spr = alienBulletB
		}
		drawColorSprite(c, int(b.x), int(b.y), spr, alienBulletPalette)
	}
}

func (p *playScene) drawExplosions(c *engine.Canvas) {
	for _, e := range p.explosions {
		u := e.t / e.dur
		var frame colorSprite
		switch {
		case u < 0.33:
			frame = explodeA
		case u < 0.66:
			frame = explodeB
		default:
			frame = explodeC
		}
		drawColorSprite(c, e.x, e.y, frame, explodePalette)
	}
}

func (p *playScene) drawCentreBanner(c *engine.Canvas, text string, col engine.Color) {
	w := engine.TextWidth(text)
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2
	c.FillRect(x-3, y-2, w+6, engine.FontHeight+4, engine.Color{R: 6, G: 6, B: 18, A: 255})
	c.DrawText(x, y, text, col)
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	w := engine.TextWidth("GAME OVER")
	x := (p.w - w) / 2
	y := (p.h-engine.FontHeight)/2 - 4
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4, engine.Color{R: 6, G: 6, B: 18, A: 255})
	c.DrawText(x, y, "GAME OVER", engine.Color{R: 250, G: 80, B: 80, A: 255})
	hint := "ENTER PLAY AGAIN   ESC QUIT"
	c.Print((c.Cols()-len(hint))/2, c.Rows()/2+2, hint, engine.White)
}

// ---------------------------------------------------------------------
// Math helpers.
// ---------------------------------------------------------------------

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func easeOutCubic(t float64) float64 {
	t = 1 - t
	return 1 - t*t*t
}

func easeInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := 2*t - 2
	return 1 + u*u*u/2
}

func bezier(t, p0, p1, p2 float64) float64 {
	u := 1 - t
	return u*u*p0 + 2*u*t*p1 + t*t*p2
}
