package scramble

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// sprite is a row-major bitmap. '#' is set; any other rune is clear.
type sprite []string

func (s sprite) width() int {
	if len(s) == 0 {
		return 0
	}
	return len(s[0])
}

func (s sprite) height() int { return len(s) }

// drawSprite blits s at (x, y) using fg for set pixels.
func drawSprite(c *engine.Canvas, x, y int, s sprite, fg engine.Color) {
	for r, line := range s {
		for col := 0; col < len(line); col++ {
			if line[col] == '#' {
				c.Set(x+col, y+r, fg)
			}
		}
	}
}

// -- Player ----------------------------------------------------------------

// playerSprite is the jet: 8x5, nose pointed right, swept tail at left.
var playerSprite = sprite{
	"##......",
	"####....",
	"########",
	"####....",
	"##......",
}

// playerExplodeA/B are two frames of the player explosion (8x5), reusing
// the same footprint so the death animation lines up with the cockpit.
var playerExplodeA = sprite{
	"#..#..#.",
	".#.#.#..",
	"..####..",
	".#.#.#..",
	"#..#..#.",
}

var playerExplodeB = sprite{
	".#.#.#.#",
	"#..#..#.",
	"##.##.##",
	"#..#..#.",
	".#.#.#.#",
}

// -- Enemies ---------------------------------------------------------------

// rocketIdle is the silhouette of an upright rocket sitting on its pad
// (3x5 — nose, fuselage, fins).
var rocketIdle = sprite{
	".#.",
	".#.",
	".#.",
	"###",
	"#.#",
}

// rocketLaunch adds an exhaust flame underneath while the rocket climbs.
var rocketLaunch = sprite{
	".#.",
	".#.",
	".#.",
	"###",
	"#.#",
	"###",
	".#.",
	"#.#",
}

// ufoA / ufoB are the saucer animation frames (7x3).
var ufoA = sprite{
	"..###..",
	"#######",
	".#.#.#.",
}

var ufoB = sprite{
	"..###..",
	"#######",
	"#.#.#.#",
}

// fireballA / fireballB are the falling-meteor frames (5x5). The sparkly
// outline cycles to suggest combustion.
var fireballA = sprite{
	".###.",
	"#####",
	"#####",
	"#####",
	".###.",
}

var fireballB = sprite{
	"#.#.#",
	".###.",
	"#####",
	".###.",
	"#.#.#",
}

// fuelTank is the cylindrical fuel depot (5x5) that refuels when bombed.
var fuelTank = sprite{
	".###.",
	"##.##",
	"#####",
	"##.##",
	".###.",
}

// baseTower is a stationary gun emplacement (5x6) used in the city and
// base sectors — fires missiles upward at the player.
var baseTower = sprite{
	".###.",
	"#####",
	"#.#.#",
	"#####",
	"##.##",
	"#####",
}

// missile is the anti-aircraft round (3x4) launched from baseTowers,
// pointing up (towards the player).
var missile = sprite{
	".#.",
	"###",
	"#.#",
	"###",
}

// reactor is the final boss target at the end of stage 6 (13x9). The
// player must shoot the core to clear the run.
var reactor = sprite{
	"....###......",
	"..#######....",
	".####.####...",
	"###.###.###..",
	"##.#####.##..",
	"###.###.###..",
	".####.####...",
	"..#######....",
	"....###......",
}

// -- Projectiles & decoration ---------------------------------------------

// playerBullet is the forward laser (4x2). The 2-pixel thickness gives
// the AABB collision a bit more y-tolerance against thin saucers and
// looks more like a beam than a hair.
var playerBullet = sprite{
	"####",
	"####",
}

// playerBomb is the dropped bomb (2x2).
var playerBomb = sprite{
	"##",
	"##",
}

// explode0..2 are generic enemy explosion frames (5x5).
var explode0 = sprite{
	"..#..",
	".###.",
	"#####",
	".###.",
	"..#..",
}

var explode1 = sprite{
	"#.#.#",
	".#.#.",
	"##.##",
	".#.#.",
	"#.#.#",
}

var explode2 = sprite{
	"#...#",
	".....",
	"..#..",
	".....",
	"#...#",
}

// starPalette is the parallax-background star colours, dim enough not to
// fight the playfield for attention.
var starPalette = []engine.Color{
	{R: 220, G: 220, B: 240, A: 255},
	{R: 180, G: 200, B: 240, A: 255},
	{R: 240, G: 220, B: 180, A: 255},
	{R: 200, G: 200, B: 220, A: 255},
}

// -- Colour palette --------------------------------------------------------

var (
	colPlayer    = engine.Color{R: 120, G: 240, B: 200, A: 255}
	colPlayerDim = engine.Color{R: 60, G: 130, B: 110, A: 255}
	colBullet    = engine.Color{R: 250, G: 250, B: 200, A: 255}
	colBomb      = engine.Color{R: 240, G: 200, B: 120, A: 255}
	colRocket    = engine.Color{R: 240, G: 240, B: 250, A: 255}
	colFlame     = engine.Color{R: 250, G: 140, B: 60, A: 255}
	colUFO       = engine.Color{R: 255, G: 220, B: 90, A: 255}
	colFire      = engine.Color{R: 255, G: 100, B: 60, A: 255}
	colFuel      = engine.Color{R: 250, G: 200, B: 80, A: 255}
	colTower     = engine.Color{R: 200, G: 90, B: 220, A: 255}
	colMissile   = engine.Color{R: 255, G: 160, B: 200, A: 255}
	colReactor   = engine.Color{R: 240, G: 80, B: 80, A: 255}
	colReactor2  = engine.Color{R: 240, G: 200, B: 80, A: 255}
	colExplode   = engine.Color{R: 255, G: 200, B: 100, A: 255}
	colMountain  = engine.Color{R: 60, G: 200, B: 90, A: 255}
	colMountain2 = engine.Color{R: 40, G: 140, B: 70, A: 255}
	colCity      = engine.Color{R: 110, G: 160, B: 230, A: 255}
	colCityLit   = engine.Color{R: 240, G: 240, B: 140, A: 255}
	colCavern    = engine.Color{R: 200, G: 160, B: 120, A: 255}
	colCavern2   = engine.Color{R: 130, G: 100, B: 80, A: 255}
	colBase      = engine.Color{R: 200, G: 200, B: 220, A: 255}
	colBase2     = engine.Color{R: 120, G: 120, B: 150, A: 255}
	colBg        = engine.Color{R: 0, G: 0, B: 10, A: 255}
)
