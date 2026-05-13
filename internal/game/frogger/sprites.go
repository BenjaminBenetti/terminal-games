package frogger

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// colorSprite is a row-major pixel-art bitmap whose pixels are coloured by
// looking up each rune in palette. A '.' (or any rune not in palette) is
// transparent — the sprite composites over whatever is already on the
// canvas. All rows in rows must be the same length.
type colorSprite struct {
	rows    []string
	palette map[byte]engine.Color
}

func (s colorSprite) width() int {
	if len(s.rows) == 0 {
		return 0
	}
	return len(s.rows[0])
}

func (s colorSprite) height() int { return len(s.rows) }

// drawColorSprite blits s into c at canvas pixel (x, y). If flip is true,
// the sprite is mirrored horizontally — used to give one source sprite two
// facings without re-authoring it.
func drawColorSprite(c *engine.Canvas, x, y int, s colorSprite, flip bool) {
	w := s.width()
	for row, line := range s.rows {
		for cx := 0; cx < len(line) && cx < w; cx++ {
			src := cx
			if flip {
				src = w - 1 - cx
			}
			ch := line[src]
			if col, ok := s.palette[ch]; ok {
				c.Set(x+cx, y+row, col)
			}
		}
	}
}

// -- Palette ------------------------------------------------------------
//
// Colours chosen to match the arcade's CRT palette as closely as 8-bit
// RGB allows: bright primary frog green, saturated cars on a flat grey
// asphalt, dark indigo river, lavender median, hedge-green safe zones.

var (
	// Frog.
	frogGreen     = engine.Color{R: 110, G: 230, B: 80, A: 255}
	frogDark      = engine.Color{R: 30, G: 130, B: 50, A: 255}
	frogEye       = engine.Color{R: 230, G: 60, B: 60, A: 255}
	frogDeadEye   = engine.Color{R: 230, G: 230, B: 230, A: 255}
	frogDeadBody  = engine.Color{R: 130, G: 50, B: 50, A: 255}
	frogLadyPink  = engine.Color{R: 240, G: 130, B: 220, A: 255}
	frogLadyDark  = engine.Color{R: 200, G: 80, B: 170, A: 255}
	homeFrogColor = engine.Color{R: 200, G: 230, B: 120, A: 255}

	// Cars — five distinct hues like the arcade strip.
	carRed     = engine.Color{R: 235, G: 60, B: 60, A: 255}
	carYellow  = engine.Color{R: 235, G: 220, B: 80, A: 255}
	carCyan    = engine.Color{R: 80, G: 220, B: 230, A: 255}
	carPink    = engine.Color{R: 235, G: 130, B: 200, A: 255}
	carPurple  = engine.Color{R: 180, G: 110, B: 230, A: 255}
	carWindow  = engine.Color{R: 30, G: 30, B: 60, A: 255}
	carTrim    = engine.Color{R: 30, G: 30, B: 30, A: 255}

	// Truck.
	truckBody  = engine.Color{R: 235, G: 220, B: 80, A: 255}
	truckCab   = engine.Color{R: 230, G: 90, B: 50, A: 255}
	truckTrim  = engine.Color{R: 30, G: 30, B: 30, A: 255}

	// Bulldozer.
	dozerBody  = engine.Color{R: 230, G: 150, B: 60, A: 255}
	dozerTread = engine.Color{R: 60, G: 60, B: 60, A: 255}
	dozerBlade = engine.Color{R: 160, G: 160, B: 170, A: 255}

	// Logs.
	logLight = engine.Color{R: 170, G: 110, B: 60, A: 255}
	logMid   = engine.Color{R: 120, G: 75, B: 40, A: 255}
	logDark  = engine.Color{R: 75, G: 45, B: 25, A: 255}

	// Turtle.
	turtleShell     = engine.Color{R: 80, G: 200, B: 110, A: 255}
	turtleShellDark = engine.Color{R: 40, G: 130, B: 70, A: 255}

	// Crocodile.
	crocBody = engine.Color{R: 50, G: 180, B: 90, A: 255}
	crocDark = engine.Color{R: 20, G: 110, B: 50, A: 255}
	crocTeeth = engine.Color{R: 250, G: 250, B: 230, A: 255}
	crocEye  = engine.Color{R: 250, G: 200, B: 80, A: 255}

	// Fly bonus.
	flyBody  = engine.Color{R: 30, G: 30, B: 30, A: 255}
	flyWing  = engine.Color{R: 180, G: 200, B: 230, A: 255}

	// Playfield.
	bgColor       = engine.Color{R: 0, G: 0, B: 0, A: 255}
	roadColor     = engine.Color{R: 35, G: 35, B: 40, A: 255}
	roadStripe    = engine.Color{R: 200, G: 200, B: 80, A: 255}
	medianColor   = engine.Color{R: 160, G: 80, B: 220, A: 255}
	medianDark    = engine.Color{R: 110, G: 50, B: 170, A: 255}
	riverColor    = engine.Color{R: 20, G: 30, B: 130, A: 255}
	riverHi       = engine.Color{R: 50, G: 70, B: 180, A: 255}
	grassColor = engine.Color{R: 30, G: 110, B: 30, A: 255}
	grassDark  = engine.Color{R: 15, G: 70, B: 15, A: 255}

	// Hedge: a very dark grey-brown stone wall, deliberately distinct
	// from any of the green safe zones so it reads as "wall, not lawn".
	hedgeColor = engine.Color{R: 60, G: 45, B: 70, A: 255}
	hedgeDark  = engine.Color{R: 25, G: 18, B: 35, A: 255}
	hedgeMoss  = engine.Color{R: 90, G: 70, B: 100, A: 255}

	// Home-slot interior: lily-pad pink/magenta (matches arcade), with a
	// brighter ring for the "pad" itself. Specifically NOT water-blue so
	// the player doesn't read it as river-and-die.
	homePadIn   = engine.Color{R: 180, G: 60, B: 140, A: 255}
	homePadEdge = engine.Color{R: 230, G: 120, B: 200, A: 255}
	homeGhost   = engine.Color{R: 70, G: 30, B: 60, A: 255} // faint frog silhouette in empty slots

	// HUD.
	scoreColor   = engine.Color{R: 250, G: 220, B: 80, A: 255}
	hiColor      = engine.Color{R: 240, G: 110, B: 200, A: 255}
	livesColor   = frogGreen
	timeBarOK    = engine.Color{R: 80, G: 220, B: 90, A: 255}
	timeBarWarn  = engine.Color{R: 240, G: 200, B: 60, A: 255}
	timeBarDanger = engine.Color{R: 240, G: 60, B: 60, A: 255}
	timeBarBack  = engine.Color{R: 60, G: 30, B: 30, A: 255}
	hintColor    = engine.Color{R: 200, G: 200, B: 220, A: 255}
	titleColor   = frogGreen
	flashColor   = engine.Color{R: 255, G: 240, B: 120, A: 255}
)

// -- Frog sprites (5x3) ------------------------------------------------
//
// The frog faces a cardinal direction. All four facings are listed
// explicitly rather than rotating one source — pixel art at this size
// reads better when each pose is hand-tuned. Each has a sitting frame
// and a mid-hop frame (legs tucked, body stretched).

var frogUpStand = colorSprite{
	rows: []string{
		"E.G.E",
		"GGGGG",
		"G.G.G",
	},
	palette: map[byte]engine.Color{'G': frogGreen, 'E': frogEye},
}

var frogUpHop = colorSprite{
	rows: []string{
		"GGGGG",
		"GGGGG",
		"GG.GG",
	},
	palette: map[byte]engine.Color{'G': frogGreen},
}

var frogDownStand = colorSprite{
	rows: []string{
		"G.G.G",
		"GGGGG",
		"E.G.E",
	},
	palette: map[byte]engine.Color{'G': frogGreen, 'E': frogEye},
}

var frogDownHop = colorSprite{
	rows: []string{
		"GG.GG",
		"GGGGG",
		"GGGGG",
	},
	palette: map[byte]engine.Color{'G': frogGreen},
}

var frogLeftStand = colorSprite{
	rows: []string{
		"GG.GE",
		"GGGGG",
		"GG.GE",
	},
	palette: map[byte]engine.Color{'G': frogGreen, 'E': frogEye},
}

var frogLeftHop = colorSprite{
	rows: []string{
		".GGGG",
		"GGGGG",
		".GGGG",
	},
	palette: map[byte]engine.Color{'G': frogGreen},
}

var frogRightStand = colorSprite{
	rows: []string{
		"EG.GG",
		"GGGGG",
		"EG.GG",
	},
	palette: map[byte]engine.Color{'G': frogGreen, 'E': frogEye},
}

var frogRightHop = colorSprite{
	rows: []string{
		"GGGG.",
		"GGGGG",
		"GGGG.",
	},
	palette: map[byte]engine.Color{'G': frogGreen},
}

// Splatter / dead frog — used when run over.
var frogSplat = colorSprite{
	rows: []string{
		"DG.GD",
		"GDXDG",
		"DG.GD",
	},
	palette: map[byte]engine.Color{
		'G': frogDeadBody, 'D': frogDark, 'X': frogDeadEye,
	},
}

// Drowning splash — three frames played in sequence over ~0.6 s.
var splashA = colorSprite{
	rows: []string{
		".W.W.",
		"W.W.W",
		".W.W.",
	},
	palette: map[byte]engine.Color{'W': engine.Color{R: 200, G: 220, B: 255, A: 255}},
}

var splashB = colorSprite{
	rows: []string{
		"W...W",
		".W.W.",
		"W...W",
	},
	palette: map[byte]engine.Color{'W': engine.Color{R: 200, G: 220, B: 255, A: 255}},
}

// Lady frog (pink, the one to rescue) — sits on a log waiting for a lift.
var ladyFrogStand = colorSprite{
	rows: []string{
		"D.P.D",
		"PPPPP",
		"P.P.P",
	},
	palette: map[byte]engine.Color{'P': frogLadyPink, 'D': frogLadyDark},
}

// Home-slot frog — appears once a frog is delivered to a home.
var homedFrog = colorSprite{
	rows: []string{
		"E.G.E",
		"GGGGG",
		"G.G.G",
	},
	palette: map[byte]engine.Color{'G': homeFrogColor, 'E': frogEye},
}

// Home-slot lady (delivered with lady-frog bonus) — same shape, pink.
var homedLady = colorSprite{
	rows: []string{
		"D.P.D",
		"PPPPP",
		"P.P.P",
	},
	palette: map[byte]engine.Color{'P': frogLadyPink, 'D': frogLadyDark},
}

// -- Cars (6x3) --------------------------------------------------------
//
// Cars come in two basic shapes (sedan and pickup) with five distinct
// colourways. The sprite faces RIGHT by default — pass flip=true to
// draw it facing left.

func sedan(body engine.Color) colorSprite {
	return colorSprite{
		rows: []string{
			".BBBB.",
			"BBWWBB",
			"TB..BT",
		},
		palette: map[byte]engine.Color{
			'B': body, 'W': carWindow, 'T': carTrim,
		},
	}
}

func pickup(body engine.Color) colorSprite {
	return colorSprite{
		rows: []string{
			"BBB.BB",
			"BWWBBB",
			"TB.BBT",
		},
		palette: map[byte]engine.Color{
			'B': body, 'W': carWindow, 'T': carTrim,
		},
	}
}

var (
	carRedSpr    = sedan(carRed)
	carYellowSpr = sedan(carYellow)
	carCyanSpr   = sedan(carCyan)
	carPinkSpr   = pickup(carPink)
	carPurpleSpr = pickup(carPurple)
)

// -- Truck (13x3) ------------------------------------------------------
//
// Long flatbed — cab on the right with a window, trailer body on the
// left, dark wheel trims along the bottom.

var truckSprite = colorSprite{
	rows: []string{
		"BBBBBBBBB.CCC",
		"BBBBBBBBBCCWC",
		"TBTBTBTBBTC.T",
	},
	palette: map[byte]engine.Color{
		'B': truckBody, 'C': truckCab, 'T': truckTrim,
		'W': carWindow,
	},
}

// -- Bulldozer (6x3) ---------------------------------------------------
//
// Front blade + treads. Visually distinct from the cars on neighbouring
// lanes so the player can read it as a hazard at a glance.

var dozerSprite = colorSprite{
	rows: []string{
		"L.BBBB",
		"LBBBBB",
		"LTTTTT",
	},
	palette: map[byte]engine.Color{
		'B': dozerBody, 'T': dozerTread, 'L': dozerBlade,
	},
}

// -- Turtle (5x3) ------------------------------------------------------
//
// Top-down turtle with a flippered shell. Three sprites for the dive
// animation: surface (full opacity), warning (shell-only), submerged
// (just a ripple). Submerged turtles do not carry the frog.

var turtleSurface = colorSprite{
	rows: []string{
		".SDS.",
		"SDDDS",
		".SDS.",
	},
	palette: map[byte]engine.Color{
		'S': turtleShell, 'D': turtleShellDark,
	},
}

var turtleSurfaceB = colorSprite{
	rows: []string{
		"S.D.S",
		".SDS.",
		"S.D.S",
	},
	palette: map[byte]engine.Color{
		'S': turtleShell, 'D': turtleShellDark,
	},
}

var turtleSinking = colorSprite{
	rows: []string{
		".....",
		".SDS.",
		".....",
	},
	palette: map[byte]engine.Color{
		'S': turtleShell, 'D': turtleShellDark,
	},
}

// -- Crocodile (9x3) ---------------------------------------------------
//
// The croc rides in the home strip occasionally — head poking out of a
// slot with a toothy maw. Frog dies on contact.

var crocSprite = colorSprite{
	rows: []string{
		"CCCCCCC.E",
		"DCWTWTC.D",
		"CCCCCCC..",
	},
	palette: map[byte]engine.Color{
		'C': crocBody, 'D': crocDark, 'W': crocTeeth,
		'T': crocBody, 'E': crocEye,
	},
}

// -- Fly (3x3) ---------------------------------------------------------

var flySprite = colorSprite{
	rows: []string{
		"WBW",
		"BBB",
		".B.",
	},
	palette: map[byte]engine.Color{'B': flyBody, 'W': flyWing},
}
