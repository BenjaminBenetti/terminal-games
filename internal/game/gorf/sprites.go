package gorf

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// sprite is a multi-colour row-major bitmap. Each character in a row
// indexes the palette supplied at draw time. The convention is:
//
//	'.'  transparent (skipped)
//	'#'  body / primary
//	'o'  secondary
//	'*'  highlight
//	'+'  trim / accent
//	'='  shield / glass
//
// All rows in a sprite must have the same length.
type sprite []string

func (s sprite) width() int {
	if len(s) == 0 {
		return 0
	}
	return len(s[0])
}

func (s sprite) height() int { return len(s) }

// drawSprite blits s into c at canvas pixel (x, y) using the supplied
// palette. Characters not in the palette are skipped so the sprite
// composites cleanly over whatever is already on the canvas.
func drawSprite(c *engine.Canvas, x, y int, s sprite, palette map[byte]engine.Color) {
	for row, line := range s {
		for col := 0; col < len(line); col++ {
			ch := line[col]
			if ch == '.' {
				continue
			}
			fg, ok := palette[ch]
			if !ok {
				continue
			}
			c.Set(x+col, y+row, fg)
		}
	}
}

// =====================================================================
// Player — defender fighter with crossed laser barrels (9x6)
// =====================================================================

// Player ship. The original Gorf ship has a tall central body flanked by
// two laser barrels; the look here keeps that silhouette in 9 px wide.
var playerSprite = sprite{
	"#.#.#.#.#",
	"#.#.#.#.#",
	".#.###.#.",
	"..#####..",
	".o#####o.",
	"oo#####oo",
	".oo###oo.",
	"##.###.##",
}

var playerExplodeA = sprite{
	".o.#.#.o.",
	"o.#.o.#.o",
	".#.o#o.#.",
	"#.#####.#",
	".o#####o.",
	"#.o###o.#",
	".#.#o#.#.",
	"o.o.#.o.o",
}

var playerExplodeB = sprite{
	"#.o.#.o.#",
	".#.o#o.#.",
	"o.o###o.o",
	".#######.",
	"##.###.##",
	".#######.",
	"o.o###o.o",
	".#.o#o.#.",
}

// Player quad-laser bolt — a 5-pixel-wide vertical beam with a hot core.
var playerLaserSprite = sprite{
	"o.#.o",
	"o*#*o",
	"o*#*o",
	"o.#.o",
	"o*#*o",
	"o.#.o",
}

// =====================================================================
// Astro Battles aliens — three rows of bird-like invaders (8x6)
// =====================================================================
//
// The three kinds share width/height so they tile cleanly into a marching
// formation; the differing colours and silhouettes are how the player
// tells them apart.

var astroBirdA = sprite{
	"..####..",
	".######.",
	"##.##.##",
	"########",
	".#.##.#.",
	"#.#..#.#",
}

var astroBirdB = sprite{
	"..####..",
	".######.",
	"##.##.##",
	"########",
	"#.#..#.#",
	".#.##.#.",
}

var astroSquidA = sprite{
	"...##...",
	"..####..",
	".######.",
	"##.##.##",
	"#.####.#",
	"#.#..#.#",
}

var astroSquidB = sprite{
	"...##...",
	"..####..",
	".######.",
	"##.##.##",
	"#.####.#",
	".#.##.#.",
}

var astroCrabA = sprite{
	"#......#",
	".#....#.",
	".######.",
	"##.##.##",
	"########",
	".##..##.",
}

var astroCrabB = sprite{
	"#......#",
	"##....##",
	".######.",
	"##.##.##",
	".######.",
	"#.#..#.#",
}

// Generic small alien explosion (8x6).
var astroExplode = sprite{
	"#.#..#.#",
	".#.##.#.",
	"##.##.##",
	".######.",
	"#.#.#.#.",
	".#.#.#.#",
}

// =====================================================================
// Astro Battles force-field shield — a curved arch (60x6, scaled at use)
// =====================================================================
//
// The force-field is drawn procedurally from a parabola because its
// width adapts to the canvas; this template is only the shape style.

// =====================================================================
// Laser Attack ships — wedge-shaped craft with under-mounted cannons
// =====================================================================
//
// Two animation frames cycle the warning lights. 9x6.

var laserShipA = sprite{
	"...#.#...",
	"..#####..",
	".#######.",
	"##*###*##",
	"#oo###oo#",
	".o.#.#.o.",
}

var laserShipB = sprite{
	"...#.#...",
	"..#####..",
	".#######.",
	"##o###o##",
	"#*o###o*#",
	".#.o.o.#.",
}

// Laser flagship — slightly bigger, leads a column of laser ships. 11x7.
var laserFlagA = sprite{
	"....###....",
	"...#####...",
	"..#######..",
	".####o####.",
	"##*#####*##",
	"#oo#####oo#",
	".#.#.#.#.#.",
}

var laserFlagB = sprite{
	"....###....",
	"...#####...",
	"..#######..",
	".####o####.",
	"##o#####o##",
	"#*o#####o*#",
	".o.#.#.#.o.",
}

// =====================================================================
// Galaxians swooper — angled-wing craft, two frames (7x6)
// =====================================================================

var galaxianA = sprite{
	"..###..",
	".#####.",
	"#######",
	"##.#.##",
	".#####.",
	"#.#.#.#",
}

var galaxianB = sprite{
	"..###..",
	".#####.",
	"#######",
	"#######",
	"##.#.##",
	".#.#.#.",
}

// Diving "anti-Gorfian fighter" variant — same skeleton, red palette. 7x6.
var galaxianDiveA = sprite{
	"#.....#",
	".#.#.#.",
	".#####.",
	"#######",
	".#####.",
	".#...#.",
}

var galaxianDiveB = sprite{
	".#...#.",
	"#.#.#.#",
	".#####.",
	"#######",
	".#####.",
	"#.....#",
}

// =====================================================================
// Space Warp ships — arrow craft emerging from a vanishing point
// =====================================================================
//
// Drawn at three sizes to fake the perspective scaling from "far away"
// to "right in your face". Each size is single-frame because they
// rotate around the warp point too quickly for wing-flap to read.

var warpShipSmall = sprite{
	"#",
}

var warpShipTiny = sprite{
	"##.",
	"###",
	"##.",
}

var warpShipMed = sprite{
	".#...",
	".###.",
	"#####",
	".###.",
	".#...",
}

var warpShipBig = sprite{
	"..#....",
	".####..",
	"#######",
	"#######",
	".####..",
	"..#....",
}

// =====================================================================
// Flag Ship (final boss) — large mothership at top of screen
// =====================================================================
//
// The Gorfian mothership is built from a wide upper hull, a central
// reactor block, and a row of square shields that slide horizontally
// underneath. The drawing code paints the hull and reactor as fixed
// sprites and the shields as a separate animated row of tiles.

var flagshipHull = sprite{
	".....#############.....",
	"....###############....",
	"...#################...",
	"..####ooooooooo####....",
	".#####o*******o#####...",
	"#####o**=====**o#####..",
	"#####o**=+++=**o#####..",
	"#####o**=====**o#####..",
	"#####o*******o######...",
	"..####ooooooooo####....",
	"...#################...",
	"....###############....",
	".....#############.....",
}

// Single shield tile — flagship's defensive belt is N of these.
var flagshipShieldTile = sprite{
	"#####",
	"#=+=#",
	"#+#+#",
	"#=+=#",
	"#####",
}

// Reactor "exposed" highlight overlay (drawn when the shield gap aligns
// with the reactor).
var flagshipReactor = sprite{
	"=+++=",
	"+###+",
	"+###+",
	"+###+",
	"=+++=",
}

// =====================================================================
// Projectiles
// =====================================================================

// Enemy bomb — small descending sprite, 2 frames.
var bombA = sprite{
	".#.",
	"###",
	".#.",
	".o.",
}

var bombB = sprite{
	".o.",
	"###",
	".#.",
	".#.",
}

// Boss bomb — slightly bigger.
var bossBomb = sprite{
	".##.",
	"####",
	"####",
	".oo.",
}

// =====================================================================
// Palettes — colour assignments per entity type
// =====================================================================

var playerPalette = map[byte]engine.Color{
	'#': {R: 230, G: 240, B: 250, A: 255},
	'o': {R: 70, G: 200, B: 240, A: 255},
}

var playerLaserPalette = map[byte]engine.Color{
	'#': {R: 255, G: 250, B: 220, A: 255},
	'o': {R: 250, G: 220, B: 80, A: 255},
	'*': {R: 250, G: 100, B: 240, A: 255},
}

var playerExplodePalette = map[byte]engine.Color{
	'#': {R: 250, G: 200, B: 80, A: 255},
	'o': {R: 240, G: 80, B: 80, A: 255},
}

// Three Astro Battle alien palettes — the row determines colour.
var astroBirdPalette = map[byte]engine.Color{
	'#': {R: 250, G: 230, B: 90, A: 255},
}

var astroSquidPalette = map[byte]engine.Color{
	'#': {R: 90, G: 230, B: 240, A: 255},
}

var astroCrabPalette = map[byte]engine.Color{
	'#': {R: 110, G: 240, B: 130, A: 255},
}

var astroExplodePalette = map[byte]engine.Color{
	'#': {R: 250, G: 180, B: 80, A: 255},
}

// Force-field shield colour — pulsates between two tones; the renderer
// picks based on per-pixel damage state.
var forceFieldPalette = map[byte]engine.Color{
	'#': {R: 110, G: 200, B: 250, A: 255},
}

// Laser-attack palettes.
var laserShipPalette = map[byte]engine.Color{
	'#': {R: 240, G: 100, B: 220, A: 255},
	'o': {R: 250, G: 200, B: 90, A: 255},
	'*': {R: 250, G: 250, B: 250, A: 255},
}

var laserFlagPalette = map[byte]engine.Color{
	'#': {R: 250, G: 80, B: 80, A: 255},
	'o': {R: 250, G: 230, B: 110, A: 255},
	'*': {R: 250, G: 250, B: 250, A: 255},
}

var laserBeamPalette = map[byte]engine.Color{
	'#': {R: 250, G: 90, B: 220, A: 255},
}

// Galaxians palettes.
var galaxianPalette = map[byte]engine.Color{
	'#': {R: 240, G: 230, B: 120, A: 255},
}

var galaxianDivePalette = map[byte]engine.Color{
	'#': {R: 250, G: 90, B: 90, A: 255},
}

// Warp-ship palettes — colour brightens with the ship's growth so it
// feels like it's "coming into focus".
var warpPaletteFar = map[byte]engine.Color{
	'#': {R: 130, G: 130, B: 200, A: 255},
}

var warpPaletteMid = map[byte]engine.Color{
	'#': {R: 200, G: 180, B: 250, A: 255},
}

var warpPaletteNear = map[byte]engine.Color{
	'#': {R: 250, G: 130, B: 240, A: 255},
}

// Flagship palette — the Gorfian boss is a saturated red with chrome
// trim and a glowing magenta reactor.
var flagshipPalette = map[byte]engine.Color{
	'#': {R: 220, G: 60, B: 60, A: 255},
	'o': {R: 250, G: 230, B: 90, A: 255},
	'*': {R: 200, G: 200, B: 220, A: 255},
	'=': {R: 250, G: 100, B: 240, A: 255},
	'+': {R: 255, G: 240, B: 250, A: 255},
}

var flagshipShieldPalette = map[byte]engine.Color{
	'#': {R: 90, G: 160, B: 240, A: 255},
	'=': {R: 160, G: 220, B: 250, A: 255},
	'+': {R: 230, G: 240, B: 250, A: 255},
}

// Bomb palette — orange-pink falling tracers.
var bombPalette = map[byte]engine.Color{
	'#': {R: 250, G: 220, B: 110, A: 255},
	'o': {R: 250, G: 130, B: 90, A: 255},
}

var bossBombPalette = map[byte]engine.Color{
	'#': {R: 250, G: 90, B: 220, A: 255},
	'o': {R: 250, G: 230, B: 250, A: 255},
}
