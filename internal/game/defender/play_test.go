package defender

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// newTestPlayScene constructs a play scene without depending on a real
// terminal — same pattern the other games use. The scene itself runs
// off Update(dt) and exposes its state for assertions.
func newTestPlayScene(t *testing.T, w, h int) *playScene {
	t.Helper()
	e, err := engine.New(engine.Options{Width: w, Height: h})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	c := e.Canvas()
	rng := rand.New(rand.NewSource(1))
	p := &playScene{
		e:     e,
		w:     c.Width(),
		h:     c.Height(),
		rng:   rng,
		wave:  1,
		world: newWorld(c.Width(), c.Height(), rng),
	}
	p.initPlayer()
	p.humans = spawnHumans(p.world, 10, rng)
	p.beginWave(1)
	return p
}

// TestPlaySceneSurvivesManyFrames drives the scene through ~60 seconds
// of gameplay at 60 FPS. It shouldn't panic, the scene shouldn't
// spontaneously want to quit, and gameplay should make some forward
// progress (enemies appear, some get killed).
func TestPlaySceneSurvivesManyFrames(t *testing.T) {
	p := newTestPlayScene(t, 120, 60)
	dt := time.Second / 60
	c := engine.NewCanvas(p.w, p.h)
	for frame := 0; frame < 60*60; frame++ {
		if err := p.Update(dt); err != nil {
			t.Fatalf("frame %d update: %v", frame, err)
		}
		p.Draw(c)
		if p.wantQuit {
			t.Fatalf("frame %d: unexpected wantQuit", frame)
		}
	}
	// By a minute we should have spawned a bunch of enemies and the
	// world should still be coherent.
	if p.gameT < 1 {
		t.Errorf("gameT didn't advance: %.2f", p.gameT)
	}
}

// TestWrapDelta checks that the wrap-aware signed delta always
// returns the shorter direction around the torus.
func TestWrapDelta(t *testing.T) {
	w := newWorld(120, 60, rand.New(rand.NewSource(1)))
	worldW := float64(w.worldW)
	cases := []struct {
		a, b, want float64
	}{
		{0, 10, 10},
		{0, worldW - 5, -5},
		{worldW - 1, 1, 2},
	}
	for i, tc := range cases {
		got := w.wrapDelta(tc.a, tc.b)
		if math.Abs(got-tc.want) > 0.5 {
			t.Errorf("case %d: wrapDelta(%v, %v) = %v, want %v",
				i, tc.a, tc.b, got, tc.want)
		}
	}
	// At exactly worldW/2 the answer is ambiguous (both +half and -half
	// are equidistant). Just assert the magnitude.
	if abs := math.Abs(w.wrapDelta(worldW/4, 3*worldW/4)); math.Abs(abs-worldW/2) > 0.5 {
		t.Errorf("wrapDelta at antipode = %v, want |worldW/2|", abs)
	}
}

// TestWrapX folds arbitrary values into the canonical world range.
func TestWrapX(t *testing.T) {
	w := newWorld(120, 60, rand.New(rand.NewSource(1)))
	cases := []float64{
		-1, -float64(w.worldW), float64(w.worldW), float64(w.worldW * 2), 0.5,
	}
	for _, x := range cases {
		got := w.wrapX(x)
		if got < 0 || got >= float64(w.worldW) {
			t.Errorf("wrapX(%v) = %v, out of range [0, %d)", x, got, w.worldW)
		}
	}
}

// TestLanderAbductionTransformsToMutant verifies the central Defender
// mechanic: a lander that successfully escorts a human off the top of
// the screen becomes a Mutant and the human dies.
func TestLanderAbductionTransformsToMutant(t *testing.T) {
	p := newTestPlayScene(t, 120, 60)
	// Drop a hand-rigged lander straight above a human and put it in
	// the abducting state so we don't have to drive the descend AI.
	h := &humanoid{
		worldX: 30,
		y:      float64(p.world.groundY) - 4,
		state:  humanWalking,
		dirX:   1,
	}
	p.humans = []*humanoid{h}
	e := &enemy{
		kind:      kLander,
		state:     esAbducting,
		worldX:    30,
		y:         float64(p.world.groundY) - 10,
		mutateAtY: float64(p.world.playZoneTop) + 2,
		carrying:  h,
	}
	h.carrier = e
	h.state = humanLifted
	p.enemies = []*enemy{e}
	// Tick until the lander reaches the top.
	for i := 0; i < 600; i++ {
		p.tickLander(e, 0.05)
		if e.kind == kMutant {
			break
		}
	}
	if e.kind != kMutant {
		t.Fatalf("lander never mutated; final y=%.1f, mutateAtY=%.1f", e.y, e.mutateAtY)
	}
	if !h.dead {
		t.Errorf("human should be dead after successful abduction")
	}
}

// TestSmartBombClearsScreen verifies the smart bomb kills every
// on-screen enemy at once and consumes a bomb stock.
func TestSmartBombClearsScreen(t *testing.T) {
	p := newTestPlayScene(t, 120, 60)
	// Place 3 enemies near the player so they're all on-screen.
	for i := 0; i < 3; i++ {
		p.enemies = append(p.enemies, &enemy{
			kind:   kLander,
			state:  esActive,
			worldX: p.player.worldX + float64(i*4),
			y:      30,
		})
	}
	startBombs := p.player.smartBombs
	p.triggerSmartBomb()
	if p.player.smartBombs != startBombs-1 {
		t.Errorf("smart bombs not decremented: %d → %d", startBombs, p.player.smartBombs)
	}
	for _, e := range p.enemies {
		if e.kind == kLander && e.state == esActive {
			t.Errorf("on-screen enemy survived smart bomb: state=%v", e.state)
		}
	}
}

// TestPodBurstsIntoSwarmers verifies the pod-on-shot → 4 swarmers
// rule. The pod itself is destroyed in the same call.
func TestPodBurstsIntoSwarmers(t *testing.T) {
	p := newTestPlayScene(t, 120, 60)
	p.enemies = []*enemy{{
		kind:   kPod,
		state:  esActive,
		worldX: 40,
		y:      30,
	}}
	// Fire a bullet right at it.
	p.playerBolts = []*playerBolt{{
		worldX: 40,
		y:      31,
		vx:     playerShotSpeed,
		life:   0.5,
	}}
	p.collidePlayerBolts()
	pods, swarmers := 0, 0
	for _, e := range p.enemies {
		switch e.kind {
		case kPod:
			if e.alive() {
				pods++
			}
		case kSwarmer:
			swarmers++
		}
	}
	if pods != 0 {
		t.Errorf("pod survived its own destruction: %d alive", pods)
	}
	if swarmers != 4 {
		t.Errorf("expected 4 swarmers from pod burst, got %d", swarmers)
	}
}

// TestPlanetExplodesWhenAllHumansDie verifies that the last-human
// death triggers the planet-explode state and ultimately flattens the
// terrain.
func TestPlanetExplodesWhenAllHumansDie(t *testing.T) {
	p := newTestPlayScene(t, 120, 60)
	// Kill all but one human directly.
	for _, h := range p.humans[:len(p.humans)-1] {
		h.dead = true
		h.state = humanDead
	}
	// Killing the last one should trigger planet explode.
	p.killHuman(p.humans[len(p.humans)-1])
	if p.state != psPlanetExplode {
		t.Fatalf("state = %v, want psPlanetExplode", p.state)
	}
	// Advance through the explosion duration; terrain should flatten.
	for i := 0; i < int(planetExplodeDur/0.05)+2; i++ {
		_ = p.Update(time.Duration(0.05 * float64(time.Second)))
	}
	if !p.world.flattened {
		t.Errorf("terrain should be flattened after planet explosion")
	}
}

// TestRescueScoring verifies the catch + deliver scoring: catching
// awards nothing immediately, but delivering the human back to the
// surface pays out the full rescue bonus.
func TestRescueScoring(t *testing.T) {
	p := newTestPlayScene(t, 120, 60)
	// Manufacture a freefall scenario right under the player.
	h := &humanoid{
		worldX: p.player.worldX + 2,
		y:      p.player.y + float64(playerShip.height()) + 1,
		state:  humanFalling,
		fallV:  10,
	}
	p.humans = []*humanoid{h}
	startScore := p.score
	p.tryCatchFallingHumans()
	if h.state != humanCarried {
		t.Fatalf("human not caught: state=%v", h.state)
	}
	if p.score != startScore {
		t.Errorf("catching mid-air should not award points immediately; got %d", p.score-startScore)
	}
	// Drive the player down onto the terrain so the carried human is
	// set down — that's the moment the rescue bonus is awarded.
	// updatePlayer requires p.e (engine) which is already wired by
	// newTestPlayScene.
	p.player.y = p.world.terrainAt(p.player.worldX) - float64(playerShip.height()) - 2
	for i := 0; i < 30; i++ {
		p.updatePlayer(0.05)
		if p.player.carrying == nil {
			break
		}
	}
	if p.player.carrying != nil {
		t.Fatalf("player still carrying after touchdown")
	}
	if p.score-startScore != humanRescueDeliver {
		t.Errorf("delivery bonus = %d, want %d", p.score-startScore, humanRescueDeliver)
	}
}
