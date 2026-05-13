package scramble

import (
	"fmt"
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Draw paints the world this frame. Layering, painter's-algorithm style:
//   1) background fill + stars
//   2) terrain (ground + ceiling)
//   3) enemies that live behind projectiles
//   4) player, projectiles, missiles
//   5) explosions on top
//   6) HUD overlay
//   7) any banner overlay (stage intro / game over / victory)
func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(colBg)
	p.drawStars(c)
	p.drawTerrain(c)
	p.drawEnemies(c)
	p.drawMissiles(c)
	p.drawBombs(c)
	p.drawBullets(c)
	p.drawPlayer(c)
	p.drawBooms(c)
	p.drawHUD(c)

	switch p.state {
	case psStageIntro:
		p.drawCentreBanner(c, stageTitle(p.stage), engine.Color{R: 250, G: 230, B: 140, A: 255})
	case psStageCleared:
		p.drawCentreBanner(c, "STAGE CLEAR", engine.Color{R: 200, G: 240, B: 160, A: 255})
	case psVictory:
		p.drawVictory(c)
	case psGameOver:
		p.drawGameOver(c)
	}
}

func stageTitle(stage int) string {
	switch stage {
	case 1:
		return "STAGE 1  MOUNTAINS"
	case 2:
		return "STAGE 2  UFO FLEET"
	case 3:
		return "STAGE 3  METEORS"
	case 4:
		return "STAGE 4  CAVERN"
	case 5:
		return "STAGE 5  CITY"
	case 6:
		return "STAGE 6  BASE"
	}
	return fmt.Sprintf("STAGE %d", stage)
}

func (p *playScene) drawStars(c *engine.Canvas) {
	for _, s := range p.stars {
		col := starPalette[s.c]
		if math.Sin(s.twink) < -0.4 {
			col = engine.Color{R: col.R / 3, G: col.G / 3, B: col.B / 3, A: 255}
		}
		c.Set(int(s.x), int(s.y), col)
	}
}

// drawTerrain renders ground and ceiling for every visible column,
// using a style appropriate to the stage.
func (p *playScene) drawTerrain(c *engine.Canvas) {
	gColA, gColB := terrainColors(p.terrain.kind)
	for sx := 0; sx < p.w; sx++ {
		wx := int(p.cameraX) + sx
		if wx < 0 || wx >= len(p.terrain.ground) {
			continue
		}
		g := p.terrain.ground[wx]
		cl := p.terrain.ceil[wx]
		// Ground from g down to pfBot.
		for y := g; y < p.pfBot; y++ {
			col := gColA
			if (y-g)%3 == 0 {
				col = gColB
			}
			c.Set(sx, y, col)
		}
		// Highlight pixel along the ground top edge.
		if g >= p.pfTop && g < p.pfBot {
			c.Set(sx, g, terrainTopColor(p.terrain.kind))
		}
		// Ceiling from pfTop down to cl (inclusive).
		if cl >= p.pfTop {
			for y := p.pfTop; y <= cl; y++ {
				col := gColB
				if (cl-y)%3 == 0 {
					col = gColA
				}
				c.Set(sx, y, col)
			}
		}
	}
	// City: paint window dots so building tops read like skyline.
	if p.terrain.kind == terCity {
		for sx := 0; sx < p.w; sx++ {
			wx := int(p.cameraX) + sx
			if wx < 0 || wx >= len(p.terrain.ground) {
				continue
			}
			g := p.terrain.ground[wx]
			// Only draw windows on the building face, not the wide low
			// ground between buildings — i.e. when the column is well
			// below the open-ground baseline.
			baseGround := p.pfTop + int(float64(p.pfBot-p.pfTop)*0.88)
			if g >= baseGround {
				continue
			}
			for y := g + 2; y < p.pfBot-1; y += 3 {
				if (sx+y)%4 == 0 {
					c.Set(sx, y, colCityLit)
				}
			}
		}
	}
}

// terrainColors returns the main/highlight colours for the current stage.
func terrainColors(k terrainKind) (a, b engine.Color) {
	switch k {
	case terMountain, terMountainNoCeil, terFlat:
		return colMountain, colMountain2
	case terCavern:
		return colCavern, colCavern2
	case terCity:
		return colCity, engine.Color{R: 60, G: 90, B: 150, A: 255}
	case terBase:
		return colBase, colBase2
	}
	return colMountain, colMountain2
}

func terrainTopColor(k terrainKind) engine.Color {
	switch k {
	case terCity:
		return colCityLit
	case terBase:
		return colReactor2
	case terCavern:
		return engine.Color{R: 230, G: 200, B: 160, A: 255}
	}
	return engine.Color{R: 130, G: 240, B: 130, A: 255}
}

// drawEnemies renders each living enemy at its screen position.
func (p *playScene) drawEnemies(c *engine.Canvas) {
	for _, e := range p.enemies {
		sx := int(e.x - p.cameraX)
		if sx < -16 || sx > p.w+16 {
			continue
		}
		switch e.kind {
		case entRocket:
			spr := rocketIdle
			col := colRocket
			if e.launched {
				spr = rocketLaunch
				col = colRocket
			}
			drawSprite(c, sx, int(e.y), spr, col)
			// Tint the flame portion in orange for visual clarity.
			if e.launched {
				for r := 5; r < spr.height(); r++ {
					for col := 0; col < spr.width(); col++ {
						if spr[r][col] == '#' {
							c.Set(sx+col, int(e.y)+r, colFlame)
						}
					}
				}
			}
		case entUFO:
			spr := ufoA
			if e.frame == 1 {
				spr = ufoB
			}
			drawSprite(c, sx, int(e.y), spr, colUFO)
		case entFireball:
			spr := fireballA
			if e.frame == 1 {
				spr = fireballB
			}
			drawSprite(c, sx, int(e.y), spr, colFire)
			// Short comet trail behind.
			for tr := 1; tr <= 3; tr++ {
				tx := sx + 2 - tr*2
				ty := int(e.y) + 2 - tr
				col := colFlame
				if tr == 3 {
					col = engine.Color{R: 200, G: 80, B: 50, A: 255}
				}
				c.Set(tx, ty, col)
			}
		case entFuel:
			drawSprite(c, sx, int(e.y), fuelTank, colFuel)
			// Letter label below tank if there's room.
			if int(e.y)+fuelTank.height()+1 < p.pfBot {
				c.Print(sx+1, (int(e.y)+fuelTank.height())/2, "F", colFuel)
			}
		case entTower:
			drawSprite(c, sx, int(e.y), baseTower, colTower)
		case entReactor:
			p.drawReactor(c, sx, int(e.y), e)
		}
	}
}

// drawReactor paints the reactor with a pulsing core that throbs faster
// once it's taken damage.
func (p *playScene) drawReactor(c *engine.Canvas, sx, sy int, e *entity) {
	drawSprite(c, sx, sy, reactor, colReactor)
	// Pulsing yellow core.
	phase := e.cooldown * (1.0 + float64(e.hits)*0.8)
	pulse := 0.5 + 0.5*math.Sin(phase*4*math.Pi)
	coreColor := engine.Color{
		R: uint8(180 + int(60*pulse)),
		G: uint8(150 + int(80*pulse)),
		B: 40,
		A: 255,
	}
	cw := reactor.width()
	ch := reactor.height()
	cx := sx + cw/2
	cy := sy + ch/2
	c.FillCircle(cx, cy, 2, coreColor)
	if e.hits > 0 {
		c.Set(cx-1, cy-1, colExplode)
		c.Set(cx+1, cy+1, colExplode)
	}
}

func (p *playScene) drawMissiles(c *engine.Canvas) {
	for _, m := range p.missiles {
		sx := int(m.x - p.cameraX)
		drawSprite(c, sx, int(m.y), missile, colMissile)
	}
}

func (p *playScene) drawBullets(c *engine.Canvas) {
	for _, b := range p.bullets {
		sx := int(b.x - p.cameraX)
		drawSprite(c, sx, int(b.y), playerBullet, colBullet)
	}
}

func (p *playScene) drawBombs(c *engine.Canvas) {
	for _, b := range p.bombs {
		sx := int(b.x - p.cameraX)
		drawSprite(c, sx, int(b.y), playerBomb, colBomb)
	}
}

// drawPlayer paints the player ship — handling explosion frames during
// death and the blink during respawn invincibility.
func (p *playScene) drawPlayer(c *engine.Canvas) {
	if p.state == psGameOver && p.player.explodeT <= 0 {
		return
	}
	if p.player.explodeT > 0 {
		// Alternate explosion frames at ~10 Hz.
		spr := playerExplodeA
		if int((playerExplodeDur-p.player.explodeT)*10)%2 == 1 {
			spr = playerExplodeB
		}
		drawSprite(c, int(p.player.x), int(p.player.y), spr, colExplode)
		return
	}
	if p.player.respawnT > 0 && int(p.player.respawnT*8)%2 == 0 {
		return
	}
	col := colPlayer
	if p.fuel < fuelLowAt && int(p.stateT*4)%2 == 0 {
		col = colPlayerDim
	}
	drawSprite(c, int(p.player.x), int(p.player.y), playerSprite, col)
}

func (p *playScene) drawBooms(c *engine.Canvas) {
	for _, b := range p.booms {
		sx := int(b.x - p.cameraX)
		sy := int(b.y)
		step := int(b.dieT / 0.15)
		var spr sprite
		switch step {
		case 0:
			spr = explode0
		case 1:
			spr = explode1
		default:
			spr = explode2
		}
		col := colExplode
		if step >= 2 {
			col = engine.Color{R: 200, G: 100, B: 60, A: 255}
		}
		drawSprite(c, sx, sy, spr, col)
	}
}

// -------- HUD ------------------------------------------------------------

// drawHUD paints the score / hi-score / stage and the fuel bar at the
// top of the screen. Lives are drawn as small player-icon repeats.
func (p *playScene) drawHUD(c *engine.Canvas) {
	cols := c.Cols()
	// Top row: SCORE, HI, STAGE.
	scoreText := fmt.Sprintf("SCORE %06d", p.score)
	hiText := fmt.Sprintf("HI %06d", p.hiScore)
	stageText := fmt.Sprintf("STAGE %d-%d", p.stage, p.loop+1)

	c.Print(1, 0, scoreText, engine.White)
	mid := (cols - len(hiText)) / 2
	if mid < len(scoreText)+2 {
		mid = len(scoreText) + 2
	}
	c.Print(mid, 0, hiText, engine.Yellow)
	right := cols - len(stageText) - 1
	if right < mid+len(hiText)+2 {
		right = mid + len(hiText) + 2
	}
	c.Print(right, 0, stageText, engine.Cyan)

	// Second cell row: FUEL bar on left, LIVES on right.
	p.drawFuelBar(c)
	p.drawLives(c)
}

// drawFuelBar paints a horizontal gauge showing remaining fuel. The
// colour shifts amber when fuel is low and red when critical.
func (p *playScene) drawFuelBar(c *engine.Canvas) {
	label := "FUEL"
	c.Print(1, 1, label, engine.White)
	barX := 6
	barW := 18
	if c.Cols() < 50 {
		barW = 12
	}
	cellsFilled := int(float64(barW) * p.fuel / fuelMax)
	if cellsFilled < 0 {
		cellsFilled = 0
	}
	if cellsFilled > barW {
		cellsFilled = barW
	}
	col := engine.Color{R: 80, G: 220, B: 100, A: 255}
	if p.fuel < fuelMax*0.45 {
		col = engine.Color{R: 240, G: 200, B: 80, A: 255}
	}
	if p.fuel < fuelMax*0.20 {
		col = engine.Color{R: 240, G: 90, B: 80, A: 255}
	}
	// Draw the bar using pixel rectangles (the pixel pair makes one cell
	// row): two rows tall covers cell row 1.
	pxX := barX
	for i := 0; i < barW; i++ {
		x := pxX + i
		if x >= c.Width() {
			break
		}
		fillCol := engine.Color{R: 40, G: 40, B: 60, A: 255}
		if i < cellsFilled {
			fillCol = col
		}
		c.Set(x, 2, fillCol)
		c.Set(x, 3, fillCol)
	}
}

// drawLives paints small player-ship icons across the right of the
// second HUD row.
func (p *playScene) drawLives(c *engine.Canvas) {
	cols := c.Cols()
	reserve := p.player.lives - 1
	if p.player.explodeT > 0 {
		reserve = p.player.lives
	}
	if reserve < 0 {
		reserve = 0
	}
	if reserve > 6 {
		reserve = 6
	}
	iconW := playerSprite.width()
	gap := 1
	totalW := reserve*(iconW+gap) - gap
	if totalW < 0 {
		totalW = 0
	}
	startX := cols - totalW - 1
	if startX < 0 {
		startX = 0
	}
	for i := 0; i < reserve; i++ {
		x := startX + i*(iconW+gap)
		drawSprite(c, x, 2, playerSprite, colPlayer)
	}
}

// -------- Banners --------------------------------------------------------

func (p *playScene) drawCentreBanner(c *engine.Canvas, text string, col engine.Color) {
	w := engine.TextWidth(text)
	x := (p.w - w) / 2
	y := (p.pfTop+p.pfBot)/2 - engine.FontHeight/2
	c.FillRect(x-3, y-2, w+6, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, text, col)
}

func (p *playScene) drawVictory(c *engine.Canvas) {
	// Big "BASE DESTROYED" flashing banner, then loop info.
	mainCol := engine.Color{R: 250, G: 200, B: 90, A: 255}
	if int(p.stateT*4)%2 == 0 {
		mainCol = engine.Color{R: 250, G: 250, B: 250, A: 255}
	}
	w1 := engine.TextWidth("BASE DESTROYED")
	x1 := (p.w - w1) / 2
	y1 := (p.pfTop+p.pfBot)/2 - engine.FontHeight - 2
	c.FillRect(x1-4, y1-2, w1+8, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x1, y1, "BASE DESTROYED", mainCol)
	bonus := "BONUS 5000"
	bw := engine.TextWidth(bonus)
	c.DrawText((p.w-bw)/2, y1+engine.FontHeight+4, bonus, engine.Color{R: 200, G: 240, B: 120, A: 255})
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	w := engine.TextWidth("GAME OVER")
	x := (p.w - w) / 2
	y := (p.pfTop+p.pfBot)/2 - engine.FontHeight - 2
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, "GAME OVER", engine.Color{R: 250, G: 90, B: 90, A: 255})

	hint := "ENTER PLAY AGAIN   ESC QUIT"
	hw := len(hint)
	c.Print((c.Cols()-hw)/2, (y+engine.FontHeight+4)/2, hint, engine.White)
}
