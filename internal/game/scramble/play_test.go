package scramble

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// newTestPlayScene constructs a playScene against a fixed-size, headless
// engine so tests can tick frames without touching a real terminal.
func newTestPlayScene(t *testing.T, stage int) (*engine.Engine, *playScene) {
	t.Helper()
	e, err := engine.New(engine.Options{
		Width:  120,
		Height: 60,
		Output: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	// Force the deterministic seed so terrain doesn't accidentally place
	// a mountain peak immediately under the player on first frame.
	p.rng = rand.New(rand.NewSource(1))
	p.beginStage(stage)
	// Skip the stage-intro banner so collision logic runs.
	p.state = psPlaying
	p.stateT = 0
	return e, p
}

// tick advances the scene by dur, broken into 60 FPS sub-frames so the
// physics step is similar to a live run.
func tick(p *playScene, dur time.Duration) {
	step := time.Second / 60
	elapsed := time.Duration(0)
	for elapsed < dur {
		if elapsed+step > dur {
			step = dur - elapsed
		}
		_ = p.Update(step)
		c := p.e.Canvas()
		p.Draw(c)
		elapsed += step
	}
}

// TestPlaySceneRunsAllStagesHeadless walks the scene through every
// stage by force-setting the stage and exercising the simulation for a
// couple of seconds each. We assert no panic and that the score / lives
// invariants stay sane.
func TestPlaySceneRunsAllStagesHeadless(t *testing.T) {
	for stage := 1; stage <= 6; stage++ {
		_, p := newTestPlayScene(t, stage)
		tick(p, 2*time.Second)
		if p.player.lives < -1 {
			t.Errorf("stage %d: lives went deeply negative: %d", stage, p.player.lives)
		}
		if p.score < 0 {
			t.Errorf("stage %d: score went negative: %d", stage, p.score)
		}
	}
}

// TestFuelTankScoring asserts the canonical Scramble distinction:
// bombing a fuel tank both scores and refuels, while shooting it with
// the laser scores less and doesn't refuel.
func TestFuelTankScoring(t *testing.T) {
	_, p := newTestPlayScene(t, 1)
	tank := &entity{kind: entFuel, alive: true}

	preScore := p.score
	preFuel := p.fuel
	p.applyHit(tank, hitKindLaser)
	if p.score-preScore != 100 {
		t.Errorf("laser hit should score +100, got +%d", p.score-preScore)
	}
	if p.fuel != preFuel {
		t.Errorf("laser hit must not refuel: pre=%v post=%v", preFuel, p.fuel)
	}

	// Reset and test bomb.
	tank.alive = true
	p.fuel = 30
	preScore = p.score
	preFuel = p.fuel
	p.applyHit(tank, hitKindBomb)
	if p.score-preScore != 150 {
		t.Errorf("bomb hit should score +150, got +%d", p.score-preScore)
	}
	if p.fuel <= preFuel {
		t.Errorf("bomb hit should refuel: pre=%v post=%v", preFuel, p.fuel)
	}
}

// TestRocketScoringScalesWithLaunchState confirms that destroying a
// rocket while it's still on its pad scores less than catching it
// mid-launch — a small but canonical Scramble detail.
func TestRocketScoringScalesWithLaunchState(t *testing.T) {
	_, p := newTestPlayScene(t, 1)
	idle := &entity{kind: entRocket, alive: true, launched: false}
	flying := &entity{kind: entRocket, alive: true, launched: true}
	pre := p.score
	p.applyHit(idle, hitKindLaser)
	idleScore := p.score - pre
	pre = p.score
	p.applyHit(flying, hitKindLaser)
	flyingScore := p.score - pre
	if idleScore != 50 || flyingScore != 80 {
		t.Errorf("rocket scoring mismatch: idle=%d, flying=%d (want 50, 80)",
			idleScore, flyingScore)
	}
}

// TestLaserCollidesWithEnemyIntegration drives the simulation: a laser
// fired toward a hand-placed UFO at the same y as the player should hit
// it within a second. This exercises the full collision pipeline rather
// than just applyHit.
func TestLaserCollidesWithEnemyIntegration(t *testing.T) {
	_, p := newTestPlayScene(t, 1)
	p.enemies = nil
	// Clear terrain along the bullet's path so the laser doesn't bury
	// itself in a mountain before reaching the target.
	for i := range p.terrain.ground {
		p.terrain.ground[i] = p.pfBot
		p.terrain.ceil[i] = p.pfTop - 1
	}
	// Drop one UFO 50 px in front of the player and aligned vertically
	// with the laser's emission y (player.y + height/2).
	laserY := p.player.y + float64(playerSprite.height())/2
	ufoY := laserY - float64(ufoA.height())/2
	worldX := p.cameraX + p.player.x + 50
	p.enemies = append(p.enemies, &entity{
		kind:  entUFO,
		x:     worldX,
		y:     ufoY,
		alive: true,
	})
	p.tryFireLaser()
	if len(p.bullets) == 0 {
		t.Fatalf("tryFireLaser did not produce a bullet")
	}
	tick(p, 1*time.Second)
	if p.score < 100 {
		t.Errorf("laser should have scored a UFO hit, score=%d", p.score)
	}
}

// TestFuelDrainKillsPlayer asserts the run ends when the fuel reaches
// zero without a refuel.
func TestFuelDrainKillsPlayer(t *testing.T) {
	_, p := newTestPlayScene(t, 1)
	p.enemies = nil // strip everything that might bomb us first
	p.fuel = 0.01
	tick(p, 500*time.Millisecond)
	if p.state == psPlaying {
		t.Fatalf("expected death from fuel drain, state still %v", p.state)
	}
}

// TestReactorDestructionTriggersVictory destroys the stage-6 reactor
// directly via applyHit and confirms the play scene transitions into
// psVictory.
func TestReactorDestructionTriggersVictory(t *testing.T) {
	_, p := newTestPlayScene(t, 6)
	// Find the reactor and ram damage at it until it dies.
	var r *entity
	for _, e := range p.enemies {
		if e.kind == entReactor {
			r = e
			break
		}
	}
	if r == nil {
		t.Fatalf("stage 6 should have spawned a reactor")
	}
	for i := 0; i < 4 && r.alive; i++ {
		p.applyHit(r, hitKindLaser)
	}
	// Need one update tick to pick up state transition.
	tick(p, 100*time.Millisecond)
	if p.state != psVictory {
		t.Errorf("reactor destruction should enter psVictory, got %v", p.state)
	}
}

// TestStageWorldWidthRespectsBounds checks the helper bounds the world
// length so wide-and-narrow terminals still produce a playable stage.
func TestStageWorldWidthRespectsBounds(t *testing.T) {
	if got := stageWorldWidth(1, 40); got < 480 {
		t.Errorf("stage 1 worldW too small: %d", got)
	}
	if got := stageWorldWidth(1, 400); got > 1600 {
		t.Errorf("stage 1 worldW too large: %d", got)
	}
}

// TestTerrainCollisionTopsAndFloors makes sure the hits() helper agrees
// with the ground/ceil heightmap on a simple synthetic terrain.
func TestTerrainCollisionTopsAndFloors(t *testing.T) {
	tr := &terrain{
		ground: make([]int, 20),
		ceil:   make([]int, 20),
		pfTop:  4,
		pfBot:  40,
	}
	for i := range tr.ground {
		tr.ground[i] = 30
		tr.ceil[i] = 6
	}
	if !tr.hits(2, 25, 6, 32) {
		t.Errorf("box overlapping the ground should be flagged as a hit")
	}
	if !tr.hits(2, 4, 6, 7) {
		t.Errorf("box overlapping the ceiling should be flagged as a hit")
	}
	if tr.hits(2, 10, 6, 20) {
		t.Errorf("box well inside the corridor should not hit")
	}
}
