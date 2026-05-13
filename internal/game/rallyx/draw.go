package rallyx

import (
	"fmt"
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// World colours, lifted to roughly match the arcade palette: walls
// are blue blocks, roads are black, the player is blue, the enemies
// are red, flags are yellow, the special flag is pink, the lucky
// flag pulses cyan, rocks are gray, smoke is a faded gray fade.
var (
	colWall       = engine.Color{R: 33, G: 50, B: 220, A: 255}
	colWallSh     = engine.Color{R: 20, G: 30, B: 160, A: 255}
	colRoad       = engine.Color{R: 0, G: 0, B: 0, A: 255}
	colRoadEdge   = engine.Color{R: 25, G: 25, B: 35, A: 255}
	colPlayer     = engine.Color{R: 70, G: 165, B: 255, A: 255}
	colPlayerWin  = engine.Color{R: 230, G: 240, B: 255, A: 255}
	colEnemy      = engine.Color{R: 235, G: 60, B: 60, A: 255}
	colEnemyDim   = engine.Color{R: 180, G: 40, B: 40, A: 255}
	colEnemyStun  = engine.Color{R: 110, G: 120, B: 230, A: 255}
	colFlag       = engine.Color{R: 255, G: 230, B: 30, A: 255}
	colFlagPole   = engine.Color{R: 230, G: 230, B: 230, A: 255}
	colSpecial    = engine.Color{R: 255, G: 130, B: 230, A: 255}
	colLucky      = engine.Color{R: 80, G: 230, B: 255, A: 255}
	colRock       = engine.Color{R: 150, G: 150, B: 150, A: 255}
	colRockSh     = engine.Color{R: 80, G: 80, B: 80, A: 255}
	colRockHi     = engine.Color{R: 220, G: 220, B: 220, A: 255}
	colRockDanger = engine.Color{R: 255, G: 90, B: 60, A: 255}
	colSmokeHot   = engine.Color{R: 230, G: 230, B: 230, A: 255}
	colSmokeCold  = engine.Color{R: 80, G: 80, B: 80, A: 255}
	colWreck      = engine.Color{R: 80, G: 80, B: 80, A: 255}
	colWreckSpark = engine.Color{R: 255, G: 180, B: 60, A: 255}
)

// geometry is the per-frame layout summary: where the main viewport
// goes, where the radar goes, what tile size we ended up with. Each
// Draw call computes it fresh so the rendering adapts to any
// terminal size.
type geometry struct {
	canvasW, canvasH int

	// Top HUD occupies the first hudTop terminal rows. The bottom HUD
	// occupies the last hudBot rows.
	hudTop int
	hudBot int

	// Main viewport in pixel coords.
	viewX, viewY int
	viewW, viewH int

	// Tile size in pixels, the same for the world view (viewport) and
	// scaled differently for the radar.
	tile int

	// Visible tile range. The viewport is anchored on the player but
	// clamped so it never shows past the world edges.
	viewStartCol int
	viewStartRow int
	viewEndCol   int
	viewEndRow   int

	// Radar dimensions.
	radarX, radarY    int
	radarW, radarH    int
	radarTileX        float64
	radarTileY        float64
}

func computeGeometry(c *engine.Canvas, p *playScene) geometry {
	g := geometry{
		canvasW: c.Width(),
		canvasH: c.Height(),
		hudTop:  2,
		hudBot:  3,
	}

	// Decide the radar width. We want it to feel like a mini-map: ~30%
	// of the canvas width up to a sensible max. Floor at 18 px so
	// there's enough resolution to read the flag positions.
	radarW := g.canvasW / 3
	if radarW > 60 {
		radarW = 60
	}
	if radarW < 18 {
		radarW = 18
	}
	if radarW > g.canvasW-18 {
		radarW = g.canvasW - 18
	}

	// Main viewport occupies the canvas minus the radar (right) and
	// the HUDs (top + bottom).
	viewX := 0
	viewY := g.hudTop * 2
	viewW := g.canvasW - radarW - 2
	viewH := g.canvasH - (g.hudTop+g.hudBot)*2
	if viewW < 10 {
		viewW = 10
	}
	if viewH < 10 {
		viewH = 10
	}
	g.viewX, g.viewY, g.viewW, g.viewH = viewX, viewY, viewW, viewH

	// Pick the largest integer tile size such that a useful chunk of
	// the world fits in the viewport. The viewport should ideally show
	// somewhere around 14×10 tiles; we lock the tile to whatever
	// allows that with the available pixels.
	desiredCols := 14
	desiredRows := 10
	tile := viewW / desiredCols
	if h := viewH / desiredRows; h < tile {
		tile = h
	}
	if tile < 2 {
		tile = 2
	}
	if tile > 6 {
		tile = 6
	}
	g.tile = tile

	// Anchor viewport on the player.
	visCols := viewW / tile
	visRows := viewH / tile
	if visCols > mazeCols {
		visCols = mazeCols
	}
	if visRows > mazeRows {
		visRows = mazeRows
	}
	startCol := int(math.Floor(p.player.x)) - visCols/2
	startRow := int(math.Floor(p.player.y)) - visRows/2
	if startCol < 0 {
		startCol = 0
	}
	if startRow < 0 {
		startRow = 0
	}
	if startCol+visCols > mazeCols {
		startCol = mazeCols - visCols
	}
	if startRow+visRows > mazeRows {
		startRow = mazeRows - visRows
	}
	g.viewStartCol = startCol
	g.viewStartRow = startRow
	g.viewEndCol = startCol + visCols
	g.viewEndRow = startRow + visRows

	// Radar — placed in the top-right corner, fits inside the
	// remaining canvas to the right of the viewport.
	radarX := g.canvasW - radarW
	radarY := g.hudTop * 2
	radarH := g.canvasH - (g.hudTop+g.hudBot)*2
	g.radarX, g.radarY, g.radarW, g.radarH = radarX, radarY, radarW, radarH
	g.radarTileX = float64(radarW) / float64(mazeCols)
	g.radarTileY = float64(radarH) / float64(mazeRows)

	return g
}

// Draw is the top-level render hook for the playScene.
func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Black)
	geo := computeGeometry(c, p)

	p.drawWorld(c, geo)
	p.drawRadar(c, geo)
	p.drawHUD(c, geo)
	p.drawOverlay(c, geo)
}

// drawWorld renders the visible portion of the maze, plus all the
// living entities and the smoke trail.
func (p *playScene) drawWorld(c *engine.Canvas, geo geometry) {
	// Backdrop: solid black behind the play area gives the road its
	// colour. The walls then sit on top in blue.
	c.FillRect(geo.viewX, geo.viewY, geo.viewW, geo.viewH, colRoad)

	// World tiles.
	for r := geo.viewStartRow; r < geo.viewEndRow; r++ {
		for col := geo.viewStartCol; col < geo.viewEndCol; col++ {
			px, py := geo.worldToPixel(float64(col)+0.5, float64(r)+0.5)
			x0 := px - geo.tile/2
			y0 := py - geo.tile/2
			t := p.maze.at(col, r)
			switch t {
			case tileWall:
				drawWallTile(c, x0, y0, geo.tile)
			case tileRock:
				drawRockTile(c, x0, y0, geo.tile)
			}
		}
	}

	// Subtle road grid for navigation feel — a single dim pixel at
	// each intersection that isn't a wall.
	if geo.tile >= 4 {
		for r := geo.viewStartRow; r < geo.viewEndRow; r++ {
			for col := geo.viewStartCol; col < geo.viewEndCol; col++ {
				if p.maze.at(col, r) != tileRoad {
					continue
				}
				px, py := geo.worldToPixel(float64(col)+0.5, float64(r)+0.5)
				c.Set(px, py, colRoadEdge)
			}
		}
	}

	// Pickups.
	for _, pk := range p.maze.flags {
		if pk.taken {
			continue
		}
		if !geo.inView(pk.col, pk.row) {
			continue
		}
		px, py := geo.worldToPixel(float64(pk.col)+0.5, float64(pk.row)+0.5)
		drawFlagSprite(c, px, py, geo.tile, colFlag)
	}
	if sf := p.maze.specialFlag; sf != nil && !sf.taken && geo.inView(sf.col, sf.row) {
		px, py := geo.worldToPixel(float64(sf.col)+0.5, float64(sf.row)+0.5)
		drawFlagSprite(c, px, py, geo.tile, colSpecial)
		// 'S' marker tucked at the flag's base in native-font text.
		if geo.tile >= 4 {
			c.Print(px/1, py/2, "S", engine.Black)
		}
	}
	if lf := p.maze.luckyFlag; lf != nil && !lf.taken && geo.inView(lf.col, lf.row) {
		px, py := geo.worldToPixel(float64(lf.col)+0.5, float64(lf.row)+0.5)
		col := colLucky
		// Pulse the lucky flag so the player can spot it across the map.
		if int(p.stateT*6)%2 == 0 {
			col = colFlag
		}
		drawFlagSprite(c, px, py, geo.tile, col)
		if geo.tile >= 4 {
			c.Print(px, py/2, "L", engine.Black)
		}
	}

	// Smoke screen — draw under the cars.
	for _, puff := range p.smoke {
		if !puff.alive() {
			continue
		}
		col := int(math.Floor(puff.x))
		row := int(math.Floor(puff.y))
		if !geo.inView(col, row) {
			continue
		}
		px, py := geo.worldToPixel(puff.x, puff.y)
		drawSmoke(c, px, py, geo.tile, puff.fade())
	}

	// Enemies (under player so a stunned enemy under the wheel is
	// drawn first, then the player is on top).
	for _, en := range p.enemies {
		if !en.alive && !en.crashed {
			continue
		}
		col := en.tileX()
		row := en.tileY()
		if !geo.inView(col, row) {
			continue
		}
		px, py := geo.worldToPixel(en.x, en.y)
		drawEnemy(c, px, py, geo.tile, en, p.stateT)
	}

	// Player car.
	if p.player.alive || p.state == psDying {
		col := p.player.tileX()
		row := p.player.tileY()
		if geo.inView(col, row) {
			px, py := geo.worldToPixel(p.player.x, p.player.y)
			drawPlayerCar(c, px, py, geo.tile, p.player.dir, p.state == psDying, p.stateT)
		}
	}

	// Score popups, in cell coords above the entities.
	for _, pop := range p.popups {
		col := int(math.Floor(pop.x))
		row := int(math.Floor(pop.y))
		if !geo.inView(col, row) {
			continue
		}
		px, py := geo.worldToPixel(pop.x, pop.y)
		frac := pop.age / pop.ttl
		if frac > 1 {
			frac = 1
		}
		dim := 1 - frac*0.6
		col2 := engine.Color{
			R: uint8(float64(pop.col.R) * dim),
			G: uint8(float64(pop.col.G) * dim),
			B: uint8(float64(pop.col.B) * dim),
			A: 255,
		}
		cellX := px - len(pop.text)/2
		cellY := py / 2
		c.Print(cellX, cellY, pop.text, col2)
	}

	// View border.
	c.DrawRect(geo.viewX, geo.viewY, geo.viewW, geo.viewH,
		engine.Color{R: 60, G: 60, B: 80, A: 255})
}

// drawRadar paints the mini-map in the right gutter. The radar shows
// the whole world at a tiny scale: walls as dim blue, flags as
// pulsing yellow, the player as blue, enemies as red.
func (p *playScene) drawRadar(c *engine.Canvas, geo geometry) {
	if geo.radarW <= 0 || geo.radarH <= 0 {
		return
	}

	// Background panel.
	c.FillRect(geo.radarX, geo.radarY, geo.radarW, geo.radarH,
		engine.Color{R: 0, G: 0, B: 0, A: 255})
	c.DrawRect(geo.radarX, geo.radarY, geo.radarW, geo.radarH,
		engine.Color{R: 60, G: 60, B: 80, A: 255})

	// Walls.
	wallCol := engine.Color{R: 25, G: 40, B: 120, A: 255}
	for r := 0; r < mazeRows; r++ {
		for col := 0; col < mazeCols; col++ {
			if p.maze.at(col, r) != tileWall {
				continue
			}
			x0 := geo.radarX + int(float64(col)*geo.radarTileX)
			y0 := geo.radarY + int(float64(r)*geo.radarTileY)
			x1 := geo.radarX + int(float64(col+1)*geo.radarTileX)
			y1 := geo.radarY + int(float64(r+1)*geo.radarTileY)
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			c.FillRect(x0, y0, x1-x0, y1-y0, wallCol)
		}
	}

	// Flags — pulse so they're easy to spot.
	pulse := int(p.stateT*4)%2 == 0
	for _, pk := range p.maze.flags {
		if pk.taken {
			continue
		}
		col := colFlag
		if !pulse {
			col = engine.Color{R: 160, G: 140, B: 30, A: 255}
		}
		x := geo.radarX + int((float64(pk.col)+0.5)*geo.radarTileX)
		y := geo.radarY + int((float64(pk.row)+0.5)*geo.radarTileY)
		c.Set(x, y, col)
		if x+1 < geo.radarX+geo.radarW {
			c.Set(x+1, y, col)
		}
	}
	if sf := p.maze.specialFlag; sf != nil && !sf.taken {
		x := geo.radarX + int((float64(sf.col)+0.5)*geo.radarTileX)
		y := geo.radarY + int((float64(sf.row)+0.5)*geo.radarTileY)
		c.Set(x, y, colSpecial)
		c.Set(x+1, y, colSpecial)
	}
	if lf := p.maze.luckyFlag; lf != nil && !lf.taken {
		col := colLucky
		if !pulse {
			col = colFlag
		}
		x := geo.radarX + int((float64(lf.col)+0.5)*geo.radarTileX)
		y := geo.radarY + int((float64(lf.row)+0.5)*geo.radarTileY)
		c.Set(x, y, col)
		c.Set(x+1, y, col)
	}

	// Player.
	{
		x := geo.radarX + int(p.player.x*geo.radarTileX)
		y := geo.radarY + int(p.player.y*geo.radarTileY)
		c.Set(x, y, colPlayer)
		c.Set(x+1, y, colPlayer)
		c.Set(x, y+1, colPlayer)
		c.Set(x+1, y+1, colPlayer)
	}

	// Enemies.
	for _, en := range p.enemies {
		if !en.alive || en.crashed {
			continue
		}
		x := geo.radarX + int(en.x*geo.radarTileX)
		y := geo.radarY + int(en.y*geo.radarTileY)
		col := colEnemy
		if en.smokeT > 0 {
			col = colEnemyStun
		}
		c.Set(x, y, col)
		c.Set(x+1, y, col)
	}

	// Viewport rectangle — shows what part of the map the player is
	// currently seeing.
	x0 := geo.radarX + int(float64(geo.viewStartCol)*geo.radarTileX)
	y0 := geo.radarY + int(float64(geo.viewStartRow)*geo.radarTileY)
	x1 := geo.radarX + int(float64(geo.viewEndCol)*geo.radarTileX)
	y1 := geo.radarY + int(float64(geo.viewEndRow)*geo.radarTileY)
	c.DrawRect(x0, y0, x1-x0, y1-y0, engine.Color{R: 200, G: 200, B: 200, A: 255})
}

// drawHUD paints the top score / hi-score line, the bottom fuel
// gauge, the lives indicator, and the flag tally.
func (p *playScene) drawHUD(c *engine.Canvas, geo geometry) {
	// Top row: "1UP <score>   HIGH SCORE <hi>   STAGE <n>".
	scoreStr := "1UP " + pad7(p.score)
	hiStr := "HIGH " + pad7(p.hiScore)
	stageStr := fmt.Sprintf("STAGE %d", p.stage)

	c.Print(1, 0, scoreStr, engine.Color{R: 255, G: 100, B: 100, A: 255})
	c.Print((c.Cols()-len(hiStr))/2, 0, hiStr, engine.White)
	c.Print(c.Cols()-len(stageStr)-1, 0, stageStr, engine.Cyan)

	// Bottom: fuel gauge + lives + flags remaining.
	bottomY := c.Rows() - geo.hudBot
	// Fuel bar.
	fuelLabel := "FUEL"
	c.Print(1, bottomY, fuelLabel, engine.Yellow)
	fuelW := c.Cols()/3 - len(fuelLabel) - 2
	if fuelW < 8 {
		fuelW = 8
	}
	if fuelW > c.Cols()-1-len(fuelLabel)-2 {
		fuelW = c.Cols() - 1 - len(fuelLabel) - 2
	}
	bx := 1 + len(fuelLabel) + 1
	by := bottomY*2 + 1
	// Outer frame.
	c.DrawRect(bx, by, fuelW, 2, engine.Color{R: 120, G: 120, B: 120, A: 255})
	// Filled portion.
	frac := p.fuel / fullTank
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	fill := int(float64(fuelW-2) * frac)
	col := engine.Color{R: 50, G: 220, B: 50, A: 255}
	if frac < 0.25 {
		col = engine.Color{R: 240, G: 220, B: 50, A: 255}
	}
	if frac < 0.1 {
		col = engine.Color{R: 240, G: 60, B: 50, A: 255}
	}
	if fill > 0 {
		c.FillRect(bx+1, by+1, fill, 1, col)
	}
	if frac < 0.15 && int(p.stateT*4)%2 == 0 {
		c.Print(bx+fuelW+2, bottomY, "LOW FUEL!",
			engine.Color{R: 255, G: 100, B: 100, A: 255})
	}

	// Lives.
	livesLabel := "CARS"
	livesX := c.Cols() - 24
	if livesX < 1 {
		livesX = 1
	}
	c.Print(livesX, bottomY, livesLabel, engine.White)
	for i := 0; i < p.lives-1 && i < 5; i++ {
		x := livesX + len(livesLabel) + 1 + i*3
		drawHUDCar(c, x*1, bottomY*2+2, colPlayer)
	}

	// Flag tally — count of remaining flags.
	flagX := c.Cols() - 11
	if flagX < 1 {
		flagX = 1
	}
	c.Print(flagX, c.Rows()-1,
		fmt.Sprintf("FLAGS %d/%d", p.maze.remainingFlags(), len(p.maze.flags)+countFromLucky(p.maze)),
		engine.Yellow)

	// Bottom hint line.
	hint := "ESC QUIT  SPACE SMOKE"
	if len(hint) < c.Cols()-2 {
		c.Print(1, c.Rows()-1, hint, engine.Gray)
	}

	// Stage indicator dots along the bottom — small ticked progression.
	if p.flagMult > 1 {
		c.Print(c.Cols()/2-3, c.Rows()-1, "X2 BONUS",
			engine.Color{R: 255, G: 130, B: 230, A: 255})
	}
}

// drawOverlay paints "READY!" / "GAME OVER" / "STAGE CLEAR" banners.
// Each banner shows the stage name underneath so the player can read
// the map they're getting before motion starts.
func (p *playScene) drawOverlay(c *engine.Canvas, geo geometry) {
	switch p.state {
	case psReady:
		if p.challenging {
			drawCentre(c, geo, "CHALLENGING STAGE",
				engine.Color{R: 80, G: 220, B: 255, A: 255})
		} else {
			drawCentre(c, geo, "READY!", engine.Color{R: 255, G: 240, B: 60, A: 255})
		}
		// Sub-label: "STAGE n — <map name>".
		sub := fmt.Sprintf("STAGE %d   %s", p.stage, p.maze.stageName)
		c.Print((c.Cols()-len(sub))/2, c.Rows()/2+3, sub, engine.White)
	case psStageClear:
		drawCentre(c, geo, fmt.Sprintf("STAGE %d CLEAR!", p.stage),
			engine.Color{R: 80, G: 220, B: 255, A: 255})
	case psGameOver:
		drawCentre(c, geo, "GAME OVER",
			engine.Color{R: 255, G: 60, B: 60, A: 255})
		hint := "ENTER / Q TO EXIT"
		c.Print((c.Cols()-len(hint))/2, c.Rows()/2+5, hint, engine.White)
	}
}

func drawCentre(c *engine.Canvas, geo geometry, text string, col engine.Color) {
	tw := engine.TextWidth(text)
	tx := geo.viewX + (geo.viewW-tw)/2
	ty := geo.viewY + geo.viewH/2 - engine.FontHeight/2
	// Background card.
	c.FillRect(tx-3, ty-2, tw+6, engine.FontHeight+4,
		engine.Color{R: 0, G: 0, B: 0, A: 255})
	c.DrawRect(tx-3, ty-2, tw+6, engine.FontHeight+4,
		engine.Color{R: 200, G: 200, B: 200, A: 255})
	c.DrawText(tx, ty, text, col)
}

// --- Tile/sprite helpers --------------------------------------------

func drawWallTile(c *engine.Canvas, x, y, tile int) {
	c.FillRect(x, y, tile, tile, colWall)
	if tile >= 3 {
		c.FillRect(x, y+tile-1, tile, 1, colWallSh)
		c.Set(x+tile-1, y, colWallSh)
	}
}

// drawRockTile paints a rounded gray boulder with a dark base shadow,
// a bright top highlight, and a small red "danger" pixel so the
// hazard reads as a hazard rather than a collectable box. The
// rendering scales down to a single 1×1 pixel at the minimum tile
// size; at tile≥3 the boulder gets enough detail to be unmistakable.
func drawRockTile(c *engine.Canvas, x, y, tile int) {
	cx := x + tile/2
	cy := y + tile/2
	r := tile / 2
	if r < 1 {
		r = 1
	}
	if tile >= 3 {
		c.FillCircle(cx, cy, r, colRock)
		// Base shadow — a strip across the bottom that drops the
		// boulder onto the road instead of floating it.
		c.FillRect(x, y+tile-1, tile, 1, colRockSh)
		// Bright top highlight pixel.
		c.Set(cx-1, cy-1, colRockHi)
		// Red danger pixel so the player reads it as a hazard at a
		// glance.
		if tile >= 4 {
			c.Set(cx+1, cy, colRockDanger)
		}
	} else {
		c.FillRect(x, y, tile, tile, colRock)
	}
}

func drawFlagSprite(c *engine.Canvas, cx, cy, tile int, body engine.Color) {
	r := tile / 2
	if r < 1 {
		r = 1
	}
	// Pole.
	c.FillRect(cx, cy-r, 1, r*2, colFlagPole)
	// Flag triangle.
	for i := 0; i < r; i++ {
		c.FillRect(cx+1, cy-r+i, r-i, 1, body)
	}
}

func drawSmoke(c *engine.Canvas, cx, cy, tile int, fade float64) {
	if fade <= 0 {
		return
	}
	hot := mixColor(colSmokeCold, colSmokeHot, fade)
	r := tile / 2
	if r < 1 {
		r = 1
	}
	c.FillCircle(cx, cy, r, hot)
	if r >= 2 && fade > 0.6 {
		c.Set(cx-r, cy, colSmokeHot)
		c.Set(cx+r, cy, colSmokeHot)
	}
}

func mixColor(a, b engine.Color, t float64) engine.Color {
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

// drawPlayerCar paints the player's car as a small rectangle with a
// directional headlight pixel. The headlight pixel sits one tile-edge
// past the body so the direction of motion is obvious.
func drawPlayerCar(c *engine.Canvas, cx, cy, tile int, dir direction, dying bool, t float64) {
	body := colPlayer
	if dying {
		// Flash white during death animation.
		if int(t*10)%2 == 0 {
			body = colPlayerWin
		}
	}
	drawCarSprite(c, cx, cy, tile, body, dir, true)
}

func drawEnemy(c *engine.Canvas, cx, cy, tile int, en *enemy, t float64) {
	body := colEnemy
	if en.smokeT > 0 {
		// Blink while stunned to telegraph "ram me for points".
		if int(t*8)%2 == 0 {
			body = colEnemyStun
		} else {
			body = colEnemyDim
		}
	}
	if en.crashed {
		body = colWreck
		drawWreck(c, cx, cy, tile)
		return
	}
	drawCarSprite(c, cx, cy, tile, body, en.dir, false)
}

// drawCarSprite renders a top-down car oriented along its direction
// of motion: a body rectangle in `body`, four dark wheel pixels
// poking out perpendicular to motion at the corners, a lighter
// cabin/windshield pixel near the middle, and a bright headlight on
// the leading edge. The same routine serves both the player and
// each enemy — only the palette changes.
//
// At tile=4 (typical) the layout is:
//
//   horizontal cars       vertical cars
//   W . . W               W B B W
//   B B B B               . B B .
//   B B B B               . B B .
//   W . . W               W B B W
//
// where W = dark wheel, B = body, and the road colour shows through
// the dots between wheels — exactly the way a top-down car reads at
// arcade-pixel scale.
func drawCarSprite(c *engine.Canvas, cx, cy, tile int, body engine.Color, dir direction, player bool) {
	r := tile / 2
	if r < 1 {
		r = 1
	}
	x0 := cx - r
	y0 := cy - r
	wheel := engine.Color{R: 30, G: 30, B: 30, A: 255}
	horizontal := dir == dirLeft || dir == dirRight

	switch {
	case tile <= 2:
		// Too small for body+wheels separation; just paint the tile.
		c.FillRect(x0, y0, tile, tile, body)
	case tile == 3:
		// 3×3 doesn't have room for the perpendicular-wheel trick.
		// Fill the body and drop two darker corner pixels so the
		// silhouette still reads as a wheeled vehicle.
		c.FillRect(x0, y0, tile, tile, body)
		if horizontal {
			c.Set(x0, y0, wheel)
			c.Set(x0+tile-1, y0+tile-1, wheel)
		} else {
			c.Set(x0+tile-1, y0, wheel)
			c.Set(x0, y0+tile-1, wheel)
		}
	default:
		// tile >= 4: full body+wheels treatment. Body fills the
		// "length" axis of the tile; wheels stick out on the other
		// axis at the four corners.
		if horizontal {
			c.FillRect(x0, y0+1, tile, tile-2, body)
		} else {
			c.FillRect(x0+1, y0, tile-2, tile, body)
		}
		c.Set(x0, y0, wheel)
		c.Set(x0+tile-1, y0, wheel)
		c.Set(x0, y0+tile-1, wheel)
		c.Set(x0+tile-1, y0+tile-1, wheel)
	}

	// Cabin highlight — a small lighter patch toward the centre,
	// nudged slightly toward the rear so the headlight is unambiguous.
	if tile >= 4 {
		cabin := engine.Color{R: 235, G: 235, B: 235, A: 255}
		if !player {
			cabin = engine.Color{R: 255, G: 220, B: 180, A: 255}
		}
		// Offset opposite the direction of motion so cabin sits
		// "behind" the windshield.
		bx, by := cx, cy
		switch dir {
		case dirRight:
			bx = cx - 1
		case dirLeft:
			bx = cx
		case dirUp:
			by = cy
		case dirDown:
			by = cy - 1
		}
		c.Set(bx, by, cabin)
	} else if tile >= 3 {
		cabin := engine.Color{R: 235, G: 235, B: 235, A: 255}
		if !player {
			cabin = engine.Color{R: 255, G: 220, B: 180, A: 255}
		}
		c.Set(cx, cy, cabin)
	}

	// Headlight — bright dot at the centre of the leading edge.
	if dir == dirNone {
		return
	}
	var hx, hy int
	switch dir {
	case dirRight:
		hx, hy = x0+tile-1, cy
	case dirLeft:
		hx, hy = x0, cy
	case dirUp:
		hx, hy = cx, y0
	case dirDown:
		hx, hy = cx, y0+tile-1
	}
	light := engine.Color{R: 255, G: 240, B: 180, A: 255}
	if !player {
		light = engine.Color{R: 255, G: 120, B: 80, A: 255}
	}
	c.Set(hx, hy, light)
}

func drawWreck(c *engine.Canvas, cx, cy, tile int) {
	r := tile / 2
	if r < 1 {
		r = 1
	}
	c.FillRect(cx-r, cy-r, tile, tile, colWreck)
	c.Set(cx-r, cy-r, colWreckSpark)
	c.Set(cx+r, cy-r, colWreckSpark)
	c.Set(cx-r, cy+r, colWreckSpark)
	c.Set(cx+r, cy+r, colWreckSpark)
}

func drawHUDCar(c *engine.Canvas, x, y int, body engine.Color) {
	c.FillRect(x-1, y, 3, 2, body)
	c.Set(x, y, engine.Color{R: 230, G: 230, B: 230, A: 255})
}

// --- geometry helpers ----------------------------------------------

// worldToPixel converts a tile-space coordinate to the centre pixel
// in canvas space, relative to the viewport origin.
func (g geometry) worldToPixel(tx, ty float64) (int, int) {
	x := g.viewX + int(math.Round((tx-float64(g.viewStartCol))*float64(g.tile)))
	y := g.viewY + int(math.Round((ty-float64(g.viewStartRow))*float64(g.tile)))
	return x, y
}

// inView reports whether tile (col, row) is currently visible in the
// scrolling viewport.
func (g geometry) inView(col, row int) bool {
	return col >= g.viewStartCol && col < g.viewEndCol &&
		row >= g.viewStartRow && row < g.viewEndRow
}

// countFromLucky returns 1 if there's a lucky flag still on the
// board (so it shows in the "N/M" tally), otherwise 0. The lucky
// flag isn't stored in maze.flags, so the total has to add it back.
func countFromLucky(m *maze) int {
	if m.luckyFlag != nil && !m.luckyFlag.taken {
		return 1
	}
	return 0
}
