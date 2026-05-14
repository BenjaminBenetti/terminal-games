package defender

import (
	"math"
	"math/rand"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// The Defender world is toroidal in X: fly far enough in either
// direction and you return to where you started. Vertically it's
// bounded — the ground sits at the bottom, the ceiling at the top.
//
// All entities store their X coordinate in **world** space. World X
// is wrapped to [0, worldW) by wrapX whenever it's read back; the
// renderer translates to screen X via world.toScreen.

const (
	// worldScale: the world is 4 screens wide. Big enough to hide
	// raiding parties off-screen and make the scanner mean something;
	// small enough to keep a wave's worth of action reachable.
	worldScale = 4
)

// world owns the static side of the map: the jagged terrain heights
// and the camera origin. The camera is updated each frame by the play
// scene; everything else is constant for the wave's lifetime.
type world struct {
	w, h          int     // canvas size in pixels
	worldW        int     // wrapping world width, == w * worldScale
	camLeft       float64 // world x of the screen's left edge
	playZoneTop   int     // first usable pixel row below the HUD/scanner
	playZoneBot   int     // pixel row of the highest mountain peak (top of terrain)
	groundY       int     // y-row of the average ground level (for spawn placement)
	terrainHeight []int   // per-world-pixel column: pixel row of the terrain TOP
	flattened     bool    // true after the planet explodes — terrain disappears
}

func newWorld(w, h int, rng *rand.Rand) *world {
	wd := &world{
		w:      w,
		h:      h,
		worldW: w * worldScale,
	}
	// Reserve the top of the canvas for HUD + scanner. The scanner is
	// 8 px tall; HUD text takes a couple cell rows above that.
	wd.playZoneTop = hudPxTop + scannerPxHeight + 2
	// Ground sits in the bottom ~25% of the play area.
	wd.groundY = h - h/6
	wd.playZoneBot = wd.groundY
	wd.buildTerrain(rng)
	return wd
}

// buildTerrain populates a jagged mountain horizon by walking a small
// fractal Brownian sum across the world width. The values are pixel
// rows: smaller numbers = higher peaks. We clamp to a band so the
// terrain doesn't intrude into the play zone or fall off the canvas.
func (wd *world) buildTerrain(rng *rand.Rand) {
	wd.terrainHeight = make([]int, wd.worldW)
	// Three octaves of value noise, summed.
	const peaks = 64
	control := make([]float64, peaks)
	for i := range control {
		control[i] = rng.Float64()
	}
	subPeaks := peaks * 4
	subControl := make([]float64, subPeaks)
	for i := range subControl {
		subControl[i] = rng.Float64()
	}
	minTop := wd.groundY - 12 // highest peak
	maxTop := wd.groundY + 1  // valley floor (a few pixels below average ground)
	if maxTop > wd.h-2 {
		maxTop = wd.h - 2
	}
	span := float64(maxTop - minTop)
	for x := 0; x < wd.worldW; x++ {
		u := float64(x) / float64(wd.worldW)
		v1 := sampleCircular(control, u)
		v2 := sampleCircular(subControl, u*4)
		v3 := sampleCircular(subControl, u*8.7) * 0.5
		mix := (v1*0.65 + v2*0.25 + v3*0.10)
		// Bias toward the upper half of the band so most terrain is
		// hilly rather than glued to the bottom edge.
		mix = math.Pow(mix, 1.2)
		wd.terrainHeight[x] = minTop + int(mix*span)
	}
}

// sampleCircular reads a periodic noise track at fractional position u
// ∈ [0,1). Linear interpolation between control points.
func sampleCircular(control []float64, u float64) float64 {
	n := len(control)
	u = u - math.Floor(u)
	if u < 0 {
		u += 1
	}
	fi := u * float64(n)
	i0 := int(fi)
	frac := fi - float64(i0)
	i1 := (i0 + 1) % n
	return control[i0]*(1-frac) + control[i1]*frac
}

// wrapX folds an arbitrary world x into [0, worldW).
func (wd *world) wrapX(x float64) float64 {
	w := float64(wd.worldW)
	x = math.Mod(x, w)
	if x < 0 {
		x += w
	}
	return x
}

// wrapDelta returns the signed shortest delta from a to b on the
// toroidal world (b - a, taking the shorter direction).
func (wd *world) wrapDelta(a, b float64) float64 {
	w := float64(wd.worldW)
	d := math.Mod(b-a, w)
	if d > w/2 {
		d -= w
	}
	if d < -w/2 {
		d += w
	}
	return d
}

// toScreen translates a world x to the closest screen x (in pixels) for
// rendering. Returns a value in roughly [-worldW/2, worldW/2]; callers
// only draw when 0 <= screenX < screenW (or close to that range, after
// accounting for sprite width).
func (wd *world) toScreen(worldX float64) int {
	w := float64(wd.worldW)
	dx := math.Mod(worldX-wd.camLeft, w)
	if dx > w/2 {
		dx -= w
	}
	if dx < -w/2 {
		dx += w
	}
	return int(math.Round(dx))
}

// updateCamera nudges camLeft toward a target that keeps the player
// offset toward the back of the screen — Defender's signature framing,
// where most of the visible space is in the direction the ship is
// facing. The interpolation is exponential so the framing pans
// smoothly when the player flips direction.
func (wd *world) updateCamera(playerWorldX float64, facing int, dt float64) {
	// Target screen x for the player: 1/3 from the back edge.
	var targetScreenX float64
	if facing >= 0 {
		targetScreenX = float64(wd.w) / 3
	} else {
		targetScreenX = float64(wd.w) * 2 / 3
	}
	targetCamLeft := playerWorldX - targetScreenX
	// Smoothing: lerp toward target with a per-second factor so the pan
	// rate is frame-rate independent.
	const camTau = 0.35 // seconds; lower = snappier
	alpha := 1 - math.Exp(-dt/camTau)
	// We're working modulo worldW — interpolate via shortest delta.
	delta := wd.wrapDelta(wd.camLeft, targetCamLeft)
	wd.camLeft = wd.wrapX(wd.camLeft + delta*alpha)
}

// terrainAt returns the pixel-row top of the terrain at world x. For
// fractional x it linearly interpolates between neighbouring columns;
// after a planet-explosion flatten this returns the canvas bottom (no
// terrain visible).
func (wd *world) terrainAt(worldX float64) float64 {
	if wd.flattened {
		return float64(wd.h)
	}
	x := wd.wrapX(worldX)
	i0 := int(x) % wd.worldW
	if i0 < 0 {
		i0 += wd.worldW
	}
	i1 := (i0 + 1) % wd.worldW
	frac := x - math.Floor(x)
	return float64(wd.terrainHeight[i0])*(1-frac) + float64(wd.terrainHeight[i1])*frac
}

// drawTerrain renders the mountain horizon by sampling the height
// array for each visible screen column, drawing the silhouette in
// terrain green and filling below in a dimmer green so the band reads
// as solid ground.
func (wd *world) drawTerrain(c *engine.Canvas) {
	if wd.flattened {
		return
	}
	camLeftI := int(math.Floor(wd.camLeft))
	camFrac := wd.camLeft - float64(camLeftI)
	for col := 0; col < wd.w; col++ {
		// We want the height a fraction-of-a-pixel into the world so
		// the terrain doesn't appear to jitter as the camera pans.
		x0 := camLeftI + col
		x1 := x0 + 1
		x0 = ((x0 % wd.worldW) + wd.worldW) % wd.worldW
		x1 = ((x1 % wd.worldW) + wd.worldW) % wd.worldW
		h := float64(wd.terrainHeight[x0])*(1-camFrac) + float64(wd.terrainHeight[x1])*camFrac
		top := int(math.Round(h))
		if top >= wd.h {
			continue
		}
		// Crest line bright.
		c.Set(col, top, colTerrain)
		// Fill below with a darker hue so peaks read clearly.
		dim := engine.Color{R: colTerrain.R / 3, G: colTerrain.G / 2, B: colTerrain.B / 3, A: 255}
		for y := top + 1; y < wd.h; y++ {
			c.Set(col, y, dim)
		}
	}
}

// drawStarfield scatters a few pixels in the upper play zone to give
// the empty space some life. Same trick the asteroids title screen uses
// — pseudo-deterministic positions hashed off the world camera so the
// stars appear to scroll past with the world.
func (wd *world) drawStarfield(c *engine.Canvas, t float64) {
	const stars = 70
	for i := 0; i < stars; i++ {
		// Hash i into a pseudo-random world x and y.
		hx := float64((i*1297 + 7) % wd.worldW)
		hy := wd.playZoneTop + ((i*73+11)%(wd.playZoneBot-wd.playZoneTop-4))
		sx := wd.toScreen(hx)
		if sx < 0 || sx >= wd.w {
			continue
		}
		// Slow twinkle.
		bright := ((int(t*2) + i) % 5) != 0
		col := colStarDim
		if bright {
			col = colStarBri
		}
		c.Set(sx, hy, col)
	}
}
