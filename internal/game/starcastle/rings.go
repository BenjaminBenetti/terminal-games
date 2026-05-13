package starcastle

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// segmentsPerRing is the original Star Castle's segment count per ring.
// Twelve segments == thirty degrees each, which is what the 1980
// cabinet drew.
const segmentsPerRing = 12

// numRings — outer, middle, inner.
const numRings = 3

// Ring indices, named for readability at call sites.
const (
	ringOuter  = 0
	ringMiddle = 1
	ringInner  = 2
)

// geometry holds every spatial constant the play and title scenes need
// to render the arena. It's recomputed from the canvas size at scene
// construction time and is stable for the lifetime of the scene.
type geometry struct {
	cx, cy float64

	outerOuterR, outerInnerR   float64
	middleOuterR, middleInnerR float64
	innerOuterR, innerInnerR   float64
	coreR                      float64
}

// computeGeometry derives the arena dimensions from canvas pixels.
// The original used a roughly 1:1 vector field; we mirror that by
// fitting the rings within the smaller of the two axes and capping
// the size so very large terminals don't get absurd rings.
func computeGeometry(w, h int) geometry {
	cx := float64(w) / 2
	cy := float64(h) / 2
	maxR := math.Min(float64(w)/2, float64(h)/2) - 4
	if maxR > 32 {
		maxR = 32
	}
	if maxR < 10 {
		maxR = 10
	}
	// Layout: three rings of equal thickness, small gaps between them,
	// solid core at the center.
	thick := maxR * 0.14
	gap := maxR * 0.05
	g := geometry{cx: cx, cy: cy}
	g.outerOuterR = maxR
	g.outerInnerR = g.outerOuterR - thick
	g.middleOuterR = g.outerInnerR - gap
	g.middleInnerR = g.middleOuterR - thick
	g.innerOuterR = g.middleInnerR - gap
	g.innerInnerR = g.innerOuterR - thick
	g.coreR = g.innerInnerR - gap - 0.5
	if g.coreR < 2 {
		g.coreR = 2
	}
	return g
}

// ringRadii returns (outerR, innerR) for ring idx using the geometry.
func ringRadii(g geometry, idx int) (float64, float64) {
	switch idx {
	case ringOuter:
		return g.outerOuterR, g.outerInnerR
	case ringMiddle:
		return g.middleOuterR, g.middleInnerR
	case ringInner:
		return g.innerOuterR, g.innerInnerR
	}
	return 0, 0
}

// segment is one of the twelve trapezoidal pieces of a ring. The
// timers are interpreted as countdowns: when alive==false and
// regenT<=0, the segment is eligible to come back; the ring updates
// it every frame.
type segment struct {
	alive  bool
	regenT float64 // seconds until it regenerates (counts down while dead)
	hitT   float64 // visual flash timer after being shot (>0 == flash)
}

// ring is one rotating ring of twelve segments. spinRate is signed
// (positive == counter-clockwise on the canvas since y grows
// downward, but visually that's anti-mathematical — see the comment
// inside Update).
type ring struct {
	index      int // 0/1/2 for outer/middle/inner — used to pick palettes
	angle      float64
	spinRate   float64 // rad/s
	segments   [segmentsPerRing]segment
	regenDelay float64 // seconds before a destroyed segment respawns
}

// newRing returns a ring at its starting orientation with every
// segment alive. The spin direction alternates per index so adjacent
// rings always counter-rotate, which is the visual signature of the
// original arcade game.
func newRing(idx int, g geometry) ring {
	r := ring{index: idx}
	// Original game: rings rotate at different rates, with adjacent
	// rings going opposite directions. Pick speeds that feel right at
	// 60 FPS in a small terminal — slow enough to track, fast enough
	// that alignments matter.
	speeds := [3]float64{0.45, 0.60, 0.85} // outer, middle, inner — rad/s
	// Alternate direction. Outer CCW, middle CW, inner CCW.
	dirs := [3]float64{+1, -1, +1}
	r.spinRate = speeds[idx] * dirs[idx]
	// Regen: outer rings repair faster than inner ones so destroying
	// the inner ring actually pays off — once you've burned a hole
	// through the inner ring, it stays gone the longest, giving you
	// time to hit the core.
	regens := [3]float64{4.5, 6.0, 8.0}
	r.regenDelay = regens[idx]
	// Stagger initial angles so the rings don't all start aligned.
	r.angle = float64(idx) * (math.Pi / 12)
	for i := range r.segments {
		r.segments[i].alive = true
	}
	_ = g
	return r
}

// segmentSpan returns the angular width of one segment (always
// 2π/12). Pulled out so callers don't have to remember the divisor.
func segmentSpan() float64 {
	return 2 * math.Pi / segmentsPerRing
}

// segmentAt returns the index of the segment a world-space angle
// falls into, given the ring's current rotation. The result is in
// [0, segmentsPerRing).
func segmentAt(r *ring, theta float64) int {
	span := segmentSpan()
	rel := theta - r.angle
	// Normalize into [0, 2π).
	rel = math.Mod(rel, 2*math.Pi)
	if rel < 0 {
		rel += 2 * math.Pi
	}
	idx := int(math.Floor(rel / span))
	if idx < 0 {
		idx = 0
	}
	if idx >= segmentsPerRing {
		idx = segmentsPerRing - 1
	}
	return idx
}

// updateRing advances ring rotation and regen timers. Returns nothing
// because the ring is mutated in place; segments that finish their
// regeneration become alive again.
func updateRing(r *ring, dt float64) {
	r.angle += r.spinRate * dt
	// Normalize angle to keep float precision stable over long runs.
	if r.angle > 2*math.Pi {
		r.angle -= 2 * math.Pi
	}
	if r.angle < -2*math.Pi {
		r.angle += 2 * math.Pi
	}
	for i := range r.segments {
		s := &r.segments[i]
		if s.hitT > 0 {
			s.hitT -= dt
		}
		if !s.alive {
			s.regenT -= dt
			if s.regenT <= 0 {
				s.alive = true
				s.hitT = 0
			}
		}
	}
}

// destroySegment marks a segment dead and schedules its regen. Caller
// owns the decision to award score / spawn particles.
func destroySegment(r *ring, idx int) {
	if idx < 0 || idx >= segmentsPerRing {
		return
	}
	s := &r.segments[idx]
	if !s.alive {
		return
	}
	s.alive = false
	s.regenT = r.regenDelay
	s.hitT = 0
}

// pointHitsRing reports whether the (x, y) point intersects any alive
// segment of ring r given a geometry g. Used for ship collisions and
// for mine-vs-segment checks. Returns the segment index for kill
// crediting, or -1 if no hit.
func pointHitsRing(r *ring, g geometry, x, y float64) int {
	dx := x - g.cx
	dy := y - g.cy
	d2 := dx*dx + dy*dy
	outerR, innerR := ringRadii(g, r.index)
	if d2 > outerR*outerR {
		return -1
	}
	if d2 < innerR*innerR {
		return -1
	}
	theta := math.Atan2(dy, dx)
	idx := segmentAt(r, theta)
	if r.segments[idx].alive {
		return idx
	}
	return -1
}

// drawRing renders ring r into c. Each pixel within the ring's bbox
// is tested for membership in an alive segment and painted with the
// supplied color (slightly brightened during a recent-hit flash).
//
// We render filled segments rather than just outlines: the half-block
// terminal canvas reads much better with filled wedges (outlines tend
// to alias into single-pixel arcs that look wrong). The original
// vector cabinet drew outlines, but at the resolution we have, fill
// is more legible.
func drawRing(c *engine.Canvas, r *ring, g geometry, color engine.Color) {
	outerR, innerR := ringRadii(g, r.index)
	if outerR <= 0 {
		return
	}
	x0 := int(math.Floor(g.cx - outerR - 1))
	y0 := int(math.Floor(g.cy - outerR - 1))
	x1 := int(math.Ceil(g.cx + outerR + 1))
	y1 := int(math.Ceil(g.cy + outerR + 1))
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 >= c.Width() {
		x1 = c.Width() - 1
	}
	if y1 >= c.Height() {
		y1 = c.Height() - 1
	}
	outer2 := outerR * outerR
	inner2 := innerR * innerR
	flashCol := engine.Color{R: 255, G: 255, B: 255, A: 255}

	for y := y0; y <= y1; y++ {
		dy := float64(y) + 0.5 - g.cy
		dy2 := dy * dy
		if dy2 > outer2 {
			continue
		}
		for x := x0; x <= x1; x++ {
			dx := float64(x) + 0.5 - g.cx
			d2 := dx*dx + dy2
			if d2 > outer2 || d2 < inner2 {
				continue
			}
			theta := math.Atan2(dy, dx)
			idx := segmentAt(r, theta)
			seg := &r.segments[idx]
			if !seg.alive {
				continue
			}
			col := color
			if seg.hitT > 0 {
				k := seg.hitT / segmentHitFlash
				if k > 1 {
					k = 1
				}
				col = lerpColor(color, flashCol, k)
			}
			c.Set(x, y, col)
		}
	}
}

// drawRingOutlines paints the radial dividing lines between segments
// — purely decorative, gives the rings a vector-arcade silhouette.
// Drawn after the filled segments so the dividers sit on top.
func drawRingOutlines(c *engine.Canvas, r *ring, g geometry, color engine.Color) {
	outerR, innerR := ringRadii(g, r.index)
	span := segmentSpan()
	for i := 0; i < segmentsPerRing; i++ {
		// Each radial divider lies at the boundary between segments i
		// and (i-1). Only draw it if at least one of the adjacent
		// segments is alive — a divider floating in empty space looks
		// wrong.
		prev := (i - 1 + segmentsPerRing) % segmentsPerRing
		if !r.segments[i].alive && !r.segments[prev].alive {
			continue
		}
		theta := r.angle + float64(i)*span
		x0 := g.cx + math.Cos(theta)*innerR
		y0 := g.cy + math.Sin(theta)*innerR
		x1 := g.cx + math.Cos(theta)*outerR
		y1 := g.cy + math.Sin(theta)*outerR
		c.DrawLine(int(x0), int(y0), int(x1), int(y1), color)
	}
}

// drawCore renders the central cannon — a small filled circle with a
// rotating triangle pointing at the player. If alive == false the
// core is drawn as broken (debris-ish look) instead.
func drawCore(c *engine.Canvas, g geometry, angle float64, alive bool, color engine.Color) {
	cx := int(g.cx)
	cy := int(g.cy)
	r := int(g.coreR)
	if r < 2 {
		r = 2
	}
	if !alive {
		// Broken core: a cross + dim ring.
		dim := engine.Color{R: color.R / 3, G: color.G / 3, B: color.B / 3, A: 255}
		c.DrawCircle(cx, cy, r, dim)
		c.DrawLine(cx-r, cy, cx+r, cy, dim)
		c.DrawLine(cx, cy-r, cx, cy+r, dim)
		return
	}
	c.FillCircle(cx, cy, r, color)
	// Aim arrow: a triangle pointing along angle, slightly longer than
	// the core radius so it sticks out a touch.
	nose := float64(r) + 1.5
	side := float64(r) * 0.55
	type p2 struct{ x, y float64 }
	rot := func(p p2) (int, int) {
		rx := p.x*math.Cos(angle) - p.y*math.Sin(angle)
		ry := p.x*math.Sin(angle) + p.y*math.Cos(angle)
		return int(g.cx + rx), int(g.cy + ry)
	}
	ax, ay := rot(p2{nose, 0})
	bx, by := rot(p2{-side, side})
	dx, dy := rot(p2{-side, -side})
	dark := engine.Color{R: 30, G: 30, B: 30, A: 255}
	c.DrawLine(ax, ay, bx, by, dark)
	c.DrawLine(bx, by, dx, dy, dark)
	c.DrawLine(dx, dy, ax, ay, dark)
}

// segmentHitFlash is how long a recently-shot segment glows white
// before fading — captures the brief CRT flash from the original.
const segmentHitFlash = 0.08

// lerpColor linearly interpolates from a to b by t∈[0,1].
func lerpColor(a, b engine.Color, t float64) engine.Color {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return engine.Color{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}
