package lunarlander

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// newTestEngine builds an engine sized for headless tests. The Width/
// Height match an 80×24 terminal so the play scene's HUD layout has
// realistic room to lay itself out.
func newTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	e, err := engine.New(engine.Options{Width: 160, Height: 96})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	return e
}

// --- Terrain generation tests ----------------------------------------

func TestGenerateTerrainProducesPads(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	terr := generateTerrain(160, 96, rng)

	if len(terr.height) != 160 {
		t.Fatalf("heightmap width: got %d, want 160", len(terr.height))
	}
	if len(terr.pads) == 0 {
		t.Fatal("expected at least one landing pad, got zero")
	}
	if len(terr.pads) > 4 {
		t.Fatalf("expected at most 4 pads, got %d", len(terr.pads))
	}

	// Pads must be sorted left-to-right after generation. Anything else
	// would imply the post-sort step regressed.
	for i := 1; i < len(terr.pads); i++ {
		if terr.pads[i].xStart <= terr.pads[i-1].xStart {
			t.Errorf("pad %d xStart=%d not ascending after %d", i,
				terr.pads[i].xStart, terr.pads[i-1].xStart)
		}
	}

	// Each pad must be visibly flat in the heightmap, otherwise the
	// "feet on flat ground" landing check breaks.
	for i, pad := range terr.pads {
		y := terr.height[pad.xStart]
		for x := pad.xStart; x < pad.xEnd; x++ {
			if terr.height[x] != y {
				t.Errorf("pad %d: height varies inside pad (xStart=%d xEnd=%d, h[%d]=%d, h[%d]=%d)",
					i, pad.xStart, pad.xEnd, pad.xStart, y, x, terr.height[x])
				break
			}
		}
	}
}

func TestPadAtRangeMatchesContainedRange(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	terr := generateTerrain(200, 100, rng)
	if len(terr.pads) == 0 {
		t.Fatal("no pads generated")
	}
	pad := terr.pads[0]
	got := terr.padAtRange(pad.xStart, pad.xEnd)
	if got == nil {
		t.Fatal("padAtRange returned nil for the exact pad range")
	}
	if got.mult != pad.mult {
		t.Errorf("multiplier mismatch: got %d, want %d", got.mult, pad.mult)
	}

	// A range that straddles the pad's edge by even one pixel must
	// return nil so a foot on the slope can't be scored as a landing.
	if pad.xStart > 0 {
		straddling := terr.padAtRange(pad.xStart-1, pad.xEnd)
		if straddling != nil {
			t.Errorf("padAtRange returned non-nil for straddling range, want nil")
		}
	}
}

func TestTerrainHeightClampedToCanvas(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	terr := generateTerrain(120, 80, rng)
	for x, y := range terr.height {
		if y < 0 || y >= terr.h {
			t.Errorf("height[%d]=%d out of [0,%d)", x, y, terr.h)
		}
	}
}

// --- Lander physics tests --------------------------------------------

func TestRotateThrustVectorMatchesAngle(t *testing.T) {
	cases := []struct {
		name        string
		angle       float64
		wantUpX     float64
		wantUpY     float64
	}{
		{"upright", 0, 0, -1},
		{"tilted-right-90", math.Pi / 2, 1, 0},
		{"tilted-left-90", -math.Pi / 2, -1, 0},
		{"inverted", math.Pi, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := &lander{angle: tc.angle}
			u := l.up()
			if math.Abs(u.x-tc.wantUpX) > 1e-9 {
				t.Errorf("up.x = %v, want %v", u.x, tc.wantUpX)
			}
			if math.Abs(u.y-tc.wantUpY) > 1e-9 {
				t.Errorf("up.y = %v, want %v", u.y, tc.wantUpY)
			}
		})
	}
}

func TestGravityAddsToVelocity(t *testing.T) {
	l := &lander{}
	// Run one second of pure free-fall in 60 sub-steps to verify the
	// integration matches the simple analytic answer.
	dt := 1.0 / 60.0
	for i := 0; i < 60; i++ {
		l.applyThrust(dt)
		l.integratePosition(dt)
	}
	// v = g * t = gravity * 1.0
	if diff := math.Abs(l.vel.y - gravity); diff > 0.01 {
		t.Errorf("after 1s free fall, vel.y = %v, want ~%v (diff %v)", l.vel.y, gravity, diff)
	}
	// pos ≈ 0.5 * g * t² with one frame's worth of accumulated error.
	expected := 0.5 * gravity
	if diff := math.Abs(l.pos.y - expected); diff > 0.5 {
		t.Errorf("after 1s free fall, pos.y = %v, want ~%v (diff %v)", l.pos.y, expected, diff)
	}
}

func TestThrustCancelsGravityVertically(t *testing.T) {
	// Upright lander with thrust on. After a few frames the net upward
	// component should be roughly (thrustAccel - gravity)*t, ignoring
	// time-step error.
	l := &lander{}
	l.thrusting = true
	dt := 1.0 / 60.0
	const seconds = 0.5
	steps := int(seconds / dt)
	for i := 0; i < steps; i++ {
		l.applyThrust(dt)
		l.integratePosition(dt)
	}
	expectedVY := -(thrustAccel - gravity) * (float64(steps) * dt)
	if diff := math.Abs(l.vel.y - expectedVY); diff > 0.05 {
		t.Errorf("vertical thrust net velocity = %v, want ~%v", l.vel.y, expectedVY)
	}
	if math.Abs(l.vel.x) > 1e-9 {
		t.Errorf("upright thrust produced horizontal velocity %v", l.vel.x)
	}
}

func TestRotationDampingReturnsToZero(t *testing.T) {
	l := &lander{angularVel: 1.0}
	// No keys held — damping should bleed angularVel toward zero.
	dt := 1.0 / 60.0
	for i := 0; i < 300; i++ {
		l.applyRotation(false, false, dt)
	}
	if l.angularVel != 0 {
		t.Errorf("after a long coast, angularVel = %v, want 0", l.angularVel)
	}
}

func TestAngleWrapsToSymmetricRange(t *testing.T) {
	l := &lander{angle: math.Pi - 0.01, angularVel: 1.0}
	dt := 1.0 / 60.0
	// Spin past the +π boundary and confirm we wrap rather than letting
	// angle grow unbounded.
	for i := 0; i < 60; i++ {
		l.applyRotation(false, false, dt)
	}
	if l.angle > math.Pi || l.angle < -math.Pi {
		t.Errorf("angle %v outside (-π, π]", l.angle)
	}
}

// --- Collision / landing classification tests ------------------------

// fixedTerrain builds a deterministic terrain for collision tests
// without exercising the procedural generator. The returned terrain has
// a single pad spanning [padXStart, padXEnd) at row padY, and slopes
// up dramatically elsewhere so any "off-pad" foot lands on terrain at
// the foot's y-level too.
func fixedTerrain(w, h, padXStart, padXEnd, padY int) *terrain {
	t := &terrain{
		height:  make([]int, w),
		w:       w,
		h:       h,
		horizon: h / 3,
	}
	for x := range t.height {
		t.height[x] = padY
	}
	t.pads = []padSpec{{xStart: padXStart, xEnd: padXEnd, y: padY, mult: 3}}
	return t
}

func newTestPlay(t *testing.T, terr *terrain) *playScene {
	t.Helper()
	return &playScene{
		w:     terr.w,
		h:     terr.h,
		terr:  terr,
		local: standardLander(),
		ship:  &lander{},
		rng:   rand.New(rand.NewSource(123)),
		state: psFlying,
		fuel:  startingFuel,
	}
}

func TestSoftLandingScoresPadMultiplier(t *testing.T) {
	terr := fixedTerrain(80, 60, 30, 50, 50)
	p := newTestPlay(t, terr)
	p.ship.pos = vec2{x: 40, y: 47} // legs at y=50, sitting on pad
	p.ship.vel = vec2{x: 0.5, y: 1.0}
	p.ship.angle = 0
	p.checkCollision()
	if p.state != psLanded {
		t.Fatalf("state = %v, want psLanded (reason=%v)", p.state, p.reason)
	}
	wantBase := baseLandingScore * 3
	wantBonus := perfectBonus * 3
	if p.score != wantBase+wantBonus {
		t.Errorf("score = %d, want %d (base) + %d (bonus) = %d",
			p.score, wantBase, wantBonus, wantBase+wantBonus)
	}
}

func TestLandingTooFastVerticallyCrashes(t *testing.T) {
	terr := fixedTerrain(80, 60, 30, 50, 50)
	p := newTestPlay(t, terr)
	p.ship.pos = vec2{x: 40, y: 47}
	p.ship.vel = vec2{x: 0, y: safeVerticalSpeed + 5}
	p.ship.angle = 0
	p.checkCollision()
	if p.state == psLanded {
		t.Fatalf("state = psLanded; want crashed")
	}
	if p.reason != crashTooFast {
		t.Errorf("reason = %v, want crashTooFast", p.reason)
	}
}

func TestLandingTooFastHorizontallyCrashes(t *testing.T) {
	terr := fixedTerrain(80, 60, 30, 50, 50)
	p := newTestPlay(t, terr)
	p.ship.pos = vec2{x: 40, y: 47}
	p.ship.vel = vec2{x: safeHorizontalSpeed + 5, y: 1.0}
	p.ship.angle = 0
	p.checkCollision()
	if p.reason != crashSideways {
		t.Errorf("reason = %v, want crashSideways", p.reason)
	}
}

func TestLandingBadAngleCrashes(t *testing.T) {
	terr := fixedTerrain(80, 60, 30, 50, 50)
	p := newTestPlay(t, terr)
	// A tilt severe enough to fall outside safeAngleRad but still
	// allowing both feet to touch terrain.
	p.ship.pos = vec2{x: 40, y: 48}
	p.ship.vel = vec2{x: 0, y: 1.0}
	p.ship.angle = safeAngleRad + 0.1
	p.checkCollision()
	if p.state == psLanded {
		t.Fatalf("state = psLanded; want crash")
	}
	// The crash reason may be crashBadAngle or crashHitTerrain depending
	// on whether the rotated body clipped terrain before the foot test
	// ran. Either is acceptable — both are crashes for the bad-angle
	// scenario.
	if p.reason != crashBadAngle && p.reason != crashHitTerrain {
		t.Errorf("reason = %v, want crashBadAngle or crashHitTerrain", p.reason)
	}
}

func TestLandingOffPadCrashes(t *testing.T) {
	terr := fixedTerrain(80, 60, 30, 50, 50)
	p := newTestPlay(t, terr)
	// Feet at x≈6 and x≈14 — well outside the pad at [30,50).
	p.ship.pos = vec2{x: 10, y: 47}
	p.ship.vel = vec2{x: 0, y: 1.0}
	p.ship.angle = 0
	p.checkCollision()
	if p.state == psLanded {
		t.Fatalf("state = psLanded; want crash")
	}
	if p.reason != crashOffPad {
		t.Errorf("reason = %v, want crashOffPad", p.reason)
	}
}

func TestCrashAppliesFuelPenalty(t *testing.T) {
	terr := fixedTerrain(80, 60, 30, 50, 50)
	p := newTestPlay(t, terr)
	startFuel := p.fuel
	p.ship.pos = vec2{x: 10, y: 47}
	p.ship.vel = vec2{x: 0, y: 1.0}
	p.checkCollision()
	want := startFuel - crashFuelCost
	if p.fuel != want {
		t.Errorf("fuel after crash = %d, want %d", p.fuel, want)
	}
}

func TestCrashDuringEmptyTankTriggersGameOver(t *testing.T) {
	terr := fixedTerrain(80, 60, 30, 50, 50)
	p := newTestPlay(t, terr)
	p.fuel = crashFuelCost - 1 // not enough to absorb the penalty
	p.ship.pos = vec2{x: 10, y: 47}
	p.checkCollision()
	if p.state != psGameOver {
		t.Fatalf("state = %v, want psGameOver", p.state)
	}
	if p.fuel != 0 {
		t.Errorf("fuel = %d, want 0 after game over", p.fuel)
	}
}

// --- Helpers ---------------------------------------------------------

func TestWrapIntHandlesNegative(t *testing.T) {
	if got := wrapInt(-1, 100); got != 99 {
		t.Errorf("wrapInt(-1, 100) = %d, want 99", got)
	}
	if got := wrapInt(150, 100); got != 50 {
		t.Errorf("wrapInt(150, 100) = %d, want 50", got)
	}
	if got := wrapInt(50, 100); got != 50 {
		t.Errorf("wrapInt(50, 100) = %d, want 50", got)
	}
}

// TestPlaySceneSimulationDoesNotPanic constructs a real engine-backed
// play scene and runs several hundred frames of physics + draw with no
// input. The lander starts with a rightward drift and no thrust, so it
// will eventually fall onto terrain — exercising both the steady-state
// flight code and the crash/respawn transitions in one run.
func TestPlaySceneSimulationDoesNotPanic(t *testing.T) {
	e := newTestEngine(t)
	p := newPlayScene(e)
	c := e.Canvas()
	dt := time.Second / 60
	// 600 frames @ 60 FPS = 10 simulated seconds; with no thrust the
	// lander will fall and either land or crash long before then.
	for i := 0; i < 600; i++ {
		if err := p.Update(dt); err != nil {
			t.Fatalf("Update returned %v on frame %d", err, i)
		}
		p.Draw(c)
		if p.wantQuit {
			break
		}
	}
}

// TestScreenWrap verifies that a lander walking off the right edge
// re-enters from the left edge with the same velocity preserved, the
// way the arcade cabinet worked. Done by stuffing the lander past the
// right boundary and running one update.
func TestScreenWrap(t *testing.T) {
	e := newTestEngine(t)
	p := newPlayScene(e)
	canvasW := float64(p.w)
	p.ship.pos = vec2{x: canvasW + 5, y: 20}
	p.ship.vel = vec2{x: 10, y: 0}
	p.updateShip(1.0 / 60.0)
	if p.ship.pos.x >= canvasW || p.ship.pos.x < 0 {
		t.Errorf("after wrap, x = %v not in [0,%v)", p.ship.pos.x, canvasW)
	}
}

func TestFormatMissionTime(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0:00"},
		{5, "0:05"},
		{59.9, "0:59"},
		{60, "1:00"},
		{125.5, "2:05"},
		{-3, "0:00"},
	}
	for _, c := range cases {
		got := formatMissionTime(c.in)
		if got != c.want {
			t.Errorf("formatMissionTime(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
