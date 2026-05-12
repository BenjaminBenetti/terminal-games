package pong

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Pixel coordinates; pixels-per-second velocities.
const (
	paddleWidth   = 2
	paddleHeight  = 8
	paddleSpeed   = 60.0
	paddleMargin  = 3 // distance from the side wall to the paddle face

	ballSize       = 2
	ballBaseSpeed  = 54.0  // initial total speed at serve
	ballMaxSpeed   = 135.0 // cap so frame-rate-bound physics stays sane
	ballSpeedup    = 1.06 // multiplier applied on each paddle bounce
	ballMaxAngle   = 60.0 * math.Pi / 180.0 // steepest deflection off paddle edge

	cpuTrackSpeed  = 48.0 // slower than the player paddle for fightability
	cpuDeadZone    = 1.0  // pixels of slop so the AI doesn't jitter on alignment

	serveDelay     = 0.9 // seconds between point and next serve
	matchPoint     = 7   // first to this many points wins

	scoreTopMargin = 2  // pixel rows above the score wordmark
	scoreBotMargin = 3  // pixel rows below before the field starts
)

// playState is the gameplay sub-state machine.
type playState int

const (
	psServing playState = iota // ball frozen in centre, waiting on a timer
	psPlaying
	psMatchOver
)

// side identifies a paddle / scorer.
type side int

const (
	sideLeft side = iota
	sideRight
)

func (s side) other() side {
	if s == sideLeft {
		return sideRight
	}
	return sideLeft
}

// paddle is a single bat. y is the top-edge pixel; x is fixed at construction.
type paddle struct {
	x      int
	y      float64
	height int
	width  int
}

func (p *paddle) center() float64 {
	return p.y + float64(p.height)/2
}

// clampY keeps the paddle wholly within [topY, bottomY).
func (p *paddle) clampY(topY, bottomY int) {
	if p.y < float64(topY) {
		p.y = float64(topY)
	}
	maxY := float64(bottomY - p.height)
	if p.y > maxY {
		p.y = maxY
	}
}

// ball is the moving square.
type ball struct {
	x, y   float64
	vx, vy float64
	size   int
}

// playScene contains the full match state.
type playScene struct {
	e    *engine.Engine
	w, h int
	mode gameMode

	left, right paddle
	ball        ball

	leftScore, rightScore int
	state                 playState
	stateT                float64
	serveTo               side // who the next serve favours (loser of last point)
	winner                side

	rng *rand.Rand

	// Field bounds, derived from canvas size.
	fieldTop    int
	fieldBottom int

	// Quit signal — top-level scene reads this to drop back to the title.
	wantQuit bool
}

func newPlayScene(e *engine.Engine, mode gameMode) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:    e,
		w:    c.Width(),
		h:    c.Height(),
		mode: mode,
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.computeLayout()
	p.resetPaddles()
	// Random first serve so the game doesn't feel scripted.
	p.serveTo = sideLeft
	if p.rng.Intn(2) == 0 {
		p.serveTo = sideRight
	}
	p.beginServe()
	return p
}

// computeLayout sets the playable field bounds based on canvas size.
func (p *playScene) computeLayout() {
	// Reserve enough vertical space for the big pixel-font score plus
	// some padding. fieldTop is the first pixel row balls and paddles
	// can occupy.
	p.fieldTop = scoreTopMargin + engine.FontHeight + scoreBotMargin
	p.fieldBottom = p.h
	// Guarantee at least enough room for a paddle plus a bit of slack.
	if p.fieldBottom-p.fieldTop < paddleHeight+4 {
		p.fieldTop = p.fieldBottom - (paddleHeight + 4)
		if p.fieldTop < 0 {
			p.fieldTop = 0
		}
	}
}

func (p *playScene) resetPaddles() {
	mid := float64(p.fieldTop+p.fieldBottom)/2 - float64(paddleHeight)/2
	p.left = paddle{x: paddleMargin, y: mid, height: paddleHeight, width: paddleWidth}
	p.right = paddle{x: p.w - paddleMargin - paddleWidth, y: mid, height: paddleHeight, width: paddleWidth}
}

// beginServe parks the ball in the centre and starts the serve countdown.
// It does not change the serve direction — call this after setting
// p.serveTo.
func (p *playScene) beginServe() {
	p.state = psServing
	p.stateT = 0
	p.ball = ball{
		x:    float64(p.w)/2 - float64(ballSize)/2,
		y:    float64(p.fieldTop+p.fieldBottom)/2 - float64(ballSize)/2,
		size: ballSize,
		vx:   0,
		vy:   0,
	}
}

// launchBall picks the initial velocity for a serve. The horizontal
// direction is toward serveTo; the vertical component is randomised
// within a moderate cone so the receiver doesn't always get a flat ball.
func (p *playScene) launchBall() {
	dirX := -1.0
	if p.serveTo == sideRight {
		dirX = 1.0
	}
	// Random angle within ±35° of horizontal so the serve isn't trivial
	// but also isn't a near-vertical bullet.
	angle := (p.rng.Float64()*2 - 1) * (35.0 * math.Pi / 180.0)
	p.ball.vx = dirX * ballBaseSpeed * math.Cos(angle)
	p.ball.vy = ballBaseSpeed * math.Sin(angle)
}

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}

	s := dt.Seconds()
	p.stateT += s

	switch p.state {
	case psServing:
		p.updatePaddles(s)
		if p.stateT >= serveDelay {
			p.launchBall()
			p.state = psPlaying
			p.stateT = 0
		}
	case psPlaying:
		p.updatePaddles(s)
		p.updateBall(s)
	case psMatchOver:
		// Wait for the player to acknowledge with Enter / ESC.
	}
	return nil
}

func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psServing, psPlaying:
			if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
			}
		case psMatchOver:
			switch k.Code {
			case engine.KeyEnter:
				p.restartMatch()
			case engine.KeyEsc:
				p.wantQuit = true
			case engine.KeyChar:
				switch k.Rune {
				case 'q', 'Q':
					p.wantQuit = true
				case 'r', 'R', ' ':
					p.restartMatch()
				}
			}
		}
	}
}

func (p *playScene) restartMatch() {
	p.leftScore = 0
	p.rightScore = 0
	p.resetPaddles()
	// Loser of last match serves first — but since we just reset, flip
	// a coin for fairness.
	p.serveTo = sideLeft
	if p.rng.Intn(2) == 0 {
		p.serveTo = sideRight
	}
	p.beginServe()
}

// updatePaddles reads input (or runs the AI) and moves each paddle.
// Movement is held-key state, so we poll IsKeyDown / IsCharDown directly
// instead of relying on the discrete event queue. This is what lets a
// player hold a direction without depending on auto-repeat.
func (p *playScene) updatePaddles(s float64) {
	// Left paddle: W/S always controls it. In single-player mode we also
	// accept Up/Down for the same paddle so muscle-memory works for
	// either set.
	leftUp := p.e.IsCharDown('w') || p.e.IsCharDown('W')
	leftDown := p.e.IsCharDown('s') || p.e.IsCharDown('S')
	if p.mode == modeVsCPU {
		leftUp = leftUp || p.e.IsKeyDown(engine.KeyUp)
		leftDown = leftDown || p.e.IsKeyDown(engine.KeyDown)
	}
	movePaddle(&p.left, leftUp, leftDown, paddleSpeed, s)
	p.left.clampY(p.fieldTop, p.fieldBottom)

	if p.mode == modeTwoPlayer {
		rightUp := p.e.IsKeyDown(engine.KeyUp)
		rightDown := p.e.IsKeyDown(engine.KeyDown)
		movePaddle(&p.right, rightUp, rightDown, paddleSpeed, s)
	} else {
		p.updateCPU(s)
	}
	p.right.clampY(p.fieldTop, p.fieldBottom)
}

func movePaddle(pd *paddle, up, down bool, speed, dt float64) {
	switch {
	case up && !down:
		pd.y -= speed * dt
	case down && !up:
		pd.y += speed * dt
	}
}

// updateCPU is the right-paddle AI: track the ball's centre with a
// capped speed and a small dead-zone so it doesn't jitter when aligned.
// While serving we re-centre instead so the AI doesn't telegraph the
// next serve direction by snapping to the y-axis early.
func (p *playScene) updateCPU(s float64) {
	target := p.ball.y + float64(p.ball.size)/2
	if p.state == psServing {
		target = float64(p.fieldTop+p.fieldBottom) / 2
	}
	diff := target - p.right.center()
	if math.Abs(diff) <= cpuDeadZone {
		return
	}
	step := cpuTrackSpeed * s
	if math.Abs(diff) < step {
		p.right.y += diff
		return
	}
	if diff > 0 {
		p.right.y += step
	} else {
		p.right.y -= step
	}
}

// updateBall moves the ball, then resolves wall / paddle / goal
// interactions. Each collision step works on the post-move position;
// at sane velocities (capped at ballMaxSpeed) and 60 FPS this is more
// than precise enough to avoid tunnelling through paddles.
func (p *playScene) updateBall(s float64) {
	p.ball.x += p.ball.vx * s
	p.ball.y += p.ball.vy * s

	// Top / bottom walls.
	if p.ball.y < float64(p.fieldTop) {
		p.ball.y = float64(p.fieldTop)
		p.ball.vy = -p.ball.vy
	}
	maxBallY := float64(p.fieldBottom - p.ball.size)
	if p.ball.y > maxBallY {
		p.ball.y = maxBallY
		p.ball.vy = -p.ball.vy
	}

	// Paddle bounces — only check the paddle the ball is heading toward.
	if p.ball.vx < 0 && ballHitsPaddle(&p.ball, &p.left) {
		// Eject the ball to the right edge of the paddle before reflecting
		// so a second collision check next frame can't re-trigger.
		p.ball.x = float64(p.left.x + p.left.width)
		bounceOffPaddle(&p.ball, &p.left, +1)
	} else if p.ball.vx > 0 && ballHitsPaddle(&p.ball, &p.right) {
		p.ball.x = float64(p.right.x - p.ball.size)
		bounceOffPaddle(&p.ball, &p.right, -1)
	}

	// Goals.
	if p.ball.x+float64(p.ball.size) < 0 {
		p.rightScore++
		p.handlePointScored(sideLeft)
	} else if p.ball.x > float64(p.w) {
		p.leftScore++
		p.handlePointScored(sideRight)
	}
}

func ballHitsPaddle(b *ball, pd *paddle) bool {
	if b.x+float64(b.size) <= float64(pd.x) || b.x >= float64(pd.x+pd.width) {
		return false
	}
	if b.y+float64(b.size) <= pd.y || b.y >= pd.y+float64(pd.height) {
		return false
	}
	return true
}

// bounceOffPaddle reflects the ball off pd with classic Pong "english":
// the vertical component is determined by where on the paddle the ball
// struck, mapped linearly from the paddle-top (max upward) to
// paddle-bottom (max downward). dirX is the outgoing horizontal sign.
func bounceOffPaddle(b *ball, pd *paddle, dirX float64) {
	ballCY := b.y + float64(b.size)/2
	rel := (ballCY - pd.center()) / (float64(pd.height) / 2)
	if rel < -1 {
		rel = -1
	}
	if rel > 1 {
		rel = 1
	}
	speed := math.Hypot(b.vx, b.vy) * ballSpeedup
	if speed > ballMaxSpeed {
		speed = ballMaxSpeed
	}
	angle := rel * ballMaxAngle
	b.vx = dirX * speed * math.Cos(angle)
	b.vy = speed * math.Sin(angle)
}

func (p *playScene) handlePointScored(loser side) {
	// Ball is served back toward the loser, giving them a chance to
	// react before the rally turns one-sided.
	p.serveTo = loser
	if p.leftScore >= matchPoint || p.rightScore >= matchPoint {
		p.state = psMatchOver
		p.stateT = 0
		if p.leftScore > p.rightScore {
			p.winner = sideLeft
		} else {
			p.winner = sideRight
		}
		return
	}
	p.beginServe()
}

// --- Rendering ---------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 12, A: 255})

	p.drawCentreLine(c)
	p.drawScores(c)
	p.drawPaddles(c)
	p.drawBall(c)
	p.drawHints(c)

	if p.state == psMatchOver {
		p.drawMatchOver(c)
	}
}

// drawCentreLine paints the classic dashed net down the middle of the
// playing field.
func (p *playScene) drawCentreLine(c *engine.Canvas) {
	x := p.w/2 - 1
	dash := 2
	gap := 2
	col := engine.Color{R: 80, G: 80, B: 120, A: 255}
	for y := p.fieldTop; y < p.fieldBottom; y += dash + gap {
		h := dash
		if y+h > p.fieldBottom {
			h = p.fieldBottom - y
		}
		c.FillRect(x, y, 2, h, col)
	}
}

// drawScores renders the two scores in the chunky pixel font, one
// quarter-width from each side.
func (p *playScene) drawScores(c *engine.Canvas) {
	leftText := fmt.Sprintf("%d", p.leftScore)
	rightText := fmt.Sprintf("%d", p.rightScore)
	lw := engine.TextWidth(leftText)
	rw := engine.TextWidth(rightText)
	ly := scoreTopMargin
	c.DrawText(p.w/4-lw/2, ly, leftText, engine.White)
	c.DrawText(3*p.w/4-rw/2, ly, rightText, engine.White)
}

func (p *playScene) drawPaddles(c *engine.Canvas) {
	leftCol := engine.Color{R: 120, G: 220, B: 255, A: 255}
	rightCol := engine.Color{R: 255, G: 180, B: 120, A: 255}
	if p.mode == modeVsCPU {
		rightCol = engine.Color{R: 240, G: 120, B: 140, A: 255}
	}
	c.FillRect(p.left.x, int(p.left.y), p.left.width, p.left.height, leftCol)
	c.FillRect(p.right.x, int(p.right.y), p.right.width, p.right.height, rightCol)
}

func (p *playScene) drawBall(c *engine.Canvas) {
	// During serve, blink the ball so it's clear play hasn't started yet.
	if p.state == psServing && int(p.stateT*4)%2 == 0 {
		return
	}
	c.FillRect(int(p.ball.x), int(p.ball.y), p.ball.size, p.ball.size, engine.White)
}

func (p *playScene) drawHints(c *engine.Canvas) {
	hint := "ESC QUIT"
	c.Print(c.Cols()-len(hint)-1, c.Rows()-1, hint, engine.Gray)
}

func (p *playScene) drawMatchOver(c *engine.Canvas) {
	var msg string
	switch {
	case p.mode == modeTwoPlayer && p.winner == sideLeft:
		msg = "PLAYER 1 WINS"
	case p.mode == modeTwoPlayer:
		msg = "PLAYER 2 WINS"
	case p.winner == sideLeft:
		msg = "YOU WIN"
	default:
		msg = "CPU WINS"
	}
	w := engine.TextWidth(msg)
	x := (p.w - w) / 2
	y := (p.h-engine.FontHeight)/2 - 4
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 16, A: 255})
	c.DrawText(x, y, msg, engine.Yellow)

	hint := "ENTER PLAY AGAIN   ESC QUIT"
	c.Print((c.Cols()-len(hint))/2, c.Rows()/2+2, hint, engine.White)
}
