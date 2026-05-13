package vanguard

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// sprite is a row-major bitmap. '#' is a set pixel; any other rune
// (typically '.') is a transparent pixel that's left untouched.
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

// drawSpriteFlipX paints s mirrored horizontally — used for enemies
// (Kemleys etc) whose pose depends on which screen edge they entered
// from.
func drawSpriteFlipX(c *engine.Canvas, x, y int, s sprite, fg engine.Color) {
	w := s.width()
	for row, line := range s {
		for col := 0; col < len(line); col++ {
			if line[col] == '#' {
				c.Set(x+w-1-col, y+row, fg)
			}
		}
	}
}

// --- Player sprites ----------------------------------------------------

// Player ship — symmetric "+" hull, top-down view. The ship has a fixed
// orientation regardless of fire direction so we don't have to swap
// sprites when the player toggles between firing axes (matches the
// arcade original where the cabinet had four discrete fire buttons but
// the on-screen ship was always drawn the same way).
var playerShip = sprite{
	"..#..",
	"..#..",
	"#####",
	".###.",
	"#.#.#",
}

// Player ship while an energy pod is active — slightly larger and with
// a halo, in line with the arcade's tinted "powered" sprite.
var playerShipPowered = sprite{
	"..#..",
	".###.",
	"#####",
	"#####",
	"#.#.#",
}

// Two-frame explosion for the player ship.
var playerExplodeA = sprite{
	"#.#.#",
	".#.#.",
	"..#..",
	".#.#.",
	"#.#.#",
}

var playerExplodeB = sprite{
	".#.#.",
	"#...#",
	"..#..",
	"#...#",
	".#.#.",
}

// --- Player projectiles ------------------------------------------------

// Vertical player bullet (used for up / down shots).
var playerBulletV = sprite{
	"#",
	"#",
}

// Horizontal player bullet (used for left / right shots).
var playerBulletH = sprite{"##"}

// --- Mountain-zone enemies --------------------------------------------

// Kemley — horizontal rocket, points right. Comes streaming in from the
// screen edges. Drawn flipped-X when entering from the right.
var kemleyA = sprite{
	"##.....",
	"#####..",
	"#######",
	"#####..",
	"##.....",
}

var kemleyB = sprite{
	".##....",
	"##.###.",
	"#######",
	"##.###.",
	".##....",
}

// Helm — small hovering enemy that fires straight down.
var helmA = sprite{
	".###.",
	"##.##",
	"#####",
	".#.#.",
	"#...#",
}

var helmB = sprite{
	".###.",
	"#.#.#",
	"#####",
	".#.#.",
	"...##",
}

// Bringer — heavier mountain-zone enemy.
var bringerA = sprite{
	"#.#.#.#",
	".#####.",
	"#######",
	".#####.",
	".#.#.#.",
}

var bringerB = sprite{
	".#.#.#.",
	"#######",
	".#####.",
	"#######",
	"#.#.#.#",
}

// --- Stripe-zone enemy -------------------------------------------------

// Bear — slow chaser that drifts toward the player.
var bearA = sprite{
	"#.....#",
	".#####.",
	"##.#.##",
	"#######",
	".##.##.",
}

var bearB = sprite{
	".#...#.",
	"#######",
	"##.#.##",
	".#####.",
	"#.....#",
}

// --- Bleak-zone enemy --------------------------------------------------

// Floater — drifts up and down, fires bombs.
var floaterA = sprite{
	".###.",
	"#####",
	"##.##",
	"#####",
	".#.#.",
}

var floaterB = sprite{
	".###.",
	"##.##",
	"#####",
	"##.##",
	"#.#.#",
}

// --- Rainbow-zone enemy ------------------------------------------------

// Dancer — descends through the rainbow tunnel.
var dancerA = sprite{
	"..#..",
	".###.",
	"##.##",
	"#####",
	"#.#.#",
}

var dancerB = sprite{
	"..#..",
	"#####",
	"##.##",
	".###.",
	"#...#",
}

// --- Styx-zone obstacles + Gond ---------------------------------------

// Gond — the chamber boss. A ringed brain with two eye sockets that
// glow when the boss is firing. Drawn at a fixed canvas position
// during psBossFight; player must land hits on the central core.
var gondBody = sprite{
	"....######....",
	"..##########..",
	".############.",
	".##........##.",
	"##.#######..##",
	"##.##...##..##",
	"##..##.##...##",
	".##..###...##.",
	".############.",
	"..##########..",
	"....######....",
}

// Gond eye-glow overlay — rendered on top of gondBody when the boss
// is "charging" a shot.
var gondEyeGlow = sprite{
	"..............",
	"..............",
	"..............",
	"..............",
	"...##.....##..",
	"...##.....##..",
	"..............",
	"..............",
	"..............",
	"..............",
	"..............",
}

// --- Energy pod --------------------------------------------------------

// Energy pod: an "E" set inside a small chamber. Picked up to refill
// the energy meter and trigger the powered-up state.
var energyPodA = sprite{
	".#####",
	"##....",
	"#####.",
	"##....",
	".#####",
}

var energyPodB = sprite{
	".#####",
	"##....",
	"###...",
	"##....",
	".#####",
}

// --- Enemy projectile --------------------------------------------------

var enemyBulletA = sprite{
	"#",
	"#",
}

var enemyBulletB = sprite{".", "#"}

// --- Generic enemy explosion ------------------------------------------

var enemyExplode0 = sprite{
	"..#..",
	".#.#.",
	"#...#",
	".#.#.",
	"..#..",
}

var enemyExplode1 = sprite{
	"#...#",
	".#.#.",
	"..#..",
	".#.#.",
	"#...#",
}

var enemyExplode2 = sprite{
	"#.#.#",
	".....",
	"#...#",
	".....",
	"#.#.#",
}

// --- Star palette (parallax background) -------------------------------

var starPalette = []engine.Color{
	{R: 255, G: 255, B: 255, A: 255},
	{R: 200, G: 220, B: 255, A: 255},
	{R: 255, G: 220, B: 180, A: 255},
	{R: 180, G: 200, B: 255, A: 255},
	{R: 255, G: 180, B: 180, A: 255},
}
