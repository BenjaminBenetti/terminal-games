package flappybird

import (
	"math"
	"strconv"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// theme picks the day/night background palette. The original Flappy Bird
// rotates between a sunny "day" sky and a darker "night" sky between
// runs; we do the same, randomly per round.
type theme int

const (
	themeDay theme = iota
	themeNight
)

// --- Bird sprite ------------------------------------------------------

type birdPalette struct {
	body      engine.Color
	bodyDark  engine.Color
	belly     engine.Color
	bellyDark engine.Color
	wing      engine.Color
	wingDark  engine.Color
	eyeWhite  engine.Color
	pupil     engine.Color
	beak      engine.Color
	beakDark  engine.Color
}

// birdVariant picks one of the three bird color schemes from the
// original. Each session randomly selects yellow, blue, or red.
type birdVariant int

const (
	birdYellow birdVariant = iota
	birdBlue
	birdRed
)

var birdPaletteYellow = birdPalette{
	body:      engine.Color{R: 252, G: 218, B: 80, A: 255},
	bodyDark:  engine.Color{R: 232, G: 178, B: 32, A: 255},
	belly:     engine.Color{R: 252, G: 246, B: 222, A: 255},
	bellyDark: engine.Color{R: 232, G: 226, B: 188, A: 255},
	wing:      engine.Color{R: 252, G: 252, B: 248, A: 255},
	wingDark:  engine.Color{R: 196, G: 196, B: 180, A: 255},
	eyeWhite:  engine.Color{R: 255, G: 255, B: 255, A: 255},
	pupil:     engine.Color{R: 30, G: 30, B: 30, A: 255},
	beak:      engine.Color{R: 252, G: 130, B: 50, A: 255},
	beakDark:  engine.Color{R: 220, G: 80, B: 24, A: 255},
}

var birdPaletteBlue = birdPalette{
	body:      engine.Color{R: 96, G: 168, B: 252, A: 255},
	bodyDark:  engine.Color{R: 56, G: 116, B: 196, A: 255},
	belly:     engine.Color{R: 232, G: 244, B: 252, A: 255},
	bellyDark: engine.Color{R: 196, G: 216, B: 236, A: 255},
	wing:      engine.Color{R: 252, G: 252, B: 248, A: 255},
	wingDark:  engine.Color{R: 196, G: 196, B: 180, A: 255},
	eyeWhite:  engine.Color{R: 255, G: 255, B: 255, A: 255},
	pupil:     engine.Color{R: 30, G: 30, B: 30, A: 255},
	beak:      engine.Color{R: 252, G: 168, B: 64, A: 255},
	beakDark:  engine.Color{R: 220, G: 116, B: 32, A: 255},
}

var birdPaletteRed = birdPalette{
	body:      engine.Color{R: 248, G: 90, B: 80, A: 255},
	bodyDark:  engine.Color{R: 196, G: 48, B: 40, A: 255},
	belly:     engine.Color{R: 252, G: 232, B: 220, A: 255},
	bellyDark: engine.Color{R: 232, G: 200, B: 180, A: 255},
	wing:      engine.Color{R: 252, G: 252, B: 248, A: 255},
	wingDark:  engine.Color{R: 196, G: 196, B: 180, A: 255},
	eyeWhite:  engine.Color{R: 255, G: 255, B: 255, A: 255},
	pupil:     engine.Color{R: 30, G: 30, B: 30, A: 255},
	beak:      engine.Color{R: 252, G: 196, B: 88, A: 255},
	beakDark:  engine.Color{R: 220, G: 148, B: 40, A: 255},
}

// birdPaletteFor returns the body palette for the given variant.
func birdPaletteFor(v birdVariant) birdPalette {
	switch v {
	case birdBlue:
		return birdPaletteBlue
	case birdRed:
		return birdPaletteRed
	default:
		return birdPaletteYellow
	}
}

var birdPaletteFlash = birdPalette{
	body:      engine.Color{R: 255, G: 255, B: 255, A: 255},
	bodyDark:  engine.Color{R: 220, G: 220, B: 220, A: 255},
	belly:     engine.Color{R: 255, G: 255, B: 255, A: 255},
	bellyDark: engine.Color{R: 220, G: 220, B: 220, A: 255},
	wing:      engine.Color{R: 255, G: 255, B: 255, A: 255},
	wingDark:  engine.Color{R: 220, G: 220, B: 220, A: 255},
	eyeWhite:  engine.Color{R: 255, G: 255, B: 255, A: 255},
	pupil:     engine.Color{R: 90, G: 90, B: 90, A: 255},
	beak:      engine.Color{R: 252, G: 240, B: 230, A: 255},
	beakDark:  engine.Color{R: 220, G: 210, B: 200, A: 255},
}

// drawBird renders the 7-wide × 5-tall bird sprite. tilt is -1 (head
// up), 0 (level), or +1 (head down). wingFrame is 0..2 for the
// three-frame flap cycle. variant picks the bird color (yellow/blue/
// red, as in the original). flash overrides everything with a
// near-white palette for the death-flash effect. Coordinates are
// pixel-space, top-left of the sprite.
//
// The sprite is composed in layers so later layers overdraw earlier
// ones: body → belly → wing → eye → beak. This avoids the wing eating
// the eye when both happen to land on the same row.
func drawBird(c *engine.Canvas, x, y int, tilt, wingFrame int, variant birdVariant, flash bool) {
	pal := birdPaletteFor(variant)
	if flash {
		pal = birdPaletteFlash
	}

	// Body: a 5-wide × 5-tall yellow blob with rounded corners (top and
	// bottom rows are 3 wide, the middle three rows are 5 wide).
	bodyPixels := [...][2]int{
		{1, 0}, {2, 0}, {3, 0},
		{0, 1}, {1, 1}, {2, 1}, {3, 1}, {4, 1},
		{0, 2}, {1, 2}, {2, 2}, {3, 2}, {4, 2},
		{0, 3}, {1, 3}, {2, 3}, {3, 3}, {4, 3},
		{1, 4}, {2, 4}, {3, 4},
	}
	for _, p := range bodyPixels {
		c.Set(x+p[0], y+p[1], pal.body)
	}
	// Dark accent: a couple of darker pixels along the right cheek
	// hinting at body shading.
	c.Set(x+4, y+3, pal.bodyDark)

	// Belly: cream-colored patch on the lower half — matches the
	// original's two-tone body. Overdraws the body pixels there.
	bellyPixels := [...][2]int{
		{1, 3}, {2, 3},
		{1, 4}, {2, 4}, {3, 4},
	}
	for _, p := range bellyPixels {
		c.Set(x+p[0], y+p[1], pal.belly)
	}
	c.Set(x+3, y+4, pal.bellyDark)

	// Wing: 2 pixels on the bird's left side, position varies with both
	// wing-frame and tilt so the wing tracks the head's orientation. We
	// clamp to the sprite bounds rather than worry about it: Canvas.Set
	// silently no-ops on out-of-bounds anyway.
	wingMidRow := 2
	switch tilt {
	case -1:
		wingMidRow = 1
	case 1:
		wingMidRow = 3
	}
	wingRow := wingMidRow + (wingFrame - 1)
	c.Set(x+0, y+wingRow, pal.wing)
	c.Set(x+1, y+wingRow, pal.wing)
	c.Set(x+0, y+wingRow+1, pal.wingDark)

	// Head row — where the eye and beak land — shifts with tilt.
	headRow := 2
	switch tilt {
	case -1:
		headRow = 1
	case 1:
		headRow = 3
	}

	// Eye: 2 pixels — white background then black pupil.
	c.Set(x+2, y+headRow, pal.eyeWhite)
	c.Set(x+3, y+headRow, pal.pupil)

	// Beak: 2-pixel orange beak protruding to the right past the body.
	c.Set(x+5, y+headRow, pal.beak)
	c.Set(x+6, y+headRow, pal.beak)
	c.Set(x+5, y+headRow+1, pal.beakDark)
	c.Set(x+6, y+headRow+1, pal.beakDark)
}

// --- Pipes ------------------------------------------------------------

type pipePalette struct {
	highlight engine.Color // brightest stripe, runs down the left of the pipe
	light     engine.Color // light stripe
	main      engine.Color // dominant green
	dark      engine.Color // shadow stripe
	outline   engine.Color // darkest, used for left+right outline columns
}

var pipePaletteGreen = pipePalette{
	highlight: engine.Color{R: 220, G: 252, B: 124, A: 255},
	light:     engine.Color{R: 160, G: 232, B: 60, A: 255},
	main:      engine.Color{R: 112, G: 196, B: 30, A: 255},
	dark:      engine.Color{R: 78, G: 140, B: 22, A: 255},
	outline:   engine.Color{R: 44, G: 84, B: 14, A: 255},
}

// pipePaletteFor exists to keep the door open for theme-specific pipe
// colors. The original keeps pipes green regardless of day/night so for
// now we return the single green palette unconditionally.
func pipePaletteFor(_ theme) pipePalette { return pipePaletteGreen }

// pipeBodyColColor returns the color for a single column of the pipe
// body. The body is shaded as: highlight | light | main… | dark |
// outline — i.e. lit from the left, darker on the right.
func pipeBodyColColor(col int, pal pipePalette) engine.Color {
	switch col {
	case 0:
		return pal.outline
	case 1:
		return pal.highlight
	case 2:
		return pal.light
	case pipeWidth - 1:
		return pal.outline
	case pipeWidth - 2:
		return pal.dark
	default:
		return pal.main
	}
}

// pipeCapColColor returns the color for a single column of the pipe
// cap. The cap is wider than the body by one pixel on each side; the
// extra columns (0 and pipeCapWidth-1) are dark outline pixels. The
// interior columns reuse the body shading so cap and body align
// vertically without color seams.
func pipeCapColColor(col int, pal pipePalette) engine.Color {
	overhang := (pipeCapWidth - pipeWidth) / 2
	if col < overhang || col >= pipeCapWidth-overhang {
		return pal.outline
	}
	return pipeBodyColColor(col-overhang, pal)
}

// drawPipeBody fills a body-sized rectangle at (x, y) using vertical
// stripes of the body palette. Negative or zero h is a no-op so callers
// don't need to special-case clipped pipes.
func drawPipeBody(c *engine.Canvas, x, y, w, h int, pal pipePalette) {
	if h <= 0 {
		return
	}
	for col := 0; col < w; col++ {
		c.FillRect(x+col, y, 1, h, pipeBodyColColor(col, pal))
	}
}

// drawPipeCap fills a cap-sized rectangle at (x, y). isTop is true when
// the cap belongs to the top pipe — the gap-facing edge gets the dark
// outline that gives the cap its characteristic "lip" facing the
// passable gap.
func drawPipeCap(c *engine.Canvas, x, y, w, h int, pal pipePalette, isTop bool) {
	for col := 0; col < w; col++ {
		c.FillRect(x+col, y, 1, h, pipeCapColColor(col, pal))
	}
	if h >= 1 {
		var lipY int
		if isTop {
			lipY = y + h - 1
		} else {
			lipY = y
		}
		c.FillRect(x, lipY, w, 1, pal.outline)
	}
}

// --- Ground ----------------------------------------------------------

type groundPalette struct {
	grassLight engine.Color
	grassDark  engine.Color
	earth      engine.Color
	earthDark  engine.Color
	earthEdge  engine.Color
}

var groundPaletteDay = groundPalette{
	grassLight: engine.Color{R: 138, G: 224, B: 86, A: 255},
	grassDark:  engine.Color{R: 90, G: 168, B: 56, A: 255},
	earth:      engine.Color{R: 222, G: 196, B: 116, A: 255},
	earthDark:  engine.Color{R: 192, G: 162, B: 84, A: 255},
	earthEdge:  engine.Color{R: 156, G: 124, B: 60, A: 255},
}

var groundPaletteNight = groundPalette{
	grassLight: engine.Color{R: 76, G: 144, B: 64, A: 255},
	grassDark:  engine.Color{R: 48, G: 100, B: 42, A: 255},
	earth:      engine.Color{R: 170, G: 144, B: 92, A: 255},
	earthDark:  engine.Color{R: 132, G: 112, B: 68, A: 255},
	earthEdge:  engine.Color{R: 92, G: 78, B: 48, A: 255},
}

func groundPaletteFor(th theme) groundPalette {
	if th == themeNight {
		return groundPaletteNight
	}
	return groundPaletteDay
}

// drawGround paints the bottom strip of the canvas: two rows of grass
// then earth with diagonal scrolling stripes. scrollOffset is the
// accumulated horizontal scroll in pixels — the diagonal pattern's
// phase depends on it so the ground reads as moving beneath the bird.
func drawGround(c *engine.Canvas, w, y0, h int, th theme, scrollOffset float64) {
	pal := groundPaletteFor(th)
	if h <= 0 {
		return
	}
	c.FillRect(0, y0, w, 1, pal.grassLight)
	if h >= 2 {
		c.FillRect(0, y0+1, w, 1, pal.grassDark)
	}
	if h >= 3 {
		c.FillRect(0, y0+2, w, h-2, pal.earth)
	}

	// Diagonal stripes through the earth — the slope is "down and to
	// the right", so subtracting scroll shifts them left over time and
	// the ground reads as moving toward us.
	earthRows := h - 2
	if earthRows <= 0 {
		return
	}
	const stride = 6
	offset := int(math.Floor(scrollOffset)) % stride
	for x := -stride; x < w+earthRows; x++ {
		if mod(x+offset, stride) != 0 {
			continue
		}
		for dy := 0; dy < earthRows; dy++ {
			px := x + dy
			if px < 0 || px >= w {
				continue
			}
			col := pal.earthDark
			if dy == 0 {
				col = pal.earthEdge
			}
			c.Set(px, y0+2+dy, col)
		}
	}

	// Top edge of earth gets a darker line — visual separation from
	// the grass above.
	if h >= 3 {
		c.FillRect(0, y0+2, w, 1, pal.earthEdge)
	}
}

// mod is a positive-result modulo so negative offsets still line up
// neatly with the stripe pattern.
func mod(a, b int) int {
	r := a % b
	if r < 0 {
		r += b
	}
	return r
}

// --- Sky / skyline ----------------------------------------------------

// drawSkyBackground paints a single solid sky color across the entire
// canvas and then layers day clouds or night stars on top depending on
// the theme. t is the scene clock and drives slow cloud drift.
func drawSkyBackground(c *engine.Canvas, th theme, t float64) {
	if th == themeNight {
		c.Clear(engine.Color{R: 28, G: 38, B: 88, A: 255})
		drawStars(c, t)
		drawMoon(c)
		return
	}
	c.Clear(engine.Color{R: 112, G: 198, B: 206, A: 255})
	drawClouds(c, t)
	drawSun(c)
}

// drawClouds paints a handful of soft white blobs that drift slowly to
// the left. Cloud positions are derived from t so they tile naturally
// without us having to track per-cloud state.
func drawClouds(c *engine.Canvas, t float64) {
	w := c.Width()
	h := c.Height()
	cloudCol := engine.Color{R: 246, G: 252, B: 252, A: 255}
	cloudShadow := engine.Color{R: 208, G: 224, B: 232, A: 255}
	// Three clouds, evenly spaced and looped via modulo across the
	// width so they always cover the visible band.
	for i := 0; i < 3; i++ {
		baseX := int(float64(w)*float64(i)/3 - t*4)
		x := mod(baseX, w+30) - 15
		y := h/6 + (i%2)*3
		// Lower body
		c.FillRect(x+1, y+1, 8, 2, cloudShadow)
		// Upper bump
		c.FillRect(x+2, y, 6, 1, cloudCol)
		c.FillRect(x+0, y+1, 10, 1, cloudCol)
	}
}

func drawSun(c *engine.Canvas) {
	cx := 12
	cy := 8
	c.FillCircle(cx, cy, 3, engine.Color{R: 255, G: 232, B: 128, A: 255})
	c.FillCircle(cx-1, cy-1, 2, engine.Color{R: 255, G: 248, B: 180, A: 255})
}

func drawMoon(c *engine.Canvas) {
	cx := c.Width() - 12
	cy := 8
	moon := engine.Color{R: 248, G: 232, B: 160, A: 255}
	bg := engine.Color{R: 28, G: 38, B: 88, A: 255}
	c.FillCircle(cx, cy, 3, moon)
	// Crescent: punch out a slightly-offset circle in the sky color to
	// carve out the dark side of the moon.
	c.FillCircle(cx-2, cy-1, 3, bg)
}

// drawStars sprinkles a fixed set of star pixels with a gentle twinkle.
// The pixel positions are chosen by hand to look "starry" without
// looking patterned.
func drawStars(c *engine.Canvas, t float64) {
	stars := [...][2]int{
		{8, 4}, {15, 9}, {22, 5}, {28, 12}, {35, 6},
		{42, 10}, {49, 4}, {56, 12}, {63, 7}, {70, 11},
		{6, 13}, {19, 14}, {32, 16}, {45, 18}, {58, 15},
		{71, 17}, {4, 18}, {26, 20}, {53, 22},
	}
	bright := engine.Color{R: 252, G: 248, B: 224, A: 255}
	dim := engine.Color{R: 180, G: 180, B: 196, A: 255}
	for i, s := range stars {
		if s[0] >= c.Width() || s[1] >= c.Height()/2 {
			continue
		}
		// Twinkle: about a third of the stars are dimmed each tick.
		phase := math.Sin(t*1.6 + float64(i)*0.7)
		col := bright
		if phase < -0.3 {
			col = dim
		}
		c.Set(s[0], s[1], col)
	}
}

// drawSkyline paints a parallax silhouette of distant buildings/hills
// just above the ground line. The skyline scrolls at roughly half the
// speed of the foreground for a sense of depth.
func drawSkyline(c *engine.Canvas, th theme, scrollOffset float64) {
	w := c.Width()
	h := c.Height()
	groundTop := h - groundHeight
	skylineH := 7
	skylineTop := groundTop - skylineH

	var silhouette engine.Color
	var window engine.Color
	if th == themeNight {
		silhouette = engine.Color{R: 28, G: 28, B: 56, A: 255}
		window = engine.Color{R: 232, G: 196, B: 80, A: 255}
	} else {
		silhouette = engine.Color{R: 92, G: 144, B: 156, A: 255}
		window = engine.Color{R: 132, G: 184, B: 196, A: 255}
	}

	// A repeating pattern of building widths/heights. The "world"
	// repeats every skylinePeriod pixels and we shift it by half the
	// scroll offset for a parallax feel.
	const skylinePeriod = 24
	offset := int(math.Floor(scrollOffset * 0.5))
	for x := 0; x < w; x++ {
		// Stable per-column pattern using a simple hash of (x + offset).
		wx := mod(x+offset, skylinePeriod)
		bldgH := skylineSilhouetteHeight(wx, skylineH)
		if bldgH <= 0 {
			continue
		}
		c.FillRect(x, skylineTop+(skylineH-bldgH), 1, bldgH, silhouette)
		// Window pixels — only on tall buildings, on alternating rows.
		if bldgH >= 4 && wx%4 == 1 {
			for dy := 2; dy < bldgH; dy += 2 {
				c.Set(x, skylineTop+(skylineH-bldgH)+dy, window)
			}
		}
	}
}

// skylineSilhouetteHeight returns the pixel height of the building
// silhouette at horizontal position wx (mod skylinePeriod). The shape
// is a hand-picked sequence designed to read as varied rooftops.
func skylineSilhouetteHeight(wx, maxH int) int {
	// Each entry is the height of one column in the period.
	pattern := [24]int{
		0, 0, 2, 3, 3, 3,
		4, 5, 5, 6, 6, 4,
		3, 3, 2, 0, 2, 4,
		5, 7, 7, 5, 3, 1,
	}
	h := pattern[wx]
	if h > maxH {
		h = maxH
	}
	return h
}

// --- Text helpers ----------------------------------------------------

// drawOutlinedPixelText draws text in the chunky pixel font with a
// 4-direction outline in the shadow color, giving the text strong
// readability against any background. This is how the original
// Flappy Bird draws its score and "GAME OVER" banner.
func drawOutlinedPixelText(c *engine.Canvas, x, y int, text string, fg, shadow engine.Color) {
	offsets := [...][2]int{
		{-1, 0}, {1, 0}, {0, -1}, {0, 1},
		{-1, -1}, {1, -1}, {-1, 1}, {1, 1},
	}
	for _, o := range offsets {
		c.DrawText(x+o[0], y+o[1], text, shadow)
	}
	c.DrawText(x, y, text, fg)
}

// intToStr is a small wrapper around strconv.Itoa — saves callers a
// strconv import in the few spots that just need a quick int → string.
func intToStr(n int) string { return strconv.Itoa(n) }

// --- Medals ----------------------------------------------------------

type medalKind int

const (
	medalNone medalKind = iota
	medalBronzeKind
	medalSilverKind
	medalGoldKind
	medalPlatinumKind
)

func medalTier(score int) medalKind {
	switch {
	case score >= medalPlatinum:
		return medalPlatinumKind
	case score >= medalGold:
		return medalGoldKind
	case score >= medalSilver:
		return medalSilverKind
	case score >= medalBronze:
		return medalBronzeKind
	default:
		return medalNone
	}
}

// drawMedal paints a small medal sprite centered on (cx, cy). The
// design is a filled circle with a slightly darker ring and a "shine"
// pixel — a 7-pixel-wide trinket that reads as a medal even at this
// resolution.
func drawMedal(c *engine.Canvas, cx, cy int, kind medalKind) {
	var main, dark, shine, label engine.Color
	switch kind {
	case medalBronzeKind:
		main = engine.Color{R: 224, G: 130, B: 60, A: 255}
		dark = engine.Color{R: 160, G: 80, B: 30, A: 255}
		shine = engine.Color{R: 252, G: 200, B: 140, A: 255}
		label = engine.Color{R: 80, G: 40, B: 16, A: 255}
	case medalSilverKind:
		main = engine.Color{R: 210, G: 218, B: 232, A: 255}
		dark = engine.Color{R: 144, G: 156, B: 176, A: 255}
		shine = engine.Color{R: 252, G: 252, B: 255, A: 255}
		label = engine.Color{R: 88, G: 96, B: 112, A: 255}
	case medalGoldKind:
		main = engine.Color{R: 248, G: 220, B: 80, A: 255}
		dark = engine.Color{R: 192, G: 152, B: 32, A: 255}
		shine = engine.Color{R: 255, G: 248, B: 200, A: 255}
		label = engine.Color{R: 96, G: 72, B: 0, A: 255}
	case medalPlatinumKind:
		main = engine.Color{R: 180, G: 232, B: 240, A: 255}
		dark = engine.Color{R: 100, G: 156, B: 184, A: 255}
		shine = engine.Color{R: 240, G: 252, B: 255, A: 255}
		label = engine.Color{R: 40, G: 80, B: 112, A: 255}
	default:
		return
	}

	c.FillCircle(cx, cy, 4, dark)
	c.FillCircle(cx, cy, 3, main)
	// Shine highlight — single bright pixel pair on the upper-left.
	c.Set(cx-1, cy-2, shine)
	c.Set(cx-2, cy-1, shine)
	// Star / dot center for identification.
	c.Set(cx, cy, label)
	c.Set(cx-1, cy, label)
	c.Set(cx+1, cy, label)
	c.Set(cx, cy-1, label)
	c.Set(cx, cy+1, label)
}

// drawScorePanel paints the rounded panel behind the end-of-round score
// readout: a tan body with a thin darker outline.
func drawScorePanel(c *engine.Canvas, x, y, w, h int, th theme) {
	body := engine.Color{R: 248, G: 232, B: 196, A: 255}
	outline := engine.Color{R: 120, G: 84, B: 48, A: 255}
	if th == themeNight {
		body = engine.Color{R: 76, G: 88, B: 132, A: 255}
		outline = engine.Color{R: 24, G: 28, B: 60, A: 255}
	}
	c.FillRect(x, y, w, h, body)
	// 1-pixel outline frame.
	c.FillRect(x, y, w, 1, outline)
	c.FillRect(x, y+h-1, w, 1, outline)
	c.FillRect(x, y, 1, h, outline)
	c.FillRect(x+w-1, y, 1, h, outline)
}
