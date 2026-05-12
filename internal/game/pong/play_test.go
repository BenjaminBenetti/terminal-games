package pong

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// newTestPlayScene builds a play scene without driving the engine loop.
// The engine still needs to allocate a canvas, but Update / Draw are
// never called via Run — we drive them directly with known dt values.
func newTestPlayScene(t *testing.T, mode gameMode, w, h int) *playScene {
	t.Helper()
	e, err := engine.New(engine.Options{Width: w, Height: h})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := &playScene{
		e:    e,
		w:    w,
		h:    h,
		mode: mode,
		rng:  rand.New(rand.NewSource(1)),
	}
	p.computeLayout()
	p.resetPaddles()
	p.serveTo = sideRight
	p.beginServe()
	return p
}

func TestGameRegistersInRegistry(t *testing.T) {
	if _, ok := registry.Get("pong"); !ok {
		t.Error("pong not registered")
	}
}

func TestBallBouncesOffTopWall(t *testing.T) {
	p := newTestPlayScene(t, modeTwoPlayer, 80, 48)
	p.state = psPlaying
	p.ball.x = float64(p.w) / 2
	p.ball.y = float64(p.fieldTop) - 5 // already past the top wall
	p.ball.vx = 20
	p.ball.vy = -30
	p.updateBall(0.0)
	if p.ball.y != float64(p.fieldTop) {
		t.Errorf("ball y=%v, want clamped to fieldTop=%d", p.ball.y, p.fieldTop)
	}
	if p.ball.vy <= 0 {
		t.Errorf("ball vy=%v, want positive after top-wall bounce", p.ball.vy)
	}
}

func TestBallBouncesOffBottomWall(t *testing.T) {
	p := newTestPlayScene(t, modeTwoPlayer, 80, 48)
	p.state = psPlaying
	p.ball.x = float64(p.w) / 2
	p.ball.y = float64(p.fieldBottom) + 5
	p.ball.vx = 20
	p.ball.vy = 30
	p.updateBall(0.0)
	maxY := float64(p.fieldBottom - p.ball.size)
	if p.ball.y != maxY {
		t.Errorf("ball y=%v, want %v", p.ball.y, maxY)
	}
	if p.ball.vy >= 0 {
		t.Errorf("ball vy=%v, want negative after bottom-wall bounce", p.ball.vy)
	}
}

func TestPaddleClampsWithinField(t *testing.T) {
	p := newTestPlayScene(t, modeTwoPlayer, 80, 48)
	p.left.y = float64(p.fieldTop) - 100
	p.left.clampY(p.fieldTop, p.fieldBottom)
	if p.left.y != float64(p.fieldTop) {
		t.Errorf("paddle y=%v after top clamp, want %d", p.left.y, p.fieldTop)
	}
	p.left.y = float64(p.fieldBottom) + 100
	p.left.clampY(p.fieldTop, p.fieldBottom)
	maxY := float64(p.fieldBottom - p.left.height)
	if p.left.y != maxY {
		t.Errorf("paddle y=%v after bottom clamp, want %v", p.left.y, maxY)
	}
}

func TestBallExitingLeftAwardsRight(t *testing.T) {
	p := newTestPlayScene(t, modeTwoPlayer, 80, 48)
	p.state = psPlaying
	p.ball.x = -10
	p.ball.y = float64(p.fieldTop + 1)
	p.ball.vx = -50
	p.updateBall(0.0)
	if p.rightScore != 1 || p.leftScore != 0 {
		t.Errorf("scores left=%d right=%d, want 0/1", p.leftScore, p.rightScore)
	}
	if p.serveTo != sideLeft {
		t.Errorf("serveTo=%v after right scored, want sideLeft (loser serves)", p.serveTo)
	}
	if p.state != psServing {
		t.Errorf("state=%v, want psServing after a point", p.state)
	}
}

func TestBallExitingRightAwardsLeft(t *testing.T) {
	p := newTestPlayScene(t, modeTwoPlayer, 80, 48)
	p.state = psPlaying
	p.ball.x = float64(p.w) + 10
	p.ball.y = float64(p.fieldTop + 1)
	p.ball.vx = 50
	p.updateBall(0.0)
	if p.leftScore != 1 || p.rightScore != 0 {
		t.Errorf("scores left=%d right=%d, want 1/0", p.leftScore, p.rightScore)
	}
	if p.serveTo != sideRight {
		t.Errorf("serveTo=%v after left scored, want sideRight", p.serveTo)
	}
}

func TestPaddleBounceReversesHorizontalVelocity(t *testing.T) {
	p := newTestPlayScene(t, modeTwoPlayer, 80, 48)
	p.state = psPlaying
	// Position the ball just inside the left paddle, moving left.
	p.ball.x = float64(p.left.x)
	p.ball.y = p.left.y + float64(p.left.height)/2 - float64(p.ball.size)/2
	p.ball.vx = -40
	p.ball.vy = 0
	p.updateBall(0.0)
	if p.ball.vx <= 0 {
		t.Errorf("ball vx=%v after left-paddle bounce, want positive", p.ball.vx)
	}
}

func TestPaddleBounceImpartsEnglishFromHitOffset(t *testing.T) {
	b := &ball{x: 10, y: 0, size: 2, vx: -40, vy: 0}
	pd := &paddle{x: 9, y: 0, width: 2, height: 8}
	// Ball is at the very top of the paddle — should deflect upward.
	bounceOffPaddle(b, pd, +1)
	if b.vx <= 0 {
		t.Errorf("vx=%v, want positive after +1 dirX bounce", b.vx)
	}
	if b.vy >= 0 {
		t.Errorf("vy=%v, want negative (upward) when hitting paddle top", b.vy)
	}

	// And the opposite: ball low on paddle should deflect down.
	b2 := &ball{x: 10, y: 6, size: 2, vx: -40, vy: 0}
	bounceOffPaddle(b2, pd, +1)
	if b2.vy <= 0 {
		t.Errorf("vy=%v, want positive (downward) when hitting paddle bottom", b2.vy)
	}
}

func TestBallSpeedCappedOnBounce(t *testing.T) {
	b := &ball{x: 10, y: 4, size: 2, vx: -200, vy: 0}
	pd := &paddle{x: 9, y: 0, width: 2, height: 8}
	bounceOffPaddle(b, pd, +1)
	speed := math.Hypot(b.vx, b.vy)
	if speed > ballMaxSpeed+0.001 {
		t.Errorf("speed=%v after bounce, want <= %v", speed, ballMaxSpeed)
	}
}

func TestServeTransitionsToPlayingAfterDelay(t *testing.T) {
	p := newTestPlayScene(t, modeTwoPlayer, 80, 48)
	if p.state != psServing {
		t.Fatalf("state=%v, want psServing initially", p.state)
	}
	if err := p.Update(time.Duration(float64(time.Second) * (serveDelay + 0.05))); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if p.state != psPlaying {
		t.Errorf("state=%v after serve delay, want psPlaying", p.state)
	}
	if p.ball.vx == 0 && p.ball.vy == 0 {
		t.Error("ball velocity should be non-zero after serve")
	}
}

func TestMatchEndsAtMatchPoint(t *testing.T) {
	p := newTestPlayScene(t, modeTwoPlayer, 80, 48)
	p.leftScore = matchPoint - 1
	p.state = psPlaying
	p.ball.x = float64(p.w) + 5
	p.ball.vx = 30
	p.updateBall(0.0)
	if p.leftScore != matchPoint {
		t.Errorf("leftScore=%d, want %d", p.leftScore, matchPoint)
	}
	if p.state != psMatchOver {
		t.Errorf("state=%v, want psMatchOver", p.state)
	}
	if p.winner != sideLeft {
		t.Errorf("winner=%v, want sideLeft", p.winner)
	}
}

func TestCPUTracksBall(t *testing.T) {
	p := newTestPlayScene(t, modeVsCPU, 80, 48)
	p.state = psPlaying
	// Pin the ball far below the right paddle and step one frame.
	p.ball.y = float64(p.fieldBottom - p.ball.size - 1)
	startY := p.right.y
	p.updateCPU(0.5)
	if p.right.y <= startY {
		t.Errorf("CPU paddle didn't move down toward ball: startY=%v y=%v", startY, p.right.y)
	}
}
