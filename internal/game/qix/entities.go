package qix

import (
	"math"
	"math/rand"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// ============================================================
// Qix monster
// ============================================================

// The Qix in the original arcade game is a tangle of bright line
// segments that writhes around the unclaimed area. We model it as an
// open polyline of N+1 "joints"; each joint has its own velocity,
// bounces off non-open cells, and randomly redirects so the whole
// shape never settles into a stable orbit. Consecutive joints are
// connected by drawn lines.
//
// Collision rules:
//   - A joint can never occupy a non-cellOpen cell. Velocity reflects
//     off cellBorder / cellClaimed / cellDraw the same way; a joint
//     bouncing off a draw cell does NOT kill the player by itself.
//   - But the *segments* connecting joints will frequently pass
//     through cellDraw if the player's trail crosses near the Qix.
//     Any segment overlapping any cellDraw kills the player. That's
//     the "the Qix touched your line" loss condition.
type qixMonster struct {
	pts   []qixJoint
	rng   *rand.Rand
	speed float64

	// colorPhase is advanced each tick so the rendering cycles through
	// hues. Purely cosmetic.
	colorPhase float64
}

type qixJoint struct {
	x, y   float64
	vx, vy float64
}

func newQix(rng *rand.Rand, f *field, speed float64, joints int) *qixMonster {
	q := &qixMonster{
		rng:   rng,
		speed: speed,
		pts:   make([]qixJoint, joints),
	}
	// Spawn the joints near the centre of the playfield, scattered
	// inside a small cluster so the segments start short.
	cx := float64(f.w) / 2
	cy := float64(f.h) / 2
	for i := range q.pts {
		ang := rng.Float64() * 2 * math.Pi
		r := rng.Float64() * 4
		jx := cx + r*math.Cos(ang)
		jy := cy + r*math.Sin(ang)
		if !f.isOpen(int(jx), int(jy)) {
			jx = cx
			jy = cy
		}
		dir := rng.Float64() * 2 * math.Pi
		q.pts[i] = qixJoint{
			x:  jx,
			y:  jy,
			vx: math.Cos(dir) * speed,
			vy: math.Sin(dir) * speed,
		}
	}
	return q
}

// tick advances every joint by s seconds, reflecting off non-open
// cells and randomly redirecting a small fraction of the time so the
// motion stays chaotic.
func (q *qixMonster) tick(s float64, f *field) {
	q.colorPhase += s
	for i := range q.pts {
		p := &q.pts[i]
		nx := p.x + p.vx*s
		ny := p.y + p.vy*s

		// Reflect on each axis independently so the joint slides along
		// walls cleanly rather than getting stuck.
		if !f.isOpen(int(nx), int(p.y)) {
			p.vx = -p.vx
			nx = p.x + p.vx*s
		}
		if !f.isOpen(int(p.x), int(ny)) {
			p.vy = -p.vy
			ny = p.y + p.vy*s
		}
		// Diagonal-corner fallback: if the destination cell is still
		// blocked (rare, only at concave corners), keep the joint where
		// it was for this frame.
		if !f.isOpen(int(nx), int(ny)) {
			nx, ny = p.x, p.y
		}
		p.x, p.y = nx, ny

		// Occasional direction nudge — about one redirect per 1.5s on
		// average per joint. Combined across joints it produces the
		// constant writhing motion the original is famous for.
		if q.rng.Float64() < 0.7*s {
			ang := q.rng.Float64() * 2 * math.Pi
			p.vx = math.Cos(ang) * q.speed
			p.vy = math.Sin(ang) * q.speed
		}
	}
}

// joints returns the joints' cell coordinates — used as flood-fill
// probes when resolving a claim.
func (q *qixMonster) jointCells() []point {
	out := make([]point, 0, len(q.pts))
	for _, p := range q.pts {
		out = append(out, point{int(p.x), int(p.y)})
	}
	return out
}

// touchesDraw walks each connecting segment with a Bresenham trace
// and reports whether any cell along the way is currently cellDraw.
// Used as the "Qix hit the player's line" lose condition.
func (q *qixMonster) touchesDraw(f *field) bool {
	for i := 0; i+1 < len(q.pts); i++ {
		a := q.pts[i]
		b := q.pts[i+1]
		if traceLineHitsDraw(int(a.x), int(a.y), int(b.x), int(b.y), f) {
			return true
		}
	}
	return false
}

// touchesCell reports whether any segment passes through (px, py).
// Used to check Qix–player collision when the player is mid-draw.
func (q *qixMonster) touchesCell(px, py int) bool {
	for i := 0; i+1 < len(q.pts); i++ {
		a := q.pts[i]
		b := q.pts[i+1]
		if traceLineHitsCell(int(a.x), int(a.y), int(b.x), int(b.y), px, py) {
			return true
		}
	}
	return false
}

// draw paints the Qix's segments as a rainbow polyline. The phase
// rotates so the colours appear to flow along the body.
func (q *qixMonster) draw(c *engine.Canvas, offX, offY int) {
	if len(q.pts) < 2 {
		return
	}
	for i := 0; i+1 < len(q.pts); i++ {
		a := q.pts[i]
		b := q.pts[i+1]
		hue := math.Mod(q.colorPhase*0.9+float64(i)*0.4, 1.0)
		col := hsvToColor(hue, 1.0, 1.0)
		c.DrawLine(int(a.x)+offX, int(a.y)+offY,
			int(b.x)+offX, int(b.y)+offY, col)
	}
	// Brighten the endpoints with a single pixel highlight so the
	// "head/tail" reads at small sizes.
	for _, p := range []qixJoint{q.pts[0], q.pts[len(q.pts)-1]} {
		c.Set(int(p.x)+offX, int(p.y)+offY, engine.White)
	}
}

// hsvToColor converts (h, s, v) in [0, 1] to an opaque RGBA colour.
// Used to give the Qix its rotating rainbow look.
func hsvToColor(h, s, v float64) engine.Color {
	if s <= 0 {
		k := uint8(v * 255)
		return engine.Color{R: k, G: k, B: k, A: 255}
	}
	h = h - math.Floor(h)
	hh := h * 6
	i := int(hh)
	f := hh - float64(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	case 5:
		r, g, b = v, p, q
	}
	return engine.Color{R: uint8(r * 255), G: uint8(g * 255), B: uint8(b * 255), A: 255}
}

// traceLineHitsDraw walks a Bresenham line from (x0, y0) to (x1, y1)
// and returns true if any cell along the way is cellDraw.
func traceLineHitsDraw(x0, y0, x1, y1 int, f *field) bool {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 >= x1 {
		sx = -1
	}
	if y0 >= y1 {
		sy = -1
	}
	err := dx + dy
	x, y := x0, y0
	for {
		if f.isDraw(x, y) {
			return true
		}
		if x == x1 && y == y1 {
			return false
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x += sx
		}
		if e2 <= dx {
			err += dx
			y += sy
		}
	}
}

// traceLineHitsCell returns true if (tx, ty) lies on the Bresenham
// line from (x0, y0) to (x1, y1).
func traceLineHitsCell(x0, y0, x1, y1, tx, ty int) bool {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 >= x1 {
		sx = -1
	}
	if y0 >= y1 {
		sy = -1
	}
	err := dx + dy
	x, y := x0, y0
	for {
		if x == tx && y == ty {
			return true
		}
		if x == x1 && y == y1 {
			return false
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x += sx
		}
		if e2 <= dx {
			err += dx
			y += sy
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ============================================================
// Sparx — border-walking enemies
// ============================================================

// sparx wall-walks the cellBorder graph using a turn-preference rule
// so the path stays consistent at junctions. Two sparx are spawned per
// level (more on higher levels), travelling in opposite rotational
// directions around the open area's perimeter.
type sparx struct {
	x, y   int
	prevX  int
	prevY  int
	dirX   int
	dirY   int
	speed  float64
	accum  float64
	// turnRight controls which way this sparx prefers to turn at a
	// junction. true → right-hand rule → clockwise around the open
	// region. false → left-hand rule → counterclockwise. Two sparx
	// with opposite preferences cover the loop in both directions.
	turnRight bool
}

func newSparx(x, y int, dirX, dirY int, speed float64, turnRight bool) *sparx {
	return &sparx{
		x:         x,
		y:         y,
		prevX:     x - dirX,
		prevY:     y - dirY,
		dirX:      dirX,
		dirY:      dirY,
		speed:     speed,
		turnRight: turnRight,
	}
}

// tick advances the sparx by s seconds. Movement is cell-discrete
// driven by an accumulator: the sparx queues up sub-cell distance and
// steps whenever it accumulates a full cell of travel.
func (sp *sparx) tick(s float64, f *field) {
	sp.accum += s * sp.speed
	for sp.accum >= 1 {
		sp.accum -= 1
		if !sp.step(f) {
			break
		}
	}
}

// step performs a single border-cell hop using the preferred turn
// direction. Returns false if no move was possible at all — extremely
// rare; only on a one-cell isolated border, which the rest of the
// game shouldn't produce.
func (sp *sparx) step(f *field) bool {
	// Build the candidate-direction list in preference order:
	// preferred turn, straight ahead, opposite turn, reverse.
	dirs := sp.candidateDirs()
	for _, d := range dirs {
		nx, ny := sp.x+d.x, sp.y+d.y
		// Don't immediately backtrack unless we have no other option.
		if nx == sp.prevX && ny == sp.prevY {
			continue
		}
		if f.isBorder(nx, ny) {
			sp.prevX, sp.prevY = sp.x, sp.y
			sp.x, sp.y = nx, ny
			sp.dirX, sp.dirY = d.x, d.y
			return true
		}
	}
	// Forced reverse — at a dead end.
	nx, ny := sp.prevX, sp.prevY
	if f.isBorder(nx, ny) {
		sp.prevX, sp.prevY = sp.x, sp.y
		sp.x, sp.y = nx, ny
		sp.dirX, sp.dirY = -sp.dirX, -sp.dirY
		return true
	}
	return false
}

// candidateDirs orders the four cardinal directions by turn preference
// relative to the sparx's current heading. The "right turn" relative
// to (dx, dy) is (dy, -dx); "left turn" is (-dy, dx).
func (sp *sparx) candidateDirs() [4]point {
	dx, dy := sp.dirX, sp.dirY
	if dx == 0 && dy == 0 {
		dx = 1
	}
	right := point{dy, -dx}
	left := point{-dy, dx}
	straight := point{dx, dy}
	reverse := point{-dx, -dy}
	if sp.turnRight {
		return [4]point{right, straight, left, reverse}
	}
	return [4]point{left, straight, right, reverse}
}

// draw paints the sparx as a small bright cross — the original arcade
// rendered each sparx as a sparkle. Two pixels stacked plus side
// pixels give the right read at 1:1 cell resolution.
func (sp *sparx) draw(c *engine.Canvas, offX, offY int, col engine.Color) {
	x := sp.x + offX
	y := sp.y + offY
	c.Set(x, y, col)
	c.Set(x-1, y, col)
	c.Set(x+1, y, col)
	c.Set(x, y-1, col)
	c.Set(x, y+1, col)
}

// ============================================================
// Fuse — chases the player along an unfinished trail
// ============================================================

// fuse is the slow spark that appears at the head of the player's
// drawn trail when the player has been idle mid-draw long enough. It
// walks the trail toward the player, killing on contact. It vanishes
// (and the timer resets) as soon as the player moves again.
type fuse struct {
	// idx is the trail index the fuse currently occupies. The trail
	// is the player's polyline starting at the original border step.
	// Higher indices are closer to the player's current head, so the
	// fuse advances by incrementing idx.
	idx   int
	speed float64
	accum float64
	// alive reflects whether the fuse should currently render and tick.
	// Spawned (alive=true) when player has been idle past
	// fuseStartDelay; cleared (alive=false) the moment the player
	// extends or undraws.
	alive bool
}

func newFuse(speed float64) *fuse {
	return &fuse{speed: speed}
}

// spawn arms the fuse at trail index 0 — the border end of the line.
func (fz *fuse) spawn() {
	fz.idx = 0
	fz.accum = 0
	fz.alive = true
}

// extinguish hides the fuse and resets it for a future spawn.
func (fz *fuse) extinguish() {
	fz.alive = false
	fz.idx = 0
	fz.accum = 0
}

// tick advances the fuse along the trail. trailLen is the current
// length of the player's trail (in cells). The fuse stops at the last
// cell — caller checks for "fuse reached the player" via idx == len(trail)-1.
func (fz *fuse) tick(s float64, trailLen int) {
	if !fz.alive {
		return
	}
	fz.accum += s * fz.speed
	for fz.accum >= 1 && fz.idx < trailLen-1 {
		fz.accum -= 1
		fz.idx++
	}
}

// caughtPlayer reports whether the fuse's cell has reached the head
// of the player's trail.
func (fz *fuse) caughtPlayer(trail []point) bool {
	if !fz.alive || len(trail) == 0 {
		return false
	}
	return fz.idx >= len(trail)-1
}

// draw paints the fuse as a small flickery cross. flicker is fed by
// the caller (a per-frame phase so the spark visibly trembles).
func (fz *fuse) draw(c *engine.Canvas, trail []point, offX, offY int, flicker bool) {
	if !fz.alive {
		return
	}
	if fz.idx < 0 || fz.idx >= len(trail) {
		return
	}
	p := trail[fz.idx]
	x, y := p.x+offX, p.y+offY
	core := engine.Color{R: 255, G: 200, B: 60, A: 255}
	edge := engine.Color{R: 255, G: 120, B: 30, A: 255}
	c.Set(x, y, core)
	if flicker {
		c.Set(x-1, y, edge)
		c.Set(x+1, y, edge)
		c.Set(x, y-1, edge)
		c.Set(x, y+1, edge)
	}
}
