package brickbreaker

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Lengths are in canvas pixels; intervals in seconds.
const (
	paddleHeight = 1
	ballSize     = 2
	initialLives = 3

	ballLostHold   = 0.9
	maxSubstepPx   = 0.5
	maxBounceAngle = 65.0
	clearLifeBonus = 100

	maxActiveBalls = 8

	widePaddleDuration = 12.0
	widePaddleScale    = 1.6
	slowBallDuration   = 10.0
	slowBallScale      = 0.65

	comboPopupLife  = 1.0
	comboPopupDrift = 6.0 // pixels of upward drift per second
)

// playState is the gameplay sub-state machine. The top-level scene only
// distinguishes title vs play; this enum is internal to playScene.
type playState int

const (
	psServe playState = iota
	psPlaying
	psBallLost
	psLevelCleared
	psGameOver
)

// ballEntity is a single ball in play. Each ball tracks its own combo
// counter so a long ricochet run on one ball isn't blown up just
// because a *different* ball touched the paddle.
type ballEntity struct {
	x, y   float64
	vx, vy float64
	combo  int
}

// brick is a single live brick on the playfield.
type brick struct {
	x, y  int
	w, h  int
	kind  brickType
	hp    int
	alive bool
}

// comboPopup is a short-lived floating multiplier label shown over the
// site of a destroyed brick. It drifts upward and fades by being
// removed from the popup slice once its lifetime expires.
type comboPopup struct {
	x, y float64
	text string
	col  engine.Color
	age  float64
}

// playScene is the active-game scene. It owns paddle, balls, bricks,
// power-ups, score, lives, and the playState machine.
type playScene struct {
	e    *engine.Engine
	w, h int

	level      levelDef
	levelIndex int

	paddleX     float64
	paddleW     int
	basePaddleW int
	paddleY     int
	paddleSpeed float64

	balls         []*ballEntity
	ballSpeed     float64
	baseBallSpeed float64

	bricks []brick
	alive  int

	powerUps []*powerUpEntity
	popups   []comboPopup

	wideUntilT float64
	slowUntilT float64

	score   int
	hiScore int
	lives   int

	state  playState
	stateT float64
	timeT  float64 // scene clock for power-up timers

	hudRows  int
	fieldTop int
	floorY   int

	rng *rand.Rand

	wantQuit bool
}

func newPlayScene(e *engine.Engine, levelIdx, hiScore int) *playScene {
	c := e.Canvas()
	lv := levels[levelIdx]
	p := &playScene{
		e:             e,
		w:             c.Width(),
		h:             c.Height(),
		level:         lv,
		levelIndex:    levelIdx,
		hiScore:       hiScore,
		lives:         initialLives,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
		ballSpeed:     lv.ballSpeed,
		baseBallSpeed: lv.ballSpeed,
		paddleW:       lv.paddleWidth,
		basePaddleW:   lv.paddleWidth,
		paddleSpeed:   lv.paddleSpeed,
	}
	p.computeLayout()
	p.buildBricks()
	p.resetForServe()
	return p
}

// computeLayout derives Y bands for the HUD, playfield, and paddle so
// the game scales with the terminal.
func (p *playScene) computeLayout() {
	p.hudRows = 2
	p.fieldTop = p.hudRows * 2
	p.floorY = p.h - 1
	p.paddleY = p.h - 3
}

// buildBricks lays out the level's brick pattern. Slot widths adapt to
// the canvas width so the wall always uses ~the full play area.
func (p *playScene) buildBricks() {
	rows := p.level.rows
	if len(rows) == 0 {
		return
	}
	cols := len(rows[0])

	margin := 2
	gap := 1
	available := p.w - 2*margin - (cols-1)*gap
	bw := available / cols
	if bw < 3 {
		bw = 3
	}
	bh := 2
	totalW := cols*bw + (cols-1)*gap
	startX := (p.w - totalW) / 2
	startY := p.fieldTop + 2

	p.bricks = nil
	for r, row := range rows {
		for c := 0; c < cols && c < len(row); c++ {
			k := brickFromRune(row[c])
			if k == brickEmpty {
				continue
			}
			p.bricks = append(p.bricks, brick{
				x:     startX + c*(bw+gap),
				y:     startY + r*(bh+gap),
				w:     bw,
				h:     bh,
				kind:  k,
				hp:    k.hits(),
				alive: true,
			})
		}
	}
	p.alive = len(p.bricks)
}

// resetForServe recenters the paddle and seats a single stationary ball
// on top of it. Falling power-ups are cleared (they'd be unreachable),
// but persistent timed effects (wide/slow) survive the re-serve as a
// continuity reward.
func (p *playScene) resetForServe() {
	p.paddleX = float64(p.w-p.paddleW) / 2
	p.balls = []*ballEntity{{
		x: p.paddleX + float64(p.paddleW)/2 - float64(ballSize)/2,
		y: float64(p.paddleY - ballSize),
	}}
	p.powerUps = nil
	p.state = psServe
	p.stateT = 0
}

func (p *playScene) launchBall() {
	if len(p.balls) == 0 {
		return
	}
	b := p.balls[0]
	angle := (p.rng.Float64()*60 - 30) * math.Pi / 180.0
	if p.rng.Intn(2) == 0 {
		angle = -angle
	}
	sin, cos := math.Sin(angle), math.Cos(angle)
	b.vx = p.ballSpeed * sin
	b.vy = -p.ballSpeed * cos
	p.state = psPlaying
	p.stateT = 0
}

// Update advances the play scene by dt seconds. Input is drained first
// so newly-pressed keys take effect this frame.
func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s
	p.timeT += s

	p.updatePaddle(s)
	p.updateEffects()

	switch p.state {
	case psServe:
		if len(p.balls) > 0 {
			b := p.balls[0]
			b.x = p.paddleX + float64(p.paddleW)/2 - float64(ballSize)/2
			b.y = float64(p.paddleY - ballSize)
		}
		p.updatePopups(s)
	case psPlaying:
		p.updateBalls(s)
		p.updatePowerUps(s)
		p.updatePopups(s)
	case psBallLost:
		if p.stateT >= ballLostHold {
			if p.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.resetForServe()
			}
		}
	case psLevelCleared, psGameOver:
		p.updatePopups(s)
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	return nil
}

// updateEffects deactivates any timed power-ups whose wall-clock
// threshold has elapsed.
func (p *playScene) updateEffects() {
	if p.wideUntilT > 0 && p.timeT >= p.wideUntilT {
		p.deactivateWide()
		p.wideUntilT = 0
	}
	if p.slowUntilT > 0 && p.timeT >= p.slowUntilT {
		p.deactivateSlow()
		p.slowUntilT = 0
	}
}

func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psServe:
			switch k.Code {
			case engine.KeyEnter:
				p.launchBall()
			case engine.KeyEsc:
				p.wantQuit = true
			case engine.KeyChar:
				switch k.Rune {
				case ' ':
					p.launchBall()
				case 'q', 'Q':
					p.wantQuit = true
				}
			}
		case psPlaying:
			switch k.Code {
			case engine.KeyEsc:
				p.wantQuit = true
			case engine.KeyChar:
				if k.Rune == 'q' || k.Rune == 'Q' {
					p.wantQuit = true
				}
			}
		case psBallLost:
			if k.Code == engine.KeyEsc {
				p.wantQuit = true
			}
		case psLevelCleared:
			switch k.Code {
			case engine.KeyEnter, engine.KeyEsc:
				p.wantQuit = true
			case engine.KeyChar:
				switch k.Rune {
				case ' ', 'q', 'Q':
					p.wantQuit = true
				}
			}
		case psGameOver:
			switch k.Code {
			case engine.KeyEnter, engine.KeyEsc:
				p.wantQuit = true
			case engine.KeyChar:
				switch k.Rune {
				case 'r', 'R':
					p.restart()
				case ' ', 'q', 'Q':
					p.wantQuit = true
				}
			}
		}
	}
}

// restart resets the active level for another attempt — fresh bricks,
// full lives, score zeroed, all effects cleared. Hi-score is preserved.
func (p *playScene) restart() {
	p.lives = initialLives
	p.score = 0
	p.paddleW = p.basePaddleW
	p.ballSpeed = p.baseBallSpeed
	p.wideUntilT = 0
	p.slowUntilT = 0
	p.timeT = 0
	p.popups = nil
	p.powerUps = nil
	p.buildBricks()
	p.resetForServe()
}

func (p *playScene) updatePaddle(s float64) {
	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	dx := 0.0
	if left && !right {
		dx -= p.paddleSpeed * s
	}
	if right && !left {
		dx += p.paddleSpeed * s
	}
	p.paddleX += dx
	p.clampPaddle()
}

func (p *playScene) clampPaddle() {
	if p.paddleX < 0 {
		p.paddleX = 0
	}
	maxX := float64(p.w - p.paddleW)
	if p.paddleX > maxX {
		p.paddleX = maxX
	}
}

// updateBalls advances every active ball, removing balls that fell off
// the floor. The player only loses a life when ALL balls are gone —
// losing one of three balls to the floor isn't a death, just an exit.
func (p *playScene) updateBalls(s float64) {
	for i := 0; i < len(p.balls); {
		b := p.balls[i]
		if !p.advanceBall(b, s) {
			p.balls = append(p.balls[:i], p.balls[i+1:]...)
			continue
		}
		i++
		if p.alive == 0 {
			p.state = psLevelCleared
			p.stateT = 0
			p.score += clearLifeBonus * p.lives
			return
		}
	}
	if len(p.balls) == 0 {
		p.lives--
		p.state = psBallLost
		p.stateT = 0
	}
}

// advanceBall moves one ball through one frame in substeps. Returns
// false if the ball was lost off the bottom.
func (p *playScene) advanceBall(b *ballEntity, s float64) bool {
	speed := math.Hypot(b.vx, b.vy)
	steps := int(math.Ceil(speed * s / maxSubstepPx))
	if steps < 1 {
		steps = 1
	}
	ds := s / float64(steps)
	for i := 0; i < steps; i++ {
		if !p.ballSubstep(b, ds) {
			return false
		}
	}
	return true
}

func (p *playScene) ballSubstep(b *ballEntity, ds float64) bool {
	b.x += b.vx * ds
	b.y += b.vy * ds

	if b.x < 0 {
		b.x = 0
		b.vx = -b.vx
	}
	rightLim := float64(p.w - ballSize)
	if b.x > rightLim {
		b.x = rightLim
		b.vx = -b.vx
	}
	topLim := float64(p.fieldTop)
	if b.y < topLim {
		b.y = topLim
		b.vy = -b.vy
	}

	// Paddle (resets the combo on contact).
	if p.collidePaddle(b) {
		b.combo = 0
	}

	if b.y+float64(ballSize) >= float64(p.floorY) {
		return false
	}

	p.collideBricks(b)
	return true
}

// collidePaddle reflects the ball off the paddle with an angle biased
// by where on the paddle the ball struck — the classic Breakout trick
// that lets the player aim by positioning the paddle. Returns true if
// a hit was resolved.
func (p *playScene) collidePaddle(b *ballEntity) bool {
	if b.vy <= 0 {
		return false
	}
	ballRight := b.x + float64(ballSize)
	ballBottom := b.y + float64(ballSize)
	paddleLeft := p.paddleX
	paddleRight := p.paddleX + float64(p.paddleW)
	paddleTop := float64(p.paddleY)
	paddleBottom := float64(p.paddleY + paddleHeight)

	if ballBottom < paddleTop || b.y > paddleBottom {
		return false
	}
	if ballRight < paddleLeft || b.x > paddleRight {
		return false
	}

	ballCentre := b.x + float64(ballSize)/2
	paddleCentre := p.paddleX + float64(p.paddleW)/2
	rel := (ballCentre - paddleCentre) / (float64(p.paddleW) / 2)
	if rel < -1 {
		rel = -1
	}
	if rel > 1 {
		rel = 1
	}
	angle := rel * (maxBounceAngle * math.Pi / 180.0)
	sin, cos := math.Sin(angle), math.Cos(angle)
	speed := p.ballSpeed
	b.vx = speed * sin
	b.vy = -speed * cos
	b.y = paddleTop - float64(ballSize) - 0.001
	return true
}

// collideBricks finds the first overlapping brick (if any), reflects the
// ball, damages or destroys the brick, and ticks the combo on destroy.
func (p *playScene) collideBricks(b *ballEntity) {
	ballRect := frect{
		x0: b.x,
		y0: b.y,
		x1: b.x + float64(ballSize),
		y1: b.y + float64(ballSize),
	}
	for i := range p.bricks {
		br := &p.bricks[i]
		if !br.alive {
			continue
		}
		brRect := frect{
			x0: float64(br.x),
			y0: float64(br.y),
			x1: float64(br.x + br.w),
			y1: float64(br.y + br.h),
		}
		if !ballRect.overlaps(brRect) {
			continue
		}

		overlapX := math.Min(ballRect.x1-brRect.x0, brRect.x1-ballRect.x0)
		overlapY := math.Min(ballRect.y1-brRect.y0, brRect.y1-ballRect.y0)
		if overlapX < overlapY {
			if b.vx > 0 {
				b.x -= overlapX
			} else {
				b.x += overlapX
			}
			b.vx = -b.vx
		} else {
			if b.vy > 0 {
				b.y -= overlapY
			} else {
				b.y += overlapY
			}
			b.vy = -b.vy
		}

		br.hp--
		if br.hp <= 0 {
			br.alive = false
			p.alive--
			b.combo++
			mult := comboMultiplier(b.combo)
			p.score += br.kind.score() * mult
			p.spawnComboPopup(br, mult)
			p.maybeDropPowerUp(br)
		} else {
			// Partial credit for damaging an armored brick. The combo
			// only ticks on destroys, so a tough brick acts as a tempo
			// pause rather than a free combo extension.
			p.score += 5
		}
		return
	}
}

// comboMultiplier maps a ball's combo counter to a score multiplier.
// Stays at 1× through the first couple of hits then ramps up — short
// combos shouldn't feel "owed" a bonus, but a sustained ricochet run
// should escalate.
func comboMultiplier(combo int) int {
	switch {
	case combo < 3:
		return 1
	case combo < 5:
		return 2
	case combo < 7:
		return 3
	case combo < 10:
		return 4
	default:
		return 5
	}
}

// spawnComboPopup queues a floating "xN" label at the brick site. Only
// emitted once the combo actually grants a multiplier (>= 2×) so the
// popup is genuine reward, not noise.
func (p *playScene) spawnComboPopup(b *brick, mult int) {
	if mult < 2 {
		return
	}
	col := engine.Color{R: 110, G: 240, B: 130, A: 255}
	switch {
	case mult >= 5:
		col = engine.Color{R: 255, G: 90, B: 220, A: 255}
	case mult >= 4:
		col = engine.Color{R: 255, G: 130, B: 90, A: 255}
	case mult >= 3:
		col = engine.Color{R: 255, G: 220, B: 90, A: 255}
	}
	p.popups = append(p.popups, comboPopup{
		x:    float64(b.x + b.w/2),
		y:    float64(b.y),
		text: fmt.Sprintf("x%d", mult),
		col:  col,
	})
}

func (p *playScene) updatePopups(s float64) {
	kept := p.popups[:0]
	for _, pop := range p.popups {
		pop.age += s
		pop.y -= comboPopupDrift * s
		if pop.age < comboPopupLife && pop.y > 0 {
			kept = append(kept, pop)
		}
	}
	p.popups = kept
}

// --- Power-ups -----------------------------------------------------

// maybeDropPowerUp rolls a drop chance based on brick toughness — the
// armored bricks are more likely to drop something, which steers the
// player toward attacking them. Type is then weighted-random.
func (p *playScene) maybeDropPowerUp(b *brick) {
	chance := 0.0
	switch b.kind {
	case brickWeak:
		chance = 0.07
	case brickStrong:
		chance = 0.13
	case brickTough:
		chance = 0.22
	}
	if p.rng.Float64() > chance {
		return
	}
	weights := []struct {
		kind powerType
		w    float64
	}{
		{powerMultiBall, 0.35},
		{powerWidePaddle, 0.30},
		{powerSlowBall, 0.25},
		{powerExtraLife, 0.10},
	}
	var total float64
	for _, e := range weights {
		total += e.w
	}
	r := p.rng.Float64() * total
	kind := weights[0].kind
	for _, e := range weights {
		if r <= e.w {
			kind = e.kind
			break
		}
		r -= e.w
	}
	p.powerUps = append(p.powerUps, &powerUpEntity{
		x:    float64(b.x + (b.w-powerUpW)/2),
		y:    float64(b.y),
		kind: kind,
	})
}

func (p *playScene) updatePowerUps(s float64) {
	kept := p.powerUps[:0]
	paddleRect := frect{
		x0: p.paddleX,
		y0: float64(p.paddleY),
		x1: p.paddleX + float64(p.paddleW),
		y1: float64(p.paddleY + paddleHeight),
	}
	for _, pu := range p.powerUps {
		pu.y += powerUpFallSpeed * s
		pu.bobT += s
		if pu.y > float64(p.floorY) {
			continue
		}
		puRect := frect{
			x0: pu.x,
			y0: pu.y,
			x1: pu.x + float64(powerUpW),
			y1: pu.y + float64(powerUpH),
		}
		if puRect.overlaps(paddleRect) {
			p.activatePowerUp(pu.kind)
			continue
		}
		kept = append(kept, pu)
	}
	p.powerUps = kept
}

func (p *playScene) activatePowerUp(kind powerType) {
	p.score += 25 // small thank-you for catching it
	switch kind {
	case powerMultiBall:
		p.spawnExtraBalls()
	case powerWidePaddle:
		p.activateWide()
	case powerExtraLife:
		if p.lives < 9 {
			p.lives++
		}
	case powerSlowBall:
		p.activateSlow()
	}
}

// spawnExtraBalls splits each currently-active ball into the original
// plus two more at ±25° from its trajectory, capped at maxActiveBalls.
// New balls inherit their parent's combo so a multi-ball mid-run keeps
// the chain going on every spawn.
func (p *playScene) spawnExtraBalls() {
	if len(p.balls) == 0 {
		return
	}
	var extras []*ballEntity
	for _, b := range p.balls {
		if len(p.balls)+len(extras) >= maxActiveBalls {
			break
		}
		for _, deg := range []float64{25, -25} {
			if len(p.balls)+len(extras) >= maxActiveBalls {
				break
			}
			rad := deg * math.Pi / 180.0
			sin, cos := math.Sin(rad), math.Cos(rad)
			newVX := b.vx*cos - b.vy*sin
			newVY := b.vx*sin + b.vy*cos
			// Stationary parent (serve state) — give the spawn a kick
			// so the power-up isn't wasted.
			if newVX == 0 && newVY == 0 {
				newVY = -p.ballSpeed
			}
			extras = append(extras, &ballEntity{
				x:     b.x,
				y:     b.y,
				vx:    newVX,
				vy:    newVY,
				combo: b.combo,
			})
		}
	}
	p.balls = append(p.balls, extras...)
}

func (p *playScene) activateWide() {
	if p.wideUntilT > p.timeT {
		// Already wide — just extend the duration.
		p.wideUntilT = p.timeT + widePaddleDuration
		return
	}
	centre := p.paddleX + float64(p.paddleW)/2
	p.paddleW = int(float64(p.basePaddleW) * widePaddleScale)
	if p.paddleW > p.w-2 {
		p.paddleW = p.w - 2
	}
	p.paddleX = centre - float64(p.paddleW)/2
	p.clampPaddle()
	p.wideUntilT = p.timeT + widePaddleDuration
}

func (p *playScene) deactivateWide() {
	centre := p.paddleX + float64(p.paddleW)/2
	p.paddleW = p.basePaddleW
	p.paddleX = centre - float64(p.paddleW)/2
	p.clampPaddle()
}

func (p *playScene) activateSlow() {
	if p.slowUntilT > p.timeT {
		p.slowUntilT = p.timeT + slowBallDuration
		return
	}
	for _, b := range p.balls {
		b.vx *= slowBallScale
		b.vy *= slowBallScale
	}
	p.ballSpeed = p.baseBallSpeed * slowBallScale
	p.slowUntilT = p.timeT + slowBallDuration
}

func (p *playScene) deactivateSlow() {
	if slowBallScale == 0 {
		return
	}
	inv := 1.0 / slowBallScale
	for _, b := range p.balls {
		b.vx *= inv
		b.vy *= inv
	}
	p.ballSpeed = p.baseBallSpeed
}

// frect is an axis-aligned bounding box in float pixel space.
type frect struct {
	x0, y0, x1, y1 float64
}

func (r frect) overlaps(o frect) bool {
	return r.x0 < o.x1 && r.x1 > o.x0 && r.y0 < o.y1 && r.y1 > o.y0
}

// --- Drawing ----------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 6, G: 5, B: 18, A: 255})

	p.drawHUD(c)
	p.drawBricks(c)
	p.drawPowerUps(c)
	p.drawPaddle(c)
	p.drawBalls(c)
	p.drawPopups(c)
	p.drawFloor(c)

	switch p.state {
	case psServe:
		if int(p.stateT*2)%2 == 0 {
			hint := "PRESS SPACE TO LAUNCH"
			c.Print((c.Cols()-len(hint))/2, c.Rows()-5, hint, engine.Yellow)
		}
	case psBallLost:
		msg := "BALL LOST"
		c.Print((c.Cols()-len(msg))/2, c.Rows()/2, msg,
			engine.Color{R: 240, G: 90, B: 90, A: 255})
	case psLevelCleared:
		drawCentreBanner(c, "LEVEL CLEARED", engine.Color{R: 110, G: 240, B: 130, A: 255})
		hint := "ENTER RETURN TO MENU"
		c.Print((c.Cols()-len(hint))/2, c.Rows()/2+3, hint, engine.White)
	case psGameOver:
		drawCentreBanner(c, "GAME OVER", engine.Color{R: 255, G: 80, B: 80, A: 255})
		hint := "R PLAY AGAIN    ENTER MENU"
		c.Print((c.Cols()-len(hint))/2, c.Rows()/2+3, hint, engine.White)
	}
}

func (p *playScene) drawHUD(c *engine.Canvas) {
	cols := c.Cols()
	scoreText := fmt.Sprintf("SCORE %05d", p.score)
	hiText := fmt.Sprintf("HI %05d", p.hiScore)
	levelText := fmt.Sprintf("LEVEL %d", p.levelIndex+1)
	livesText := fmt.Sprintf("LIVES %d", p.lives)

	c.Print(1, 0, scoreText, engine.White)
	mid := (cols - len(hiText)) / 2
	if mid < len(scoreText)+2 {
		mid = len(scoreText) + 2
	}
	c.Print(mid, 0, hiText, engine.Yellow)
	rightCol := cols - len(levelText) - 1
	if rightCol < mid+len(hiText)+2 {
		rightCol = mid + len(hiText) + 2
	}
	c.Print(rightCol, 0, levelText, engine.Cyan)

	c.Print(1, 1, livesText, engine.Color{R: 130, G: 240, B: 150, A: 255})

	// Right side of HUD row 1: active timed effects and ball count.
	var effects []string
	if p.wideUntilT > p.timeT {
		effects = append(effects,
			fmt.Sprintf("WIDE %ds", int(math.Ceil(p.wideUntilT-p.timeT))))
	}
	if p.slowUntilT > p.timeT {
		effects = append(effects,
			fmt.Sprintf("SLOW %ds", int(math.Ceil(p.slowUntilT-p.timeT))))
	}
	if len(p.balls) > 1 {
		effects = append(effects, fmt.Sprintf("x%d BALLS", len(p.balls)))
	}
	if len(effects) > 0 {
		text := strings.Join(effects, "  ")
		col := engine.Color{R: 220, G: 160, B: 255, A: 255}
		if pos := cols - len(text) - 1; pos > len(livesText)+3 {
			c.Print(pos, 1, text, col)
		}
	} else {
		name := p.level.name
		nameCol := cols - len(name) - 1
		if nameCol > len(livesText)+3 {
			c.Print(nameCol, 1, name, engine.Color{R: 220, G: 160, B: 255, A: 255})
		}
	}
}

func (p *playScene) drawBricks(c *engine.Canvas) {
	for _, b := range p.bricks {
		if !b.alive {
			continue
		}
		col := b.kind.color(b.hp)
		c.FillRect(b.x, b.y, b.w, b.h, col)
		hl := engine.Color{
			R: clampLighter(col.R, 60),
			G: clampLighter(col.G, 60),
			B: clampLighter(col.B, 60),
			A: 255,
		}
		c.FillRect(b.x, b.y, b.w, 1, hl)
	}
}

func (p *playScene) drawPaddle(c *engine.Canvas) {
	if p.state == psGameOver {
		return
	}
	body := engine.Color{R: 235, G: 235, B: 245, A: 255}
	if p.wideUntilT > p.timeT {
		// Cyan tint for the duration of the wide effect — same hue as
		// the wide power-up capsule.
		body = engine.Color{R: 120, G: 220, B: 255, A: 255}
	}
	c.FillRect(int(p.paddleX), p.paddleY, p.paddleW, paddleHeight, body)
	tip := engine.Color{R: 200, G: 180, B: 255, A: 255}
	c.Set(int(p.paddleX), p.paddleY, tip)
	c.Set(int(p.paddleX)+p.paddleW-1, p.paddleY, tip)
}

func (p *playScene) drawBalls(c *engine.Canvas) {
	if p.state == psBallLost || p.state == psGameOver {
		return
	}
	base := engine.Color{R: 255, G: 220, B: 100, A: 255}
	if p.slowUntilT > p.timeT {
		// Cooled-down hue while slow is active.
		base = engine.Color{R: 180, G: 200, B: 255, A: 255}
	}
	for _, b := range p.balls {
		c.FillRect(int(b.x), int(b.y), ballSize, ballSize, base)
	}
}

func (p *playScene) drawPowerUps(c *engine.Canvas) {
	for _, pu := range p.powerUps {
		col := pu.kind.color()
		x := int(pu.x)
		y := int(pu.y)
		c.FillRect(x, y, powerUpW, powerUpH, col)
		// Darker bottom strip for a chiselled-capsule look.
		dim := engine.Color{
			R: uint8(int(col.R) * 2 / 3),
			G: uint8(int(col.G) * 2 / 3),
			B: uint8(int(col.B) * 2 / 3),
			A: 255,
		}
		c.FillRect(x, y+powerUpH-1, powerUpW, 1, dim)
		// Centre the glyph in the top cell.
		row := y / 2
		col2 := x + powerUpW/2 - 1
		c.Print(col2, row, pu.kind.label(), engine.Black)
	}
}

func (p *playScene) drawPopups(c *engine.Canvas) {
	for _, pop := range p.popups {
		frac := pop.age / comboPopupLife
		if frac > 1 {
			frac = 1
		}
		dim := 1.0 - frac*0.6
		faded := engine.Color{
			R: uint8(float64(pop.col.R) * dim),
			G: uint8(float64(pop.col.G) * dim),
			B: uint8(float64(pop.col.B) * dim),
			A: 255,
		}
		cellX := int(pop.x) - len(pop.text)/2
		if cellX < 0 {
			cellX = 0
		}
		row := int(pop.y) / 2
		if row < 0 {
			row = 0
		}
		c.Print(cellX, row, pop.text, faded)
	}
}

func (p *playScene) drawFloor(c *engine.Canvas) {
	c.FillRect(0, p.floorY, p.w, 1, engine.Color{R: 60, G: 40, B: 90, A: 255})
}

func drawCentreBanner(c *engine.Canvas, text string, col engine.Color) {
	w := engine.TextWidth(text)
	x := (c.Width() - w) / 2
	y := (c.Height() - engine.FontHeight) / 2
	c.FillRect(x-3, y-2, w+6, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 20, A: 255})
	c.DrawText(x, y, text, col)
}
