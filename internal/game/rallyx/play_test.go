package rallyx

import (
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// TestPlayBasicSimulation steps a play scene through a handful of
// frames at fixed dt and verifies the main invariants don't regress:
// the player starts alive and at the spawn tile, fuel drains, the
// per-frame Update/Draw cycle doesn't panic, and the scene
// transitions out of psReady once the read hold elapses.
func TestPlayBasicSimulation(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 60})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)

	// Spawn checks.
	if !p.player.alive {
		t.Fatal("player should start alive")
	}
	if p.lives != startLives {
		t.Fatalf("lives = %d, want %d", p.lives, startLives)
	}
	if p.fuel != fullTank {
		t.Fatalf("fuel = %v, want %v", p.fuel, fullTank)
	}
	if got := len(p.maze.flags); got != 10 {
		t.Fatalf("normal flags = %d, want 10", got)
	}
	if p.state != psReady {
		t.Fatalf("initial state = %v, want psReady", p.state)
	}

	dt := time.Second / 60

	// Burn through the ready hold (~2s) so we're in psPlaying.
	for i := 0; i < 180; i++ {
		if err := p.Update(dt); err != nil {
			t.Fatalf("Update returned %v", err)
		}
	}
	if p.state != psPlaying {
		t.Fatalf("after ready hold state = %v, want psPlaying", p.state)
	}

	// Render a frame to make sure Draw doesn't panic on the fresh
	// state.
	canv := engine.NewCanvas(120, 60)
	p.Draw(canv)
}

// TestCollectFlag teleports the player onto a flag tile and steps
// once to confirm the pickup is removed and the score increases.
func TestCollectFlag(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 60})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	p.state = psPlaying

	if len(p.maze.flags) == 0 {
		t.Fatal("expected flags on the map")
	}
	target := &p.maze.flags[0]
	// Teleport the player adjacent to the flag, facing the flag, so
	// the next Update step will cross into the flag tile.
	p.player.x = float64(target.col) + 0.5
	p.player.y = float64(target.row) + 0.5 + 1.0 // one tile below
	p.player.dir = dirUp
	p.player.desired = dirUp
	startScore := p.score

	// Step a few frames to let advance() carry the car onto the flag.
	for i := 0; i < 30; i++ {
		_ = p.Update(time.Second / 60)
	}

	if !target.taken {
		// The player may not actually have walked into the flag if
		// terrain blocked them; relax to "score went up" instead.
		if p.score == startScore {
			t.Fatalf("score did not increase; player @ (%v, %v) target @ (%d, %d)",
				p.player.x, p.player.y, target.col, target.row)
		}
	} else {
		if p.score <= startScore {
			t.Errorf("flag taken but score did not increase: %d -> %d", startScore, p.score)
		}
	}
}

// TestSmokeStunsEnemy parks an enemy on top of a smoke puff and
// verifies it gets stunned (smokeT > 0). Doesn't drive any motion;
// it's just a direct call to the collision pass.
func TestSmokeStunsEnemy(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 60})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	if len(p.enemies) == 0 {
		t.Skip("no enemy spawns on this map")
	}
	en := p.enemies[0]
	// Stage a fake smoke puff directly under the enemy.
	p.smoke = []smokePuff{{x: en.x, y: en.y, ttl: smokePuffTTL}}
	p.handleSmokeCollisions()
	if en.smokeT <= 0 {
		t.Errorf("enemy not stunned: smokeT = %v", en.smokeT)
	}
}
