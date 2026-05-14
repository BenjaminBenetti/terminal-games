package defender

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// HUD/scanner layout constants. The HUD reserves the top of the canvas
// for score, lives, smart-bomb count, and wave on cell row 0; the
// scanner occupies a fixed pixel band beneath it. Everything below
// hudPxTop + scannerPxHeight is play area.
const (
	hudPxTop        = 2 // first pixel row used by the HUD text (cell row 0 = pixels 0-1)
	scannerPxHeight = 10
	scannerPxTop    = hudPxTop + 2
	scannerPxBot    = scannerPxTop + scannerPxHeight
)

// scanner draws the radar strip at the top of the screen. It depicts
// the entire toroidal world condensed to the screen width, centred on
// the player so a target a quarter-world away on either side appears
// at the same relative position regardless of facing direction. A pair
// of vertical bars in the middle frames the chunk of the world that's
// currently on-screen.
func (p *playScene) drawScanner(c *engine.Canvas) {
	scrW := p.w
	top := scannerPxTop
	bot := scannerPxBot

	// Outline + dim background fill so the scanner reads as a distinct
	// instrument panel rather than getting lost in the starfield.
	bg := engine.Color{R: 8, G: 12, B: 24, A: 255}
	c.FillRect(0, top, scrW, bot-top, bg)
	for x := 0; x < scrW; x++ {
		c.Set(x, top, colScanFrame)
		c.Set(x, bot-1, colScanFrame)
	}

	worldW := float64(p.world.worldW)
	playerX := p.player.worldX

	// projectScanner maps a world x to a scanner-strip x, centring the
	// player. Returns -1 for "off-strip" (never happens for in-world
	// entities since the world is exactly one strip wide).
	project := func(worldX float64) int {
		d := p.world.wrapDelta(playerX, worldX)
		// d ∈ [-worldW/2, worldW/2]. Map to [0, scrW).
		u := (d + worldW/2) / worldW
		return int(u * float64(scrW))
	}

	// View-window markers — the slice of world currently on the main
	// screen. Drawn first, so entity blips paint over them.
	winLeft := project(p.world.camLeft)
	winRight := project(p.world.camLeft + float64(p.w))
	if winRight < winLeft {
		// Window straddles the wrap seam — split into two segments.
		drawWindowSeg(c, 0, winRight, top+1, bot-2)
		drawWindowSeg(c, winLeft, scrW-1, top+1, bot-2)
	} else {
		drawWindowSeg(c, winLeft, winRight, top+1, bot-2)
	}

	// Mid-rows: blips. We use 3 rows of pixels in the centre of the
	// strip so vertical position can encode altitude roughly — high
	// enemies up top, ground-level (humans, landed landers) at bottom.
	midTop := top + 2
	midBot := bot - 2
	band := float64(midBot - midTop)
	playH := float64(p.world.playZoneBot - p.world.playZoneTop)

	// Humans — green dots near the bottom.
	for _, h := range p.humans {
		if h.dead {
			continue
		}
		x := project(h.worldX)
		if x < 0 || x >= scrW {
			continue
		}
		y := midBot - 1
		if h.state == humanLifted {
			// Show the human at the lander's altitude for context.
			altY := altitudeToScanY(h.y, p.world.playZoneTop, playH, midTop, band)
			y = altY
		}
		c.Set(x, y, colHumanoid)
	}

	// Enemies — colour-coded per type.
	for _, e := range p.enemies {
		if !e.alive() {
			continue
		}
		x := project(e.worldX)
		if x < 0 || x >= scrW {
			continue
		}
		y := altitudeToScanY(e.y, p.world.playZoneTop, playH, midTop, band)
		col := colLander
		switch e.kind {
		case kLander:
			col = colLander
		case kMutant:
			col = colMutant
		case kBomber:
			col = colBomber
		case kPod:
			col = colPod
		case kSwarmer:
			col = colSwarmer
		case kBaiter:
			col = colBaiter
		}
		c.Set(x, y, col)
	}

	// Player — always centred (definitionally), and a bit fatter so
	// it stands out. Bright white "+" shape spanning 3 pixels.
	px := project(playerX)
	py := (midTop + midBot) / 2
	c.Set(px, py, colPlayer)
	c.Set(px-1, py, colPlayer)
	c.Set(px+1, py, colPlayer)
	c.Set(px, py-1, colPlayer)
	c.Set(px, py+1, colPlayer)
}

// altitudeToScanY proportionally maps a world y to a y within the
// scanner blip band.
func altitudeToScanY(worldY float64, playTop int, playH float64, scanTop int, scanBand float64) int {
	t := (worldY - float64(playTop)) / playH
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return scanTop + int(math.Round(t*(scanBand-1)))
}

// drawWindowSeg paints the two vertical bars + top/bottom hashes that
// frame the on-screen slice of the scanner strip.
func drawWindowSeg(c *engine.Canvas, x0, x1, y0, y1 int) {
	if x0 < 0 {
		x0 = 0
	}
	if x1 < 0 {
		x1 = 0
	}
	// Vertical brackets at each end.
	for y := y0; y <= y1; y++ {
		c.Set(x0, y, colScanFrame)
		c.Set(x1, y, colScanFrame)
	}
	// Faint horizontal hash on the top/bottom edge to imply the band.
	dim := engine.Color{R: colScanFrame.R / 2, G: colScanFrame.G / 2, B: colScanFrame.B / 2, A: 255}
	for x := x0; x <= x1; x += 2 {
		c.Set(x, y0, dim)
		c.Set(x, y1, dim)
	}
}

// drawHUD writes the score, hi-score, lives, smart-bomb count, and
// wave indicator above the scanner. Each label sits in cell row 0.
func (p *playScene) drawHUD(c *engine.Canvas) {
	cols := c.Cols()

	score := "SCORE " + zeroPad(p.score, 6)
	c.Print(1, 0, score, engine.White)

	hi := "HI " + zeroPad(p.hiScore, 6)
	hiX := (cols - len(hi)) / 2
	if hiX < len(score)+3 {
		hiX = len(score) + 3
	}
	c.Print(hiX, 0, hi, engine.Yellow)

	wave := "WAVE " + zeroPad(p.wave, 2)
	waveX := cols - len(wave) - 1
	c.Print(waveX, 0, wave, engine.Cyan)

	// Second row: lives, smart bombs, humans remaining.
	lives := "SHIPS " + zeroPad(p.player.lives, 2)
	bombs := "BOMBS " + zeroPad(p.player.smartBombs, 2)
	left := lives + "  " + bombs
	c.Print(1, 1, left, colPlayer)

	hr := 0
	for _, h := range p.humans {
		if !h.dead {
			hr++
		}
	}
	humans := "HUMANS " + zeroPad(hr, 2)
	hx := cols - len(humans) - 1
	c.Print(hx, 1, humans, colHumanoid)
}
