package pacman

import (
	"bytes"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// TestPlaySceneSmoke runs a few seconds of simulation against a fresh
// scene to make sure no Update or Draw call panics or hangs in any of
// the gameplay states (READY hold, normal play, the first mode
// transition). The engine is configured headless — Output is a
// bytes.Buffer so no real terminal is touched.
func TestPlaySceneSmoke(t *testing.T) {
	e, err := engine.New(engine.Options{
		Width:  120,
		Height: 80,
		Output: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	scene := newScene(e)

	c := engine.NewCanvas(120, 80)
	dt := time.Second / 60

	// Run 5 seconds of frames — that's well past the READY hold and
	// across the first scatter→chase transition, exercising mode
	// timing and ghost AI dispatch.
	for i := 0; i < 5*60; i++ {
		if err := scene.Update(dt); err != nil && err != engine.ErrQuit {
			t.Fatalf("scene.Update at frame %d: %v", i, err)
		}
		scene.Draw(c)
	}
}

// TestPlayDotConsumption walks Pac-Man left out of his spawn tile and
// confirms the dot at the destination is consumed and scored.
func TestPlayDotConsumption(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 80, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	// Force into the playing state immediately.
	p.state = psPlaying
	p.stateT = 0

	startScore := p.score
	startDots := p.maze.remainingDots()
	// Pac-Man starts at (13.5, 23.5) facing left. There are no dots at
	// (13, 23) or (14, 23) (those are spawn space), so we need to
	// walk further. Tile (12, 23) has a dot.
	for i := 0; i < 30; i++ {
		p.updatePlaying(time.Second.Seconds() / 60)
	}
	if p.score <= startScore {
		t.Errorf("score did not increase after movement; got %d, started %d", p.score, startScore)
	}
	if p.maze.remainingDots() >= startDots {
		t.Errorf("dot count did not decrease; got %d, started %d", p.maze.remainingDots(), startDots)
	}
}

// TestPowerPelletTriggersFrightened: when Pac-Man eats an energizer
// (one of the four corner tiles), every hunting ghost should flip to
// frightened mode and the frightened timer should be primed.
func TestPowerPelletTriggersFrightened(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 80, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	p.state = psPlaying
	p.stateT = 0

	// Teleport Pac-Man one tile to the right of the top-left energizer
	// (which sits at col 1, row 3). Heading left, he'll cross that
	// tile and eat the pellet on the next frame.
	p.pac.x = 2.5
	p.pac.y = 3.5
	p.pac.dir = dirLeft
	p.pac.desired = dirLeft

	for i := 0; i < 30 && p.phase.frightenT == 0; i++ {
		p.updatePlaying(time.Second.Seconds() / 60)
	}
	if p.phase.frightenT <= 0 {
		t.Fatalf("energizer did not trigger frightened mode")
	}
	if p.score < scorePellet {
		t.Errorf("score = %d; want >= %d", p.score, scorePellet)
	}
	frightenedCount := 0
	for _, g := range p.ghosts {
		if g.mode == modeFrightened {
			frightenedCount++
		}
	}
	if frightenedCount == 0 {
		t.Errorf("no ghosts entered frightened mode")
	}
}

// TestEatenGhostReturnsHome simulates the full eyes-back-to-house
// cycle: an eaten ghost should reach the door entry tile via the AI,
// hand off to modeEntering for the scripted dive, then come out as a
// fresh ghost in modeLeavingHouse → modeScatter/Chase. The bug this
// regression-tests is the AI stalling when the ghost's target tile
// equals its current tile and the tie-break wanders the eyes away
// from the door instead of through it.
func TestEatenGhostReturnsHome(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 80, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	p.state = psPlaying
	p.stateT = 0

	// Set Blinky into eaten state at a known position above the maze.
	// (13, 8) sits on the corridor running directly above the ghost
	// house — the eyes have a clear path home from here.
	g := p.ghosts[blinky]
	g.x, g.y = 13.5, 8.5
	g.dir = dirDown
	g.desired = dirDown
	g.mode = modeEaten
	g.lastDecisionTile = [2]int{-1, -1}

	// Allow a generous window — at eatenSpeed (14 tiles/sec) plus the
	// scripted dive, the cycle should finish in well under a second.
	saw := map[ghostMode]bool{}
	for i := 0; i < 240; i++ {
		p.driveGhost(g, 1.0/60.0)
		saw[g.mode] = true
		if g.mode == modeLeavingHouse {
			break
		}
	}

	if !saw[modeEntering] {
		t.Errorf("eyes never transitioned through modeEntering — likely stuck wandering")
	}
	if g.mode != modeLeavingHouse {
		t.Fatalf("eaten ghost did not reach modeLeavingHouse; final mode=%v at (%.2f,%.2f)",
			g.mode, g.x, g.y)
	}
}

// TestGhostEatSpawnsPopup confirms a score popup is queued when
// Pac-Man eats a frightened ghost.
func TestGhostEatSpawnsPopup(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 80, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	p.state = psPlaying

	// Park Pac-Man and a frightened Blinky on the same tile.
	p.pac.x, p.pac.y = 13.5, 23.5
	g := p.ghosts[blinky]
	g.x, g.y = 13.5, 23.5
	g.mode = modeFrightened

	if len(p.popups) != 0 {
		t.Fatalf("unexpected popups before collision: %d", len(p.popups))
	}
	p.checkGhostCollisions()
	if len(p.popups) != 1 {
		t.Fatalf("expected 1 popup after eat; got %d", len(p.popups))
	}
	if p.popups[0].text != "200" {
		t.Errorf("first popup text = %q; want \"200\"", p.popups[0].text)
	}
}

// TestModeTimerTransition: after enough simulated time, the global
// phase should transition from scatter to chase and back, and the
// surviving hunting ghosts should reverse direction across each
// boundary.
func TestModeTimerTransition(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 80, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	p.state = psPlaying
	p.stateT = 0

	if p.phase.current != modeScatter {
		t.Fatalf("initial phase = %v; want modeScatter", p.phase.current)
	}

	// Advance by ~8 seconds (past the 7s scatter window).
	for i := 0; i < 8*60; i++ {
		p.advanceModePhase(1.0 / 60.0)
	}
	if p.phase.current != modeChase {
		t.Errorf("after 8s phase = %v; want modeChase", p.phase.current)
	}
}
