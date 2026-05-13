package galaga

import (
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// newTestPlayScene constructs a play scene without going through the
// engine's input/output setup. Like in spaceinvaders, the engine itself
// is not headless-testable but the playScene struct is, since it runs
// purely off of Update(dt) and exposes its state.
func newTestPlayScene(t *testing.T, w, h int) *playScene {
	t.Helper()
	e, err := engine.New(engine.Options{Width: w, Height: h})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		hiScore: 0,
		stage:   1,
		rng:     rand.New(rand.NewSource(1)),
	}
	p.player.lives = 3
	p.computeLayout()
	p.spawnStars()
	p.beginStage(1)
	return p
}

func TestPlaySceneSurvivesManyFrames(t *testing.T) {
	// Drive the play scene through 30 simulated seconds of gameplay
	// (1800 frames at 60 FPS). This shouldn't panic, and should at
	// least complete one stage's entry script.
	p := newTestPlayScene(t, 80, 48)
	dt := time.Second / 60
	c := engine.NewCanvas(p.w, p.h)
	for frame := 0; frame < 60*30; frame++ {
		if err := p.Update(dt); err != nil {
			t.Fatalf("frame %d update: %v", frame, err)
		}
		p.Draw(c)
		if p.wantQuit {
			t.Fatalf("frame %d: unexpected wantQuit", frame)
		}
	}
	// By now the formation should have fully spawned at least once.
	totalSpawned := 0
	for _, n := range p.pendingSpawn {
		totalSpawned += n
	}
	if totalSpawned == 0 {
		t.Errorf("no enemies spawned after 30s")
	}
}

func TestFormationLayoutCounts(t *testing.T) {
	if n := totalFormationSlots(); n != 36 {
		t.Errorf("formation slot count = %d, want 36 (4 bosses + 16 butterflies + 16 bees)", n)
	}
}

func TestBezierPathArcLength(t *testing.T) {
	// A straight segment from (0,0) to (10,0) with collinear control
	// points should report arc length ≈ 10.
	seg := bezierSeg{
		p0: vec2{0, 0},
		p1: vec2{3, 0},
		p2: vec2{7, 0},
		p3: vec2{10, 0},
	}
	p := newPath([]bezierSeg{seg})
	if abs(p.total-10) > 0.5 {
		t.Errorf("straight Bezier arc length = %.2f, want ≈10", p.total)
	}
	mid := p.at(5)
	if abs(mid.x-5) > 0.5 || abs(mid.y) > 0.5 {
		t.Errorf("mid-path = (%.2f, %.2f), want ≈(5, 0)", mid.x, mid.y)
	}
	end := p.at(20) // past total — should clamp to endpoint.
	if abs(end.x-10) > 0.001 {
		t.Errorf("past-end = %v, want clamped to (10, 0)", end)
	}
}

func TestKindScores(t *testing.T) {
	cases := []struct {
		k             enemyKind
		wantFormation int
		wantFlight    int
	}{
		{enemyBee, 50, 100},
		{enemyButterfly, 80, 160},
		{enemyBoss, 150, 400},
	}
	for _, c := range cases {
		if got := c.k.formationScore(); got != c.wantFormation {
			t.Errorf("%v formation score = %d, want %d", c.k, got, c.wantFormation)
		}
		if got := c.k.flightScore(); got != c.wantFlight {
			t.Errorf("%v flight score = %d, want %d", c.k, got, c.wantFlight)
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestCaptureFlow(t *testing.T) {
	// Drive a synthetic capture / rescue cycle through the playScene's
	// public-facing state transitions. We don't go through the engine
	// loop; we just exercise capturePlayer / kill / collision helpers
	// directly to ensure the state changes line up.
	p := newTestPlayScene(t, 80, 48)

	// Fast-forward through the stage-intro banner so updatePlaying runs.
	p.state = psPlaying
	p.stateT = 0

	// Insert a synthetic Boss in formation directly.
	boss := &enemy{
		kind:    enemyBoss,
		slotRow: 0,
		slotCol: 3,
		state:   esFormation,
		x:       20,
		y:       8,
	}
	p.enemies = append(p.enemies, boss)

	// Trigger a capture as if the player walked into a fully-open beam.
	livesBefore := p.player.lives
	p.capturePlayer(boss)
	if !p.player.captured {
		t.Fatalf("after capturePlayer, player.captured should be true")
	}
	if p.bossWithShip != boss {
		t.Fatalf("bossWithShip not set to boss")
	}
	if !boss.carryHasShip {
		t.Fatalf("boss.carryHasShip not set")
	}
	if p.player.lives != livesBefore-1 {
		t.Fatalf("lives = %d, want %d after capture", p.player.lives, livesBefore-1)
	}
	if p.player.dual {
		t.Fatalf("dual fighter should be cleared on capture")
	}

	// Tick forward through the capture animation; the player should be
	// flagged un-captured and respawnT armed afterwards.
	for i := 0; i < 40; i++ {
		_ = p.Update(time.Second / 60)
	}
	if p.player.captured {
		t.Fatalf("player should no longer be captured after %.1fs", captureAnimDur)
	}

	// Now simulate the player shooting the boss. Formation Bosses take
	// 2 hits to kill, so we fire two bullets back-to-back and run the
	// collision resolver twice.
	boss.x = 30
	boss.y = 30
	p.bullets = []*playerBulletEntity{{x: 32, y: 30}}
	p.resolveBulletEnemyHits()
	if p.bossWithShip == nil {
		t.Fatalf("first hit should NOT kill a formation boss yet")
	}
	if boss.hits != 1 {
		t.Fatalf("boss hits after first shot = %d, want 1", boss.hits)
	}
	p.bullets = []*playerBulletEntity{{x: 32, y: 30}}
	p.resolveBulletEnemyHits()
	if p.bossWithShip != nil {
		t.Fatalf("bossWithShip should be cleared after second hit kills boss")
	}
	if !p.player.dual {
		t.Fatalf("player.dual should be true after rescuing captured ship")
	}
	if boss.state != esDying {
		t.Fatalf("boss state = %v, want esDying", boss.state)
	}
}
