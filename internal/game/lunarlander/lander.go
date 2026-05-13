package lunarlander

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Physics tuning. All distances are canvas pixels, all times seconds.
// These values are deliberately forgiving: terminals can't deliver
// frame-accurate "just tap the key" inputs (auto-repeat introduces
// 100ms+ of slop on legacy terminals), so the simulation compensates
// with very gentle gravity, a high thrust-to-weight ratio, and a
// generous soft-landing envelope. The "perfect" tier stays narrow so
// the precision bonus still rewards a careful touchdown.
const (
	gravity         = 2.5  // px/s² downward acceleration
	thrustAccel     = 12.0 // px/s² along ship-up vector when thrust is held
	rotateSpeed     = 1.2  // rad/s while a rotate key is held
	rotateDamping   = 10.0 // rad/s² applied when no rotate key is held — bleeds spin smoothly to zero
	startingFuel    = 1000 // arbitrary "units"
	fuelBurnPerSec  = 28.0 // units consumed per second of full thrust
	crashFuelCost   = 80   // penalty applied to the global tank after a crash
	bonusFuelOnLand = 350  // refuel reward for a successful touchdown

	// Safe-landing thresholds. The "perfect" tier nests inside the
	// regular "safe" tier — landing inside the inner thresholds adds a
	// finesse bonus on top of the pad multiplier.
	safeVerticalSpeed   = 6.0
	safeHorizontalSpeed = 4.0
	safeAngleRad        = 0.80 // ~46°

	perfectVerticalSpeed   = 5.0
	perfectHorizontalSpeed = 2.5
	perfectAngleRad        = 0.12 // ~6.9°
)

// landerLocal is the lander's collection of vector points in its own
// frame: origin = centre of mass, +y = ship's belly, ship-up = -y.
// Each field is a vec2 in unrotated pixel-space.
//
// All edges, foot positions, and collision points are derived from
// these so the silhouette stays consistent between draw and collision.
type landerLocal struct {
	nose       vec2 // dome apex
	domeL      vec2
	domeR      vec2
	shoulderL  vec2 // top corners of descent stage
	shoulderR  vec2
	hipL       vec2 // bottom corners of descent stage
	hipR       vec2
	footL      vec2 // outer toe-tip of left footpad
	footLInner vec2 // inner toe-tip of left footpad
	footR      vec2
	footRInner vec2
	legL       vec2 // ankle joint where leg meets footpad
	legR       vec2
}

// landerFootOffsetY is the local-frame y of the leg/footpad row. Used
// by collision and HUD-altitude code to translate the ship's centre
// into a "feet on the ground" world y without redrawing the silhouette.
const landerFootOffsetY = 3

func standardLander() landerLocal {
	return landerLocal{
		nose:       vec2{0, -3},
		domeL:      vec2{-1, -2},
		domeR:      vec2{1, -2},
		shoulderL:  vec2{-2, -1},
		shoulderR:  vec2{2, -1},
		hipL:       vec2{-2, 1},
		hipR:       vec2{2, 1},
		legL:       vec2{-3, landerFootOffsetY},
		legR:       vec2{3, landerFootOffsetY},
		footL:      vec2{-4, landerFootOffsetY},
		footLInner: vec2{-2, landerFootOffsetY},
		footR:      vec2{4, landerFootOffsetY},
		footRInner: vec2{2, landerFootOffsetY},
	}
}

// vec2 is a 2-D float vector. The lander uses it both for local-frame
// vertex coordinates and rotated world-space positions.
type vec2 struct {
	x, y float64
}

// rotate returns v rotated by theta radians around the origin. With our
// canvas y-down convention, a positive theta rotates clockwise — i.e.
// the lander tilts to its right when angle increases.
func (v vec2) rotate(theta float64) vec2 {
	cs, sn := math.Cos(theta), math.Sin(theta)
	return vec2{
		x: v.x*cs - v.y*sn,
		y: v.x*sn + v.y*cs,
	}
}

// add translates v by another vector — used to convert rotated
// local-frame points into world coordinates.
func (v vec2) add(o vec2) vec2 { return vec2{v.x + o.x, v.y + o.y} }

// lander is the simulated craft. position is the centre of mass in
// world pixel coordinates; angle is the tilt off vertical in radians.
type lander struct {
	pos        vec2
	vel        vec2
	angle      float64
	angularVel float64
	thrusting  bool
	// flameT advances while thrust is on; the renderer uses it to
	// animate a flickering exhaust plume so a held boost doesn't render
	// as a static triangle.
	flameT float64
}

// up returns the lander's current ship-up direction in world space.
// Multiplying this by thrustAccel and adding to velocity is how the
// boost integrates.
func (l *lander) up() vec2 {
	// Ship-up in local frame is (0, -1); rotate that.
	return vec2{x: math.Sin(l.angle), y: -math.Cos(l.angle)}
}

// rotatedShape returns the lander silhouette transformed into world
// space. Used by both the renderer and the collision tester so the
// visual outline and the hit-box are guaranteed to agree.
func (l *lander) rotatedShape(local landerLocal) landerLocal {
	rot := func(p vec2) vec2 { return p.rotate(l.angle).add(l.pos) }
	return landerLocal{
		nose:       rot(local.nose),
		domeL:      rot(local.domeL),
		domeR:      rot(local.domeR),
		shoulderL:  rot(local.shoulderL),
		shoulderR:  rot(local.shoulderR),
		hipL:       rot(local.hipL),
		hipR:       rot(local.hipR),
		footL:      rot(local.footL),
		footLInner: rot(local.footLInner),
		footR:      rot(local.footR),
		footRInner: rot(local.footRInner),
		legL:       rot(local.legL),
		legR:       rot(local.legR),
	}
}

// bodyPoints returns the non-foot collision points of the lander —
// anything touching terrain here is a destroyed-ship event regardless
// of velocity or angle.
func bodyPoints(s landerLocal) []vec2 {
	return []vec2{s.nose, s.domeL, s.domeR, s.shoulderL, s.shoulderR, s.hipL, s.hipR}
}

// footPoints returns the two leg-foot points used by the soft-landing
// test. Order is left-then-right.
func footPoints(s landerLocal) []vec2 {
	return []vec2{
		// Use the leg-tip / outer-pad point for each foot; the inner
		// pad point is also collidable but the leg tip is the "lowest"
		// in the upright pose so it touches first.
		s.legL,
		s.legR,
	}
}

// applyThrust integrates a single physics frame's velocity change. dt
// is in seconds.
func (l *lander) applyThrust(dt float64) {
	if l.thrusting {
		u := l.up()
		l.vel.x += u.x * thrustAccel * dt
		l.vel.y += u.y * thrustAccel * dt
		l.flameT += dt
	}
	// Gravity is always on regardless of thrust state.
	l.vel.y += gravity * dt
}

// applyRotation integrates the lander's tilt for one frame, applying
// per-frame damping when neither rotate key is held so the ship
// gradually returns to a coast instead of spinning forever.
func (l *lander) applyRotation(rotLeft, rotRight bool, dt float64) {
	switch {
	case rotLeft && !rotRight:
		l.angularVel = -rotateSpeed
	case rotRight && !rotLeft:
		l.angularVel = rotateSpeed
	default:
		// Linear damp toward zero — quick enough to feel responsive,
		// slow enough that very small tilts can be coast-corrected.
		damp := rotateDamping * dt
		switch {
		case l.angularVel > damp:
			l.angularVel -= damp
		case l.angularVel < -damp:
			l.angularVel += damp
		default:
			l.angularVel = 0
		}
	}
	l.angle += l.angularVel * dt

	// Keep angle in (-π, π] so the upright check is symmetric.
	if l.angle > math.Pi {
		l.angle -= 2 * math.Pi
	}
	if l.angle < -math.Pi {
		l.angle += 2 * math.Pi
	}
}

// integratePosition steps position by dt seconds using the current
// velocity. Called after applyThrust so the velocity already includes
// this frame's acceleration.
func (l *lander) integratePosition(dt float64) {
	l.pos.x += l.vel.x * dt
	l.pos.y += l.vel.y * dt
}

// drawLander paints the lander outline plus thrust plume onto the
// canvas. World-space vertices are derived once and then connected with
// Bresenham line segments — there's no fill, matching the vector look
// of the original arcade cabinet.
func drawLander(c *engine.Canvas, l *lander, local landerLocal) {
	shape := l.rotatedShape(local)
	// Steel-grey hull — distinctly cooler and darker than the pale
	// star palette so the silhouette pops against the sky.
	bodyCol := engine.Color{R: 170, G: 178, B: 195, A: 255}

	// Dome (V apex with two legs back to the shoulders).
	drawSegment(c, shape.nose, shape.domeL, bodyCol)
	drawSegment(c, shape.nose, shape.domeR, bodyCol)
	drawSegment(c, shape.domeL, shape.shoulderL, bodyCol)
	drawSegment(c, shape.domeR, shape.shoulderR, bodyCol)
	drawSegment(c, shape.domeL, shape.domeR, bodyCol)

	// Descent stage rectangle.
	drawSegment(c, shape.shoulderL, shape.hipL, bodyCol)
	drawSegment(c, shape.shoulderR, shape.hipR, bodyCol)
	drawSegment(c, shape.shoulderL, shape.shoulderR, bodyCol)
	drawSegment(c, shape.hipL, shape.hipR, bodyCol)

	// Legs.
	drawSegment(c, shape.hipL, shape.legL, bodyCol)
	drawSegment(c, shape.hipR, shape.legR, bodyCol)

	// Foot pads — a short horizontal stripe per leg.
	drawSegment(c, shape.footL, shape.footLInner, bodyCol)
	drawSegment(c, shape.footR, shape.footRInner, bodyCol)

	if l.thrusting {
		drawFlame(c, l)
	}
}

// drawFlame draws an animated exhaust plume behind the lander. The
// plume direction is opposite of ship-up; length flickers per frame to
// suggest a live thrust without ever being still.
func drawFlame(c *engine.Canvas, l *lander) {
	// All flame geometry is expressed in the lander's local frame
	// (origin = ship centre, +y = belly-out). The plume hangs straight
	// down from the descent stage; rotation comes from the same matrix
	// the body uses, so a tilted ship's flame trails behind correctly.
	const anchorY = 1.5
	// Length wobbles per-frame so a held thrust visibly pulses rather
	// than rendering as a static triangle.
	osc := 0.5*math.Sin(l.flameT*22) + 0.5
	length := 2.5 + 2.0*osc

	rot := func(p vec2) vec2 { return p.rotate(l.angle).add(l.pos) }
	anchorW := rot(vec2{x: 0, y: anchorY})
	tipW := rot(vec2{x: 0, y: anchorY + length})
	leftW := rot(vec2{x: -1, y: anchorY + 0.5})
	rightW := rot(vec2{x: 1, y: anchorY + 0.5})

	// Outer plume — a hot orange-yellow triangle.
	outer := engine.Color{R: 255, G: 170, B: 50, A: 255}
	drawSegment(c, leftW, tipW, outer)
	drawSegment(c, rightW, tipW, outer)
	drawSegment(c, leftW, rightW, outer)

	// Inner core: a brighter, slightly shorter streak so the flame has
	// some depth instead of looking like a flat triangle.
	innerTip := rot(vec2{x: 0, y: anchorY + length*0.65})
	inner := engine.Color{R: 255, G: 240, B: 180, A: 255}
	drawSegment(c, anchorW, innerTip, inner)
}

// drawSegment draws a straight line between two world-space float
// points by truncating to integer canvas pixels. Slightly looser than
// the canvas's own DrawLine signature (which only accepts ints), but
// that's the whole reason this helper exists.
func drawSegment(c *engine.Canvas, a, b vec2, col engine.Color) {
	c.DrawLine(int(math.Round(a.x)), int(math.Round(a.y)),
		int(math.Round(b.x)), int(math.Round(b.y)), col)
}
