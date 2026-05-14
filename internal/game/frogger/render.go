package frogger

import (
	"fmt"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Draw is the per-frame render. Order matters — backgrounds, then lanes,
// then bonus entities, then the frog (so it's never occluded), then HUD
// overlay text last.
func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(bgColor)
	p.drawBackgrounds(c)
	p.drawHomes(c)
	p.drawLanes(c)
	p.drawLadyFrog(c)
	p.drawFrog(c)
	p.drawPopups(c)
	p.drawHUD(c)
	p.drawTimeBar(c)

	switch p.state {
	case psPreStage:
		p.drawPreStageBanner(c)
	case psWaveClear:
		p.drawWaveClearBanner(c)
	case psGameOver:
		p.drawGameOverBanner(c)
	}
}

// -- playfield → canvas helpers ---------------------------------------

func (p *playScene) px(x int) int { return p.offX + x }
func (p *playScene) py(y int) int { return p.offY + y }

// fillRectPF fills a rectangle in playfield-local coordinates.
func (p *playScene) fillRectPF(c *engine.Canvas, x, y, w, h int, col engine.Color) {
	c.FillRect(p.px(x), p.py(y), w, h, col)
}

// setPF sets a pixel in playfield-local coordinates.
func (p *playScene) setPF(c *engine.Canvas, x, y int, col engine.Color) {
	c.Set(p.px(x), p.py(y), col)
}

// -- Backgrounds ------------------------------------------------------

func (p *playScene) drawBackgrounds(c *engine.Canvas) {
	// Top hedge (home strip) is drawn by drawHomes — leave that for it.

	// River body (under the slot row).
	p.fillRectPF(c, 0, riverY0, p.playfieldW, riverH, riverColor)
	// River shimmer: a few sparse highlight pixels that scroll with time.
	for y := riverY0; y < riverY0+riverH; y++ {
		for x := (y * 3) % 4; x < p.playfieldW; x += 7 {
			if (x*y+int(p.stateT*2))%13 == 0 {
				p.setPF(c, x, y, riverHi)
			}
		}
	}

	// Median strip.
	p.fillRectPF(c, 0, medianY, p.playfieldW, medianH, medianColor)
	// Dotted darker accent along the centre — reads as a sidewalk seam.
	for x := 0; x < p.playfieldW; x += 3 {
		p.setPF(c, x, medianY+1, medianDark)
	}

	// Road.
	p.fillRectPF(c, 0, roadY0, p.playfieldW, roadH, roadColor)
	// Lane dividers — yellow dashed lines on each lane seam.
	for laneI := 1; laneI < 5; laneI++ {
		y := roadY0 + laneI*laneH - 1
		for x := 0; x < p.playfieldW; x += 4 {
			p.setPF(c, x, y, roadStripe)
			p.setPF(c, x+1, y, roadStripe)
		}
	}

	// Start strip (bottom safe grass).
	p.fillRectPF(c, 0, startY, p.playfieldW, startH, grassColor)
	// Thin darker stripe to suggest grass blades.
	for x := 0; x < p.playfieldW; x += 5 {
		p.setPF(c, x, startY+startH-1, grassDark)
	}
}

// -- Homes ------------------------------------------------------------

func (p *playScene) drawHomes(c *engine.Canvas) {
	// Hedge backdrop — dark stone wall, NOT another shade of grass. Two
	// tones of mortar plus the occasional moss fleck so it reads as a
	// solid barrier you can't hop onto.
	p.fillRectPF(c, 0, homeStripY, p.playfieldW, homeStripH, hedgeColor)
	// Top crenellation row — alternating dark/light stones gives the
	// silhouette a jagged "wall" outline rather than a smooth lawn edge.
	for x := 0; x < p.playfieldW; x++ {
		if (x/2)%2 == 0 {
			p.setPF(c, x, homeStripY, hedgeDark)
		}
	}
	// Sub-top moss row + bottom shadow.
	for x := 0; x < p.playfieldW; x += 3 {
		p.setPF(c, x, homeStripY+1, hedgeMoss)
	}
	p.fillRectPF(c, 0, homeStripY+homeStripH-1, p.playfieldW, 1, hedgeDark)

	for i := 0; i < numHomes; i++ {
		x0, x1 := homeSlotX(i, p.playfieldW)
		slotW := x1 - x0
		slotTop := homeStripY + 1
		slotH := homeStripH - 2

		// Lily-pad interior: bright pink ring around a darker centre.
		p.fillRectPF(c, x0, slotTop, slotW, slotH, homePadEdge)
		if slotW > 2 && slotH > 2 {
			p.fillRectPF(c, x0+1, slotTop+1, slotW-2, slotH-2, homePadIn)
		}
		// Bevel corners — pull the edge stones in so the slot feels
		// recessed into the wall.
		p.setPF(c, x0, slotTop, hedgeDark)
		p.setPF(c, x1-1, slotTop, hedgeDark)
		p.setPF(c, x0, slotTop+slotH-1, hedgeDark)
		p.setPF(c, x1-1, slotTop+slotH-1, hedgeDark)

		switch {
		case p.homes[i].occupied:
			fx := (x0+x1)/2 - frogW/2
			spr := homedFrog
			if p.homes[i].hadLady {
				spr = homedLady
			}
			drawColorSprite(c, p.px(fx), p.py(slotTop), spr, false)
		case p.crocSlot == i:
			drawColorSprite(c, p.px(x0-1), p.py(slotTop), crocSprite, false)
		default:
			// Empty open slot: faint frog silhouette so the player reads
			// "this is where the frog goes" instead of "this is more
			// hazard". Drawn before the fly bonus so the fly sits on top.
			drawEmptyHomeGhost(c, p.px(x0)+(slotW-frogW)/2, p.py(slotTop))
			if p.flySlot == i {
				fxC := (x0+x1)/2 - flySprite.width()/2
				drawColorSprite(c, p.px(fxC), p.py(slotTop), flySprite, false)
			}
		}
	}
}

// drawEmptyHomeGhost paints a faint frog-shaped marker inside an empty
// slot — same shape as homedFrog but in a low-contrast colour. It says
// "land here" without competing visually with the live frog.
func drawEmptyHomeGhost(c *engine.Canvas, x, y int) {
	ghost := colorSprite{
		rows:    homedFrog.rows,
		palette: map[byte]engine.Color{'G': homeGhost, 'E': homeGhost},
	}
	drawColorSprite(c, x, y, ghost, false)
}

// -- Lanes (cars, logs, turtles) --------------------------------------

func (p *playScene) drawLanes(c *engine.Canvas) {
	for laneIdx := range p.lanes {
		ln := p.lanes[laneIdx]
		switch {
		case ln.spec.isLog:
			p.drawLogLane(c, ln)
		case ln.spec.isTurtle:
			p.drawTurtleLane(c, ln)
		default:
			p.drawVehicleLane(c, ln)
		}
	}
}

func (p *playScene) drawVehicleLane(c *engine.Canvas, ln laneState) {
	lo, hi := ln.visibleEntityIndices(p.playfieldW)
	for i := lo; i <= hi; i++ {
		x := ln.entityX(i)
		spr := ln.spec.carSprites[((i%len(ln.spec.carSprites))+len(ln.spec.carSprites))%len(ln.spec.carSprites)]
		// Faced opposite of dir: sedans/sprites are authored facing
		// right, so flip when moving left.
		flip := ln.spec.dir < 0
		drawColorSprite(c, p.px(int(x)), p.py(ln.spec.yTop), spr, flip)
	}
}

func (p *playScene) drawLogLane(c *engine.Canvas, ln laneState) {
	lo, hi := ln.visibleEntityIndices(p.playfieldW)
	for i := lo; i <= hi; i++ {
		x := int(ln.entityX(i))
		p.drawLog(c, x, ln.spec.yTop, ln.spec.entityW)
	}
}

// drawLog paints a log of width w at playfield (x, y). The log is laneH
// pixels tall with bark-coloured end caps and a lighter ridge along the
// middle row.
func (p *playScene) drawLog(c *engine.Canvas, x, y, w int) {
	// Body.
	p.fillRectPF(c, x, y, w, laneH, logMid)
	// Ridge on top.
	p.fillRectPF(c, x, y, w, 1, logLight)
	// Bottom shadow.
	p.fillRectPF(c, x, y+laneH-1, w, 1, logDark)
	// Bark notches every 4 px, alternating positions for visual rhythm.
	for nx := x + 2; nx < x+w-1; nx += 4 {
		p.setPF(c, nx, y+1, logDark)
	}
	// End caps — slightly darker rings at both ends.
	p.setPF(c, x, y, logDark)
	p.setPF(c, x, y+laneH-1, logDark)
	p.setPF(c, x+w-1, y, logDark)
	p.setPF(c, x+w-1, y+laneH-1, logDark)
}

func (p *playScene) drawTurtleLane(c *engine.Canvas, ln laneState) {
	phase := ln.turtleDivePhase()
	lo, hi := ln.visibleEntityIndices(p.playfieldW)
	// Each entity is a row of turtles. Compute how many turtles fit in
	// the entity width (one turtle is 5 px wide).
	turtleW := turtleSurface.width()
	// Group size = entityW / turtleW (roughly), with 1-px gaps between
	// turtles.
	gap := 1
	count := (ln.spec.entityW + gap) / (turtleW + gap)
	if count < 1 {
		count = 1
	}
	for i := lo; i <= hi; i++ {
		x := int(ln.entityX(i))
		for j := 0; j < count; j++ {
			tx := x + j*(turtleW+gap)
			var spr colorSprite
			switch phase {
			case 0:
				// Surface — alternate two frames per turtle for animated paddle.
				if (i+j+int(ln.diveT*3))%2 == 0 {
					spr = turtleSurface
				} else {
					spr = turtleSurfaceB
				}
			case 1:
				// Warning blink: shell-only.
				if int(ln.diveT*8)%2 == 0 {
					spr = turtleSurface
				} else {
					spr = turtleSinking
				}
			case 2:
				continue // submerged — nothing to draw
			}
			// Turtles face the direction of motion.
			flip := ln.spec.dir > 0
			drawColorSprite(c, p.px(tx), p.py(ln.spec.yTop), spr, flip)
		}
	}
}

// -- Lady frog --------------------------------------------------------

func (p *playScene) drawLadyFrog(c *engine.Canvas) {
	if p.ladyLaneIdx < 0 {
		return
	}
	ln := p.lanes[p.ladyLaneIdx]
	ex := ln.entityX(p.ladyEntityIdx)
	// Sit her near the leading edge of the log so she's visible from far away.
	logEdgeX := int(ex) + ln.spec.entityW/2 - frogW/2
	drawColorSprite(c, p.px(logEdgeX), p.py(ln.spec.yTop), ladyFrogStand, false)
}

// -- Frog ------------------------------------------------------------

func (p *playScene) drawFrog(c *engine.Canvas) {
	x := int(p.frog.x)
	y := int(p.frog.y)
	switch p.frog.state {
	case fsSplat:
		drawColorSprite(c, p.px(x), p.py(y), frogSplat, false)
		return
	case fsSplash:
		// Animate over the death duration.
		t := (deathDuration - p.frog.dieT) / deathDuration
		spr := splashA
		if int(t*6)%2 == 0 {
			spr = splashB
		}
		drawColorSprite(c, p.px(x), p.py(y), spr, false)
		return
	case fsHome:
		// Drawn by drawHomes once the slot is occupied; nothing extra
		// to render here.
		return
	}

	spr := p.frogSpriteFor(p.frog.facing, p.frog.state == fsHopping)
	drawColorSprite(c, p.px(x), p.py(y), spr, false)
}

func (p *playScene) frogSpriteFor(d hopDir, hopping bool) colorSprite {
	switch d {
	case hopDown:
		if hopping {
			return frogDownHop
		}
		return frogDownStand
	case hopLeft:
		if hopping {
			return frogLeftHop
		}
		return frogLeftStand
	case hopRight:
		if hopping {
			return frogRightHop
		}
		return frogRightStand
	default: // hopUp / hopNone
		if hopping {
			return frogUpHop
		}
		return frogUpStand
	}
}

// -- Time bar ---------------------------------------------------------

func (p *playScene) drawTimeBar(c *engine.Canvas) {
	// Two-pixel-tall bar across the bottom of the playfield.
	p.fillRectPF(c, 0, timeY, p.playfieldW, 2, timeBarBack)
	frac := p.timeLeft / timeBarDuration
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	w := int(float64(p.playfieldW) * frac)
	col := timeBarOK
	switch {
	case frac < 0.2:
		// Flicker when nearly out.
		if int(p.stateT*8)%2 == 0 {
			col = timeBarDanger
		} else {
			col = engine.Color{R: 120, G: 30, B: 30, A: 255}
		}
	case frac < 0.5:
		col = timeBarWarn
	}
	p.fillRectPF(c, 0, timeY, w, 2, col)
	// "TIME" label inside the bar.
	const label = "TIME"
	labelCol := engine.Color{R: 10, G: 10, B: 10, A: 255}
	// Position label near the right side of the playfield in cell coords.
	row := (p.py(timeY)) / 2
	col0 := p.px(p.playfieldW) - len(label) - 1
	if col0 >= 0 {
		c.Print(col0, row, label, labelCol)
	}
}

// -- HUD --------------------------------------------------------------

func (p *playScene) drawHUD(c *engine.Canvas) {
	// HUD background — black strip.
	p.fillRectPF(c, 0, 0, p.playfieldW, hudH, bgColor)

	// Cell rows occupied by the HUD.
	row0 := p.py(0) / 2
	row1 := row0 + 1

	scoreText := fmt.Sprintf("1UP %06d", p.score)
	hiText := fmt.Sprintf("HI %06d", p.hiScore)
	levelText := fmt.Sprintf("LV %d", p.level)

	c.Print(p.px(1), row0, scoreText, scoreColor)
	// HI centred on the playfield, in the same cell row.
	hiCol := p.px((p.playfieldW-len(hiText))/2)
	c.Print(hiCol, row0, hiText, hiColor)
	// Level top-right.
	c.Print(p.px(p.playfieldW)-len(levelText)-1, row0, levelText, livesColor)

	// Lives display on second HUD row — small frog icons.
	for i := 0; i < p.lives-1 && i < 6; i++ {
		x := 1 + i*(frogW+1)
		drawColorSprite(c, p.px(x), p.py(hudH-frogH), frogUpStand, false)
	}
	// Caption hint on bottom-right of HUD.
	hint := "ESC QUIT"
	c.Print(p.px(p.playfieldW)-len(hint)-1, row1, hint, hintColor)
}

// -- Popups -----------------------------------------------------------

func (p *playScene) drawPopups(c *engine.Canvas) {
	for _, pop := range p.popups {
		frac := pop.age / popupLifetime
		if frac > 1 {
			frac = 1
		}
		dim := 1.0 - frac*0.6
		col := engine.Color{
			R: uint8(float64(pop.col.R) * dim),
			G: uint8(float64(pop.col.G) * dim),
			B: uint8(float64(pop.col.B) * dim),
			A: 255,
		}
		cellCol := p.px(int(pop.x) - len(pop.text)/2)
		if cellCol < 0 {
			cellCol = 0
		}
		row := p.py(int(pop.y)) / 2
		if row < 0 {
			row = 0
		}
		c.Print(cellCol, row, pop.text, col)
	}
}

// -- Banners ----------------------------------------------------------

func (p *playScene) drawPreStageBanner(c *engine.Canvas) {
	msg := fmt.Sprintf("LEVEL %d", p.level)
	if p.level == 1 {
		msg = "GET READY"
	}
	w := engine.TextWidth(msg)
	cx := p.px(p.playfieldW / 2)
	cy := p.py(playfieldH / 2)
	c.FillRect(cx-w/2-4, cy-engine.FontHeight/2-2, w+8, engine.FontHeight+4,
		engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(cx-w/2, cy-engine.FontHeight/2, msg, flashColor)
}

func (p *playScene) drawWaveClearBanner(c *engine.Canvas) {
	msg := "ALL HOMES! +1000"
	w := engine.TextWidth(msg)
	cx := p.px(p.playfieldW / 2)
	cy := p.py(playfieldH / 2)
	flash := int(p.stateT*4)%2 == 0
	bg := engine.Color{R: 8, G: 8, B: 24, A: 255}
	if flash {
		bg = engine.Color{R: 40, G: 24, B: 60, A: 255}
	}
	c.FillRect(cx-w/2-4, cy-engine.FontHeight/2-2, w+8, engine.FontHeight+4, bg)
	c.DrawText(cx-w/2, cy-engine.FontHeight/2, msg, flashColor)
}

func (p *playScene) drawGameOverBanner(c *engine.Canvas) {
	msg := "GAME OVER"
	w := engine.TextWidth(msg)
	cx := p.px(p.playfieldW / 2)
	cy := p.py(playfieldH / 2 - 4)
	c.FillRect(cx-w/2-4, cy-engine.FontHeight/2-2, w+8, engine.FontHeight+4,
		engine.Color{R: 30, G: 8, B: 8, A: 255})
	c.DrawText(cx-w/2, cy-engine.FontHeight/2, msg, engine.Color{R: 255, G: 80, B: 80, A: 255})

	hint := "ENTER: TITLE   ESC: QUIT"
	hx := cx - len(hint)/2
	hy := (cy + engine.FontHeight + 4) / 2
	c.Print(hx, hy, hint, hintColor)
}

