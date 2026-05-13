package flappybird

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Lengths are in canvas pixels (one pixel = half a
// terminal cell vertically); velocities are pixels/second; intervals are
// seconds.
const (
	// Physics — chosen to feel roughly like the original at 60 FPS on an
	// 80×48 auto-sized canvas: gravity strong enough that a missed flap
	// drops you into a pipe, flap velocity strong enough to clear a pipe
	// cap with a single tap.
	gravity      = 90.0
	flapVelocity = -35.0
	maxFallSpeed = 65.0

	// Bird sprite — 7 wide × 5 tall pixels. The bird body is 5 wide; the
	// beak adds two columns on the right.
	birdSpriteWidth  = 7
	birdSpriteHeight = 5
	// Hitbox is 1 pixel inside the sprite on all sides so a near-miss
	// past a pipe lip doesn't kill you — small fairness margin matching
	// the original's forgiving collisions.
	birdHitboxInset = 1

	// Pipes.
	pipeWidth      = 8       // body width in pixels
	pipeCapWidth   = 10      // cap width (1 px overhang each side)
	pipeCapHeight  = 2       // cap height
	pipeSpeed      = 22.0    // scroll velocity in pixels/sec
	pipeSpacing    = 30      // distance between consecutive pipe spawns
	pipeGap        = 17      // vertical gap between top and bottom pipes
	pipeGapMargin  = 5       // min pixels of pipe body on either side of gap

	// Ground.
	groundHeight = 6

	// Animation.
	deathFlashTime = 0.35 // how long the bird flashes white on death
	bobAmplitude   = 1.5
	bobFrequency   = 1.2
	wingPeriod     = 0.13  // seconds per wing frame in the 3-frame cycle

	// Score thresholds for end-of-round medals.
	medalBronze   = 10
	medalSilver   = 20
	medalGold     = 30
	medalPlatinum = 40
)

// playState is the gameplay sub-state machine. psReady is the "Get Ready"
// screen (bird bobs, no pipes yet, waiting on first flap). psPlaying is
// the active run. psDying covers the flash-and-fall after a collision.
// psGameOver freezes the world and shows the score panel.
type playState int

const (
	psReady playState = iota
	psPlaying
	psDying
	psGameOver
)

// pipe is a single top/bottom pipe pair. gapY is the pixel-row of the
// top of the gap; the gap is always pipeGap pixels tall. scored flips
// true the frame the bird's left edge passes the pipe's right edge so
// each pipe scores exactly once.
type pipe struct {
	x      float64
	gapY   int
	scored bool
}

// playScene owns the round state. It's constructed fresh for every run
// (newPlayScene), so any cross-round state — currently just the rolling
// hi-score — has to be passed in by the caller.
type playScene struct {
	e    *engine.Engine
	w, h int

	state  playState
	stateT float64 // seconds in the current state
	timeT  float64 // wall clock for scene-wide animations

	// Bird kinematics. birdX is fixed; birdY is the top-left of the
	// sprite as a float so sub-pixel motion accumulates correctly.
	birdX  int
	birdY  float64
	birdVY float64

	pipes        []pipe
	pipeSpawnIn  float64 // pixels of scroll remaining until the next spawn
	scrollOffset float64 // total scrolled pixels, used by the ground for parallax

	score   int
	hiScore int
	newHi   bool

	fieldTop    int // first playable pixel-row (top of canvas)
	fieldBottom int // first non-playable row (= top of ground)

	theme   theme
	variant birdVariant

	rng *rand.Rand

	// wantQuit is the signal back to the top-level scene that the player
	// asked to leave this round (ESC, Q, etc.). The top-level scene reads
	// it after Update to bounce back to the title menu.
	wantQuit bool
}

func newPlayScene(e *engine.Engine, hiScore int, th theme, variant birdVariant) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		hiScore: hiScore,
		theme:   th,
		variant: variant,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.computeLayout()
	p.resetForReady()
	return p
}

// computeLayout derives the playable field based on canvas size.
// Everything from fieldTop to fieldBottom is fair game for the bird and
// pipes; the ground occupies the strip from fieldBottom to h.
func (p *playScene) computeLayout() {
	p.fieldTop = 0
	p.fieldBottom = p.h - groundHeight
	// Bird sits ~1/3 of the way across the screen — far enough left that
	// the player has reaction time on incoming pipes, far enough right
	// that the bird isn't crammed against the wall.
	p.birdX = p.w / 3
	if p.birdX < 6 {
		p.birdX = 6
	}
}

// resetForReady puts the world back into the "Get Ready" state: bird
// centered, no pipes, no score. Hi-score is preserved.
func (p *playScene) resetForReady() {
	p.score = 0
	p.newHi = false
	p.pipes = nil
	p.pipeSpawnIn = pipeSpacing * 1.5 // small delay before the first pipe
	p.scrollOffset = 0
	p.birdY = float64(p.fieldTop+p.fieldBottom)/2 - float64(birdSpriteHeight)/2
	p.birdVY = 0
	p.state = psReady
	p.stateT = 0
}

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s
	p.timeT += s

	switch p.state {
	case psReady:
		p.updateReady(s)
	case psPlaying:
		p.updatePlaying(s)
	case psDying:
		p.updateDying(s)
	case psGameOver:
		// Frozen — player must press a key.
	}
	return nil
}

func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		// ESC and Q always unwind to the title, regardless of state.
		if k.Code == engine.KeyEsc {
			p.wantQuit = true
			return
		}
		if k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q') {
			p.wantQuit = true
			return
		}
		switch p.state {
		case psReady:
			if isFlap(k) {
				p.flap()
				p.state = psPlaying
				p.stateT = 0
			}
		case psPlaying:
			if isFlap(k) {
				p.flap()
			}
		case psDying:
			// Ignore input — let the fall animation play out.
		case psGameOver:
			if k.Code == engine.KeyChar && (k.Rune == 'r' || k.Rune == 'R') {
				p.resetForReady()
				continue
			}
			if isFlap(k) || k.Code == engine.KeyEnter {
				p.resetForReady()
			}
		}
	}
}

// isFlap returns true if the key event should trigger a flap. We accept
// space, up arrow, W, K, and enter — covering the standard tap-button,
// vi keys, WASD up, and "confirm" for muscle memory across games.
func isFlap(k engine.Key) bool {
	switch k.Code {
	case engine.KeyUp, engine.KeyEnter:
		return true
	case engine.KeyChar:
		switch k.Rune {
		case ' ', 'w', 'W', 'k', 'K':
			return true
		}
	}
	return false
}

func (p *playScene) flap() {
	p.birdVY = flapVelocity
}

// updateReady runs the bird's gentle bob and keeps everything else
// frozen. The bird does NOT respond to gravity in this state — the run
// begins on the first flap.
func (p *playScene) updateReady(_ float64) {
	centerY := float64(p.fieldTop+p.fieldBottom)/2 - float64(birdSpriteHeight)/2
	p.birdY = centerY + bobAmplitude*math.Sin(p.timeT*bobFrequency*2*math.Pi)
}

func (p *playScene) updatePlaying(s float64) {
	// Bird physics: gravity, clamped fall velocity, simple Euler step.
	p.birdVY += gravity * s
	if p.birdVY > maxFallSpeed {
		p.birdVY = maxFallSpeed
	}
	p.birdY += p.birdVY * s

	// Scroll world and pipes.
	p.scrollOffset += pipeSpeed * s
	p.pipeSpawnIn -= pipeSpeed * s
	for p.pipeSpawnIn <= 0 {
		p.spawnPipe()
		p.pipeSpawnIn += pipeSpacing
	}
	for i := range p.pipes {
		p.pipes[i].x -= pipeSpeed * s
	}

	// Garbage-collect pipes that have scrolled fully off-screen.
	kept := p.pipes[:0]
	for _, pp := range p.pipes {
		if pp.x+pipeCapWidth/2+pipeWidth >= 0 {
			kept = append(kept, pp)
		}
	}
	p.pipes = kept

	// Score: a pipe is "passed" when its right edge crosses the bird's
	// left edge.
	for i := range p.pipes {
		if p.pipes[i].scored {
			continue
		}
		pipeRight := p.pipes[i].x + pipeWidth
		if pipeRight < float64(p.birdX) {
			p.pipes[i].scored = true
			p.score++
			if p.score > p.hiScore {
				p.hiScore = p.score
				p.newHi = true
			}
		}
	}

	// Collisions: ground first (most common death), then pipes.
	if p.birdY+birdSpriteHeight >= float64(p.fieldBottom) {
		p.birdY = float64(p.fieldBottom) - birdSpriteHeight
		p.die()
		return
	}
	if p.collide() {
		p.die()
		return
	}
	// Soft ceiling — original doesn't kill on hitting the top, just halts
	// upward motion. Bird can sit at y=0 indefinitely.
	if p.birdY < 0 {
		p.birdY = 0
		if p.birdVY < 0 {
			p.birdVY = 0
		}
	}
}

// updateDying runs the post-collision animation: bird continues to fall,
// pipes freeze in place. After the bird touches the ground (or a short
// dwell if it died while already on the ground), we transition to game
// over and persist the hi-score.
func (p *playScene) updateDying(s float64) {
	p.birdVY += gravity * s
	if p.birdVY > maxFallSpeed {
		p.birdVY = maxFallSpeed
	}
	p.birdY += p.birdVY * s

	groundY := float64(p.fieldBottom) - birdSpriteHeight
	if p.birdY >= groundY {
		p.birdY = groundY
		p.birdVY = 0
		// Hold on the ground briefly past the flash so the player has a
		// moment to register the death before the game-over panel pops.
		if p.stateT > deathFlashTime+0.25 {
			p.state = psGameOver
			p.stateT = 0
			saveHiScore(p.hiScore)
		}
	}
}

// spawnPipe queues a new pipe pair at the right edge of the screen with
// a random gap position. The gap is constrained so neither pipe is
// thinner than pipeGapMargin pixels, giving the player a fair target.
func (p *playScene) spawnPipe() {
	minY := pipeGapMargin
	maxY := p.fieldBottom - pipeGap - pipeGapMargin
	if maxY < minY {
		maxY = minY
	}
	gapY := minY + p.rng.Intn(maxY-minY+1)
	p.pipes = append(p.pipes, pipe{
		x:    float64(p.w),
		gapY: gapY,
	})
}

// die transitions from psPlaying to psDying. Idempotent so multiple
// collision sources in the same frame don't double-trigger.
func (p *playScene) die() {
	if p.state != psPlaying {
		return
	}
	p.state = psDying
	p.stateT = 0
	// Death gives the bird a small upward kick like the original "ouch"
	// bounce before the fall.
	if p.birdVY > 0 {
		p.birdVY = -15
	}
}

// collide reports whether the bird's hitbox overlaps any non-gap area of
// any pipe. Pipe bodies and (wider) caps are checked separately so the
// 1-pixel cap overhang counts for collision.
func (p *playScene) collide() bool {
	bx0 := p.birdX + birdHitboxInset
	by0 := int(p.birdY) + birdHitboxInset
	bx1 := p.birdX + birdSpriteWidth - birdHitboxInset
	by1 := int(p.birdY) + birdSpriteHeight - birdHitboxInset
	for _, pp := range p.pipes {
		bodyX0 := int(pp.x)
		bodyX1 := bodyX0 + pipeWidth
		capOverhang := (pipeCapWidth - pipeWidth) / 2
		capX0 := bodyX0 - capOverhang
		capX1 := bodyX1 + capOverhang

		topCapY0 := pp.gapY - pipeCapHeight
		topCapY1 := pp.gapY
		botCapY0 := pp.gapY + pipeGap
		botCapY1 := botCapY0 + pipeCapHeight

		// Top pipe body (above top cap).
		if rectOverlap(bx0, by0, bx1, by1, bodyX0, 0, bodyX1, topCapY0) {
			return true
		}
		// Top cap.
		if rectOverlap(bx0, by0, bx1, by1, capX0, topCapY0, capX1, topCapY1) {
			return true
		}
		// Bottom cap.
		if rectOverlap(bx0, by0, bx1, by1, capX0, botCapY0, capX1, botCapY1) {
			return true
		}
		// Bottom pipe body (below bottom cap).
		if rectOverlap(bx0, by0, bx1, by1, bodyX0, botCapY1, bodyX1, p.fieldBottom) {
			return true
		}
	}
	return false
}

// rectOverlap is an AABB intersection test on inclusive-exclusive
// half-open ranges [x0,x1) × [y0,y1).
func rectOverlap(ax0, ay0, ax1, ay1, bx0, by0, bx1, by1 int) bool {
	return ax0 < bx1 && ax1 > bx0 && ay0 < by1 && ay1 > by0
}

// birdTilt maps the bird's current vertical velocity to a sprite tilt:
// -1 (head up) when climbing, +1 (head down) when falling fast, 0
// otherwise. The thresholds are tuned so a fresh flap snaps to the
// "up" pose for a single frame, then settles back as the velocity
// decays into the level band.
func (p *playScene) birdTilt() int {
	switch {
	case p.birdVY < -8:
		return -1
	case p.birdVY > 20:
		return 1
	default:
		return 0
	}
}

// --- Rendering --------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	drawSkyBackground(c, p.theme, p.timeT)
	drawSkyline(c, p.theme, p.scrollOffset)

	// Pipes between skyline and ground so the ground draws on top, hiding
	// any pipe pixels that try to dip below the gap-bottom into the
	// ground.
	p.drawPipes(c)
	drawGround(c, p.w, p.fieldBottom, groundHeight, p.theme, p.scrollOffset)

	// Bird flash effect: white-tinted for first deathFlashTime seconds of
	// psDying.
	flash := p.state == psDying && p.stateT < deathFlashTime
	wing := 1
	switch p.state {
	case psReady, psPlaying:
		wing = int(p.timeT/wingPeriod) % 3
	case psDying:
		// While falling, freeze the wing in the bottom position — bird
		// looks limp rather than mid-flap.
		wing = 2
	case psGameOver:
		wing = 1
	}
	drawBird(c, p.birdX, int(p.birdY), p.birdTilt(), wing, p.variant, flash)

	p.drawHUD(c)

	switch p.state {
	case psReady:
		p.drawReadyOverlay(c)
	case psGameOver:
		p.drawGameOverPanel(c)
	}
}

func (p *playScene) drawPipes(c *engine.Canvas) {
	pal := pipePaletteFor(p.theme)
	for _, pp := range p.pipes {
		bodyX := int(pp.x)
		capOverhang := (pipeCapWidth - pipeWidth) / 2
		capX := bodyX - capOverhang

		// Top pipe — body above its cap, cap flush with gap top.
		topBodyH := pp.gapY - pipeCapHeight
		drawPipeBody(c, bodyX, 0, pipeWidth, topBodyH, pal)
		drawPipeCap(c, capX, pp.gapY-pipeCapHeight, pipeCapWidth, pipeCapHeight, pal, true)

		// Bottom pipe — cap flush with gap bottom, body below.
		botCapY := pp.gapY + pipeGap
		drawPipeCap(c, capX, botCapY, pipeCapWidth, pipeCapHeight, pal, false)
		drawPipeBody(c, bodyX, botCapY+pipeCapHeight, pipeWidth,
			p.fieldBottom-(botCapY+pipeCapHeight), pal)
	}
}

// drawHUD renders the score during play. The original shows the score
// large at the top center; we use the chunky pixel font with a 1-pixel
// outline for the same readable-against-anything effect.
func (p *playScene) drawHUD(c *engine.Canvas) {
	if p.state == psReady || p.state == psGameOver {
		return
	}
	text := fmt.Sprintf("%d", p.score)
	tw := engine.TextWidth(text)
	tx := (p.w - tw) / 2
	ty := 3
	drawOutlinedPixelText(c, tx, ty, text, engine.White,
		engine.Color{R: 30, G: 30, B: 30, A: 255})
}

func (p *playScene) drawReadyOverlay(c *engine.Canvas) {
	title := "GET READY"
	tw := engine.TextWidth(title)
	tx := (p.w - tw) / 2
	ty := p.h/3 - engine.FontHeight/2
	drawOutlinedPixelText(c, tx, ty, title,
		engine.Color{R: 255, G: 220, B: 80, A: 255},
		engine.Color{R: 60, G: 40, B: 0, A: 255})

	if int(p.timeT*2)%2 == 0 {
		hint := "PRESS SPACE TO FLAP"
		c.Print((c.Cols()-len(hint))/2, c.Rows()*3/4, hint, engine.White)
	}
	quit := "ESC QUIT"
	c.Print((c.Cols()-len(quit))/2, c.Rows()-2, quit, engine.Gray)
}

// drawGameOverPanel paints the iconic end-of-round score plate: GAME
// OVER banner, final score, best score, and a medal badge if the player
// cleared a tier threshold. Layout adapts to the canvas — on tight
// terminals the medal sits beside the score column rather than separate.
func (p *playScene) drawGameOverPanel(c *engine.Canvas) {
	// "GAME OVER" wordmark.
	banner := "GAME OVER"
	bw := engine.TextWidth(banner)
	bx := (p.w - bw) / 2
	by := p.h/4 - engine.FontHeight/2
	drawOutlinedPixelText(c, bx, by, banner,
		engine.Color{R: 255, G: 200, B: 80, A: 255},
		engine.Color{R: 60, G: 30, B: 0, A: 255})

	// Score plate background. Tight panel — labels + values on shared
	// rows so the panel doesn't look cavernous around two tiny numbers.
	// panelH stays a multiple of 4 so panelY lands on an even pixel
	// (each terminal cell holds two stacked pixels — a non-even Y would
	// chop the outline into half-cells).
	panelW := 36
	if panelW > p.w-4 {
		panelW = p.w - 4
	}
	panelH := 12
	panelX := (p.w - panelW) / 2
	panelY := p.h/2 - panelH/2 + 2
	panelY -= panelY % 2
	drawScorePanel(c, panelX, panelY, panelW, panelH, p.theme)

	// Theme-aware text colors. The day panel is light cream and needs
	// dark text; the night panel is dark navy and needs light text.
	var labelClr, valueClr, accentClr engine.Color
	if p.theme == themeNight {
		labelClr = engine.Color{R: 248, G: 228, B: 160, A: 255}
		valueClr = engine.Color{R: 255, G: 255, B: 255, A: 255}
		accentClr = engine.Color{R: 255, G: 120, B: 80, A: 255}
	} else {
		labelClr = engine.Color{R: 92, G: 56, B: 24, A: 255}
		valueClr = engine.Color{R: 56, G: 32, B: 12, A: 255}
		accentClr = engine.Color{R: 200, G: 40, B: 24, A: 255}
	}

	// Medal — left side of the plate, only if the score earned one. If
	// there's no medal, the text gets centered across the full panel
	// width instead of getting marooned on the right.
	tier := medalTier(p.score)
	hasMedal := tier != medalNone
	contentLeftBound := panelX + 2
	if hasMedal {
		drawMedal(c, panelX+7, panelY+panelH/2, tier)
		contentLeftBound = panelX + 13
	}
	contentRightBound := panelX + panelW - 2

	// Layout: "SCORE  <value>" on one row, "BEST   <value>" on the next,
	// with values right-aligned so the digit columns line up vertically
	// even when score and best differ in width. The whole block is
	// centered within the content area.
	scoreText := fmt.Sprintf("%d", p.score)
	bestText := fmt.Sprintf("%d", p.hiScore)
	valueW := len(scoreText)
	if len(bestText) > valueW {
		valueW = len(bestText)
	}
	const labelW = 5 // "SCORE" — BEST is shorter, but we use the bigger width
	const gap = 2
	blockW := labelW + gap + valueW
	contentLeft := (contentLeftBound + contentRightBound - blockW) / 2
	if contentLeft < contentLeftBound {
		contentLeft = contentLeftBound
	}
	valueRight := contentLeft + blockW - 1

	scoreRow := panelY/2 + 1
	bestRow := scoreRow + 2

	c.Print(contentLeft, scoreRow, "SCORE", labelClr)
	c.Print(valueRight-len(scoreText)+1, scoreRow, scoreText, valueClr)

	c.Print(contentLeft, bestRow, "BEST", labelClr)
	c.Print(valueRight-len(bestText)+1, bestRow, bestText, valueClr)

	if p.newHi {
		// NEW! sits on the row between SCORE and BEST, in the gap that
		// would otherwise be empty, so it reads as a flag on the BEST
		// line without overlapping the number.
		c.Print(contentLeft+labelW+gap-3, scoreRow+1, "NEW!", accentClr)
	}

	// Restart hint. Placed above the ground so it doesn't camouflage
	// against grass; backed by a dark strip so it's always readable
	// regardless of theme or what scrolled behind it.
	hint := "SPACE PLAY AGAIN    ESC MENU"
	hintCol := (c.Cols() - len(hint)) / 2
	hintRow := c.Rows() - 6
	if hintRow < 0 {
		hintRow = 0
	}
	if int(p.stateT*2)%2 == 0 {
		c.FillRect(hintCol-2, hintRow*2, len(hint)+4, 2,
			engine.Color{R: 16, G: 16, B: 32, A: 255})
		c.Print(hintCol, hintRow, hint,
			engine.Color{R: 255, G: 230, B: 96, A: 255})
	}
}
