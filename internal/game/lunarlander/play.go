package lunarlander

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// playState is the gameplay sub-state machine. A scene starts in
// psFlying, drops into psLanded or psCrashed on touchdown or impact,
// and may end in psGameOver once the fuel tank empties after a crash.
type playState int

const (
	psFlying playState = iota
	psLanded
	psCrashed
	psGameOver
)

// hudReserve is the number of pixel rows along the top of the canvas
// reserved for the heads-up display. Anything below this is playable
// sky.
const hudReserve = 8

// Scoring tuning. The base award is awarded for any safe touchdown;
// the precision bonus is granted only when speeds and angle are inside
// the (much tighter) perfect-landing envelope.
const (
	baseLandingScore = 50
	perfectBonus     = 100
)

// crashReason describes the cause of a busted attempt. Surfaced on the
// crash banner so the player can adjust on the next try.
type crashReason int

const (
	crashUnknown crashReason = iota
	crashTooFast
	crashSideways
	crashBadAngle
	crashOffPad
	crashHitTerrain
)

func (r crashReason) String() string {
	switch r {
	case crashTooFast:
		return "TOO FAST"
	case crashSideways:
		return "TOO MUCH SIDEWAYS"
	case crashBadAngle:
		return "BAD ANGLE"
	case crashOffPad:
		return "MISSED THE PAD"
	case crashHitTerrain:
		return "TERRAIN STRIKE"
	default:
		return "CRASHED"
	}
}

// particle is one short-lived debris speck for the crash explosion. Age
// counts up; once it reaches maxAge the slice element is dropped.
type particle struct {
	pos    vec2
	vel    vec2
	age    float64
	maxAge float64
	col    engine.Color
}

// landingResult records the points awarded by the most recent
// successful touchdown so the on-screen banner can break the number
// down for the player. Lives long enough to display on the next
// terrain's flight as a faint header until a new event replaces it.
type landingResult struct {
	pad     padSpec
	base    int
	bonus   int
	awarded int
	perfect bool
}

// playScene is the active gameplay scene. It owns the simulation, the
// terrain, the HUD, the camera-less viewport, and the state-transition
// glue back to the top-level title.
type playScene struct {
	e    *engine.Engine
	w, h int

	terr     *terrain
	ship     *lander
	local    landerLocal
	rng      *rand.Rand
	stars    []star
	missionT float64

	score   int
	fuel    int
	state   playState
	stateT  float64
	reason  crashReason
	result  *landingResult
	mission int // counter shown in HUD, 1-indexed

	particles []particle

	wantQuit bool
}

// star is a static background dot. Positions are pre-rolled so the sky
// doesn't twinkle every frame — that'd be too noisy at terminal
// resolution.
type star struct {
	x, y int
	col  engine.Color
}

// newPlayScene builds a play scene seeded against the current wall
// clock. The first terrain is generated immediately so the title hand-
// off renders a populated scene on the first frame.
func newPlayScene(e *engine.Engine) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		local:   standardLander(),
		fuel:    startingFuel,
		state:   psFlying,
		mission: 1,
	}
	p.stars = generateStars(p.w, p.h, p.rng)
	p.terr = generateTerrain(p.w, p.h, p.rng)
	p.spawnLander()
	return p
}

// generateStars rolls a sparse field of background stars in the upper
// 3/4 of the canvas (below that is terrain). Density scales with canvas
// area so a larger window doesn't look comparatively empty.
func generateStars(w, h int, rng *rand.Rand) []star {
	count := w * h / 90
	if count < 20 {
		count = 20
	}
	stars := make([]star, 0, count)
	for i := 0; i < count; i++ {
		x := rng.Intn(w)
		y := rng.Intn(h * 3 / 4)
		// Picking from a small palette of pale tints stops the field
		// from looking like a single line of white pixels.
		var col engine.Color
		switch rng.Intn(4) {
		case 0:
			col = engine.Color{R: 200, G: 200, B: 230, A: 255}
		case 1:
			col = engine.Color{R: 230, G: 230, B: 255, A: 255}
		case 2:
			col = engine.Color{R: 180, G: 180, B: 220, A: 255}
		default:
			col = engine.Color{R: 240, G: 220, B: 200, A: 255}
		}
		stars = append(stars, star{x: x, y: y, col: col})
	}
	return stars
}

// spawnLander seats a fresh lander above the terrain with a small
// rightward drift — same initial-state recipe as the arcade so the
// first mission isn't a free hover.
func (p *playScene) spawnLander() {
	p.ship = &lander{
		pos:   vec2{x: float64(p.w) / 4, y: float64(hudReserve) + 4},
		vel:   vec2{x: 4.0, y: 0},
		angle: 0,
	}
	p.particles = nil
	p.state = psFlying
	p.stateT = 0
	p.reason = crashUnknown
	p.missionT = 0
}

// Update drives input, physics, and state transitions. Returns ErrQuit
// only on a hard quit — soft "back to menu" is communicated via the
// wantQuit flag so the top-level scene can clean up its own state.
func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}

	s := dt.Seconds()
	p.stateT += s

	switch p.state {
	case psFlying:
		p.missionT += s
		p.updateShip(s)
		p.checkCollision()
		p.updateParticles(s)
	case psLanded:
		p.updateParticles(s)
		// After a brief celebration delay, accept input to launch the
		// next mission. Input handling reads stateT to enforce the
		// minimum hold so the player can't accidentally skip the score
		// breakdown.
	case psCrashed:
		p.updateParticles(s)
	case psGameOver:
		p.updateParticles(s)
	}
	return nil
}

// handleInput drains the discrete-event queue. The control bindings
// match the rest of the games in this collection: arrow keys for
// directional play, ESC for quit/back, ENTER for confirmation. Thrust
// is intentionally held-key (read in updateShip) rather than a discrete
// event so auto-repeat lag doesn't choke the boost.
func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psFlying:
			if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
			}
		case psLanded:
			switch k.Code {
			case engine.KeyEsc:
				p.wantQuit = true
			case engine.KeyEnter:
				if p.stateT >= 0.6 {
					p.nextMission()
				}
			case engine.KeyChar:
				switch k.Rune {
				case 'q', 'Q':
					p.wantQuit = true
				case ' ':
					if p.stateT >= 0.6 {
						p.nextMission()
					}
				}
			}
		case psCrashed:
			switch k.Code {
			case engine.KeyEsc:
				p.wantQuit = true
			case engine.KeyEnter:
				if p.stateT >= 1.2 {
					p.nextMission()
				}
			case engine.KeyChar:
				switch k.Rune {
				case 'q', 'Q':
					p.wantQuit = true
				case ' ', 'r', 'R':
					if p.stateT >= 1.2 {
						p.nextMission()
					}
				}
			}
		case psGameOver:
			switch k.Code {
			case engine.KeyEsc, engine.KeyEnter:
				p.wantQuit = true
			case engine.KeyChar:
				switch k.Rune {
				case 'q', 'Q', ' ', 'r', 'R':
					p.wantQuit = true
				}
			}
		}
	}
}

// updateShip integrates the lander for a single frame. Held-key input
// drives thrust + rotation; we burn fuel inversely-proportional to
// dt so a 30 FPS terminal and a 60 FPS terminal lose fuel at the same
// real-world rate.
func (p *playScene) updateShip(s float64) {
	rotLeft := p.e.IsKeyDown(engine.KeyLeft) ||
		p.e.IsCharDown('a') || p.e.IsCharDown('A')
	rotRight := p.e.IsKeyDown(engine.KeyRight) ||
		p.e.IsCharDown('d') || p.e.IsCharDown('D')
	thrusting := (p.e.IsKeyDown(engine.KeyUp) ||
		p.e.IsCharDown('w') || p.e.IsCharDown('W') ||
		p.e.IsCharDown(' ')) && p.fuel > 0

	p.ship.thrusting = thrusting
	if thrusting {
		burn := fuelBurnPerSec * s
		// Round upward when partial to make sure we don't see "fuel: 0
		// but still thrusting" frames near empty.
		used := int(math.Ceil(burn))
		if used > p.fuel {
			used = p.fuel
			p.ship.thrusting = false
		}
		p.fuel -= used
	}

	p.ship.applyRotation(rotLeft, rotRight, s)
	p.ship.applyThrust(s)
	p.ship.integratePosition(s)

	// Horizontal screen-wrap. The original arcade did this; it also
	// rescues a player whose rightward drift would otherwise push them
	// permanently off into the void.
	if p.ship.pos.x < 0 {
		p.ship.pos.x += float64(p.w)
	}
	if p.ship.pos.x >= float64(p.w) {
		p.ship.pos.x -= float64(p.w)
	}

	// Ceiling — bouncing off the top would feel arbitrary, so we just
	// clamp velocity instead. The terrain handles the bottom.
	if p.ship.pos.y < 1 {
		p.ship.pos.y = 1
		if p.ship.vel.y < 0 {
			p.ship.vel.y = 0
		}
	}
}

// checkCollision tests the lander against the terrain heightmap. Foot
// contact takes priority over body contact: any foot touching the
// surface counts as a landing attempt and gets judged by the soft-
// landing envelope. Body-point strikes are only a crash when *no*
// foot is touching — i.e. genuine nose- or side-first impacts.
//
// The priority matters because at wide safe-tilt angles the
// silhouette's hip on the low side will inevitably dip into terrain
// before the high side's foot can drop to it, so a body-first check
// would crash the player on what looks (and should be scored) like a
// hard but legal landing.
func (p *playScene) checkCollision() {
	shape := p.ship.rotatedShape(p.local)

	feet := footPoints(shape)
	if len(feet) != 2 {
		return
	}
	footLX := wrapInt(int(math.Round(feet[0].x)), p.w)
	footRX := wrapInt(int(math.Round(feet[1].x)), p.w)
	footLY := int(math.Round(feet[0].y))
	footRY := int(math.Round(feet[1].y))

	leftTouching := footLY >= p.terr.heightAt(footLX)
	rightTouching := footRY >= p.terr.heightAt(footRX)

	if leftTouching || rightTouching {
		// The padForFeet check still requires both x-positions to fall
		// inside the same pad — you can't catch one leg on a pad and
		// the other on a slope.
		pad := p.padForFeet(footLX, footRX)
		switch {
		case pad == nil:
			p.handleCrash(crashOffPad)
		case math.Abs(p.ship.angle) > safeAngleRad:
			p.handleCrash(crashBadAngle)
		case math.Abs(p.ship.vel.y) > safeVerticalSpeed:
			p.handleCrash(crashTooFast)
		case math.Abs(p.ship.vel.x) > safeHorizontalSpeed:
			p.handleCrash(crashSideways)
		default:
			p.handleLanding(*pad)
		}
		return
	}

	// Nothing on the feet. The only path to a crash now is a non-foot
	// body point striking terrain — nose-first or hard sideways impact
	// before the legs could swing into contact.
	for _, pt := range bodyPoints(shape) {
		ix := wrapInt(int(math.Round(pt.x)), p.w)
		if int(math.Round(pt.y)) >= p.terr.heightAt(ix) {
			p.handleCrash(crashHitTerrain)
			return
		}
	}
}

// padForFeet returns the pad that fully contains both foot x-positions,
// or nil if either foot is outside any pad. Wrap-around (foot x > foot
// x' because they straddle the screen seam) is handled by sorting first.
func (p *playScene) padForFeet(xa, xb int) *padSpec {
	lo, hi := xa, xb
	if lo > hi {
		lo, hi = hi, lo
	}
	return p.terr.padAtRange(lo, hi+1)
}

// wrapInt mirrors the lander's horizontal wrap behaviour for collision
// lookups so a craft straddling the seam still gets sensible heightmap
// reads.
func wrapInt(x, w int) int {
	if w <= 0 {
		return 0
	}
	x %= w
	if x < 0 {
		x += w
	}
	return x
}

// handleLanding finalises a successful touchdown: assigns score, banks
// the precision bonus if the envelope tightened, and refuels the tank a
// little so a finesse player can play longer than a brute-force one.
func (p *playScene) handleLanding(pad padSpec) {
	perfect := math.Abs(p.ship.vel.y) <= perfectVerticalSpeed &&
		math.Abs(p.ship.vel.x) <= perfectHorizontalSpeed &&
		math.Abs(p.ship.angle) <= perfectAngleRad
	bonus := 0
	if perfect {
		bonus = perfectBonus
	}
	award := (baseLandingScore + bonus) * pad.mult
	p.score += award
	p.fuel += bonusFuelOnLand
	if p.fuel > startingFuel*3 {
		p.fuel = startingFuel * 3
	}
	p.result = &landingResult{
		pad:     pad,
		base:    baseLandingScore * pad.mult,
		bonus:   bonus * pad.mult,
		awarded: award,
		perfect: perfect,
	}
	// Snap the ship to the pad with feet planted and velocity zero so
	// the freeze-frame looks resolved instead of frozen mid-bounce.
	p.ship.pos.y = float64(pad.y) - landerFootOffsetY
	p.ship.vel = vec2{}
	p.ship.angle = 0
	p.ship.angularVel = 0
	p.ship.thrusting = false
	p.state = psLanded
	p.stateT = 0
}

// handleCrash kicks off the explosion: particle burst, fuel penalty,
// transition into either crashed (still have fuel) or game-over (tank
// empty after the penalty applies).
func (p *playScene) handleCrash(reason crashReason) {
	p.reason = reason
	p.spawnCrashParticles()
	p.fuel -= crashFuelCost
	if p.fuel <= 0 {
		p.fuel = 0
		p.state = psGameOver
	} else {
		p.state = psCrashed
	}
	p.stateT = 0
	p.ship.thrusting = false
}

// spawnCrashParticles seeds ~30 debris particles radiating from the
// lander's centre. Each particle picks a random direction and speed; a
// slight downward bias makes the explosion read as ground-impact
// rather than free-space.
func (p *playScene) spawnCrashParticles() {
	const count = 36
	cx, cy := p.ship.pos.x, p.ship.pos.y
	for i := 0; i < count; i++ {
		theta := p.rng.Float64() * 2 * math.Pi
		speed := 12 + p.rng.Float64()*38
		vx := math.Cos(theta) * speed
		vy := math.Sin(theta)*speed - 8 // a kick up so a fountain shape develops
		age := 0.0
		life := 0.8 + p.rng.Float64()*0.9
		// Heat-tinted palette: bright yellow core, orange middle, red
		// edge — same convention as classic vector-game explosions.
		var col engine.Color
		switch p.rng.Intn(3) {
		case 0:
			col = engine.Color{R: 255, G: 235, B: 120, A: 255}
		case 1:
			col = engine.Color{R: 255, G: 150, B: 50, A: 255}
		default:
			col = engine.Color{R: 230, G: 70, B: 60, A: 255}
		}
		p.particles = append(p.particles, particle{
			pos:    vec2{x: cx, y: cy},
			vel:    vec2{x: vx, y: vy},
			age:    age,
			maxAge: life,
			col:    col,
		})
	}
}

// updateParticles advances each debris speck and prunes expired ones in
// place. Gravity applies so the fountain falls back to the ground.
func (p *playScene) updateParticles(s float64) {
	dst := p.particles[:0]
	for _, pt := range p.particles {
		pt.age += s
		if pt.age >= pt.maxAge {
			continue
		}
		pt.vel.y += gravity * s
		pt.pos.x += pt.vel.x * s
		pt.pos.y += pt.vel.y * s
		dst = append(dst, pt)
	}
	p.particles = dst
}

// nextMission is the "what happens after a landing or crash" hook. It
// regenerates terrain and reseats the lander, preserving score and
// fuel between attempts so the player feels a running campaign rather
// than disconnected rounds.
func (p *playScene) nextMission() {
	if p.fuel <= 0 {
		p.wantQuit = true
		return
	}
	p.mission++
	p.terr = generateTerrain(p.w, p.h, p.rng)
	p.spawnLander()
}

// --- Drawing ---------------------------------------------------------

// Draw paints the entire frame: backdrop, terrain, lander, particles,
// HUD, and whichever modal banner the current state warrants.
func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 14, A: 255})
	p.drawStars(c)
	p.drawTerrain(c)
	p.drawPadLabels(c)
	p.drawParticles(c)
	if p.state != psCrashed && p.state != psGameOver {
		drawLander(c, p.ship, p.local)
	}
	p.drawHUD(c)
	p.drawModal(c)
}

func (p *playScene) drawStars(c *engine.Canvas) {
	for _, s := range p.stars {
		// Don't paint stars beneath the terrain — they'd peek through
		// only if the terrain wasn't fully opaque, but skipping them is
		// cheap and avoids a rare overdraw bug if the terrain ever
		// renders with alpha later.
		if s.y >= p.terr.heightAt(s.x) {
			continue
		}
		c.Set(s.x, s.y, s.col)
	}
}

// drawTerrain paints the surface as a connected silhouette with a
// solid fill below it. A 1-px brighter rim is drawn along the top of
// each pad so they read as "metallic" against the gray rock.
func (p *playScene) drawTerrain(c *engine.Canvas) {
	groundCol := engine.Color{R: 90, G: 90, B: 110, A: 255}
	rimCol := engine.Color{R: 160, G: 160, B: 180, A: 255}

	for x := 0; x < p.w; x++ {
		hy := p.terr.heightAt(x)
		// Fill column from terrain top to canvas bottom.
		if hy < p.h {
			c.FillRect(x, hy, 1, p.h-hy, groundCol)
			c.Set(x, hy, rimCol)
		}
	}

	for _, pad := range p.terr.pads {
		col := padColor(pad.mult)
		c.FillRect(pad.xStart, pad.y, pad.width(), 1, col)
	}
}

// drawPadLabels paints the multiplier text below each pad in the same
// hue as the pad surface — small enough to feel like a panel decal but
// readable enough to choose targets at a glance.
func (p *playScene) drawPadLabels(c *engine.Canvas) {
	for _, pad := range p.terr.pads {
		label := fmt.Sprintf("%dX", pad.mult)
		labelCol := padColor(pad.mult)
		// Below the pad: convert the pixel y to a text-row index. If
		// there's no room beneath, fall back to placing the label above
		// the pad surface.
		row := (pad.y + 2) / 2
		if row >= c.Rows()-1 {
			row = (pad.y - 4) / 2
		}
		col := pad.xStart + (pad.width()-len(label))/2
		if col < 0 {
			col = 0
		}
		c.Print(col, row, label, labelCol)
	}
}

func padColor(mult int) engine.Color {
	switch mult {
	case 5:
		return engine.Color{R: 255, G: 90, B: 90, A: 255}
	case 4:
		return engine.Color{R: 255, G: 200, B: 80, A: 255}
	case 3:
		return engine.Color{R: 120, G: 230, B: 130, A: 255}
	default:
		return engine.Color{R: 110, G: 200, B: 255, A: 255}
	}
}

// drawParticles paints each surviving debris speck as a single pixel,
// fading toward black as it ages.
func (p *playScene) drawParticles(c *engine.Canvas) {
	for _, pt := range p.particles {
		t := pt.age / pt.maxAge
		if t > 1 {
			t = 1
		}
		k := 1.0 - t
		col := engine.Color{
			R: uint8(float64(pt.col.R) * k),
			G: uint8(float64(pt.col.G) * k),
			B: uint8(float64(pt.col.B) * k),
			A: 255,
		}
		c.Set(int(pt.pos.x), int(pt.pos.y), col)
	}
}

// drawHUD paints the readout strip across the top of the canvas. The
// telemetry block (right of the score row) colour-codes the speed
// readings green/orange/red according to how they compare against the
// soft-landing envelope, so the player can see at a glance whether
// they're inside the safe window.
func (p *playScene) drawHUD(c *engine.Canvas) {
	scoreText := fmt.Sprintf("SCORE %05d", p.score)
	fuelText := fmt.Sprintf("FUEL %4d", p.fuel)
	missionText := fmt.Sprintf("MISSION %d", p.mission)
	timeText := fmt.Sprintf("TIME %s", formatMissionTime(p.missionT))

	c.Print(1, 0, scoreText, engine.White)
	c.Print(1+len(scoreText)+3, 0, missionText, engine.Cyan)
	rightX := c.Cols() - len(timeText) - 1
	if rightX < 1+len(scoreText)+3+len(missionText)+3 {
		rightX = 1 + len(scoreText) + 3 + len(missionText) + 3
	}
	c.Print(rightX, 0, timeText, engine.Yellow)

	// Fuel is colour-tinted as it drains so the player can panic
	// proportionally to the gauge.
	fuelCol := engine.Color{R: 130, G: 240, B: 150, A: 255}
	switch {
	case p.fuel <= startingFuel/5:
		fuelCol = engine.Color{R: 255, G: 90, B: 90, A: 255}
	case p.fuel <= startingFuel*2/5:
		fuelCol = engine.Color{R: 255, G: 200, B: 80, A: 255}
	}
	c.Print(1, 1, fuelText, fuelCol)

	// Telemetry strip.
	alt := p.altitude()
	hSpd := p.ship.vel.x
	vSpd := p.ship.vel.y

	altText := fmt.Sprintf("ALT %03d", alt)
	hSpdText := fmt.Sprintf("H-SPD %+05.1f", hSpd)
	vSpdText := fmt.Sprintf("V-SPD %+05.1f", vSpd)

	telemetryStart := 1 + len(fuelText) + 3
	c.Print(telemetryStart, 1, altText, engine.White)
	c.Print(telemetryStart+len(altText)+3, 1, hSpdText, speedColor(math.Abs(hSpd), safeHorizontalSpeed))
	c.Print(telemetryStart+len(altText)+3+len(hSpdText)+3, 1, vSpdText, speedColor(math.Abs(vSpd), safeVerticalSpeed))

	// Angle indicator (small text near right edge). Useful given the
	// blocky lander silhouette can be hard to read at small terminal
	// sizes.
	deg := p.ship.angle * 180 / math.Pi
	angleText := fmt.Sprintf("TILT %+04.0f", deg)
	c.Print(c.Cols()-len(angleText)-1, 1, angleText, tiltColor(math.Abs(deg)))

	// Faint separator line so the HUD reads as its own panel.
	c.FillRect(0, hudReserve-2, p.w, 1, engine.Color{R: 30, G: 30, B: 50, A: 255})
}

// speedColor returns a status colour for a speed reading: green inside
// the safe envelope, orange when within 1.5× the envelope (a warning),
// red beyond.
func speedColor(abs, safe float64) engine.Color {
	switch {
	case abs <= safe*0.6:
		return engine.Color{R: 130, G: 240, B: 150, A: 255}
	case abs <= safe:
		return engine.Color{R: 255, G: 230, B: 130, A: 255}
	default:
		return engine.Color{R: 255, G: 100, B: 100, A: 255}
	}
}

// tiltColor mirrors speedColor for the angle readout, in degrees. The
// safe-landing constant lives in radians, so convert once here so the
// HUD threshold tracks whatever the simulation actually enforces.
func tiltColor(absDeg float64) engine.Color {
	safeDeg := safeAngleRad * 180 / math.Pi
	switch {
	case absDeg <= safeDeg*0.6:
		return engine.Color{R: 130, G: 240, B: 150, A: 255}
	case absDeg <= safeDeg:
		return engine.Color{R: 255, G: 230, B: 130, A: 255}
	default:
		return engine.Color{R: 255, G: 100, B: 100, A: 255}
	}
}

// altitude returns the player's clearance over the column directly
// below the lander. Equivalent to "how far the feet would still need
// to fall before contact", which is the readout the original cabinet
// surfaced as ALT.
func (p *playScene) altitude() int {
	ix := wrapInt(int(math.Round(p.ship.pos.x)), p.w)
	terrainY := p.terr.heightAt(ix)
	// Use the leg-foot offset so altitude reads "0" exactly when the
	// upright lander would plant on flat ground at this column.
	bottom := p.ship.pos.y + landerFootOffsetY
	alt := terrainY - int(math.Round(bottom))
	if alt < 0 {
		alt = 0
	}
	return alt
}

// formatMissionTime turns an elapsed-seconds float into a M:SS string —
// the format on every classic arcade screen.
func formatMissionTime(t float64) string {
	if t < 0 {
		t = 0
	}
	total := int(t)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// drawModal paints whichever banner the current scene state warrants.
// All banners are centred horizontally and live below the HUD.
func (p *playScene) drawModal(c *engine.Canvas) {
	switch p.state {
	case psLanded:
		p.drawLandedBanner(c)
	case psCrashed:
		p.drawCrashBanner(c)
	case psGameOver:
		p.drawGameOverBanner(c)
	}
}

func (p *playScene) drawLandedBanner(c *engine.Canvas) {
	col := engine.Color{R: 130, G: 240, B: 150, A: 255}
	if p.result != nil && p.result.perfect {
		col = engine.Color{R: 255, G: 240, B: 130, A: 255}
	}
	title := "LANDED"
	if p.result != nil && p.result.perfect {
		title = "PERFECT LANDING"
	}
	w := engine.TextWidth(title)
	x := (c.Width() - w) / 2
	y := c.Height()/2 - engine.FontHeight
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4, engine.Color{R: 6, G: 14, B: 10, A: 255})
	c.DrawText(x, y, title, col)

	if p.result != nil {
		breakdown := fmt.Sprintf("%dX PAD   +%d", p.result.pad.mult, p.result.awarded)
		bx := (c.Cols() - len(breakdown)) / 2
		br := y/2 + engine.FontHeight/2 + 2
		c.Print(bx, br, breakdown, engine.White)

		if p.result.bonus > 0 {
			pb := fmt.Sprintf("PRECISION BONUS +%d", p.result.bonus)
			pbx := (c.Cols() - len(pb)) / 2
			c.Print(pbx, br+1, pb, engine.Yellow)
		}
	}

	hint := "ENTER / SPACE  CONTINUE"
	hx := (c.Cols() - len(hint)) / 2
	c.Print(hx, c.Rows()-2, hint, engine.Gray)
}

func (p *playScene) drawCrashBanner(c *engine.Canvas) {
	title := "CRASHED"
	col := engine.Color{R: 255, G: 100, B: 100, A: 255}
	w := engine.TextWidth(title)
	x := (c.Width() - w) / 2
	y := c.Height()/2 - engine.FontHeight
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4, engine.Color{R: 14, G: 6, B: 6, A: 255})
	c.DrawText(x, y, title, col)

	reasonText := p.reason.String()
	rx := (c.Cols() - len(reasonText)) / 2
	rr := y/2 + engine.FontHeight/2 + 2
	c.Print(rx, rr, reasonText, engine.White)

	// Show the impact stats — ship state is frozen the moment the crash
	// fires so reading directly off p.ship gives the values at impact.
	// Each readout is colour-coded against the same thresholds the
	// simulation enforces, so the over-limit reading lights up red.
	p.drawCrashStats(c, rr+2)

	penalty := fmt.Sprintf("-%d FUEL", crashFuelCost)
	px := (c.Cols() - len(penalty)) / 2
	c.Print(px, rr+4, penalty, engine.Color{R: 255, G: 200, B: 80, A: 255})

	if p.stateT >= 1.2 {
		hint := "ENTER / SPACE  RETRY"
		hx := (c.Cols() - len(hint)) / 2
		c.Print(hx, c.Rows()-2, hint, engine.Gray)
	}
}

// drawCrashStats paints the three impact readings centred on row r:
// V-SPD, H-SPD, TILT. The exact same colour ramp as the in-flight HUD
// is used so the player learns to read "red on the banner" the same
// way as "red on the gauge".
func (p *playScene) drawCrashStats(c *engine.Canvas, row int) {
	vSpd := p.ship.vel.y
	hSpd := p.ship.vel.x
	tiltDeg := p.ship.angle * 180 / math.Pi

	vText := fmt.Sprintf("V-SPD %+05.1f", vSpd)
	hText := fmt.Sprintf("H-SPD %+05.1f", hSpd)
	tText := fmt.Sprintf("TILT %+04.0f", tiltDeg)

	const sep = "   "
	total := len(vText) + len(sep) + len(hText) + len(sep) + len(tText)
	startCol := (c.Cols() - total) / 2
	if startCol < 0 {
		startCol = 0
	}

	col := startCol
	c.Print(col, row, vText, speedColor(math.Abs(vSpd), safeVerticalSpeed))
	col += len(vText) + len(sep)
	c.Print(col, row, hText, speedColor(math.Abs(hSpd), safeHorizontalSpeed))
	col += len(hText) + len(sep)
	c.Print(col, row, tText, tiltColor(math.Abs(tiltDeg)))
}

func (p *playScene) drawGameOverBanner(c *engine.Canvas) {
	title := "FUEL EXHAUSTED"
	col := engine.Color{R: 255, G: 90, B: 90, A: 255}
	w := engine.TextWidth(title)
	x := (c.Width() - w) / 2
	y := c.Height()/2 - engine.FontHeight
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4, engine.Color{R: 14, G: 6, B: 6, A: 255})
	c.DrawText(x, y, title, col)

	totals := fmt.Sprintf("FINAL SCORE  %05d", p.score)
	tx := (c.Cols() - len(totals)) / 2
	tr := y/2 + engine.FontHeight/2 + 2
	c.Print(tx, tr, totals, engine.White)

	hint := "ENTER  RETURN TO MENU"
	hx := (c.Cols() - len(hint)) / 2
	c.Print(hx, c.Rows()-2, hint, engine.Gray)
}
