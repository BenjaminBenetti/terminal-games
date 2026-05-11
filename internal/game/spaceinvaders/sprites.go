package spaceinvaders

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// sprite is a row-major bitmap. Each string is one pixel row; '#' is a set
// pixel and any other rune (typically '.') is a clear pixel. All rows in a
// sprite must be the same length.
type sprite []string

func (s sprite) width() int {
	if len(s) == 0 {
		return 0
	}
	return len(s[0])
}

func (s sprite) height() int { return len(s) }

// drawSprite blits s into c at the canvas pixel position (x, y). Pixels in
// the sprite outside the canvas are silently clipped. Only '#' pixels are
// drawn — everything else is left untouched so sprites composite over
// whatever's already on the canvas.
func drawSprite(c *engine.Canvas, x, y int, s sprite, fg engine.Color) {
	for row, line := range s {
		for col := 0; col < len(line); col++ {
			if line[col] == '#' {
				c.Set(x+col, y+row, fg)
			}
		}
	}
}

// Original pixel-art designs for the game. Width is fixed within each
// category so the formation lays out on a regular grid.

// Player defender cannon (11x6). A flat-topped turret with a small barrel.
var playerSprite = sprite{
	".....#.....",
	"....###....",
	"....###....",
	".#########.",
	"###########",
	"###########",
}

// Player explosion (11x6). Two frames for a brief death animation.
var playerExplodeA = sprite{
	"..#..#..#..",
	"#..#.#.#..#",
	".###.#.###.",
	"#.#######.#",
	"###########",
	"#.#.###.#.#",
}

var playerExplodeB = sprite{
	"#..#...#..#",
	".#..#.#..#.",
	"..#.###.#..",
	".#.#####.#.",
	"#.#######.#",
	".#.#.#.#.#.",
}

// Top-row "drifter" alien — 30 points. 8x5, two animation frames.
var alienTopA = sprite{
	"...##...",
	"..####..",
	".######.",
	"#.####.#",
	".#.##.#.",
}

var alienTopB = sprite{
	"...##...",
	"..####..",
	".######.",
	"#.####.#",
	"#.#..#.#",
}

// Middle-rows "skitterer" alien — 20 points. 8x5, two animation frames.
var alienMidA = sprite{
	"#......#",
	".#....#.",
	".######.",
	"##.##.##",
	"########",
}

var alienMidB = sprite{
	"#......#",
	"##....##",
	".######.",
	"##.##.##",
	".#....#.",
}

// Bottom-rows "lurker" alien — 10 points. 8x5, two animation frames.
var alienBotA = sprite{
	".######.",
	"########",
	"##.##.##",
	".##..##.",
	"#.#..#.#",
}

var alienBotB = sprite{
	".######.",
	"########",
	"##.##.##",
	"##.##.##",
	"#......#",
}

// Generic alien explosion (8x5).
var alienExplode = sprite{
	"#.#..#.#",
	".#.##.#.",
	"##.##.##",
	".#.##.#.",
	"#.#..#.#",
}

// Mystery UFO (13x5).
var ufoSprite = sprite{
	"...######....",
	"..########...",
	".###########.",
	"##.##.##.##.#",
	".##.....##...",
}

// Player projectile (1x4).
var playerBulletSprite = sprite{
	"#",
	"#",
	"#",
	"#",
}

// Alien projectiles. Three animated variants give the formation's return
// fire some visual variety; each one cycles between two frames as it falls.
var alienBulletStraightA = sprite{
	"#",
	"#",
	"#",
	"#",
}

var alienBulletStraightB = sprite{
	"#",
	".",
	"#",
	".",
}

var alienBulletZigA = sprite{
	".#.",
	"#..",
	".#.",
	"..#",
}

var alienBulletZigB = sprite{
	".#.",
	"..#",
	".#.",
	"#..",
}

var alienBulletForkA = sprite{
	"#.#",
	".#.",
	"#.#",
	".#.",
}

var alienBulletForkB = sprite{
	".#.",
	"#.#",
	".#.",
	"#.#",
}

// Bunker shield (16x10). A rounded-top fortress with a small notch
// underneath. Per-bunker damage is tracked in a separate bitmap that
// starts as a copy of this mask, so individual pixels can be eroded by
// hits without altering this template.
var bunkerSprite = sprite{
	"...##########...",
	"..############..",
	".##############.",
	"################",
	"################",
	"################",
	"################",
	"################",
	"####........####",
	"###..........###",
}
