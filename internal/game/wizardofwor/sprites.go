package wizardofwor

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// sprite is a row-major pixel-art bitmap. Each rune in rows is looked
// up in palette to find its colour; runes not in the palette are
// transparent, leaving whatever was already on the canvas alone.
//
// Sprites are authored at a 5×5 base size so they fit a single maze
// cell on small terminals. The renderer scales them up by an integer
// factor when the cell is taller (drawSpriteScaled), which keeps the
// pixel-art look crisp.
type sprite struct {
	rows    []string
	palette map[byte]engine.Color
}

func (s sprite) width() int {
	if len(s.rows) == 0 {
		return 0
	}
	return len(s.rows[0])
}

func (s sprite) height() int { return len(s.rows) }

// drawSpriteScaled blits s into c with its top-left at (x, y), with
// each source pixel expanded into a `scale × scale` block. scale=1
// draws the sprite at native pixel size.
func drawSpriteScaled(c *engine.Canvas, x, y int, s sprite, scale int) {
	if scale <= 1 {
		for row, line := range s.rows {
			for cx := 0; cx < len(line); cx++ {
				ch := line[cx]
				if col, ok := s.palette[ch]; ok {
					c.Set(x+cx, y+row, col)
				}
			}
		}
		return
	}
	for row, line := range s.rows {
		for cx := 0; cx < len(line); cx++ {
			ch := line[cx]
			col, ok := s.palette[ch]
			if !ok {
				continue
			}
			c.FillRect(x+cx*scale, y+row*scale, scale, scale, col)
		}
	}
}

// -- Palette -----------------------------------------------------------
//
// Wizard of Wor's arcade hardware used a punchy retro palette: vivid
// blue for the maze, saturated yellow/red/blue for the monster tiers,
// and a green/yellow Worrior. We approximate those tones here.

var (
	wallColor     = engine.Color{R: 88, G: 120, B: 255, A: 255}
	wallHighlight = engine.Color{R: 140, G: 170, B: 255, A: 255}
	cageBarColor  = engine.Color{R: 255, G: 220, B: 80, A: 255}

	worriorBody = engine.Color{R: 255, G: 230, B: 60, A: 255}
	worriorGun  = engine.Color{R: 235, G: 60, B: 60, A: 255}
	worriorEye  = engine.Color{R: 30, G: 30, B: 60, A: 255}

	burworBody   = engine.Color{R: 80, G: 150, B: 255, A: 255}
	garworBody   = engine.Color{R: 250, G: 200, B: 60, A: 255}
	thorworBody  = engine.Color{R: 230, G: 80, B: 60, A: 255}
	monsterEye   = engine.Color{R: 250, G: 250, B: 250, A: 255}
	monsterPupil = engine.Color{R: 20, G: 20, B: 40, A: 255}

	worlukBody = engine.Color{R: 255, G: 120, B: 200, A: 255}
	worlukWing = engine.Color{R: 200, G: 80, B: 160, A: 255}

	wizardRobe  = engine.Color{R: 160, G: 90, B: 220, A: 255}
	wizardHat   = engine.Color{R: 80, G: 40, B: 140, A: 255}
	wizardBeard = engine.Color{R: 240, G: 240, B: 240, A: 255}

	bulletColor = engine.Color{R: 255, G: 255, B: 120, A: 255}
	fireballHot = engine.Color{R: 255, G: 240, B: 120, A: 255}

	explodeOuter = engine.Color{R: 255, G: 90, B: 40, A: 255}
	explodeMid   = engine.Color{R: 255, G: 220, B: 70, A: 255}
	explodeCore  = engine.Color{R: 255, G: 255, B: 230, A: 255}

	radarFrame  = engine.Color{R: 80, G: 100, B: 200, A: 255}
	radarPlayer = worriorBody
	radarWizard = wizardRobe
	radarWorluk = worlukBody
)

// -- Worrior (player) --------------------------------------------------
//
// Four directional poses, 5×5. The red gun barrel rotates with the
// player so their facing direction is unambiguous. Y = yellow body,
// R = red gun, E = white eye-pupil contrast.

var worriorUp = sprite{
	rows: []string{
		"..R..",
		".YYY.",
		"YEYEY",
		"YYYYY",
		".Y.Y.",
	},
	palette: map[byte]engine.Color{
		'Y': worriorBody, 'R': worriorGun, 'E': worriorEye,
	},
}

var worriorDown = sprite{
	rows: []string{
		".Y.Y.",
		"YYYYY",
		"YEYEY",
		".YYY.",
		"..R..",
	},
	palette: map[byte]engine.Color{
		'Y': worriorBody, 'R': worriorGun, 'E': worriorEye,
	},
}

var worriorLeft = sprite{
	rows: []string{
		"..YYY",
		"R.YEY",
		"RRYYY",
		"R.YEY",
		"..YYY",
	},
	palette: map[byte]engine.Color{
		'Y': worriorBody, 'R': worriorGun, 'E': worriorEye,
	},
}

var worriorRight = sprite{
	rows: []string{
		"YYY..",
		"YEY.R",
		"YYYRR",
		"YEY.R",
		"YYY..",
	},
	palette: map[byte]engine.Color{
		'Y': worriorBody, 'R': worriorGun, 'E': worriorEye,
	},
}

// worriorSprite returns the right pose for the given facing.
func worriorSprite(d direction) sprite {
	switch d {
	case dirUp:
		return worriorUp
	case dirDown:
		return worriorDown
	case dirLeft:
		return worriorLeft
	case dirRight:
		return worriorRight
	}
	return worriorRight
}

// -- Monster bodies ----------------------------------------------------
//
// All three monster tiers share one silhouette — in the arcade, the
// only differences between Burwor, Garwor and Thorwor are colour and
// behaviour. Two walk frames cycle the legs.
// B = body, W = eye whites, P = pupils.

var monsterA = sprite{
	rows: []string{
		".BBB.",
		"WPBPW",
		"BBBBB",
		"BBBBB",
		".B.B.",
	},
}

var monsterB = sprite{
	rows: []string{
		".BBB.",
		"WPBPW",
		"BBBBB",
		"BBBBB",
		"B.B.B",
	},
}

// monsterPalette returns the sprite palette for a given body colour.
func monsterPalette(body engine.Color) map[byte]engine.Color {
	return map[byte]engine.Color{
		'B': body, 'W': monsterEye, 'P': monsterPupil,
	}
}

// monsterFrame picks the walk frame for the given step counter.
func monsterFrame(step int) sprite {
	if step&1 == 0 {
		return monsterA
	}
	return monsterB
}

// drawMonster paints a monster of the given body colour at (x, y),
// cycling the legs based on the step counter.
func drawMonster(c *engine.Canvas, x, y int, body engine.Color, step, scale int) {
	src := monsterFrame(step)
	pal := monsterPalette(body)
	s := sprite{rows: src.rows, palette: pal}
	drawSpriteScaled(c, x, y, s, scale)
}

// drawMonsterGhost paints the faint outline of an invisible Garwor /
// Thorwor — only the eyes are visible. Radar still shows it normally.
func drawMonsterGhost(c *engine.Canvas, x, y int, body engine.Color, scale int) {
	dim := engine.Color{
		R: body.R / 4, G: body.G / 4, B: body.B / 4, A: 255,
	}
	if scale <= 1 {
		c.Set(x, y+1, dim)
		c.Set(x+4, y+1, dim)
		return
	}
	c.FillRect(x, y+scale, scale, scale, dim)
	c.FillRect(x+4*scale, y+scale, scale, scale, dim)
}

// -- Worluk ------------------------------------------------------------
//
// The Worluk is a winged thing that wants to escape via the side
// tunnels — its sprite is wide and unmistakable.

var worlukSprite = sprite{
	rows: []string{
		"W...W",
		"WBWBW",
		"BBWBB",
		".BWB.",
		"..W..",
	},
	palette: map[byte]engine.Color{
		'W': worlukBody, 'B': worlukWing,
	},
}

// -- Wizard ------------------------------------------------------------
//
// Tall pointed hat, white beard, dark robe.

var wizardSprite = sprite{
	rows: []string{
		"..H..",
		".HHH.",
		".RWR.",
		"RWWWR",
		".RWR.",
	},
	palette: map[byte]engine.Color{
		'H': wizardHat,
		'R': wizardRobe,
		'W': wizardBeard,
	},
}

// -- Explosion ---------------------------------------------------------
//
// Three-frame explosion: bright core, fading rings. Indexed by the age
// fraction of the dying timer.

var explosionFrame0 = sprite{
	rows: []string{
		"..O..",
		".OMO.",
		"OMCMO",
		".OMO.",
		"..O..",
	},
	palette: map[byte]engine.Color{
		'O': explodeOuter, 'M': explodeMid, 'C': explodeCore,
	},
}

var explosionFrame1 = sprite{
	rows: []string{
		".OMO.",
		"OMMMO",
		"MMCMM",
		"OMMMO",
		".OMO.",
	},
	palette: map[byte]engine.Color{
		'O': explodeOuter, 'M': explodeMid, 'C': explodeCore,
	},
}

var explosionFrame2 = sprite{
	rows: []string{
		"OO.OO",
		"O.M.O",
		".M.M.",
		"O.M.O",
		"OO.OO",
	},
	palette: map[byte]engine.Color{
		'O': explodeOuter, 'M': explodeMid,
	},
}

// explosionSprite picks the right frame for the given age fraction
// (0..1). Returns nil when the explosion is fully done.
func explosionSprite(frac float64) *sprite {
	switch {
	case frac < 0.33:
		return &explosionFrame0
	case frac < 0.66:
		return &explosionFrame1
	case frac < 1:
		return &explosionFrame2
	}
	return nil
}
