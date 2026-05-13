package galaga

import "math"

// vec2 is a 2D float position used for sub-pixel motion along curves.
type vec2 struct{ x, y float64 }

func (v vec2) sub(o vec2) vec2 { return vec2{v.x - o.x, v.y - o.y} }
func (v vec2) add(o vec2) vec2 { return vec2{v.x + o.x, v.y + o.y} }
func (v vec2) scale(k float64) vec2 {
	return vec2{v.x * k, v.y * k}
}
func (v vec2) len() float64 { return math.Sqrt(v.x*v.x + v.y*v.y) }
func (v vec2) norm() vec2 {
	l := v.len()
	if l == 0 {
		return vec2{0, 0}
	}
	return vec2{v.x / l, v.y / l}
}

// bezierSeg is one cubic Bezier segment from p0 to p3 with control
// points p1, p2.
type bezierSeg struct{ p0, p1, p2, p3 vec2 }

// at evaluates the segment at parameter t ∈ [0, 1].
func (s bezierSeg) at(t float64) vec2 {
	u := 1 - t
	uu := u * u
	uuu := uu * u
	tt := t * t
	ttt := tt * t
	return vec2{
		x: uuu*s.p0.x + 3*uu*t*s.p1.x + 3*u*tt*s.p2.x + ttt*s.p3.x,
		y: uuu*s.p0.y + 3*uu*t*s.p1.y + 3*u*tt*s.p2.y + ttt*s.p3.y,
	}
}

// arcLen approximates the length of this segment by sampling.
func (s bezierSeg) arcLen(samples int) float64 {
	if samples < 1 {
		samples = 1
	}
	var total float64
	prev := s.p0
	for i := 1; i <= samples; i++ {
		t := float64(i) / float64(samples)
		cur := s.at(t)
		total += cur.sub(prev).len()
		prev = cur
	}
	return total
}

// path is a sequence of cubic Bezier segments traversed at a constant
// (pixel-per-second) speed regardless of the curve's local sharpness.
// We precompute per-segment arc lengths so a global arc-length parameter
// d maps cleanly to a (segment, local t) tuple.
type path struct {
	segs    []bezierSeg
	lengths []float64 // cumulative arc length up to and including segs[i]
	total   float64
}

func newPath(segs []bezierSeg) *path {
	p := &path{segs: segs}
	cum := 0.0
	for _, s := range segs {
		cum += s.arcLen(32)
		p.lengths = append(p.lengths, cum)
	}
	p.total = cum
	return p
}

// at returns the canvas position d pixels along the path. Distances
// past total clamp to the path's end point; distances ≤ 0 clamp to the
// start. This lets callers integrate position by distance += speed * dt
// without worrying about overshoot bookkeeping.
func (p *path) at(d float64) vec2 {
	if len(p.segs) == 0 {
		return vec2{}
	}
	if d <= 0 {
		return p.segs[0].p0
	}
	if d >= p.total {
		last := p.segs[len(p.segs)-1]
		return last.p3
	}
	prevCum := 0.0
	for i, cum := range p.lengths {
		if d <= cum {
			segLen := cum - prevCum
			local := 0.0
			if segLen > 0 {
				local = (d - prevCum) / segLen
			}
			// We approximated each segment as having uniform speed in t,
			// but Bezier arc length isn't uniform in t. Refine local using
			// one Newton step against a finer length estimate. For our
			// purposes the visual artifact of skipping this is small, so
			// keep the simple linear approximation for now.
			return p.segs[i].at(local)
		}
		prevCum = cum
	}
	last := p.segs[len(p.segs)-1]
	return last.p3
}

// tangent returns a unit vector pointing along the direction of motion at
// arc-length d. Used for orienting an enemy sprite along its flight path
// when we want a "diving" pose, though sprites in this build don't
// rotate so the result is mostly informational.
func (p *path) tangent(d float64) vec2 {
	a := p.at(d)
	b := p.at(d + 0.5)
	t := b.sub(a)
	if t.len() == 0 {
		return vec2{0, 1}
	}
	return t.norm()
}

// --- Canonical paths ---------------------------------------------------
//
// The four classic Galaga entry patterns and their dive equivalents.
// Each builder takes the canvas dimensions and any per-enemy params and
// returns a path. Coordinates are in canvas pixels.

// entryTopLoopLeft: enemy emerges from above-screen on the left, loops
// once heading right and downward, then sweeps up to its formation slot.
// (targetX, targetY) is the formation slot's current pixel position.
func entryTopLoopLeft(canvasW, canvasH int, targetX, targetY float64) *path {
	_ = canvasH
	return newPath([]bezierSeg{
		{
			p0: vec2{-8, -8},
			p1: vec2{float64(canvasW) * 0.25, -8},
			p2: vec2{float64(canvasW) * 0.30, float64(canvasH) * 0.55},
			p3: vec2{float64(canvasW) * 0.20, float64(canvasH) * 0.60},
		},
		{
			p0: vec2{float64(canvasW) * 0.20, float64(canvasH) * 0.60},
			p1: vec2{float64(canvasW) * 0.05, float64(canvasH) * 0.50},
			p2: vec2{float64(canvasW) * 0.05, float64(canvasH) * 0.30},
			p3: vec2{float64(canvasW) * 0.25, float64(canvasH) * 0.30},
		},
		{
			p0: vec2{float64(canvasW) * 0.25, float64(canvasH) * 0.30},
			p1: vec2{float64(canvasW) * 0.45, float64(canvasH) * 0.35},
			p2: vec2{targetX - 14, targetY - 6},
			p3: vec2{targetX, targetY},
		},
	})
}

// entryTopLoopRight: mirror of entryTopLoopLeft. Enemy comes from the
// upper right, loops left and back, finishes at the formation slot.
func entryTopLoopRight(canvasW, canvasH int, targetX, targetY float64) *path {
	_ = canvasH
	return newPath([]bezierSeg{
		{
			p0: vec2{float64(canvasW) + 8, -8},
			p1: vec2{float64(canvasW) * 0.75, -8},
			p2: vec2{float64(canvasW) * 0.70, float64(canvasH) * 0.55},
			p3: vec2{float64(canvasW) * 0.80, float64(canvasH) * 0.60},
		},
		{
			p0: vec2{float64(canvasW) * 0.80, float64(canvasH) * 0.60},
			p1: vec2{float64(canvasW) * 0.95, float64(canvasH) * 0.50},
			p2: vec2{float64(canvasW) * 0.95, float64(canvasH) * 0.30},
			p3: vec2{float64(canvasW) * 0.75, float64(canvasH) * 0.30},
		},
		{
			p0: vec2{float64(canvasW) * 0.75, float64(canvasH) * 0.30},
			p1: vec2{float64(canvasW) * 0.55, float64(canvasH) * 0.35},
			p2: vec2{targetX + 14, targetY - 6},
			p3: vec2{targetX, targetY},
		},
	})
}

// entryBottomSweepLeft: enemy zooms up from the lower-left, arcs across
// the centre, and settles into formation. Used for bees in classic
// Galaga's third entry wave.
func entryBottomSweepLeft(canvasW, canvasH int, targetX, targetY float64) *path {
	return newPath([]bezierSeg{
		{
			p0: vec2{-8, float64(canvasH) + 8},
			p1: vec2{float64(canvasW) * 0.10, float64(canvasH) * 0.85},
			p2: vec2{float64(canvasW) * 0.30, float64(canvasH) * 0.45},
			p3: vec2{float64(canvasW) * 0.45, float64(canvasH) * 0.45},
		},
		{
			p0: vec2{float64(canvasW) * 0.45, float64(canvasH) * 0.45},
			p1: vec2{float64(canvasW) * 0.55, float64(canvasH) * 0.45},
			p2: vec2{targetX - 8, targetY + 6},
			p3: vec2{targetX, targetY},
		},
	})
}

// entryBottomSweepRight: mirror of entryBottomSweepLeft.
func entryBottomSweepRight(canvasW, canvasH int, targetX, targetY float64) *path {
	return newPath([]bezierSeg{
		{
			p0: vec2{float64(canvasW) + 8, float64(canvasH) + 8},
			p1: vec2{float64(canvasW) * 0.90, float64(canvasH) * 0.85},
			p2: vec2{float64(canvasW) * 0.70, float64(canvasH) * 0.45},
			p3: vec2{float64(canvasW) * 0.55, float64(canvasH) * 0.45},
		},
		{
			p0: vec2{float64(canvasW) * 0.55, float64(canvasH) * 0.45},
			p1: vec2{float64(canvasW) * 0.45, float64(canvasH) * 0.45},
			p2: vec2{targetX + 8, targetY + 6},
			p3: vec2{targetX, targetY},
		},
	})
}

// entryTopDirect: simple curved descent from the top to the slot. Used
// when we need a plain "fly in" without the dramatic loop.
func entryTopDirect(canvasW, canvasH int, targetX, targetY float64) *path {
	_ = canvasW
	startX := targetX
	return newPath([]bezierSeg{
		{
			p0: vec2{startX, -10},
			p1: vec2{startX, float64(canvasH) * 0.20},
			p2: vec2{targetX, float64(canvasH) * 0.40},
			p3: vec2{targetX, targetY},
		},
	})
}

// diveStraight: enemy plunges from (startX, startY) toward the player's
// current x at the bottom of the screen, ending below the canvas so the
// enemy disappears cleanly.
func diveStraight(canvasW, canvasH int, startX, startY, playerX float64) *path {
	_ = canvasW
	return newPath([]bezierSeg{
		{
			p0: vec2{startX, startY},
			p1: vec2{startX, startY + 8},
			p2: vec2{playerX + 4, float64(canvasH) * 0.70},
			p3: vec2{playerX, float64(canvasH) + 8},
		},
	})
}

// diveSwoop: enemy peels off sideways before turning down, ending below
// the canvas. Creates the satisfying "fly out, then dive" curve.
func diveSwoop(canvasW, canvasH int, startX, startY, playerX float64, dir int) *path {
	_ = canvasW
	side := float64(dir) * 16
	return newPath([]bezierSeg{
		{
			p0: vec2{startX, startY},
			p1: vec2{startX + side, startY + 4},
			p2: vec2{startX + side*1.4, float64(canvasH) * 0.45},
			p3: vec2{startX + side*0.6, float64(canvasH) * 0.55},
		},
		{
			p0: vec2{startX + side*0.6, float64(canvasH) * 0.55},
			p1: vec2{playerX - side*0.2, float64(canvasH) * 0.70},
			p2: vec2{playerX, float64(canvasH) * 0.85},
			p3: vec2{playerX - side*0.3, float64(canvasH) + 8},
		},
	})
}

// diveLoop: enemy loops underneath then drops. Bosses use this when not
// attempting a tractor-beam capture.
func diveLoop(canvasW, canvasH int, startX, startY, playerX float64, dir int) *path {
	side := float64(dir)
	cw := float64(canvasW)
	return newPath([]bezierSeg{
		{
			p0: vec2{startX, startY},
			p1: vec2{startX + side*20, startY + 6},
			p2: vec2{cw*0.5 + side*30, float64(canvasH) * 0.40},
			p3: vec2{cw * 0.5, float64(canvasH) * 0.55},
		},
		{
			p0: vec2{cw * 0.5, float64(canvasH) * 0.55},
			p1: vec2{cw*0.5 - side*30, float64(canvasH) * 0.65},
			p2: vec2{playerX + side*15, float64(canvasH) * 0.75},
			p3: vec2{playerX, float64(canvasH) + 8},
		},
	})
}

// diveTractor: boss path that descends and stops at a hover position
// above the player area where it deploys the tractor beam. The path
// ends at that hover spot — the boss then plays the beam animation
// before flying back along returnTractor (computed at beam end).
func diveTractor(canvasW, canvasH int, startX, startY, hoverX, hoverY float64) *path {
	_ = canvasW
	return newPath([]bezierSeg{
		{
			p0: vec2{startX, startY},
			p1: vec2{startX, startY + 8},
			p2: vec2{hoverX, float64(canvasH) * 0.35},
			p3: vec2{hoverX, hoverY},
		},
	})
}

// returnPath: from a starting off-screen position back to the formation
// slot at (targetX, targetY). Used after a dive completes — the enemy
// re-appears at the top of the screen and curves to its slot.
func returnPath(canvasW, canvasH int, targetX, targetY float64) *path {
	_ = canvasW
	startX := targetX
	if startX < float64(canvasW)*0.5 {
		startX -= 10
	} else {
		startX += 10
	}
	return newPath([]bezierSeg{
		{
			p0: vec2{startX, -8},
			p1: vec2{startX, float64(canvasH) * 0.15},
			p2: vec2{targetX, float64(canvasH) * 0.30},
			p3: vec2{targetX, targetY},
		},
	})
}
