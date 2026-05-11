package engine_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

type quitScene struct{ updates int }

func (q *quitScene) Update(time.Duration) error {
	q.updates++
	return engine.ErrQuit
}
func (q *quitScene) Draw(*engine.Canvas) {}

func TestRunReturnsNilOnErrQuit(t *testing.T) {
	var buf bytes.Buffer
	e, err := engine.New(engine.Options{Width: 4, Height: 4, Output: &buf})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	scene := &quitScene{}
	if err := e.Run(scene); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if scene.updates != 1 {
		t.Errorf("Update called %d times, want 1", scene.updates)
	}
	out := buf.String()
	if !strings.Contains(out, "\x1b[?1049h") {
		t.Errorf("missing alt-screen enter in %q", out)
	}
	if !strings.Contains(out, "\x1b[?1049l") {
		t.Errorf("missing alt-screen exit in %q", out)
	}
	if !strings.Contains(out, "\x1b[?25l") {
		t.Errorf("missing cursor-hide in %q", out)
	}
	if !strings.Contains(out, "\x1b[?25h") {
		t.Errorf("missing cursor-show in %q", out)
	}
}

type stopScene struct{ e *engine.Engine }

func (s stopScene) Update(time.Duration) error {
	s.e.Stop()
	return nil
}
func (stopScene) Draw(*engine.Canvas) {}

func TestRunReturnsOnStop(t *testing.T) {
	var buf bytes.Buffer
	e, err := engine.New(engine.Options{Width: 4, Height: 4, Output: &buf, TargetFPS: 240})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- e.Run(stopScene{e: e}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after Stop")
	}
}

type boomScene struct{}

var errBoom = errors.New("boom")

func (boomScene) Update(time.Duration) error { return errBoom }
func (boomScene) Draw(*engine.Canvas)        {}

func TestRunPropagatesUpdateError(t *testing.T) {
	var buf bytes.Buffer
	e, _ := engine.New(engine.Options{Width: 4, Height: 4, Output: &buf})
	err := e.Run(boomScene{})
	if !errors.Is(err, errBoom) {
		t.Fatalf("Run err = %v, want %v", err, errBoom)
	}
}

func TestNewDefaults(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 10, Height: 10, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if e.FPS() != engine.DefaultFPS {
		t.Errorf("FPS = %d, want %d", e.FPS(), engine.DefaultFPS)
	}
	if e.Canvas() == nil {
		t.Fatal("Canvas() returned nil")
	}
	if w, h := e.Canvas().Width(), e.Canvas().Height(); w != 10 || h != 10 {
		t.Errorf("canvas = %dx%d, want 10x10", w, h)
	}
}

func TestNewExplicitFPS(t *testing.T) {
	e, _ := engine.New(engine.Options{Width: 4, Height: 4, TargetFPS: 30, Output: &bytes.Buffer{}})
	if e.FPS() != 30 {
		t.Errorf("FPS = %d, want 30", e.FPS())
	}
}

func TestStopBeforeRunExitsImmediately(t *testing.T) {
	var buf bytes.Buffer
	e, _ := engine.New(engine.Options{Width: 4, Height: 4, Output: &buf})
	e.Stop()
	// Use a scene whose Update would block forever if called repeatedly.
	// In practice Run still calls the initial tick once before reading stopCh,
	// so we use a benign scene.
	done := make(chan error, 1)
	go func() { done <- e.Run(stopScene{e: e}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after pre-Run Stop")
	}
}
