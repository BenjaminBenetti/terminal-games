package battlezone

import (
	"fmt"
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// HUD layout constants. The radar and labels are drawn directly on top
// of the 3D viewport — the original cabinet had no on-screen frame; the
// distinctive periscope chrome was molded plastic on the front panel.
// We mimic the look by drawing thin angular brackets around the radar.
const (
	radarRadius = 9
	radarMargin = 2
)

// drawHUD paints the score, hi-score, lives indicator, radar, scope
// crosshair, and (when active) the crack overlay on top of the 3D
// viewport.
func (p *playScene) drawHUD(c *engine.Canvas) {
	w := c.Width()
	rows := c.Rows()

	// Top-row labels — score on the left, hi-score in the centre,
	// tank-icons on the right tally remaining lives.
	scoreText := fmt.Sprintf("SCORE %06d", p.score)
	hiText := fmt.Sprintf("HI %06d", p.hiScore)
	c.Print(1, 0, scoreText, hudGreen)
	c.Print((c.Cols()-len(hiText))/2, 0, hiText, hudGreen)
	livesLabel := "TANKS"
	livesX := c.Cols() - 1 - len(livesLabel) - p.lives*2 - 1
	if livesX < 0 {
		livesX = 0
	}
	c.Print(livesX, 0, livesLabel, hudGreen)
	// Lives counter — small "tank" icons.
	for i := 0; i < p.lives; i++ {
		x := livesX + len(livesLabel) + 1 + i*2
		drawLifeIcon(c, x, 0, hudGreen)
	}

	// Radar — circular sweep at top centre with a wedge for the
	// player's field of view and an enemy blip if applicable.
	radarCX := w / 2
	radarCY := radarRadius + radarMargin + 4
	drawRadarHousing(c, radarCX, radarCY, hudGreen)
	drawRadar(c, radarCX, radarCY, p)

	// Crosshair / aiming reticle at the centre of the viewport.
	drawCrosshair(c, p.cam.cx, p.cam.cy, hudGreen)

	// Crack overlay flashes after a hit.
	if p.crackT > 0 {
		drawCrack(c, p.cam.cx, p.cam.cy, p.crackT/crackDuration, p.crackPattern)
	}

	// Status banners.
	switch p.state {
	case psGameOver:
		drawCenterText(c, "GAME OVER", c.Height()/2-engine.FontHeight, hudBright)
		hint := "ENTER PLAY AGAIN   ESC QUIT"
		c.Print((c.Cols()-len(hint))/2, rows-3, hint, hudGreen)
	case psPlayerDying:
		// During the dying flash we leave the crack visible; nothing
		// else.
	case psWaiting:
		// Brief pause between enemies; no banner needed.
	}
}

// drawRadarHousing renders the angular brackets that hint at the
// metallic periscope chrome surrounding the radar.
func drawRadarHousing(c *engine.Canvas, cx, cy int, color engine.Color) {
	// Top arc — a flat bar with stepped shoulders.
	c.DrawLine(cx-radarRadius-3, cy-radarRadius-2, cx+radarRadius+3, cy-radarRadius-2, color)
	c.DrawLine(cx-radarRadius-3, cy-radarRadius-2, cx-radarRadius-3, cy-1, color)
	c.DrawLine(cx+radarRadius+3, cy-radarRadius-2, cx+radarRadius+3, cy-1, color)
	// Lower brackets — angular feet.
	c.DrawLine(cx-radarRadius-3, cy-1, cx-radarRadius-1, cy+radarRadius+1, color)
	c.DrawLine(cx+radarRadius+3, cy-1, cx+radarRadius+1, cy+radarRadius+1, color)
	c.DrawLine(cx-radarRadius-1, cy+radarRadius+1, cx+radarRadius+1, cy+radarRadius+1, color)
}

// drawRadar paints the actual radar dish — outer circle, sweep line,
// view-cone wedge, and enemy blip if present.
func drawRadar(c *engine.Canvas, cx, cy int, p *playScene) {
	col := hudGreen
	dim := hudDim
	c.DrawCircle(cx, cy, radarRadius, col)
	// View-cone wedge: ±FOV/2 in front of the player (which is +Y on
	// the radar). FOV here matches the camera focal length: half-angle
	// = atan(width/2 / focal).
	halfFOV := math.Atan2(float64(c.Width())/2, p.cam.focal)
	// Triangle from centre out to the rim along ±halfFOV.
	x1 := cx + int(math.Round(math.Sin(-halfFOV)*float64(radarRadius)))
	y1 := cy - int(math.Round(math.Cos(-halfFOV)*float64(radarRadius)))
	x2 := cx + int(math.Round(math.Sin(halfFOV)*float64(radarRadius)))
	y2 := cy - int(math.Round(math.Cos(halfFOV)*float64(radarRadius)))
	c.DrawLine(cx, cy, x1, y1, dim)
	c.DrawLine(cx, cy, x2, y2, dim)

	// Sweep line — rotates around the centre at a steady rate.
	sweepAngle := p.radarSweep
	sx := cx + int(math.Round(math.Sin(sweepAngle)*float64(radarRadius)))
	sy := cy - int(math.Round(math.Cos(sweepAngle)*float64(radarRadius)))
	c.DrawLine(cx, cy, sx, sy, col)

	// Enemy blip — always visible while an enemy is alive, with sweep
	// proximity producing a brighter "fresh" pulse for that authentic
	// phosphor-radar look. The continuous baseline is what the player
	// actually navigates by; the original cabinet's radar worked the
	// same way.
	if p.enemy != nil {
		d := shortestDelta(p.cam.pos, p.enemy.pos)
		bearing := math.Atan2(d.x, d.z) - p.cam.yaw
		dist := math.Hypot(d.x, d.z)
		// Match the spawn radius so newly-arrived enemies appear at
		// the outer rim of the radar and close inward as they approach.
		const maxR = 110.0
		r := dist / maxR
		if r > 1 {
			r = 1
		}
		rp := float64(radarRadius-1) * r
		bx := cx + int(math.Round(math.Sin(bearing)*rp))
		by := cy - int(math.Round(math.Cos(bearing)*rp))
		// Brighten when the sweep recently passed this bearing.
		blipAge := normalizeAngle(p.radarSweep - bearing)
		intensity := 0.55
		if blipAge >= 0 && blipAge < 1.5 {
			intensity = 1 - 0.45*(blipAge/1.5)
		}
		cc := scaleColor(hudBright, intensity)
		// Cross-shape blip so a single pixel doesn't get lost on the
		// radar at the small canvas sizes we render at.
		c.Set(bx, by, cc)
		c.Set(bx+1, by, cc)
		c.Set(bx-1, by, cc)
		c.Set(bx, by+1, cc)
		c.Set(bx, by-1, cc)
	}
}

// drawCrosshair renders the centre-screen aiming reticle — a pair of
// angular brackets that read as a tank gunsight without obscuring the
// world behind it.
func drawCrosshair(c *engine.Canvas, cx, cy int, color engine.Color) {
	const arm = 4
	const gap = 2
	// Left bracket.
	c.DrawLine(cx-arm-gap, cy-1, cx-gap, cy-1, color)
	c.DrawLine(cx-arm-gap, cy-1, cx-arm-gap, cy+1, color)
	c.DrawLine(cx-arm-gap, cy+1, cx-gap, cy+1, color)
	// Right bracket.
	c.DrawLine(cx+gap, cy-1, cx+arm+gap, cy-1, color)
	c.DrawLine(cx+arm+gap, cy-1, cx+arm+gap, cy+1, color)
	c.DrawLine(cx+arm+gap, cy+1, cx+gap, cy+1, color)
	// Small notch above/below for vertical alignment.
	c.Set(cx, cy-gap, color)
	c.Set(cx, cy+gap, color)
}

// drawLifeIcon paints a tiny 2x1 tank icon used for the lives tally.
// The icon is drawn over the underlying pixels at cell coordinates.
func drawLifeIcon(c *engine.Canvas, col, row int, color engine.Color) {
	// One cell holds two stacked pixels, so we paint a 2-px-wide
	// shape over (col*2, row*2) and (col*2+1, row*2) for the body and
	// add a turret pixel above for personality.
	x := col
	y := row * 2
	c.Set(x, y, color)
	c.Set(x+1, y, color)
	c.Set(x, y+1, color)
}

// drawCenterText prints a centered DrawText banner with a dim outline
// so it reads against the green wireframes behind it.
func drawCenterText(c *engine.Canvas, text string, baseY int, color engine.Color) {
	tw := engine.TextWidth(text)
	tx := (c.Width() - tw) / 2
	// Knock out a strip behind the text so wireframes don't bleed
	// through and turn it into mush.
	c.FillRect(tx-3, baseY-1, tw+6, engine.FontHeight+2, engine.Black)
	c.DrawText(tx, baseY, text, color)
}

// crackDuration is how long the cracked-screen overlay stays up after a
// hit before fading away.
const crackDuration = 2.2

// crackPattern is a precomputed cracked-glass shatter. Each element is
// a polyline starting at the impact centre and forking outwards.
type crackPattern struct {
	lines [][2]vec2D
}

type vec2D struct{ x, y float64 }

// makeCrackPattern generates a fresh crack pattern centred on the
// impact point. The pattern is regenerated each hit so it never reads
// as identical from one death to the next.
func makeCrackPattern(rng intRNG, cx, cy int, maxR int) *crackPattern {
	const branches = 10
	out := &crackPattern{}
	for i := 0; i < branches; i++ {
		angle := rng.Float64() * 2 * math.Pi
		length := float64(maxR) * (0.55 + rng.Float64()*0.65)
		// Main fracture, drawn as a series of zig-zag segments.
		var prev vec2D = vec2D{x: float64(cx), y: float64(cy)}
		segs := 3 + rng.Intn(3)
		for s := 0; s < segs; s++ {
			a := angle + (rng.Float64()-0.5)*0.5
			r := length / float64(segs) * (0.7 + rng.Float64()*0.6)
			next := vec2D{
				x: prev.x + math.Cos(a)*r,
				y: prev.y + math.Sin(a)*r,
			}
			out.lines = append(out.lines, [2]vec2D{prev, next})
			prev = next
		}
		// Tiny offshoot from the third joint.
		if len(out.lines) >= 2 {
			start := out.lines[len(out.lines)-2][1]
			offshoot := vec2D{
				x: start.x + math.Cos(angle+1.0)*float64(maxR)/8,
				y: start.y + math.Sin(angle+1.0)*float64(maxR)/8,
			}
			out.lines = append(out.lines, [2]vec2D{start, offshoot})
		}
	}
	return out
}

// intRNG is the minimal RNG interface drawCrack needs. The play scene
// has one of these via rand.Rand and we accept it indirectly to keep
// tests trivial to write.
type intRNG interface {
	Float64() float64
	Intn(n int) int
}

// drawCrack overlays the cracked-glass pattern on the canvas. progress
// runs 1.0 (fresh hit) to 0.0 (faded out). At progress < 0.2 we begin
// dropping segments to give a shattering sense.
func drawCrack(c *engine.Canvas, _, _ int, progress float64, pat *crackPattern) {
	if pat == nil {
		return
	}
	k := progress
	if k < 0 {
		return
	}
	if k > 1 {
		k = 1
	}
	col := engine.Color{
		R: uint8(180 * k),
		G: uint8(255 * k),
		B: uint8(120 * k),
		A: 255,
	}
	for _, ln := range pat.lines {
		c.DrawLine(int(ln[0].x), int(ln[0].y), int(ln[1].x), int(ln[1].y), col)
	}
}

// scaleColor multiplies an RGB color by k, clamped to [0,1].
func scaleColor(c engine.Color, k float64) engine.Color {
	if k < 0 {
		k = 0
	}
	if k > 1 {
		k = 1
	}
	return engine.Color{
		R: uint8(float64(c.R) * k),
		G: uint8(float64(c.G) * k),
		B: uint8(float64(c.B) * k),
		A: 255,
	}
}

// HUD color palette. We keep these in one place so the entire interface
// reads as monochrome phosphor.
var (
	hudBright = engine.Color{R: 130, G: 255, B: 150, A: 255}
	hudGreen  = engine.Color{R: 80, G: 220, B: 110, A: 255}
	hudDim    = engine.Color{R: 30, G: 110, B: 50, A: 255}
)
