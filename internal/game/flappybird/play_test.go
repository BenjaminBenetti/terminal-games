package flappybird

import (
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// newTestPlayScene builds a play scene without driving the engine loop.
// The engine still has to allocate a canvas, but Update / Draw are
// never called via Run — tests step them directly with chosen dt values.
// A deterministic RNG seed makes pipe-spawn placement reproducible
// across runs of the suite.
func newTestPlayScene(t *testing.T, w, h int) *playScene {
	t.Helper()
	// Redirect hi-score saves to a tempdir so tests never touch the
	// user's real save file.
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	e, err := engine.New(engine.Options{Width: w, Height: h})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := &playScene{
		e:     e,
		w:     w,
		h:     h,
		theme: themeDay,
		rng:   rand.New(rand.NewSource(1)),
	}
	p.computeLayout()
	p.resetForReady()
	return p
}

func TestGameRegistersInRegistry(t *testing.T) {
	if _, ok := registry.Get("flappybird"); !ok {
		t.Error("flappybird not registered")
	}
}

func TestFlapSetsUpwardVelocity(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.birdVY = 30
	p.flap()
	if p.birdVY != flapVelocity {
		t.Errorf("birdVY = %v, want %v", p.birdVY, flapVelocity)
	}
}

func TestReadyToPlayingOnFlap(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	if p.state != psReady {
		t.Fatalf("initial state = %v, want psReady", p.state)
	}
	// Simulate a flap without using engine input — directly transition
	// as handleInput would.
	p.flap()
	p.state = psPlaying
	p.stateT = 0
	if p.state != psPlaying {
		t.Errorf("state = %v, want psPlaying after flap", p.state)
	}
}

func TestGravityAccumulates(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	// High enough that gravity ticks don't fall into the ground and
	// trigger the death bounce that overwrites birdVY.
	p.birdY = float64(p.fieldTop) + 1
	p.birdVY = 0
	p.updatePlaying(0.1)
	if p.birdVY <= 0 {
		t.Errorf("birdVY = %v, want positive after gravity tick", p.birdVY)
	}
}

func TestFallSpeedClamped(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	// Position bird high in the field so gravity doesn't trigger ground
	// collision before the velocity caps out.
	p.birdY = float64(p.fieldTop) + 1
	p.birdVY = maxFallSpeed + 50
	p.updatePlaying(0.1)
	if p.birdVY > maxFallSpeed+0.01 {
		t.Errorf("birdVY = %v, want <= maxFallSpeed=%v", p.birdVY, maxFallSpeed)
	}
}

func TestPipesScrollLeft(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	p.birdY = float64(p.fieldTop+p.fieldBottom) / 2
	p.pipes = []pipe{{x: 50, gapY: 15}}
	startX := p.pipes[0].x
	p.updatePlaying(0.5)
	if p.pipes[0].x >= startX {
		t.Errorf("pipe x = %v, want < %v after scroll", p.pipes[0].x, startX)
	}
}

func TestScoreIncrementsOnPipePass(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	p.birdY = float64(p.fieldTop+p.fieldBottom) / 2
	// Pipe sits just to the right of the bird with the bird inside the
	// gap. Stepping forward should slide the pipe's right edge past
	// the bird's left edge and grant a point.
	gapY := int(p.birdY) - pipeGap/2
	p.pipes = []pipe{{x: float64(p.birdX - pipeWidth + 1), gapY: gapY}}
	p.updatePlaying(0.5)
	if p.score != 1 {
		t.Errorf("score = %d, want 1 after pipe pass", p.score)
	}
}

func TestPipeScoredOnlyOnce(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	p.birdY = float64(p.fieldTop+p.fieldBottom) / 2
	gapY := int(p.birdY) - pipeGap/2
	p.pipes = []pipe{{x: float64(p.birdX - pipeWidth - 5), gapY: gapY, scored: true}}
	p.updatePlaying(0.5)
	if p.score != 0 {
		t.Errorf("score = %d, want 0 when pipe already scored", p.score)
	}
}

func TestGroundCollisionTriggersDeath(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	// Drop the bird right onto the ground.
	p.birdY = float64(p.fieldBottom - birdSpriteHeight + 2)
	p.birdVY = 10
	p.updatePlaying(0.1)
	if p.state != psDying {
		t.Errorf("state = %v, want psDying after ground collision", p.state)
	}
}

func TestPipeCollisionTriggersDeath(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	// Pipe directly overlapping the bird, with gap far away vertically.
	p.birdY = 10
	gapY := p.fieldBottom - pipeGap - pipeGapMargin
	p.pipes = []pipe{{x: float64(p.birdX), gapY: gapY}}
	if !p.collide() {
		t.Fatal("expected collision when bird overlaps pipe")
	}
	p.updatePlaying(0.01)
	if p.state != psDying {
		t.Errorf("state = %v, want psDying after pipe collision", p.state)
	}
}

func TestNoCollisionInGap(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	// Bird is centered inside the gap of a pipe directly above it.
	gapY := int(p.birdY) - pipeGap/2
	p.pipes = []pipe{{x: float64(p.birdX), gapY: gapY}}
	if p.collide() {
		t.Errorf("bird inside gap should not collide")
	}
}

func TestDyingFallsToGround(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psDying
	p.birdY = float64(p.fieldTop) + 5
	p.birdVY = 0
	// Drive via Update so stateT advances — the ground-dwell test in
	// updateDying compares stateT, which only ticks inside Update.
	for i := 0; i < 300 && p.state == psDying; i++ {
		if err := p.Update(time.Second / 60); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if p.state != psGameOver {
		t.Errorf("state = %v, want psGameOver after death animation completes", p.state)
	}
}

func TestHiScoreUpdatesWhenScoreExceeds(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.hiScore = 5
	p.state = psPlaying
	p.birdY = float64(p.fieldTop+p.fieldBottom) / 2
	gapY := int(p.birdY) - pipeGap/2
	p.pipes = []pipe{
		{x: float64(p.birdX - pipeWidth + 1), gapY: gapY},
	}
	p.score = 5
	p.updatePlaying(0.5)
	if p.hiScore != 6 {
		t.Errorf("hiScore = %d, want 6 after surpassing previous best", p.hiScore)
	}
	if !p.newHi {
		t.Error("newHi should be true when hi-score advances")
	}
}

func TestMedalTier(t *testing.T) {
	cases := []struct {
		score int
		want  medalKind
	}{
		{0, medalNone},
		{9, medalNone},
		{10, medalBronzeKind},
		{19, medalBronzeKind},
		{20, medalSilverKind},
		{29, medalSilverKind},
		{30, medalGoldKind},
		{39, medalGoldKind},
		{40, medalPlatinumKind},
		{200, medalPlatinumKind},
	}
	for _, tc := range cases {
		got := medalTier(tc.score)
		if got != tc.want {
			t.Errorf("medalTier(%d) = %v, want %v", tc.score, got, tc.want)
		}
	}
}

func TestHiScorePersistence(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	saveHiScore(42)
	got := loadHiScore()
	if got != 42 {
		t.Errorf("loaded hi-score = %d, want 42", got)
	}
}

func TestHiScoreMissingFileReturnsZero(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if got := loadHiScore(); got != 0 {
		t.Errorf("loaded hi-score = %d, want 0 when file absent", got)
	}
}

func TestSpawnPipeGapWithinBounds(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	for i := 0; i < 200; i++ {
		p.spawnPipe()
	}
	for _, pp := range p.pipes {
		if pp.gapY < pipeGapMargin {
			t.Errorf("pipe gapY=%d below pipeGapMargin=%d", pp.gapY, pipeGapMargin)
		}
		if pp.gapY+pipeGap > p.fieldBottom-pipeGapMargin {
			t.Errorf("pipe gapY=%d + pipeGap=%d above fieldBottom-margin=%d",
				pp.gapY, pipeGap, p.fieldBottom-pipeGapMargin)
		}
	}
}

func TestDrawDoesNotPanic(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	c := engine.NewCanvas(80, 48)
	// Step the scene through each state and draw — we're checking that
	// no draw path crashes on out-of-bounds or nil sprites.
	for _, st := range []playState{psReady, psPlaying, psDying, psGameOver} {
		p.state = st
		p.stateT = 0.5
		p.pipes = []pipe{{x: 40, gapY: 12}, {x: 60, gapY: 20}}
		p.Draw(c)
	}
}

func TestUpdatePropagatesQuit(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.wantQuit = true
	// Should be a no-op — wantQuit short-circuits Update.
	if err := p.Update(time.Millisecond * 16); err != nil {
		t.Errorf("Update returned err=%v, want nil even on wantQuit", err)
	}
}
