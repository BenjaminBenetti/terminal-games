package galaga

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// sprite is a row-major bitmap. '#' is a set pixel; any other rune
// (typically '.') is a clear pixel. All rows must be the same length.
type sprite []string

func (s sprite) width() int {
	if len(s) == 0 {
		return 0
	}
	return len(s[0])
}

func (s sprite) height() int { return len(s) }

// drawSprite blits s onto c at canvas pixel position (x, y). Only '#'
// pixels are drawn — everything else composites with whatever's there.
func drawSprite(c *engine.Canvas, x, y int, s sprite, fg engine.Color) {
	for row, line := range s {
		for col := 0; col < len(line); col++ {
			if line[col] == '#' {
				c.Set(x+col, y+row, fg)
			}
		}
	}
}

// drawSpriteFlipY paints s upside-down, so an enemy whose home pose
// "faces down" can be reused when it's diving "up" relative to its
// formation orientation. We don't currently use this since the top-down
// view means sprites read the same either way, but it's here if needed.
func drawSpriteFlipY(c *engine.Canvas, x, y int, s sprite, fg engine.Color) {
	h := s.height()
	for row, line := range s {
		for col := 0; col < len(line); col++ {
			if line[col] == '#' {
				c.Set(x+col, y+h-1-row, fg)
			}
		}
	}
}

// All gameplay sprites are 7 pixels wide so the formation lays out on a
// regular 7×6 pitch (7-px sprite + 1-px gap horizontally, 5-px sprite +
// 1-px gap vertically). Enemies have two animation frames for wing flap.

// Player fighter (7x5). Pointed nose with twin wing tips.
var playerSprite = sprite{
	"...#...",
	"..###..",
	".#####.",
	".##.##.",
	"#######",
}

// Dual fighter (15x5). Two fighters joined at the wing — the reward
// state after rescuing a captured ship from a Boss Galaga.
var dualPlayerSprite = sprite{
	"...#.......#...",
	"..###.....###..",
	".#####...#####.",
	".##.##...##.##.",
	"###############",
}

// Captured-ship indicator drawn beneath a Boss Galaga that holds it.
// The ship is inverted while the Boss carries it back to formation —
// we use the same playerSprite drawn flipped via drawSpriteFlipY.

// Player explosion (7x5). Two frames cycled for a brief death anim.
var playerExplodeA = sprite{
	"#..#..#",
	".#.#.#.",
	"..###..",
	".#.#.#.",
	"#..#..#",
}

var playerExplodeB = sprite{
	".#.#.#.",
	"#..#..#",
	".#####.",
	"#..#..#",
	".#.#.#.",
}

// Bee (Zako) — bottom-row enemy, 50 pts in formation / 100 pts in flight.
var beeA = sprite{
	"..#.#..",
	".#####.",
	"#.#.#.#",
	"#######",
	"##.#.##",
}

var beeB = sprite{
	"..#.#..",
	"#.###.#",
	".#.#.#.",
	"#######",
	"##...##",
}

// Butterfly (Goei) — middle-row enemy, 80 / 160 pts.
var butterflyA = sprite{
	".##.##.",
	".#####.",
	"#######",
	"##.#.##",
	"##...##",
}

var butterflyB = sprite{
	"##.#.##",
	".#####.",
	"#######",
	".#####.",
	"##...##",
}

// Boss Galaga — top-row enemy, 150 / 400 pts (800 with two escorts).
var bossA = sprite{
	".#####.",
	"#.###.#",
	"#######",
	".#.#.#.",
	"##...##",
}

var bossB = sprite{
	"##...##",
	"#.###.#",
	"#######",
	".#.#.#.",
	"##.#.##",
}

// Generic enemy explosion (7x5). Three frames for an outward starburst.
var enemyExplode0 = sprite{
	"...#...",
	".#.#.#.",
	"..###..",
	".#.#.#.",
	"...#...",
}

var enemyExplode1 = sprite{
	"#..#..#",
	".#.#.#.",
	"##...##",
	".#.#.#.",
	"#..#..#",
}

var enemyExplode2 = sprite{
	"#.#.#.#",
	".......",
	"#.....#",
	".......",
	"#.#.#.#",
}

// Player projectile (1x4) — twin shots fly straight up.
var playerBullet = sprite{
	"#",
	"#",
	"#",
	"#",
}

// Enemy bomb (1x3) — falls straight down. Multiple frames for animation.
var enemyBombA = sprite{
	"#",
	"#",
	"#",
}

var enemyBombB = sprite{
	".",
	"#",
	".",
}

// Star pixels for the parallax background — single pixels rendered
// directly without a sprite struct, but the colour palette lives here.
var starPalette = []engine.Color{
	{R: 255, G: 255, B: 255, A: 255},
	{R: 200, G: 220, B: 255, A: 255},
	{R: 255, G: 220, B: 180, A: 255},
	{R: 180, G: 200, B: 255, A: 255},
	{R: 255, G: 180, B: 180, A: 255},
}
