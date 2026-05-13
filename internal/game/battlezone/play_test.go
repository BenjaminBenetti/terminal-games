package battlezone

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// newTestEngine builds an engine that writes its output into a byte
// buffer so the tests don't try to touch a real terminal.
func newTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	e, err := engine.New(engine.Options{Width: 80, Height: 48, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	return e
}

// step advances the play scene one frame, running both Update and Draw
// (so any rendering panics surface).
func step(t *testing.T, p *playScene, dt time.Duration) {
	t.Helper()
	if err := p.Update(dt); err != nil {
		t.Fatalf("Update: %v", err)
	}
	p.Draw(p.e.Canvas())
}

// TestPlaySceneSmoke verifies that a freshly constructed scene can be
// stepped for many frames without panicking and that enemies spawn.
// The test stops as soon as an enemy is observed — the unattended bot
// gets shot up quickly so we shouldn't assert "alive after N seconds".
func TestPlaySceneSmoke(t *testing.T) {
	e := newTestEngine(t)
	p := newPlayScene(e, 0)
	p.rng = rand.New(rand.NewSource(1))

	sawEnemy := false
	for i := 0; i < 600; i++ {
		step(t, p, 16*time.Millisecond)
		if p.enemy != nil {
			sawEnemy = true
		}
	}
	if !sawEnemy {
		t.Fatal("expected at least one enemy to spawn in 10 simulated seconds")
	}
}

// TestPlayerShellSingleInFlight verifies the original's one-shot-at-a-
// time constraint.
func TestPlayerShellSingleInFlight(t *testing.T) {
	e := newTestEngine(t)
	p := newPlayScene(e, 0)
	p.rng = rand.New(rand.NewSource(2))
	p.tryFire()
	p.tryFire() // cooldown should block second
	if got := countPlayerShells(p); got != 1 {
		t.Fatalf("expected 1 shell, got %d", got)
	}
}

// TestPlayerDeathDecrementsLives drives the player into an enemy shell
// and verifies the death state transitions and life decrement.
func TestPlayerDeathDecrementsLives(t *testing.T) {
	e := newTestEngine(t)
	p := newPlayScene(e, 0)
	p.rng = rand.New(rand.NewSource(3))
	startingLivesBefore := p.lives

	// Directly damage the player.
	p.killPlayer()
	if p.state != psPlayerDying {
		t.Fatalf("expected psPlayerDying, got %v", p.state)
	}
	if p.lives != startingLivesBefore-1 {
		t.Fatalf("expected %d lives, got %d", startingLivesBefore-1, p.lives)
	}
	if p.crackT <= 0 {
		t.Fatal("expected crack overlay to be active after death")
	}
}

// TestScoreEnemyAwardsPoints checks the per-kind score values match the
// values quoted on the original cabinet marquee.
func TestScoreEnemyAwardsPoints(t *testing.T) {
	cases := []struct {
		kind enemyKind
		want int
	}{
		{enemyTank, scoreTank},
		{enemySuperTank, scoreSuperTank},
		{enemyMissile, scoreMissile},
		{enemySaucer, scoreSaucer},
	}
	for _, c := range cases {
		e := newTestEngine(t)
		p := newPlayScene(e, 0)
		p.scoreEnemy(&enemy{kind: c.kind})
		if p.score != c.want {
			t.Errorf("scoreEnemy(%v): want %d, got %d", c.kind, c.want, p.score)
		}
	}
}

// TestBonusLifeThreshold confirms an extra tank is awarded at the bonus
// score boundary.
func TestBonusLifeThreshold(t *testing.T) {
	e := newTestEngine(t)
	p := newPlayScene(e, 0)
	p.score = bonusLifeEvery - scoreTank
	livesBefore := p.lives
	p.scoreEnemy(&enemy{kind: enemyTank})
	if p.lives != livesBefore+1 {
		t.Fatalf("expected +1 life after crossing bonus threshold, got %d", p.lives)
	}
}

func countPlayerShells(p *playScene) int {
	n := 0
	for _, pr := range p.projectiles {
		if pr.fromPlayer {
			n++
		}
	}
	return n
}
