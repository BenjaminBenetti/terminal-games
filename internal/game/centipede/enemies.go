package centipede

import (
	"math/rand"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// --- Spider --------------------------------------------------------------
//
// The spider zigzags around the player zone, eating mushrooms it touches
// and threatening the player. Score awarded when shot scales with the
// player's distance — close shots (within ~3 cells) are worth 900, mid
// range 600, long-range 300.

const (
	spiderSpeedBase    = 24.0 // pixels per second along the diagonal
	spiderEatChance    = 0.55 // chance per cell-entry that it eats the mushroom there
	spiderTurnInterval = 0.45 // average seconds between random direction tweaks

	// Score thresholds in pixels — distance from spider center to
	// player center when the shot lands.
	spiderClose = 16 // within this many pixels = 900 pts
	spiderMid   = 36 // within this many pixels = 600 pts; otherwise 300
)

type spider struct {
	x, y   float64 // pixel position of sprite top-left
	vx, vy float64 // pixel/sec
	alive  bool
	frameT float64
	frame  int
	turnT  float64
	// Tracks the cell most recently entered so we don't munch a
	// mushroom every frame the spider lingers on the same cell.
	lastCellCol, lastCellRow int
}

func (s *spider) width() int  { return spiderA.width() }
func (s *spider) height() int { return spiderA.height() }

func (s *spider) rect() rect {
	return rect{
		x0: int(s.x),
		y0: int(s.y),
		x1: int(s.x) + s.width(),
		y1: int(s.y) + s.height(),
	}
}

// spawnSpider creates a spider entering the player zone from one of the
// sides at a random angle.
func spawnSpider(rng *rand.Rand, f *field, level int) *spider {
	speed := spiderSpeedBase + float64(level-1)*4.0
	fromLeft := rng.Intn(2) == 0
	x := -float64(spiderA.width()) - 2
	if !fromLeft {
		x = float64(f.cols*cellW) + 2
	}
	yMin, _ := f.cellPixel(0, f.playerZoneTop-2)
	_, yMax := f.cellPixel(0, f.rows-1)
	y := float64(yMin) + rng.Float64()*float64(yMax-yMin)
	vx := speed
	if !fromLeft {
		vx = -speed
	}
	vy := speed
	if rng.Intn(2) == 0 {
		vy = -speed
	}
	return &spider{
		x: x, y: y,
		vx: vx, vy: vy,
		alive:       true,
		lastCellCol: -1,
		lastCellRow: -1,
	}
}

// tick advances the spider's motion and resolves mushroom-eating.
// Returns true when the spider has wandered off-screen.
func (s *spider) tick(dt float64, f *field, rng *rand.Rand) (offscreen bool) {
	s.frameT += dt
	if s.frameT >= 0.16 {
		s.frameT -= 0.16
		s.frame = 1 - s.frame
	}
	s.turnT += dt
	if s.turnT >= spiderTurnInterval {
		s.turnT = 0
		// Random vertical direction tweak.
		if rng.Float64() < 0.6 {
			s.vy = -s.vy
		}
	}
	s.x += s.vx * dt
	s.y += s.vy * dt

	// Keep the spider in the player zone band vertically.
	yTopPx, _ := f.cellPixel(0, f.playerZoneTop-1)
	_, yBotPx := f.cellPixel(0, f.rows-1)
	yMax := yBotPx - s.height()
	if s.y < float64(yTopPx) {
		s.y = float64(yTopPx)
		s.vy = -s.vy
	}
	if s.y > float64(yMax) {
		s.y = float64(yMax)
		s.vy = -s.vy
	}

	// Mushroom eating — sample the spider's centre cell on entry.
	cx := int(s.x) + s.width()/2
	cy := int(s.y) + s.height()/2
	col, row := f.cellAtPixel(cx, cy)
	if col >= 0 && (col != s.lastCellCol || row != s.lastCellRow) {
		s.lastCellCol = col
		s.lastCellRow = row
		if f.hasMushroom(col, row) && rng.Float64() < spiderEatChance {
			f.eat(col, row)
		}
	}

	// Off-screen detection on the sides.
	if s.vx > 0 && s.x > float64(f.cols*cellW)+8 {
		return true
	}
	if s.vx < 0 && s.x+float64(s.width()) < -8 {
		return true
	}
	return false
}

// scoreFor returns the spider score awarded for a shot landed when the
// spider's centre was within the given pixel distance of the player's
// centre.
func spiderScore(distPx float64) int {
	switch {
	case distPx < spiderClose:
		return 900
	case distPx < spiderMid:
		return 600
	default:
		return 300
	}
}

// draw the spider, alternating frames and choosing a colour that
// flickers between the two palette options for the arcade-eye look.
func (s *spider) draw(c *engine.Canvas) {
	spr := spiderA
	if s.frame == 1 {
		spr = spiderB
	}
	col := colorSpider
	if s.frame == 1 {
		col = engine.Color{R: 100, G: 240, B: 200, A: 255}
	}
	drawSprite(c, int(s.x), int(s.y), spr, col)
}

// --- Flea ----------------------------------------------------------------
//
// Drops straight down a single column at a brisk pace, planting random
// mushrooms below it. Two hits to kill — the first hit doubles its
// speed; the second hit destroys it (200 pts).

const (
	fleaSpeedBase    = 28.0
	fleaSpeedBoosted = 60.0
	fleaPlantChance  = 0.55 // probability per cell-row traversed
)

type flea struct {
	x, y       float64 // pixel position of sprite top-left
	speed      float64
	hits       int
	alive      bool
	frame      int
	frameT     float64
	lastRowSeen int
}

func (fl *flea) width() int  { return fleaA.width() }
func (fl *flea) height() int { return fleaA.height() }

func (fl *flea) rect() rect {
	return rect{
		x0: int(fl.x), y0: int(fl.y),
		x1: int(fl.x) + fl.width(),
		y1: int(fl.y) + fl.height(),
	}
}

func spawnFlea(rng *rand.Rand, f *field) *flea {
	col := rng.Intn(f.cols)
	x, _ := f.cellPixel(col, 0)
	return &flea{
		x:           float64(x),
		y:           float64(f.originY) - float64(fleaA.height()),
		speed:       fleaSpeedBase,
		alive:       true,
		lastRowSeen: -1,
	}
}

// tick moves the flea down and probabilistically plants mushrooms. Returns
// true when the flea has reached the bottom of the field and should be
// removed.
func (fl *flea) tick(dt float64, f *field, rng *rand.Rand) (gone bool) {
	fl.frameT += dt
	if fl.frameT >= 0.12 {
		fl.frameT -= 0.12
		fl.frame = 1 - fl.frame
	}
	fl.y += fl.speed * dt
	cx := int(fl.x) + fl.width()/2
	cy := int(fl.y) + fl.height()/2
	col, row := f.cellAtPixel(cx, cy)
	if col >= 0 && row != fl.lastRowSeen {
		fl.lastRowSeen = row
		// Only plant in cells below the entry row and above the bottom.
		if row > 0 && row < f.rows-1 && rng.Float64() < fleaPlantChance {
			f.plant(col, row)
		}
	}
	if fl.y >= float64(f.originY+f.rows*cellH) {
		return true
	}
	return false
}

// hit applies a single bullet hit. Returns (destroyed, scoreAwarded).
// First hit just boosts speed; second hit destroys.
func (fl *flea) hit() (destroyed bool, score int) {
	fl.hits++
	if fl.hits >= 2 {
		fl.alive = false
		return true, 200
	}
	fl.speed = fleaSpeedBoosted
	return false, 0
}

func (fl *flea) draw(c *engine.Canvas) {
	spr := fleaA
	if fl.frame == 1 {
		spr = fleaB
	}
	drawSprite(c, int(fl.x), int(fl.y), spr, colorFlea)
}

// --- Scorpion ------------------------------------------------------------
//
// Walks horizontally across the upper playfield, poisoning every
// mushroom it passes over. 1000 pts when shot.

const (
	scorpionSpeedBase = 20.0
)

type scorpion struct {
	x, y    float64
	vx      float64
	alive   bool
	frame   int
	frameT  float64
	lastCol int
	row     int
}

func (sc *scorpion) width() int  { return scorpionA.width() }
func (sc *scorpion) height() int { return scorpionA.height() }

func (sc *scorpion) rect() rect {
	return rect{
		x0: int(sc.x), y0: int(sc.y),
		x1: int(sc.x) + sc.width(),
		y1: int(sc.y) + sc.height(),
	}
}

func spawnScorpion(rng *rand.Rand, f *field, level int) *scorpion {
	speed := scorpionSpeedBase + float64(level-1)*2.5
	fromLeft := rng.Intn(2) == 0
	var x float64
	if fromLeft {
		x = -float64(scorpionA.width()) - 2
	} else {
		x = float64(f.cols*cellW) + 2
	}
	// Scorpion walks in the upper third of the playfield.
	maxRow := f.playerZoneTop - 2
	if maxRow < 2 {
		maxRow = 2
	}
	row := 1 + rng.Intn(maxRow-1)
	_, yPx := f.cellPixel(0, row)
	vx := speed
	if !fromLeft {
		vx = -speed
	}
	return &scorpion{
		x: x, y: float64(yPx),
		vx:      vx,
		alive:   true,
		row:     row,
		lastCol: -1,
	}
}

// tick walks the scorpion and poisons mushrooms it passes through.
// Returns true once it has fully walked off the opposite side.
func (sc *scorpion) tick(dt float64, f *field) (offscreen bool) {
	sc.frameT += dt
	if sc.frameT >= 0.12 {
		sc.frameT -= 0.12
		sc.frame = 1 - sc.frame
	}
	sc.x += sc.vx * dt
	cx := int(sc.x) + sc.width()/2
	cy := int(sc.y) + sc.height()/2
	col, row := f.cellAtPixel(cx, cy)
	if col >= 0 && col != sc.lastCol {
		sc.lastCol = col
		// Poison whatever's in the scorpion's row and the row directly
		// below, so its trail visibly covers the height of the sprite.
		f.poison(col, row)
	}
	if sc.vx > 0 && sc.x > float64(f.cols*cellW)+10 {
		return true
	}
	if sc.vx < 0 && sc.x+float64(sc.width()) < -10 {
		return true
	}
	return false
}

func (sc *scorpion) draw(c *engine.Canvas) {
	spr := scorpionA
	if sc.frame == 1 {
		spr = scorpionB
	}
	if sc.vx < 0 {
		drawSpriteMirror(c, int(sc.x), int(sc.y), spr, colorScorpion)
	} else {
		drawSprite(c, int(sc.x), int(sc.y), spr, colorScorpion)
	}
}

// --- Explosion (visual only, brief) -------------------------------------

type explosion struct {
	x, y float64
	t    float64
}

const explosionDur = 0.45

func (e *explosion) tick(dt float64) (done bool) {
	e.t += dt
	return e.t >= explosionDur
}

func (e *explosion) draw(c *engine.Canvas) {
	step := int(e.t / 0.15)
	var spr sprite
	switch step {
	case 0:
		spr = enemyExplodeA
	case 1:
		spr = enemyExplodeB
	default:
		spr = enemyExplodeC
	}
	drawSprite(c, int(e.x), int(e.y), spr, colorExplosion)
}
