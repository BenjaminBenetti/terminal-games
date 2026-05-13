package donkeykong

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
// the sprite is mirrored horizontally — useful for facing left/right with
// only one source sprite per pose.
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

// -- Colour palette -----------------------------------------------------

var (
	// Mario.
	marioRed   = engine.Color{R: 224, G: 60, B: 40, A: 255}
	marioBlue  = engine.Color{R: 50, G: 90, B: 220, A: 255}
	marioSkin  = engine.Color{R: 245, G: 200, B: 150, A: 255}
	marioShoe  = engine.Color{R: 120, G: 70, B: 30, A: 255}
	marioHair  = engine.Color{R: 80, G: 40, B: 20, A: 255}

	// Donkey Kong.
	dkDark  = engine.Color{R: 120, G: 60, B: 25, A: 255}
	dkLight = engine.Color{R: 210, G: 140, B: 70, A: 255}
	dkWhite = engine.Color{R: 240, G: 240, B: 240, A: 255}
	dkBlack = engine.Color{R: 30, G: 20, B: 10, A: 255}
	dkMouth = engine.Color{R: 180, G: 90, B: 50, A: 255}

	// Pauline.
	paulineHair  = engine.Color{R: 240, G: 220, B: 60, A: 255}
	paulineDress = engine.Color{R: 230, G: 70, B: 140, A: 255}
	paulineSkin  = marioSkin

	// Barrel.
	barrelMain = engine.Color{R: 205, G: 120, B: 40, A: 255}
	barrelDark = engine.Color{R: 120, G: 55, B: 20, A: 255}

	// Girder / ladder.
	girderRed    = engine.Color{R: 220, G: 80, B: 50, A: 255}
	girderDark   = engine.Color{R: 140, G: 40, B: 20, A: 255}
	ladderYellow = engine.Color{R: 230, G: 200, B: 70, A: 255}
	ladderDark   = engine.Color{R: 160, G: 130, B: 30, A: 255}

	// Hammer.
	hammerHead   = engine.Color{R: 220, G: 220, B: 230, A: 255}
	hammerHandle = engine.Color{R: 140, G: 80, B: 40, A: 255}

	// Oil drum + flame.
	oilMain  = engine.Color{R: 100, G: 90, B: 110, A: 255}
	oilDark  = engine.Color{R: 50, G: 45, B: 60, A: 255}
	flameOut = engine.Color{R: 240, G: 90, B: 30, A: 255}
	flameMid = engine.Color{R: 240, G: 180, B: 50, A: 255}
	flameIn  = engine.Color{R: 250, G: 240, B: 130, A: 255}

	// Fireball.
	fireOut = engine.Color{R: 220, G: 90, B: 40, A: 255}
	fireIn  = engine.Color{R: 250, G: 220, B: 90, A: 255}

	// World.
	bgColor       = engine.Color{R: 0, G: 0, B: 0, A: 255}
	bonusColor    = engine.Color{R: 80, G: 200, B: 250, A: 255}
	scoreColor    = engine.Color{R: 240, G: 210, B: 80, A: 255}
	livesColor    = engine.Color{R: 120, G: 240, B: 140, A: 255}
	heartColor    = marioRed
	titleColor    = engine.Color{R: 240, G: 210, B: 80, A: 255}
	hintColor     = engine.Color{R: 200, G: 200, B: 220, A: 255}
	exclaimColor = engine.Color{R: 255, G: 240, B: 120, A: 255}
)

// -- Mario sprites (5x6) -----------------------------------------------
//
// Mario faces right by default. Pass flip=true when drawing to face left.
// R=hat/shirt red, S=skin, B=overalls blue, W=shoe, H=back-of-head hair.

var marioStand = colorSprite{
	rows: []string{
		".RRR.",
		"RRRRR",
		".SSS.",
		"RRRRR",
		".BBB.",
		".W.W.",
	},
	palette: map[byte]engine.Color{
		'R': marioRed, 'S': marioSkin, 'B': marioBlue, 'W': marioShoe,
	},
}

// Walking frame A — left foot slightly forward.
var marioWalkA = colorSprite{
	rows: []string{
		".RRR.",
		"RRRRR",
		".SSS.",
		"RRRR.",
		".BBBB",
		"WW.W.",
	},
	palette: map[byte]engine.Color{
		'R': marioRed, 'S': marioSkin, 'B': marioBlue, 'W': marioShoe,
	},
}

// Walking frame B — right foot slightly forward.
var marioWalkB = colorSprite{
	rows: []string{
		".RRR.",
		"RRRRR",
		".SSS.",
		".RRRR",
		"BBBB.",
		".W.WW",
	},
	palette: map[byte]engine.Color{
		'R': marioRed, 'S': marioSkin, 'B': marioBlue, 'W': marioShoe,
	},
}

// Jumping — arm raised, legs tucked.
var marioJump = colorSprite{
	rows: []string{
		".RRR.",
		"RRRRR",
		".SSS.",
		".RRRR",
		"RBBB.",
		"W...W",
	},
	palette: map[byte]engine.Color{
		'R': marioRed, 'S': marioSkin, 'B': marioBlue, 'W': marioShoe,
	},
}

// Climbing — back view, hands on rails.
var marioClimbA = colorSprite{
	rows: []string{
		".HHH.",
		"HHHHH",
		"RSSSR",
		"RBBBR",
		".BBB.",
		".W.W.",
	},
	palette: map[byte]engine.Color{
		'H': marioHair, 'R': marioRed, 'S': marioSkin,
		'B': marioBlue, 'W': marioShoe,
	},
}

var marioClimbB = colorSprite{
	rows: []string{
		".HHH.",
		"HHHHH",
		"RSSSR",
		"RBBBR",
		".B.B.",
		"W...W",
	},
	palette: map[byte]engine.Color{
		'H': marioHair, 'R': marioRed, 'S': marioSkin,
		'B': marioBlue, 'W': marioShoe,
	},
}

// Death pose — flipped upside down.
var marioDead = colorSprite{
	rows: []string{
		".W.W.",
		".BBB.",
		"RRRRR",
		".SSS.",
		"RRRRR",
		".RRR.",
	},
	palette: map[byte]engine.Color{
		'R': marioRed, 'S': marioSkin, 'B': marioBlue, 'W': marioShoe,
	},
}

// Mario with the hammer — wider sprite (7x7) since the hammer head sits
// above (or beside) Mario's body. Two swing frames let it cycle between
// "high" (overhead) and "low" (in front).

var marioHammerHigh = colorSprite{
	rows: []string{
		"..HHH..",
		"..HHH..",
		"..AAA..",
		".RRR...",
		"RRRRR..",
		".SSS...",
		".BBB...",
	},
	palette: map[byte]engine.Color{
		'H': hammerHead, 'A': hammerHandle, 'R': marioRed,
		'S': marioSkin, 'B': marioBlue,
	},
}

var marioHammerLow = colorSprite{
	rows: []string{
		".RRR...",
		"RRRRR..",
		".SSS...",
		"RRRR.AA",
		".BBB.HH",
		".B.B.HH",
		".W.W...",
	},
	palette: map[byte]engine.Color{
		'H': hammerHead, 'A': hammerHandle, 'R': marioRed,
		'S': marioSkin, 'B': marioBlue, 'W': marioShoe,
	},
}

// -- Pauline (5x7) ------------------------------------------------------

var paulineSprite = colorSprite{
	rows: []string{
		"YYYYY",
		".YYY.",
		".SSS.",
		"DDDDD",
		".DDD.",
		".DDD.",
		".S.S.",
	},
	palette: map[byte]engine.Color{
		'Y': paulineHair, 'S': paulineSkin, 'D': paulineDress,
	},
}

// -- Donkey Kong (14x8) ------------------------------------------------
//
// D=dark fur, L=light fur (face), W=white eye, B=black pupil, N=nose,
// O=open mouth. Two frames give the throw a tiny bit of life: idle and
// arm-raised.

var dkIdle = colorSprite{
	rows: []string{
		".DDDDDDDDDDDD.",
		"DDLLLLLLLLLLDD",
		"DLWBLLLLLLBWLD",
		"DLLLLLNNLLLLLD",
		"DLLLOOOOOOLLLD",
		"DDLLLLLLLLLLDD",
		"LLDDDDDDDDDDLL",
		"LDDLDDDDDDLDDL",
	},
	palette: map[byte]engine.Color{
		'D': dkDark, 'L': dkLight, 'W': dkWhite, 'B': dkBlack,
		'N': dkBlack, 'O': dkMouth,
	},
}

// Throw frame — left arm raised.
var dkThrow = colorSprite{
	rows: []string{
		".DDDDDDDDDDDD.",
		"DDLLLLLLLLLLDD",
		"DLWBLLLLLLBWLD",
		"DLLLLLNNLLLLLD",
		"DLLLOOOOOOLLLD",
		"DDLLLLLLLLLLDD",
		"LLDDDDDDDDDDDD",
		"LLDDDDDDDDLDDL",
	},
	palette: map[byte]engine.Color{
		'D': dkDark, 'L': dkLight, 'W': dkWhite, 'B': dkBlack,
		'N': dkBlack, 'O': dkMouth,
	},
}

// -- Barrel (5x3), two roll frames -------------------------------------

var barrelA = colorSprite{
	rows: []string{
		".OOO.",
		"OXXXO",
		".OOO.",
	},
	palette: map[byte]engine.Color{
		'O': barrelMain, 'X': barrelDark,
	},
}

var barrelB = colorSprite{
	rows: []string{
		".OXO.",
		"OOOXO",
		".OOO.",
	},
	palette: map[byte]engine.Color{
		'O': barrelMain, 'X': barrelDark,
	},
}

// Falling barrel — vertical orientation (3x5).
var barrelFall = colorSprite{
	rows: []string{
		".O.",
		"OOO",
		"OXO",
		"OOO",
		".O.",
	},
	palette: map[byte]engine.Color{
		'O': barrelMain, 'X': barrelDark,
	},
}

// -- Hammer pickup (3x5) -----------------------------------------------

var hammerPickup = colorSprite{
	rows: []string{
		"HHH",
		"HHH",
		".A.",
		".A.",
		".A.",
	},
	palette: map[byte]engine.Color{
		'H': hammerHead, 'A': hammerHandle,
	},
}

// -- Oil drum (7x6) -----------------------------------------------------

var oilDrum = colorSprite{
	rows: []string{
		".OOOOO.",
		"OOOOOOO",
		"OXXXXXO",
		"OOOOOOO",
		"OXXXXXO",
		"OOOOOOO",
	},
	palette: map[byte]engine.Color{
		'O': oilMain, 'X': oilDark,
	},
}

// Flame (5x4) — two frames for flicker.
var flameA = colorSprite{
	rows: []string{
		"..F..",
		".FMF.",
		"FMIMF",
		".FMF.",
	},
	palette: map[byte]engine.Color{
		'F': flameOut, 'M': flameMid, 'I': flameIn,
	},
}

var flameB = colorSprite{
	rows: []string{
		".F.F.",
		"FMFMF",
		".FIF.",
		"FMMMF",
	},
	palette: map[byte]engine.Color{
		'F': flameOut, 'M': flameMid, 'I': flameIn,
	},
}

// -- Fireball (5x5), two frames ----------------------------------------

var fireballA = colorSprite{
	rows: []string{
		"..F..",
		".FIF.",
		"FIWIF",
		".FIF.",
		"..F..",
	},
	palette: map[byte]engine.Color{
		'F': fireOut, 'I': fireIn, 'W': dkWhite,
	},
}

var fireballB = colorSprite{
	rows: []string{
		".F.F.",
		"FIFIF",
		".FWF.",
		"FIFIF",
		".F.F.",
	},
	palette: map[byte]engine.Color{
		'F': fireOut, 'I': fireIn, 'W': dkWhite,
	},
}

