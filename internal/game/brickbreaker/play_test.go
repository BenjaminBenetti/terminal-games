package brickbreaker

import (
	"math"
	"testing"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

func newTestScene(t *testing.T, level int) *playScene {
	t.Helper()
	e, err := engine.New(engine.Options{Width: 80, Height: 48})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	return newPlayScene(e, level, 0)
}

func TestLevelsParseable(t *testing.T) {
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	for i, lv := range levels {
		if len(lv.rows) == 0 {
			t.Errorf("level %d (%s) has no brick rows", i, lv.name)
		}
		if lv.paddleWidth <= 0 {
			t.Errorf("level %d has non-positive paddle width %d", i, lv.paddleWidth)
		}
		if lv.ballSpeed <= 0 {
			t.Errorf("level %d has non-positive ball speed %v", i, lv.ballSpeed)
		}
		if lv.paddleSpeed <= 0 {
			t.Errorf("level %d has non-positive paddle speed %v", i, lv.paddleSpeed)
		}
		w := len(lv.rows[0])
		for r, row := range lv.rows {
			if len(row) != w {
				t.Errorf("level %d row %d width %d != header width %d", i, r, len(row), w)
			}
		}
	}
}

func TestBrickFromRune(t *testing.T) {
	cases := []struct {
		r    byte
		want brickType
	}{
		{'#', brickWeak},
		{'@', brickStrong},
		{'*', brickTough},
		{'.', brickEmpty},
		{' ', brickEmpty},
		{'x', brickEmpty},
	}
	for _, c := range cases {
		if got := brickFromRune(c.r); got != c.want {
			t.Errorf("brickFromRune(%q) = %v, want %v", c.r, got, c.want)
		}
	}
}

func TestBrickHitsAndScore(t *testing.T) {
	if brickWeak.hits() != 1 || brickStrong.hits() != 2 || brickTough.hits() != 3 {
		t.Errorf("hits mismatch: weak=%d strong=%d tough=%d",
			brickWeak.hits(), brickStrong.hits(), brickTough.hits())
	}
	if brickWeak.score() <= 0 || brickStrong.score() <= brickWeak.score() ||
		brickTough.score() <= brickStrong.score() {
		t.Errorf("score should strictly increase with brick toughness: %d %d %d",
			brickWeak.score(), brickStrong.score(), brickTough.score())
	}
}

func TestBuildBricksCountsAlive(t *testing.T) {
	for i, lv := range levels {
		p := newTestScene(t, i)
		expected := 0
		for _, row := range lv.rows {
			for j := 0; j < len(row); j++ {
				if brickFromRune(row[j]) != brickEmpty {
					expected++
				}
			}
		}
		if p.alive != expected {
			t.Errorf("level %d: alive=%d want=%d", i, p.alive, expected)
		}
		if len(p.bricks) != expected {
			t.Errorf("level %d: len(bricks)=%d want=%d", i, len(p.bricks), expected)
		}
	}
}

func TestLaunchBallHasUpwardVelocity(t *testing.T) {
	p := newTestScene(t, 0)
	for i := 0; i < 200; i++ {
		// Reset to serve so launchBall has a stationary ball to push.
		p.resetForServe()
		p.launchBall()
		if len(p.balls) != 1 {
			t.Fatalf("expected 1 ball after launch, got %d", len(p.balls))
		}
		b := p.balls[0]
		if b.vy >= 0 {
			t.Fatalf("iter %d: launched ball not moving up: vy=%v", i, b.vy)
		}
		speed := math.Hypot(b.vx, b.vy)
		if math.Abs(speed-p.ballSpeed) > 1e-6 {
			t.Fatalf("iter %d: launched speed %v != ballSpeed %v", i, speed, p.ballSpeed)
		}
	}
}

func TestPaddleEdgeBounceAngles(t *testing.T) {
	p := newTestScene(t, 0)
	p.state = psPlaying

	// Centre hit -> straight up.
	p.paddleX = 10
	b := &ballEntity{
		x:  p.paddleX + float64(p.paddleW)/2 - float64(ballSize)/2,
		y:  float64(p.paddleY-ballSize) + 0.1,
		vy: 1,
	}
	if !p.collidePaddle(b) {
		t.Fatalf("centre hit: expected collision")
	}
	if math.Abs(b.vx) > 1e-9 {
		t.Errorf("centre hit: expected vx≈0, got %v", b.vx)
	}
	if b.vy >= 0 {
		t.Errorf("centre hit: expected vy<0 after bounce, got %v", b.vy)
	}

	// Far-right edge -> positive vx.
	p.paddleX = 10
	b = &ballEntity{
		x:  p.paddleX + float64(p.paddleW) - float64(ballSize),
		y:  float64(p.paddleY-ballSize) + 0.1,
		vy: 1,
	}
	if !p.collidePaddle(b) {
		t.Fatalf("right-edge hit: expected collision")
	}
	if b.vx <= 0 {
		t.Errorf("right-edge hit: expected vx>0, got %v", b.vx)
	}

	// Far-left edge -> negative vx.
	p.paddleX = 10
	b = &ballEntity{
		x:  p.paddleX,
		y:  float64(p.paddleY-ballSize) + 0.1,
		vy: 1,
	}
	if !p.collidePaddle(b) {
		t.Fatalf("left-edge hit: expected collision")
	}
	if b.vx >= 0 {
		t.Errorf("left-edge hit: expected vx<0, got %v", b.vx)
	}
}

func TestPaddleHitResetsCombo(t *testing.T) {
	p := newTestScene(t, 0)
	p.state = psPlaying
	b := &ballEntity{
		x:     p.paddleX + float64(p.paddleW)/2,
		y:     float64(p.paddleY-ballSize) + 0.1,
		vy:    1,
		combo: 7,
	}
	p.balls = []*ballEntity{b}
	if !p.ballSubstep(b, 0) {
		t.Fatalf("substep should not lose ball")
	}
	if b.combo != 0 {
		t.Errorf("combo should reset to 0 on paddle hit, got %d", b.combo)
	}
}

func TestComboMultiplierTiers(t *testing.T) {
	cases := []struct {
		combo, want int
	}{
		{0, 1}, {1, 1}, {2, 1},
		{3, 2}, {4, 2},
		{5, 3}, {6, 3},
		{7, 4}, {8, 4}, {9, 4},
		{10, 5}, {25, 5},
	}
	for _, c := range cases {
		if got := comboMultiplier(c.combo); got != c.want {
			t.Errorf("comboMultiplier(%d) = %d, want %d", c.combo, got, c.want)
		}
	}
}

func TestMultiBallSpawnsExtrasUpToCap(t *testing.T) {
	p := newTestScene(t, 0)
	// One ball in motion.
	p.balls = []*ballEntity{{x: 40, y: 30, vx: 0, vy: -p.ballSpeed}}

	p.spawnExtraBalls()
	if len(p.balls) != 3 {
		t.Fatalf("after first multi-ball, expected 3 balls, got %d", len(p.balls))
	}

	// Trigger again from 3 balls — 3 parents × 2 extras = 6 more, but
	// we should cap at maxActiveBalls.
	p.spawnExtraBalls()
	if len(p.balls) > maxActiveBalls {
		t.Errorf("ball count %d exceeds cap %d", len(p.balls), maxActiveBalls)
	}
	if len(p.balls) <= 3 {
		t.Errorf("expected more balls after second multi-ball, got %d", len(p.balls))
	}

	// Speed must be preserved by the rotation.
	parentSpeed := p.ballSpeed
	for _, b := range p.balls {
		got := math.Hypot(b.vx, b.vy)
		if math.Abs(got-parentSpeed) > 1e-6 {
			t.Errorf("ball speed %v != parent %v", got, parentSpeed)
		}
	}
}

func TestWidePaddleActivateExpiresAndRestores(t *testing.T) {
	p := newTestScene(t, 0)
	base := p.basePaddleW
	if p.paddleW != base {
		t.Fatalf("initial paddle width %d != base %d", p.paddleW, base)
	}

	p.timeT = 1
	p.activateWide()
	if p.paddleW <= base {
		t.Errorf("activateWide did not enlarge paddle: got %d, base %d", p.paddleW, base)
	}
	if p.wideUntilT <= p.timeT {
		t.Errorf("wideUntilT %v should be > timeT %v after activate", p.wideUntilT, p.timeT)
	}

	// Advance past the duration and run the effect tick.
	p.timeT = p.wideUntilT + 0.01
	p.updateEffects()
	if p.paddleW != base {
		t.Errorf("paddle should revert to base width %d after expiry, got %d", base, p.paddleW)
	}
	if p.wideUntilT != 0 {
		t.Errorf("wideUntilT should clear after expiry, got %v", p.wideUntilT)
	}
}

func TestSlowBallScalesAndRestoresVelocities(t *testing.T) {
	p := newTestScene(t, 0)
	p.balls = []*ballEntity{{x: 40, y: 30, vx: p.ballSpeed, vy: 0}}
	baseSpeed := p.baseBallSpeed

	p.timeT = 1
	p.activateSlow()
	got := math.Hypot(p.balls[0].vx, p.balls[0].vy)
	want := baseSpeed * slowBallScale
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("after slow activate: ball speed %v, want %v", got, want)
	}
	if math.Abs(p.ballSpeed-want) > 1e-6 {
		t.Errorf("after slow activate: p.ballSpeed %v, want %v", p.ballSpeed, want)
	}

	p.timeT = p.slowUntilT + 0.01
	p.updateEffects()
	got = math.Hypot(p.balls[0].vx, p.balls[0].vy)
	if math.Abs(got-baseSpeed) > 1e-6 {
		t.Errorf("after slow expiry: ball speed %v, want %v", got, baseSpeed)
	}
	if math.Abs(p.ballSpeed-baseSpeed) > 1e-6 {
		t.Errorf("after slow expiry: p.ballSpeed %v, want %v", p.ballSpeed, baseSpeed)
	}
}

func TestExtraLifeCaps(t *testing.T) {
	p := newTestScene(t, 0)
	p.lives = 8
	p.activatePowerUp(powerExtraLife)
	if p.lives != 9 {
		t.Errorf("expected lives=9 after 1up at 8, got %d", p.lives)
	}
	p.activatePowerUp(powerExtraLife)
	if p.lives != 9 {
		t.Errorf("expected lives capped at 9, got %d", p.lives)
	}
}

func TestPowerUpCaughtByPaddleActivates(t *testing.T) {
	p := newTestScene(t, 0)
	p.state = psPlaying
	startLives := p.lives
	// Drop a 1Up just above the paddle in its x range.
	pu := &powerUpEntity{
		x:    p.paddleX + 1,
		y:    float64(p.paddleY) - float64(powerUpH),
		kind: powerExtraLife,
	}
	p.powerUps = []*powerUpEntity{pu}
	// One short frame of falling brings the capsule onto the paddle —
	// a longer step would tunnel right past at this fall speed.
	p.updatePowerUps(0.1)
	if len(p.powerUps) != 0 {
		t.Errorf("expected power-up consumed, %d remain", len(p.powerUps))
	}
	if p.lives <= startLives {
		t.Errorf("extra-life power-up did not grant a life: lives %d -> %d", startLives, p.lives)
	}
}
