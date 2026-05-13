package vanguard

import (
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// newTestPlayScene constructs a play scene without going through the
// engine's input/output setup. The engine itself is not headless-
// testable but the playScene runs purely off Update(dt) and exposes
// its state, so a deterministic seed gives us a reproducible
// simulation we can drive through many frames.
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
		rng:     rand.New(rand.NewSource(1)),
	}
	p.hudH = 4
	p.playTop = p.hudH
	p.playH = p.h - p.hudH - 2
	p.player.lives = 99 // generous so the test rarely game-overs
	p.player.energy = 1.0
	p.spawnStars()
	p.beginZone(0)
	return p
}

// TestPlaySceneSurvivesManyFrames runs the playScene for ~3 minutes of
// simulated time. That's long enough to walk through every zone, get
// to the Gond, run through the death animation, and loop. The point
// is to verify nothing panics during transitions and every state path
// is exercised at least once.
func TestPlaySceneSurvivesManyFrames(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	dt := time.Second / 60
	c := engine.NewCanvas(p.w, p.h)
	for frame := 0; frame < 60*180; frame++ {
		if err := p.Update(dt); err != nil {
			t.Fatalf("frame %d update: %v", frame, err)
		}
		p.Draw(c)
		if p.wantQuit {
			t.Fatalf("frame %d: unexpected wantQuit", frame)
		}
	}
}

// TestZoneOrderHasFiveDistinctZones is a sanity check on the static
// zone table — Vanguard's whole identity is the five-zone loop and
// it'd be easy to accidentally drop one when editing the slice.
func TestZoneOrderHasFiveDistinctZones(t *testing.T) {
	if len(zoneOrder) != 5 {
		t.Fatalf("zoneOrder has %d zones, want 5", len(zoneOrder))
	}
	seen := map[zoneKind]bool{}
	for _, z := range zoneOrder {
		if seen[z.kind] {
			t.Errorf("duplicate zone kind %d", z.kind)
		}
		seen[z.kind] = true
		if z.duration <= 0 {
			t.Errorf("zone %s has non-positive duration %.2f", z.name, z.duration)
		}
		if z.scrollSpd <= 0 {
			t.Errorf("zone %s has non-positive scroll speed %.2f", z.name, z.scrollSpd)
		}
	}
	for _, k := range []zoneKind{zoneMountain, zoneStripe, zoneBleak, zoneRainbow, zoneStyx} {
		if !seen[k] {
			t.Errorf("zone kind %d missing from zoneOrder", k)
		}
	}
}

// TestMountainTerrainNeverFullyCloses spot-checks that the Mountain
// zone's procedural cave never collapses to zero free space, which
// would soft-lock the player (no path through).
func TestMountainTerrainNeverFullyCloses(t *testing.T) {
	playH := 40
	for w := 0; w < 10000; w++ {
		ts := terrainAt(zoneOrder[0], w, 80, playH)
		free := playH - ts.nearH - ts.farH
		if free < 8 {
			t.Fatalf("mountain world=%d only has %d px free (top=%d, bot=%d)",
				w, free, ts.nearH, ts.farH)
		}
	}
}

// TestVertWorldRowIncreasesOverTime verifies the vertical-scroll
// direction: at any given screen row, increasing worldOffI must yield
// a higher world row, so on-screen content flows DOWN toward the
// player at the bottom.
func TestVertWorldRowIncreasesOverTime(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	// Move to a vertical zone (Rainbow).
	p.beginZone(3)
	p.worldOffI = 0
	a := p.vertWorldRow(p.playTop)
	p.worldOffI = 10
	b := p.vertWorldRow(p.playTop)
	if b <= a {
		t.Errorf("vertWorldRow at top: worldOffI=0 → %d, worldOffI=10 → %d (want increase)", a, b)
	}
}
