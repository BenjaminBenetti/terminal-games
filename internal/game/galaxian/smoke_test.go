package galaxian

import (
	"bytes"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// TestSmokeRun ticks the play scene for 10 simulated seconds across a
// range of canvas sizes. The point isn't to drive specific gameplay —
// it's to catch crashes, NaNs, and runaway state machines (e.g. an
// alien stuck mid-dive that never makes it to asExited).
func TestSmokeRun(t *testing.T) {
	for _, sz := range []struct{ w, h int }{
		{80, 48},
		{100, 60},
		{120, 80},
		{160, 100},
	} {
		t.Run("", func(t *testing.T) {
			p := newTestPlayScene(t, sz.w, sz.h)
			// 10 simulated seconds at 60 FPS.
			step := time.Second / 60
			for i := 0; i < 60*10; i++ {
				if err := p.Update(step); err != nil {
					t.Fatalf("frame %d: update err: %v", i, err)
				}
				// Also draw — catches Draw-side panics.
				p.Draw(p.e.Canvas())
				// Sanity: player coords stay finite and on screen.
				if p.player.x < -1 || p.player.x > float64(p.w) {
					t.Fatalf("frame %d: player.x=%v out of range", i, p.player.x)
				}
			}
		})
	}
}

// TestRenderToBuffer makes sure the engine's renderer doesn't choke on
// a galaxian draw call when given a non-tty writer. Catches anything
// the canvas API would reject (e.g. out-of-bounds writes that the
// canvas silently clips but the renderer trips over).
func TestRenderToBuffer(t *testing.T) {
	var buf bytes.Buffer
	e, err := engine.New(engine.Options{Width: 100, Height: 60, Output: &buf})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	s := newScene(e)
	// Two ticks: one on title, one after pressing enter to start.
	if err := s.Update(time.Millisecond * 16); err != nil {
		t.Fatalf("title update: %v", err)
	}
	s.Draw(e.Canvas())

	// Force into play state and tick a bit.
	s.play = newPlayScene(e, 0)
	s.state = statePlay
	for i := 0; i < 30; i++ {
		if err := s.Update(time.Millisecond * 16); err != nil {
			t.Fatalf("play update %d: %v", i, err)
		}
		s.Draw(e.Canvas())
	}
}
