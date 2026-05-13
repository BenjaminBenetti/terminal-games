package centipede

import (
	"bytes"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// newTestEngine builds a headless engine writing to /dev/null so tests
// don't pollute stdout and can be run in CI without a real TTY.
func newTestEngine(t *testing.T, w, h int) *engine.Engine {
	t.Helper()
	e, err := engine.New(engine.Options{
		Width:  w,
		Height: h,
		Output: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("engine.New failed: %v", err)
	}
	return e
}

func tickN(t *testing.T, p *playScene, steps int, dt time.Duration) {
	t.Helper()
	for i := 0; i < steps; i++ {
		if err := p.Update(dt); err != nil {
			t.Fatalf("Update returned %v", err)
		}
		p.Draw(p.e.Canvas())
	}
}

// TestPlayScene_DoesNotPanic runs a few hundred frames of the play scene
// and verifies it can tick through normal gameplay without panicking.
// We can't drive input from here (the input reader is no-op in tests),
// but the centipede / enemies / mushroom interactions all advance.
func TestPlayScene_DoesNotPanic(t *testing.T) {
	e := newTestEngine(t, 80, 48)
	p := newPlayScene(e, 0)
	tickN(t, p, 600, 16*time.Millisecond) // ~10 seconds of game-time
}

// TestCentipedeStep verifies the basic head bounce: a head that walks
// into a side wall must drop down one row and reverse direction.
func TestCentipedeStep(t *testing.T) {
	e := newTestEngine(t, 80, 48)
	p := newPlayScene(e, 0)
	// Clear the field so the centipede only interacts with walls.
	for r := 0; r < p.field.rows; r++ {
		for c := 0; c < p.field.cols; c++ {
			p.field.cells[r][c] = mushroom{}
		}
	}
	// Use a single-segment chain at (0, 0) heading left.
	cp := newCentipede(1, p.field.cols, 0.1)
	cp.segments[0].col = 0
	cp.segments[0].row = 0
	cp.segments[0].dx = -1
	cp.segments[0].dy = 1
	cp.step(p.field)
	h := cp.segments[0]
	if h.row != 1 || h.dx != 1 || h.col != 0 {
		t.Fatalf("expected (col=0, row=1, dx=+1) after wall bump, got col=%d row=%d dx=%d",
			h.col, h.row, h.dx)
	}
}

// TestCentipedeSplit checks that hitting a body segment splits the
// chain correctly, with the new head's horizontal direction flipped.
func TestCentipedeSplit(t *testing.T) {
	e := newTestEngine(t, 80, 48)
	p := newPlayScene(e, 0)
	cp := newCentipede(6, p.field.cols, 0.1)
	// Snapshot initial dx values.
	beforeDx := cp.segments[3].dx
	split, wasHead, empty := cp.applyHitAt(2)
	if wasHead {
		t.Fatalf("hit at idx 2 should not be a head kill")
	}
	if empty {
		t.Fatalf("original chain shouldn't be empty after split")
	}
	if cp.length() != 2 {
		t.Fatalf("head chain length = %d, want 2", cp.length())
	}
	if split == nil || split.length() != 3 {
		t.Fatalf("split chain length = %d, want 3", split.length())
	}
	if split.segments[0].dx != -beforeDx {
		t.Fatalf("new head should reverse dx; before=%d after=%d", beforeDx, split.segments[0].dx)
	}
}

// TestMushroomDamage verifies it takes 4 hits to destroy and that the
// destruction awards 1 point on a normal mushroom and 5 on a poisoned
// one.
func TestMushroomDamage(t *testing.T) {
	e := newTestEngine(t, 80, 48)
	p := newPlayScene(e, 0)
	// Place a mushroom at (5, 5).
	p.field.cells[5][5] = mushroom{hp: mushroomHP}
	for i := 0; i < mushroomHP-1; i++ {
		score, destroyed := p.field.damage(5, 5)
		if score != 0 || destroyed {
			t.Fatalf("hit %d: got score=%d destroyed=%v, want 0 / false", i+1, score, destroyed)
		}
	}
	score, destroyed := p.field.damage(5, 5)
	if !destroyed || score != 1 {
		t.Fatalf("killing hit: got score=%d destroyed=%v, want 1 / true", score, destroyed)
	}
	// Poisoned mushroom: 5 pts.
	p.field.cells[5][5] = mushroom{hp: 1, poisoned: true}
	score, destroyed = p.field.damage(5, 5)
	if !destroyed || score != 5 {
		t.Fatalf("poisoned kill: got score=%d destroyed=%v, want 5 / true", score, destroyed)
	}
}
