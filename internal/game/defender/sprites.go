package defender

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// sprite is a row-major bitmap. '#' is a set pixel; any other rune is
// transparent (skipped at draw time so it composites over whatever's
// underneath).
type sprite []string

func (s sprite) width() int {
	if len(s) == 0 {
		return 0
	}
	return len(s[0])
}

func (s sprite) height() int { return len(s) }

// drawSprite blits s onto c at canvas pixel position (x, y). Only '#'
// pixels are emitted.
func drawSprite(c *engine.Canvas, x, y int, s sprite, fg engine.Color) {
	for row, line := range s {
		for col := 0; col < len(line); col++ {
			if line[col] == '#' {
				c.Set(x+col, y+row, fg)
			}
		}
	}
}

// drawSpriteFlipX blits s mirrored along the vertical axis — used to
// give the player and a couple of enemy sprites a left-facing pose
// without authoring a separate bitmap.
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

// ----- Player ship -----------------------------------------------------
//
// The Defender ship is a flat delta with a stubby tail. It's authored
// facing right; drawSpriteFlipX flips it for left-facing flight. The
// shape is wider than tall to read at small scales — terminal pixels
// are roughly square once half-blocks are accounted for.

var playerShip = sprite{
	".......##",
	".......##",
	"##########",
	"###########",
	"##########",
	".......##",
	".......##",
}

// Player thrust flame, drawn behind the tail when accelerating. Two
// frames cycled fast for flicker.
var thrustFlameA = sprite{
	".",
	"#",
	"##",
	"#",
	".",
}

var thrustFlameB = sprite{
	"#",
	"##",
	"#",
	".",
	".",
}

// Player explosion is rendered as a procedural starburst (see
// drawExplosion). No sprite needed.

// ----- Humanoid -------------------------------------------------------
//
// Stick-figure with a slightly bulbous head — three pixels wide so the
// scanner can distinguish them from enemy blips.

var humanoidSprite = sprite{
	".#.",
	"###",
	".#.",
	"#.#",
}

// Humanoid being lifted (limbs dangling) — same silhouette, just the
// "walking" frame frozen. Reused for the falling pose too.

// ----- Lander ---------------------------------------------------------
//
// Inverted-U with a single eye dome. Two frames so it has a subtle
// bobble; Defender's landers actually flash colour rather than animate
// position but the wing-flap-style flicker reads well in the terminal.

var landerA = sprite{
	"..###..",
	".#####.",
	"#######",
	"#.#.#.#",
	"#.....#",
}

var landerB = sprite{
	"..###..",
	".#####.",
	"#######",
	"#######",
	".#.#.#.",
}

// ----- Mutant ---------------------------------------------------------
//
// Jagged, chaotic — what a lander becomes after a successful abduction.

var mutantA = sprite{
	"#.#.#.#",
	".#####.",
	"##.#.##",
	".#####.",
	"#.#.#.#",
}

var mutantB = sprite{
	".#.#.#.",
	"##.#.##",
	".#####.",
	"##.#.##",
	".#.#.#.",
}

// ----- Bomber ---------------------------------------------------------
//
// Slow, wide, drops cross-shaped mines along its track.

var bomberA = sprite{
	"..#######..",
	".####.####.",
	"###########",
	".####.####.",
	"..#######..",
}

var bomberB = sprite{
	"..#######..",
	".#########.",
	"###.###.###",
	".#########.",
	"..#######..",
}

// ----- Pod ------------------------------------------------------------
//
// Round-ish target that bursts into a quartet of Swarmers when shot.

var podA = sprite{
	".###.",
	"#####",
	"##.##",
	"#####",
	".###.",
}

var podB = sprite{
	".###.",
	"##.##",
	"#####",
	"##.##",
	".###.",
}

// ----- Swarmer --------------------------------------------------------
//
// Small, fast, irritating. Appears in 4-packs from a destroyed Pod.

var swarmerA = sprite{
	".#.",
	"###",
	".#.",
}

var swarmerB = sprite{
	"#.#",
	".#.",
	"#.#",
}

// ----- Baiter ---------------------------------------------------------
//
// Fast yo-yo. Spawns if the player stalls on the current wave.

var baiterA = sprite{
	"...###...",
	"..#####..",
	"#########",
	"..#####..",
	"...###...",
}

var baiterB = sprite{
	"...###...",
	"#.#####.#",
	"#########",
	"#.#####.#",
	"...###...",
}

// ----- Projectiles ----------------------------------------------------
//
// The player's laser is a long horizontal bar (Defender's signature
// "smoking" shot). It's authored facing right; flipping is geometry,
// not a separate sprite — the world position just shifts.

const playerShotLen = 9

// Enemy bolts are slow, short bars.
var enemyBolt = sprite{
	"###",
}

// Bomber mine — a cross that hangs in space for a couple seconds.
var bomberMine = sprite{
	"..#..",
	"..#..",
	"#####",
	"..#..",
	"..#..",
}

// ----- Smart-bomb shockwave ------------------------------------------
//
// Drawn procedurally as expanding concentric rings (see drawSmartBomb).

// ----- Palette --------------------------------------------------------

var (
	colPlayer    = engine.Color{R: 220, G: 240, B: 255, A: 255}
	colThrust    = engine.Color{R: 255, G: 200, B: 60, A: 255}
	colHumanoid  = engine.Color{R: 80, G: 240, B: 120, A: 255}
	colLander    = engine.Color{R: 255, G: 80, B: 80, A: 255}
	colMutant    = engine.Color{R: 220, G: 80, B: 220, A: 255}
	colBomber    = engine.Color{R: 80, G: 200, B: 255, A: 255}
	colPod       = engine.Color{R: 255, G: 160, B: 60, A: 255}
	colSwarmer   = engine.Color{R: 255, G: 240, B: 80, A: 255}
	colBaiter    = engine.Color{R: 200, G: 255, B: 80, A: 255}
	colPlayerLas = engine.Color{R: 255, G: 255, B: 200, A: 255}
	colEnemyShot = engine.Color{R: 255, G: 120, B: 60, A: 255}
	colMine      = engine.Color{R: 200, G: 80, B: 80, A: 255}
	colTerrain   = engine.Color{R: 0, G: 200, B: 80, A: 255}
	colScanFrame = engine.Color{R: 80, G: 100, B: 140, A: 255}
	colStarDim   = engine.Color{R: 80, G: 80, B: 120, A: 255}
	colStarBri   = engine.Color{R: 240, G: 240, B: 255, A: 255}
)
