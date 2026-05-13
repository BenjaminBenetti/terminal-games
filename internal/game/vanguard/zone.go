package vanguard

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// zoneKind identifies one of Vanguard's five linked sections. The arcade
// cycles through them in order; after the Gond at the end of Styx falls
// the loop restarts at Mountain with bumped difficulty.
type zoneKind int

const (
	zoneMountain zoneKind = iota
	zoneStripe
	zoneBleak
	zoneRainbow
	zoneStyx
)

// scrollAxis is the direction the world translates under the player.
// Mountain/Stripe/Bleak scroll horizontally (right-to-left). Rainbow and
// Styx scroll vertically (bottom-to-top), matching the original.
type scrollAxis int

const (
	scrollHoriz scrollAxis = iota
	scrollVert
)

// zoneConfig is the static description of a zone — name, look, scroll
// axis, length, and which spawn-rules apply to it. Per-zone enemy spawn
// logic lives in spawn.go.
type zoneConfig struct {
	kind     zoneKind
	name     string
	duration float64 // seconds the zone runs before transitioning
	axis     scrollAxis
	scrollSpd float64 // pixels per second of world translation
	bg       engine.Color
	wallCol  engine.Color
	accent   engine.Color
}

// zoneOrder is the canonical play order. The play scene walks this list
// once per loop; on completing Styx (and the Gond) the index wraps and
// difficulty scales up.
var zoneOrder = []zoneConfig{
	{
		kind: zoneMountain, name: "MOUNTAIN ZONE",
		duration:  35,
		axis:      scrollHoriz, scrollSpd: 18,
		bg:      engine.Color{R: 0, G: 0, B: 18, A: 255},
		wallCol: engine.Color{R: 90, G: 60, B: 30, A: 255},
		accent:  engine.Color{R: 220, G: 160, B: 90, A: 255},
	},
	{
		kind: zoneStripe, name: "STRIPE ZONE",
		duration:  28,
		axis:      scrollHoriz, scrollSpd: 22,
		bg:      engine.Color{R: 5, G: 0, B: 20, A: 255},
		wallCol: engine.Color{R: 190, G: 70, B: 200, A: 255},
		accent:  engine.Color{R: 240, G: 220, B: 90, A: 255},
	},
	{
		kind: zoneBleak, name: "BLEAK ZONE",
		duration:  28,
		axis:      scrollHoriz, scrollSpd: 24,
		bg:      engine.Color{R: 4, G: 4, B: 12, A: 255},
		wallCol: engine.Color{R: 80, G: 90, B: 110, A: 255},
		accent:  engine.Color{R: 160, G: 200, B: 240, A: 255},
	},
	{
		kind: zoneRainbow, name: "RAINBOW ZONE",
		duration:  26,
		axis:      scrollVert, scrollSpd: 18,
		bg:      engine.Color{R: 0, G: 0, B: 25, A: 255},
		wallCol: engine.Color{R: 220, G: 90, B: 60, A: 255},
		accent:  engine.Color{R: 240, G: 200, B: 90, A: 255},
	},
	{
		kind: zoneStyx, name: "STYX ZONE",
		duration:  32,
		axis:      scrollVert, scrollSpd: 16,
		bg:      engine.Color{R: 8, G: 0, B: 14, A: 255},
		wallCol: engine.Color{R: 200, G: 60, B: 60, A: 255},
		accent:  engine.Color{R: 240, G: 240, B: 90, A: 255},
	},
}

// terrainSample describes the solid bands around a single column / row
// of the play area. For horizontal-scroll zones, "near" and "far" are
// the top and bottom wall heights. For vertical-scroll zones, they are
// the left and right wall widths.
type terrainSample struct {
	nearH, farH int
}

// terrainAt returns the wall depths for the given world coordinate (in
// the scroll axis) for the supplied zone. World coordinates are stable
// across frames — the play scene tracks an integer worldOffset and
// queries this function as it scans across the visible band each frame.
//
// Each zone has its own deterministic procedural pattern keyed only off
// the world coordinate, which keeps the world consistent regardless of
// frame rate jitter or scroll-speed changes.
func terrainAt(z zoneConfig, world int, playW, playH int) terrainSample {
	switch z.kind {
	case zoneMountain:
		return mountainTerrain(world, playH)
	case zoneStripe:
		return stripeTerrain(world, playH)
	case zoneBleak:
		return bleakTerrain(world, playH)
	case zoneRainbow:
		return rainbowTerrain(world, playW)
	case zoneStyx:
		return styxTerrain(world, playW)
	}
	return terrainSample{}
}

// mountainTerrain builds craggy stalactite/stalagmite walls. Multiple
// sin frequencies produce a craggy, non-periodic feel; an absolute
// floor of 1px keeps the cave from ever being totally open.
func mountainTerrain(world, playH int) terrainSample {
	wf := float64(world)
	top := 4.0 + 3.5*math.Sin(wf*0.13) + 2.0*math.Sin(wf*0.07+1.3) +
		1.5*math.Sin(wf*0.31+2.7)
	bot := 4.0 + 3.0*math.Cos(wf*0.11+0.9) + 2.0*math.Sin(wf*0.06+0.4) +
		1.5*math.Cos(wf*0.27)
	if top < 1 {
		top = 1
	}
	if bot < 1 {
		bot = 1
	}
	// Don't ever close the cave — leave at least 12 px of free space.
	maxWall := float64(playH) - 12
	if maxWall < 2 {
		maxWall = 2
	}
	if top > maxWall*0.5 {
		top = maxWall * 0.5
	}
	if bot > maxWall*0.5 {
		bot = maxWall * 0.5
	}
	return terrainSample{nearH: int(top), farH: int(bot)}
}

// stripeTerrain leaves the play area open and lets the renderer paint
// vertical stripes on top — there's no collision terrain.
func stripeTerrain(world, _ int) terrainSample {
	_ = world
	return terrainSample{}
}

// bleakTerrain is mostly open space with the occasional pillar that
// blocks vertical movement.
func bleakTerrain(world, playH int) terrainSample {
	// A pillar every ~24 world units, 4 wide, alternating top/bottom.
	mod := ((world % 60) + 60) % 60
	if mod < 4 {
		// Pillar from top of size 12.
		return terrainSample{nearH: 12, farH: 0}
	}
	if mod >= 30 && mod < 34 {
		// Pillar from bottom of size 12.
		return terrainSample{nearH: 0, farH: 12}
	}
	_ = playH
	return terrainSample{}
}

// rainbowTerrain (vertical scroll): no wall collision — the rainbow
// stripes are decorative.
func rainbowTerrain(world, _ int) terrainSample {
	_ = world
	return terrainSample{}
}

// styxTerrain (vertical scroll): periodic horizontal walls with a
// single gap. nearH/farH report wall thickness from the left and right
// edges; a "gap" lives between them.
func styxTerrain(world, playW int) terrainSample {
	mod := ((world % 28) + 28) % 28
	if mod >= 24 {
		// 4-row-tall wall with a gap.
		gapStart := ((world / 28) * 11) % (playW - 14)
		if gapStart < 4 {
			gapStart = 4
		}
		return terrainSample{nearH: gapStart, farH: playW - gapStart - 14}
	}
	return terrainSample{}
}

// rainbowStripeColor returns a colour for a single row of the rainbow
// scroll background, keyed off the world row so the bands move
// smoothly with the scroll.
func rainbowStripeColor(world int) engine.Color {
	pal := []engine.Color{
		{R: 220, G: 60, B: 60, A: 255},
		{R: 220, G: 140, B: 50, A: 255},
		{R: 220, G: 220, B: 60, A: 255},
		{R: 60, G: 200, B: 90, A: 255},
		{R: 60, G: 130, B: 220, A: 255},
		{R: 130, G: 60, B: 220, A: 255},
	}
	idx := ((world / 3) % len(pal))
	if idx < 0 {
		idx += len(pal)
	}
	return pal[idx]
}

// stripeStripeColor is the equivalent for the horizontal-scroll Stripe
// zone — vertical bands of colour painted column-by-column.
func stripeStripeColor(world int) engine.Color {
	pal := []engine.Color{
		{R: 200, G: 60, B: 220, A: 255},
		{R: 60, G: 60, B: 220, A: 255},
		{R: 60, G: 220, B: 220, A: 255},
		{R: 220, G: 60, B: 200, A: 255},
	}
	idx := ((world / 4) % len(pal))
	if idx < 0 {
		idx += len(pal)
	}
	return pal[idx]
}

// bleakDot returns a tiny accent dot colour for the Bleak zone — used
// to scatter sparse ambient stars on top of the dark backdrop, drawn
// just like a starfield but tinted differently per column.
func bleakDot(world int) (engine.Color, bool) {
	if (world*131+13)%47 == 0 {
		return engine.Color{R: 120, G: 140, B: 200, A: 255}, true
	}
	return engine.Color{}, false
}
