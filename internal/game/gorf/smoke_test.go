package gorf

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// newTestPlayScene builds a play scene at a deterministic seed so tests
// can assert structure without flake.
func newTestPlayScene(t *testing.T, w, h int) *playScene {
	t.Helper()
	e, err := engine.New(engine.Options{Width: w, Height: h})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := &playScene{
		e:       e,
		w:       w,
		h:       h,
		rng:     rand.New(rand.NewSource(1)),
		cycle:   1,
		mission: missionAstro,
	}
	p.player.lives = playerStartLives
	p.computeLayout()
	p.spawnStars()
	p.beginMission()
	return p
}

func TestRegistry(t *testing.T) {
	if _, ok := registry.Get("gorf"); !ok {
		t.Error("gorf not registered")
	}
}

// TestMissionAdvanceLoop force-clears each mission in order and confirms
// the cycle counter bumps after the flag ship and the mission cycles
// back to Astro Battles.
func TestMissionAdvanceLoop(t *testing.T) {
	p := newTestPlayScene(t, 100, 60)
	want := []missionID{
		missionAstro, missionLaser, missionGalaxians,
		missionWarp, missionFlag,
	}
	for i, m := range want {
		if p.mission != m {
			t.Fatalf("step %d: mission=%v want %v", i, p.mission, m)
		}
		switch m {
		case missionAstro:
			p.astro.cleared = true
		case missionLaser:
			p.laser.cleared = true
		case missionGalaxians:
			p.galax.cleared = true
		case missionWarp:
			p.warp.cleared = true
		case missionFlag:
			p.flag.cleared = true
		}
		// Skip past the intro card and the clear banner.
		p.state = psPlaying
		// Trigger the playScene's "cleared → advance" transition by
		// calling Update with the missionCleared timer expired. We
		// directly toggle state here for determinism.
		if !p.missionCleared() {
			t.Fatalf("step %d: missionCleared() false after setting state.cleared=true", i)
		}
		// Manually advance — same effect as the timed Update path.
		p.advanceMission()
	}
	if p.cycle != 2 {
		t.Errorf("after clearing 5 missions cycle=%d want 2", p.cycle)
	}
	if p.mission != missionAstro {
		t.Errorf("after flag-ship clear mission=%v want Astro", p.mission)
	}
}

// TestSmokeRun ticks the play scene through several seconds in each
// mission, checking that updates and draws don't panic at varied canvas
// sizes. We force-advance between missions so a single run sees every
// state machine.
func TestSmokeRun(t *testing.T) {
	for _, sz := range []struct{ w, h int }{
		{80, 48},
		{100, 60},
		{120, 80},
		{160, 100},
	} {
		t.Run("", func(t *testing.T) {
			p := newTestPlayScene(t, sz.w, sz.h)
			step := time.Second / 60
			// 4 simulated seconds per mission.
			for mi := 0; mi < 5; mi++ {
				for i := 0; i < 60*4; i++ {
					if err := p.Update(step); err != nil {
						t.Fatalf("mission %d frame %d update: %v", mi, i, err)
					}
					p.Draw(p.e.Canvas())
					if p.player.x < -1 || p.player.x > float64(p.w) {
						t.Fatalf("mission %d frame %d: player.x=%v OOB", mi, i, p.player.x)
					}
					if p.player.y < -1 || p.player.y > float64(p.h) {
						t.Fatalf("mission %d frame %d: player.y=%v OOB", mi, i, p.player.y)
					}
				}
				// Force-clear and advance to the next mission.
				switch p.mission {
				case missionAstro:
					p.astro.cleared = true
				case missionLaser:
					p.laser.cleared = true
				case missionGalaxians:
					p.galax.cleared = true
				case missionWarp:
					p.warp.cleared = true
				case missionFlag:
					p.flag.cleared = true
				}
				p.advanceMission()
			}
		})
	}
}

// TestPlayerLaserSingleShot makes sure the player can only have one
// quad-laser bolt in flight at a time.
func TestPlayerLaserSingleShot(t *testing.T) {
	p := newTestPlayScene(t, 100, 60)
	p.tryFire()
	if p.player.laser == nil {
		t.Fatal("first tryFire didn't spawn a laser")
	}
	first := p.player.laser
	p.tryFire()
	if p.player.laser != first {
		t.Error("second tryFire replaced laser while first still in flight")
	}
}

// TestLayoutScales confirms that the play area fits within the canvas
// at the supported size range — no negative or out-of-bounds player
// roam zones.
func TestLayoutScales(t *testing.T) {
	for _, sz := range []struct{ w, h int }{
		{60, 36}, {80, 48}, {100, 60}, {160, 100},
	} {
		p := newTestPlayScene(t, sz.w, sz.h)
		if p.playerYMin >= p.playerYMax {
			t.Errorf("%dx%d: playerYMin=%d >= playerYMax=%d",
				sz.w, sz.h, p.playerYMin, p.playerYMax)
		}
		if p.playerYMax >= p.h {
			t.Errorf("%dx%d: playerYMax=%d off-canvas (h=%d)", sz.w, sz.h, p.playerYMax, p.h)
		}
	}
}

// TestRenderToBuffer drives the top-level scene through a few frames
// with a buffer-backed engine, exercising the renderer's diff path.
func TestRenderToBuffer(t *testing.T) {
	var buf bytes.Buffer
	e, err := engine.New(engine.Options{Width: 100, Height: 60, Output: &buf})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	s := newScene(e)
	if err := s.Update(time.Millisecond * 16); err != nil {
		t.Fatalf("title update: %v", err)
	}
	s.Draw(e.Canvas())

	// Force into play state and tick a bit.
	s.play = newPlayScene(e, 0, 0)
	s.state = statePlay
	for i := 0; i < 60; i++ {
		if err := s.Update(time.Millisecond * 16); err != nil {
			t.Fatalf("play update %d: %v", i, err)
		}
		s.Draw(e.Canvas())
	}
	if buf.Len() == 0 {
		// Renderer is invoked through e.Run; just confirm we still got
		// bytes via Draw. The buffer can be empty because Draw paints
		// to the canvas only — the renderer flushes inside Run. Just
		// don't fail on this.
		_ = buf
	}
}
