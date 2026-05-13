package galaxian

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// colorSprite is a row-major multi-colour bitmap. Each character in the
// row strings indexes a palette supplied at draw time. Common keys:
//
//	'.'  transparent (no pixel drawn)
//	'#'  body / primary
//	'o'  wing / secondary
//	'*'  highlight / tertiary
//	'+'  trim / quaternary
//
// All rows in a sprite must be the same length.
type colorSprite []string

func (s colorSprite) width() int {
	if len(s) == 0 {
		return 0
	}
	return len(s[0])
}

func (s colorSprite) height() int { return len(s) }

// drawColorSprite blits a multi-colour sprite at canvas pixel (x, y).
// Characters not present in palette (and the literal '.') are skipped so
// the sprite composites cleanly over whatever is already on the canvas.
func drawColorSprite(c *engine.Canvas, x, y int, s colorSprite, palette map[byte]engine.Color) {
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

// ---------------------------------------------------------------------
// Alien sprites — all face "down" (toward the player). Two animation
// frames per type for wing-flap motion. Designed to fit on an 8x8 cell
// while leaving a 1-px gap between formation neighbours.
// ---------------------------------------------------------------------

// Drone — the most numerous foe, occupies the bottom three rows of the
// formation. Two body colours (blue/cyan) plus red eye accents.
var droneA = colorSprite{
	"...##...",
	"..####..",
	".oo##oo.",
	"oo*##*oo",
	"oo####oo",
	".######.",
	".#.##.#.",
	"#.#..#.#",
}

var droneB = colorSprite{
	"...##...",
	"..####..",
	"oo*##*oo",
	"oo####oo",
	".######.",
	".######.",
	"##....##",
	".#....#.",
}

// Bee — middle-row alien (rows 2 and 3 in some patterns). Red body with
// yellow accent.
var beeA = colorSprite{
	"...##...",
	"...##...",
	"..o##o..",
	".oo##oo.",
	"###**###",
	".######.",
	"##.##.##",
	"#......#",
}

var beeB = colorSprite{
	"...##...",
	"...##...",
	"o..##..o",
	"oo.##.oo",
	"###**###",
	".######.",
	".######.",
	"#.#..#.#",
}

// Boss — purple commander, sits between the bees and the flagship.
// Three colours: purple body, magenta wings, white eye.
var bossA = colorSprite{
	"...##...",
	"..####..",
	".o####o.",
	"oo*##*oo",
	"oo####oo",
	"oo####oo",
	".#.##.#.",
	"##....##",
}

var bossB = colorSprite{
	"...##...",
	"..####..",
	"oo*##*oo",
	"oo####oo",
	"oo####oo",
	".######.",
	"##....##",
	"#......#",
}

// Flagship — the prize. Two-frame animation; bigger and broader than the
// rest of the formation. Three palette slots (body / wing / accent) so
// the classic red/yellow flagship colours read clearly.
var flagshipA = colorSprite{
	"....##....",
	"...####...",
	"..o####o..",
	".oo*##*oo.",
	".########.",
	"##########",
	"oo######oo",
	"##.####.##",
	".#......#.",
	"#.#....#.#",
}

var flagshipB = colorSprite{
	"....##....",
	"...####...",
	".oo####oo.",
	"oo*####*oo",
	".########.",
	".########.",
	"oo######oo",
	"##.####.##",
	"##......##",
	".#......#.",
}

// ---------------------------------------------------------------------
// Player and projectile sprites.
// ---------------------------------------------------------------------

// Player defender. Pointed top, wide base — classic Galaxian fighter.
var playerSprite = colorSprite{
	"....#....",
	"....#....",
	"...###...",
	"...###...",
	"..o###o..",
	"..#####..",
	".oo###oo.",
	".#######.",
	"#########",
	"#########",
}

// Two-frame player explosion — alternating debris pattern.
var playerExplodeA = colorSprite{
	".o.#.#.o.",
	"o.#.o.#.o",
	".#.o#o.#.",
	"#.#####.#",
	".o#####o.",
	"#.o###o.#",
	".#.#o#.#.",
	"o.o.#.o.o",
	".#.o.o.#.",
	"#.#.#.#.#",
}

var playerExplodeB = colorSprite{
	"#.o.#.o.#",
	".#.o#o.#.",
	"o.o###o.o",
	".#######.",
	"##.###.##",
	".#######.",
	"o.o###o.o",
	".#.o#o.#.",
	"#.o.#.o.#",
	".o.#.#.o.",
}

// Player bullet — 1×3 vertical streak. Drawn in a hot white-yellow.
var playerBulletSprite = colorSprite{
	"#",
	"#",
	"*",
}

// Alien bullet — small angled dart. Two frames give it a faint shimmer
// as it falls. Bullets are 3×3 so they read clearly against the stars.
var alienBulletA = colorSprite{
	".#.",
	"###",
	"o.o",
}

var alienBulletB = colorSprite{
	".o.",
	"###",
	"#.#",
}

// Small twinkle marker used when something is destroyed mid-air. Three
// frames, cycled quickly, then the entity is removed. Single-colour.
var explodeA = colorSprite{
	"..#..",
	".#.#.",
	"#.o.#",
	".#.#.",
	"..#..",
}

var explodeB = colorSprite{
	".#.#.",
	"#.o.#",
	".o.o.",
	"#.o.#",
	".#.#.",
}

var explodeC = colorSprite{
	"#...#",
	".#.#.",
	"..o..",
	".#.#.",
	"#...#",
}

// ---------------------------------------------------------------------
// Palettes — picked to evoke the original cabinet artwork.
// ---------------------------------------------------------------------

// alienKind is what an alien is. The kind determines its sprite frames,
// palette, scoring, and whether it can lead a convoy.
type alienKind int

const (
	kindDrone alienKind = iota
	kindBee
	kindBoss
	kindFlagship
)

func (k alienKind) frames() (colorSprite, colorSprite) {
	switch k {
	case kindDrone:
		return droneA, droneB
	case kindBee:
		return beeA, beeB
	case kindBoss:
		return bossA, bossB
	case kindFlagship:
		return flagshipA, flagshipB
	}
	return droneA, droneB
}

func (k alienKind) palette() map[byte]engine.Color {
	switch k {
	case kindDrone:
		// Bright cyan body, royal-blue wings, red highlights.
		return map[byte]engine.Color{
			'#': {R: 120, G: 220, B: 255, A: 255},
			'o': {R: 80, G: 130, B: 240, A: 255},
			'*': {R: 255, G: 120, B: 120, A: 255},
		}
	case kindBee:
		// Crimson body, white-yellow wings, hot pink accents.
		return map[byte]engine.Color{
			'#': {R: 240, G: 70, B: 70, A: 255},
			'o': {R: 250, G: 220, B: 120, A: 255},
			'*': {R: 255, G: 240, B: 220, A: 255},
		}
	case kindBoss:
		// Violet body, magenta wings, white highlight.
		return map[byte]engine.Color{
			'#': {R: 180, G: 90, B: 230, A: 255},
			'o': {R: 235, G: 80, B: 220, A: 255},
			'*': {R: 250, G: 250, B: 250, A: 255},
		}
	case kindFlagship:
		// Red body, yellow wings, white highlights — the trophy ship.
		return map[byte]engine.Color{
			'#': {R: 250, G: 60, B: 60, A: 255},
			'o': {R: 250, G: 230, B: 90, A: 255},
			'*': {R: 250, G: 250, B: 250, A: 255},
		}
	}
	return nil
}

// stationaryScore is the points awarded when the alien is killed while
// still in formation.
func (k alienKind) stationaryScore() int {
	switch k {
	case kindDrone:
		return 30
	case kindBee:
		return 40
	case kindBoss:
		return 50
	case kindFlagship:
		return 60
	}
	return 0
}

// divingScore is the points awarded when the alien is killed while it
// has left the formation (in a dive or returning to slot). Flagships
// dive solo or with 1–2 escorts; their bonus is handled separately.
func (k alienKind) divingScore() int {
	switch k {
	case kindDrone:
		return 60
	case kindBee:
		return 80
	case kindBoss:
		return 100
	case kindFlagship:
		return 150
	}
	return 0
}

// playerPalette is the defender's colour scheme — cyan hull with yellow
// trim, classic Galaxian "good guy" colours.
var playerPalette = map[byte]engine.Color{
	'#': {R: 230, G: 240, B: 250, A: 255},
	'o': {R: 80, G: 220, B: 240, A: 255},
}

var playerBulletPalette = map[byte]engine.Color{
	'#': {R: 255, G: 250, B: 220, A: 255},
	'*': {R: 250, G: 220, B: 80, A: 255},
}

// alienBulletPalette is the falling dart used by diving aliens. Hot
// yellow body, orange tail.
var alienBulletPalette = map[byte]engine.Color{
	'#': {R: 250, G: 240, B: 120, A: 255},
	'o': {R: 250, G: 150, B: 70, A: 255},
}

var explodePalette = map[byte]engine.Color{
	'#': {R: 250, G: 180, B: 80, A: 255},
	'o': {R: 255, G: 240, B: 200, A: 255},
}

var playerExplodePalette = map[byte]engine.Color{
	'#': {R: 250, G: 200, B: 90, A: 255},
	'o': {R: 240, G: 80, B: 80, A: 255},
}
