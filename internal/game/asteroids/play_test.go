package asteroids

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// newTestPlayScene constructs a play scene against a fixed-size canvas
// with a deterministic RNG. We never call Engine.Run, so the engine's
// input goroutine and terminal handling never start — perfect for
// driving Update / collision logic in unit tests.
func newTestPlayScene(t *testing.T, w, h int) *playScene {
	t.Helper()
	e, err := engine.New(engine.Options{Width: w, Height: h})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := &playScene{
		e:         e,
		w:         w,
		h:         h,
		lives:     startingLives,
		nextBonus: bonusLifeEvery,
		rng:       rand.New(rand.NewSource(1)),
	}
	p.resetShip(false)
	p.startWave(1)
	return p
}

func TestGameRegistersInRegistry(t *testing.T) {
	if _, ok := registry.Get("asteroids"); !ok {
		t.Error("asteroids game not registered")
	}
}

func TestStartWaveSpawnsLargeAsteroids(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	if got, want := len(p.asteroids), waveStartCount; got != want {
		t.Errorf("wave 1 spawned %d asteroids, want %d", got, want)
	}
	for i, a := range p.asteroids {
		if a.size != sizeLarge {
			t.Errorf("asteroid %d size=%d, want %d (large)", i, a.size, sizeLarge)
		}
	}
}

func TestWaveCountClimbsThenCaps(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	p.startWave(2)
	if got, want := len(p.asteroids), waveStartCount+waveExtraPerWave; got != want {
		t.Errorf("wave 2 count=%d, want %d", got, want)
	}
	// Beyond the cap, count should stop growing.
	p.startWave(20)
	if got := len(p.asteroids); got != waveMaxCount {
		t.Errorf("wave 20 count=%d, want cap %d", got, waveMaxCount)
	}
}

func TestAsteroidScoresMatchOriginal(t *testing.T) {
	cases := []struct {
		size int
		want int
	}{
		{sizeLarge, 20},
		{sizeMedium, 50},
		{sizeSmall, 100},
	}
	for _, c := range cases {
		if got := asteroidScores[c.size]; got != c.want {
			t.Errorf("score for size %d = %d, want %d", c.size, got, c.want)
		}
	}
}

func TestPlayerBulletKillsAndSplitsLargeAsteroid(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	// Pick the first asteroid and aim a bullet at it.
	a := p.asteroids[0]
	startCount := len(p.asteroids)
	p.bullets = append(p.bullets, &bullet{
		x: a.x, y: a.y, vx: 0, vy: 0, life: 1, fromPlayer: true,
	})
	p.resolveBulletCollisions()
	// Bullet consumed.
	if len(p.bullets) != 0 {
		t.Errorf("bullets=%d, want 0", len(p.bullets))
	}
	// Score awarded.
	if p.score != 20 {
		t.Errorf("score=%d, want 20", p.score)
	}
	// Large asteroid splits into two mediums, so total goes from N to N+1.
	if got, want := len(p.asteroids), startCount+1; got != want {
		t.Errorf("asteroid count after split=%d, want %d", got, want)
	}
	// Children are mediums.
	mediums := 0
	for _, x := range p.asteroids {
		if x.size == sizeMedium {
			mediums++
		}
	}
	if mediums < 2 {
		t.Errorf("medium children=%d, want >=2", mediums)
	}
}

func TestSmallAsteroidIsRemovedWithoutSplit(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	p.asteroids = []*asteroid{newAsteroid(p.rng, 40, 40, sizeSmall)}
	p.bullets = append(p.bullets, &bullet{
		x: 40, y: 40, vx: 0, vy: 0, life: 1, fromPlayer: true,
	})
	p.resolveBulletCollisions()
	if len(p.asteroids) != 0 {
		t.Errorf("expected small asteroid to be cleanly removed; got %d", len(p.asteroids))
	}
	if p.score != 100 {
		t.Errorf("small asteroid score=%d, want 100", p.score)
	}
}

func TestSaucerBulletDoesNotScore(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	a := p.asteroids[0]
	startScore := p.score
	p.bullets = append(p.bullets, &bullet{
		x: a.x, y: a.y, vx: 0, vy: 0, life: 1, fromPlayer: false,
	})
	p.resolveBulletCollisions()
	if p.score != startScore {
		t.Errorf("score changed on saucer bullet kill: got %d, want %d", p.score, startScore)
	}
}

func TestShipHitByAsteroidLosesLife(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	// Place an asteroid directly on the ship.
	p.asteroids = []*asteroid{newAsteroid(p.rng, p.ship.x, p.ship.y, sizeLarge)}
	livesBefore := p.lives
	p.resolveCollisions()
	if p.lives != livesBefore-1 {
		t.Errorf("lives=%d, want %d", p.lives, livesBefore-1)
	}
	if p.state != psShipDying {
		t.Errorf("state=%v, want psShipDying", p.state)
	}
	if p.ship.alive {
		t.Error("ship should be marked dead after fatal collision")
	}
}

func TestInvulnerableShipSurvivesAsteroidContact(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	p.ship.invul = 2.0
	p.asteroids = []*asteroid{newAsteroid(p.rng, p.ship.x, p.ship.y, sizeLarge)}
	livesBefore := p.lives
	p.resolveCollisions()
	if p.lives != livesBefore {
		t.Errorf("invulnerable ship lost a life; lives=%d, want %d", p.lives, livesBefore)
	}
	if !p.ship.alive {
		t.Error("invulnerable ship should still be alive")
	}
}

func TestPlayerBulletLimitEnforced(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	for i := 0; i < shipMaxBullets+3; i++ {
		p.ship.cooldown = 0
		p.tryFire()
	}
	got := countPlayerBullets(p.bullets)
	if got != shipMaxBullets {
		t.Errorf("player bullets in flight=%d, want %d", got, shipMaxBullets)
	}
}

func TestBulletLifetimeExpires(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	p.bullets = []*bullet{{x: 10, y: 10, vx: 0, vy: 0, life: 0.1, fromPlayer: true}}
	p.tickBullets(0.2)
	if len(p.bullets) != 0 {
		t.Errorf("expected expired bullet to be removed; got %d", len(p.bullets))
	}
}

func TestWrapAroundCoordinates(t *testing.T) {
	if got, want := wrapF(-1, 10), 9.0; got != want {
		t.Errorf("wrapF(-1, 10)=%v, want %v", got, want)
	}
	if got, want := wrapF(11, 10), 1.0; got != want {
		t.Errorf("wrapF(11, 10)=%v, want %v", got, want)
	}
	if got, want := wrapF(5, 10), 5.0; got != want {
		t.Errorf("wrapF(5, 10)=%v, want %v", got, want)
	}
}

func TestWrapDeltaPicksShortestPath(t *testing.T) {
	// On a torus of length 10, the shortest hop from 9 to 1 is +2, not -8.
	if got := wrapDelta(9-1, 10); math.Abs(got - -2) > 1e-9 {
		t.Errorf("wrapDelta(8, 10)=%v, want -2", got)
	}
	if got := wrapDelta(1-9, 10); math.Abs(got-2) > 1e-9 {
		t.Errorf("wrapDelta(-8, 10)=%v, want 2", got)
	}
}

func TestToroidalCollisionAcrossEdge(t *testing.T) {
	p := newTestPlayScene(t, 100, 100)
	// Two objects on opposite sides of the wrap should still collide if
	// their wrapped distance is small.
	if !p.circlesOverlap(2, 50, 3, 99, 50, 3) {
		t.Error("expected wrap-aware collision between x=2 and x=99 at radius 3")
	}
}

func TestBonusLifeAwardedEveryThreshold(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	startLives := p.lives
	p.addScore(bonusLifeEvery)
	if p.lives != startLives+1 {
		t.Errorf("expected bonus life at %d; lives=%d, want %d", bonusLifeEvery, p.lives, startLives+1)
	}
	// Two thresholds in one big award also count.
	p.addScore(bonusLifeEvery * 2)
	if p.lives != startLives+3 {
		t.Errorf("expected two more bonus lives; lives=%d, want %d", p.lives, startLives+3)
	}
}

func TestShipBulletInheritsShipVelocity(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	p.ship.vx = 25
	p.ship.vy = 0
	p.ship.angle = 0 // facing +X
	p.tryFire()
	if len(p.bullets) != 1 {
		t.Fatalf("bullets=%d, want 1", len(p.bullets))
	}
	b := p.bullets[0]
	wantVX := p.ship.vx + shipBulletSpeed
	if math.Abs(b.vx-wantVX) > 1e-6 {
		t.Errorf("bullet vx=%v, want %v (ship vx + bullet speed)", b.vx, wantVX)
	}
}

func TestSplitAsteroidProducesTwoChildren(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	p.asteroids = []*asteroid{newAsteroid(p.rng, 50, 50, sizeLarge)}
	p.splitAsteroid(p.asteroids[0])
	if len(p.asteroids) != 2 {
		t.Errorf("after splitting large asteroid, count=%d, want 2", len(p.asteroids))
	}
	for _, a := range p.asteroids {
		if a.size != sizeMedium {
			t.Errorf("child size=%d, want %d (medium)", a.size, sizeMedium)
		}
	}
}

func TestSplitMediumProducesTwoSmall(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	p.asteroids = []*asteroid{newAsteroid(p.rng, 50, 50, sizeMedium)}
	p.splitAsteroid(p.asteroids[0])
	if len(p.asteroids) != 2 {
		t.Errorf("after splitting medium, count=%d, want 2", len(p.asteroids))
	}
	for _, a := range p.asteroids {
		if a.size != sizeSmall {
			t.Errorf("child size=%d, want %d (small)", a.size, sizeSmall)
		}
	}
}

func TestWaveClearedTransition(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	p.asteroids = nil
	p.saucer = nil
	if err := p.Update(time.Millisecond * 16); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if p.state != psWaveCleared {
		t.Errorf("state=%v, want psWaveCleared", p.state)
	}
}

func TestNextWaveStartsAfterDelay(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	p.asteroids = nil
	p.saucer = nil
	startWave := p.wave
	// First tick transitions to cleared.
	_ = p.Update(time.Millisecond * 16)
	// Wait out the wave-clear delay.
	_ = p.Update(time.Duration(waveClearedDelay*float64(time.Second)) + time.Second/30)
	if p.wave != startWave+1 {
		t.Errorf("wave=%d, want %d after clear", p.wave, startWave+1)
	}
	if p.state != psPlaying {
		t.Errorf("state=%v after new wave start, want psPlaying", p.state)
	}
}

func TestHyperspaceMovesShipOrKillsIt(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	startX, startY := p.ship.x, p.ship.y
	startLives := p.lives
	// Force several hyperspace jumps; under deterministic seed 1, at least
	// one must either teleport or destroy the ship.
	moved := false
	died := false
	for i := 0; i < 30; i++ {
		p.ship.hyperCool = 0
		p.tryHyperspace()
		if !p.ship.alive {
			died = true
			break
		}
		if p.ship.x != startX || p.ship.y != startY {
			moved = true
			break
		}
	}
	if !moved && !died {
		t.Error("hyperspace produced no movement and no destruction across 30 attempts")
	}
	if died && p.lives != startLives-1 {
		t.Errorf("hyperspace death didn't burn a life; lives=%d, want %d", p.lives, startLives-1)
	}
}

func TestEscQuitsToTitle(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	p.handlePlayKey(engine.Key{Code: engine.KeyEsc})
	if !p.wantQuit {
		t.Error("expected wantQuit=true after ESC")
	}
}

func TestSpaceFiresBulletDuringPlay(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	if len(p.bullets) != 0 {
		t.Fatal("expected no bullets initially")
	}
	p.handlePlayKey(engine.Key{Code: engine.KeyChar, Rune: ' '})
	if countPlayerBullets(p.bullets) == 0 {
		t.Error("expected at least one player bullet after space press")
	}
}

func TestCenterClearReportsBlockedByAsteroid(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	// Place an asteroid at the centre.
	p.asteroids = []*asteroid{newAsteroid(p.rng, float64(p.w)/2, float64(p.h)/2, sizeLarge)}
	if p.centerClear() {
		t.Error("expected centerClear to be false when an asteroid sits on the centre")
	}
	// Move the asteroid far away.
	p.asteroids[0].x = 5
	p.asteroids[0].y = 5
	if !p.centerClear() {
		t.Error("expected centerClear to be true with no asteroid near centre")
	}
}
