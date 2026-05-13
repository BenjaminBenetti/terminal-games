package centipede

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// sprite is a row-major bitmap. '#' is a set pixel; '.' (or any other
// rune) is clear. All rows must be the same length.
type sprite []string

func (s sprite) width() int {
	if len(s) == 0 {
		return 0
	}
	return len(s[0])
}

func (s sprite) height() int { return len(s) }

// drawSprite paints s onto c at pixel (x, y). Only '#' pixels are drawn.
func drawSprite(c *engine.Canvas, x, y int, s sprite, fg engine.Color) {
	for row, line := range s {
		for col := 0; col < len(line); col++ {
			if line[col] == '#' {
				c.Set(x+col, y+row, fg)
			}
		}
	}
}

// drawSpriteMirror paints s mirrored horizontally — used for left/right
// facing variants of the same sprite.
func drawSpriteMirror(c *engine.Canvas, x, y int, s sprite, fg engine.Color) {
	w := s.width()
	for row, line := range s {
		for col := 0; col < len(line); col++ {
			if line[col] == '#' {
				c.Set(x+(w-1-col), y+row, fg)
			}
		}
	}
}

// --- Cell geometry -------------------------------------------------------

// cellW × cellH is the pixel pitch of the mushroom / centipede grid. All
// cell-snapped entities (mushrooms, centipede segments, fleas, scorpions)
// draw at the top-left of the cell they occupy.
const (
	cellW = 4
	cellH = 3
)

// --- Mushroom sprites ----------------------------------------------------
//
// Mushrooms take 4 hits to destroy. Each successive hit erodes the cap so
// the damage is readable at a glance — full → cap-bitten → half →
// stump. The colour for poisoned mushrooms is set at draw time.

var mushroomFull = sprite{
	".##.",
	"####",
	".##.",
}

var mushroomDmg1 = sprite{
	".##.",
	"###.",
	".##.",
}

var mushroomDmg2 = sprite{
	"..#.",
	".##.",
	".##.",
}

var mushroomDmg3 = sprite{
	"....",
	".##.",
	".#..",
}

var mushroomFrames = []sprite{mushroomFull, mushroomDmg1, mushroomDmg2, mushroomDmg3}

// --- Centipede sprites ---------------------------------------------------
//
// A segment occupies one grid cell. The head has eyes; body segments are
// rounded beads. We render two animation frames cycled by the body's
// alive-time for a subtle "legs wiggling" effect.

var centipedeHeadA = sprite{
	"####",
	"#.#.",
	"####",
}

var centipedeHeadB = sprite{
	"####",
	".#.#",
	"####",
}

var centipedeBodyA = sprite{
	".##.",
	"####",
	".##.",
}

var centipedeBodyB = sprite{
	"####",
	".##.",
	"####",
}

// --- Player ("bug blaster") and bullet ----------------------------------

var playerSprite = sprite{
	".##.",
	"####",
	"#..#",
}

// Player explosion plays for a brief moment on death.
var playerExplodeA = sprite{
	"#.#.",
	".#.#",
	"#.#.",
}

var playerExplodeB = sprite{
	".#.#",
	"#.#.",
	".#.#",
}

var bulletSprite = sprite{
	"#",
	"#",
}

// --- Spider --------------------------------------------------------------
//
// Two animation frames — legs swap which side they're flexed on. 6×4
// pixels so the spider feels visibly bigger than a mushroom.

var spiderA = sprite{
	"#.##.#",
	".####.",
	"######",
	"#.##.#",
}

var spiderB = sprite{
	".#..#.",
	"######",
	".####.",
	"#.##.#",
}

// --- Flea ----------------------------------------------------------------
//
// Falls straight down in 4×3 — same cell pitch as a mushroom so it lines
// up with the column it's planting in.

var fleaA = sprite{
	".##.",
	"####",
	".##.",
}

var fleaB = sprite{
	"####",
	".##.",
	"####",
}

// --- Scorpion ------------------------------------------------------------
//
// Walks horizontally across the upper playfield. The "tail" curls up on
// the trailing side; we draw the same sprite mirrored for the other
// direction at draw time.

var scorpionA = sprite{
	"##..##",
	"######",
	".###..",
}

var scorpionB = sprite{
	"##..##",
	"######",
	"..###.",
}

// --- Explosion (used for enemy deaths and pickups) ----------------------

var enemyExplodeA = sprite{
	"#.#.",
	".#.#",
	"#.#.",
}

var enemyExplodeB = sprite{
	".#.#",
	"#.#.",
	".#.#",
}

var enemyExplodeC = sprite{
	"#..#",
	"....",
	"#..#",
}

// --- Palette -------------------------------------------------------------
//
// Centipede's arcade palette varied per level. We pick a sympathetic set
// of saturated colours — green centipede with a yellow head, red
// mushrooms (purple when poisoned), magenta spider, yellow flea, cyan
// scorpion, cyan player.

var (
	colorMushroom         = engine.Color{R: 230, G: 80, B: 80, A: 255}
	colorMushroomPoisoned = engine.Color{R: 220, G: 100, B: 240, A: 255}
	colorCentipedeBody    = engine.Color{R: 120, G: 230, B: 120, A: 255}
	colorCentipedeHead    = engine.Color{R: 240, G: 230, B: 100, A: 255}
	colorPlayer           = engine.Color{R: 120, G: 230, B: 230, A: 255}
	colorBullet           = engine.Color{R: 250, G: 250, B: 220, A: 255}
	colorSpider           = engine.Color{R: 230, G: 90, B: 220, A: 255}
	colorFlea             = engine.Color{R: 240, G: 220, B: 90, A: 255}
	colorScorpion         = engine.Color{R: 80, G: 220, B: 220, A: 255}
	colorExplosion        = engine.Color{R: 255, G: 200, B: 100, A: 255}
	colorBackground       = engine.Color{R: 8, G: 8, B: 20, A: 255}
	colorPlayerZoneTint   = engine.Color{R: 16, G: 18, B: 40, A: 255}
)
