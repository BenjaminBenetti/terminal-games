package galaxian

import (
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// newTestPlayScene builds a play scene at a deterministic seed so tests
// can assert structure without flake. The engine itself isn't tested
// headlessly — most logic lives on playScene directly.
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
		hiScore: 0,
	}
	p.player.lives = 3
	p.player.nextBonusAt = bonusLifeAt
	p.computeLayout()
	p.spawnStars()
	p.startWave(1)
	return p
}

func TestRegistry(t *testing.T) {
	if _, ok := registry.Get("galaxian"); !ok {
		t.Error("galaxian not registered")
	}
}

func TestLayoutFitsCanvas(t *testing.T) {
	cases := []struct{ w, h int }{
		{80, 48}, {100, 60}, {120, 80}, {160, 100},
	}
	for _, c := range cases {
		p := newTestPlayScene(t, c.w, c.h)
		if p.formationCols < 6 {
			t.Errorf("%dx%d cols=%d want >=6", c.w, c.h, p.formationCols)
		}
		if p.formationRows < 3 {
			t.Errorf("%dx%d rows=%d want >=3", c.w, c.h, p.formationRows)
		}
		// Bottom of deepest formation alien must clear the player.
		bottom := p.formationY0 + (p.formationRows-1)*p.formationRowPitch + flagshipA.height()
		if bottom >= p.playerY {
			t.Errorf("%dx%d formation bottom=%d overlaps playerY=%d",
				c.w, c.h, bottom, p.playerY)
		}
	}
}

func TestStartWaveBuildsFormation(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	if len(p.aliens) == 0 {
		t.Fatal("no aliens spawned")
	}
	// Top row is the flagship sparse row — at most 2 flagships.
	flagships := 0
	for _, a := range p.aliens {
		if a.kind == kindFlagship {
			flagships++
		}
	}
	if flagships < 1 || flagships > 2 {
		t.Errorf("flagship count=%d, want 1 or 2", flagships)
	}
	// Every alien starts in formation and alive.
	for _, a := range p.aliens {
		if !a.alive || a.state != asFormation {
			t.Errorf("alien %+v not in formation state", a)
			break
		}
	}
}

func TestScoreValues(t *testing.T) {
	cases := []struct {
		kind            alienKind
		stationary, div int
	}{
		{kindDrone, 30, 60},
		{kindBee, 40, 80},
		{kindBoss, 50, 100},
		{kindFlagship, 60, 150},
	}
	for _, c := range cases {
		if got := c.kind.stationaryScore(); got != c.stationary {
			t.Errorf("%v stationary=%d want %d", c.kind, got, c.stationary)
		}
		if got := c.kind.divingScore(); got != c.div {
			t.Errorf("%v diving=%d want %d", c.kind, got, c.div)
		}
	}
}

func TestPlayerBulletKillsStationaryAlien(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	// Find a drone — known to exist in any canvas size.
	var target *alien
	for _, a := range p.aliens {
		if a.kind == kindDrone {
			target = a
			break
		}
	}
	if target == nil {
		t.Fatal("no drone found")
	}
	ax, ay := p.slotPos(target.row, target.col)
	p.player.bullet = &bullet{
		x:          float64(ax + 3),
		y:          float64(ay + 3),
		fromPlayer: true,
	}
	p.collidePlayerBullet()
	if target.alive {
		t.Error("drone should be dead")
	}
	if p.score != kindDrone.stationaryScore() {
		t.Errorf("score=%d want %d", p.score, kindDrone.stationaryScore())
	}
	if p.player.bullet != nil {
		t.Error("bullet should have been consumed")
	}
}

func TestPlayerBulletAwardsDivingScoreForDivingAlien(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	var target *alien
	for _, a := range p.aliens {
		if a.kind == kindBee {
			target = a
			break
		}
	}
	if target == nil {
		t.Fatal("no bee found")
	}
	// Force the alien into descent state at known coords.
	target.state = asDescend
	target.x = 30
	target.y = 30
	p.player.bullet = &bullet{
		x:          target.x + 3,
		y:          target.y + 2,
		fromPlayer: true,
	}
	p.collidePlayerBullet()
	if target.alive {
		t.Error("bee should be dead")
	}
	if p.score != kindBee.divingScore() {
		t.Errorf("score=%d want %d (diving)", p.score, kindBee.divingScore())
	}
}

func TestFlagshipConvoyBonus(t *testing.T) {
	p := newTestPlayScene(t, 120, 80)
	// Grab the flagship and two adjacent bosses, set them into a convoy
	// state, then kill the flagship and check the bonus.
	var flag *alien
	for _, a := range p.aliens {
		if a.kind == kindFlagship {
			flag = a
			break
		}
	}
	if flag == nil {
		t.Fatal("no flagship")
	}
	var escorts []*alien
	for _, a := range p.aliens {
		if a.kind == kindBoss && len(escorts) < 2 {
			escorts = append(escorts, a)
		}
	}
	if len(escorts) < 2 {
		t.Fatalf("need at least 2 bosses, got %d", len(escorts))
	}

	cid := 42
	flag.convoyID = cid
	flag.convoyRole = 0
	flag.state = asDescend
	flag.x, flag.y = 40, 20
	for i, e := range escorts {
		e.convoyID = cid
		e.convoyRole = i + 1
		e.state = asDescend
		e.x = 40 + float64(i*5)
		e.y = 22
	}

	p.player.bullet = &bullet{x: flag.x + 4, y: flag.y + 4, fromPlayer: true}
	p.collidePlayerBullet()
	if flag.alive {
		t.Fatal("flagship should be dead")
	}
	// Both escorts alive → 800-point bonus.
	if p.score != flagshipBonusForEscorts[2] {
		t.Errorf("score=%d want %d (flagship + 2 escorts)",
			p.score, flagshipBonusForEscorts[2])
	}
}

func TestAlienBulletKillsPlayer(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	p.alienBullets = []*bullet{{
		x:  p.player.x + 4,
		y:  float64(p.player.y) + 2,
		vy: alienBulletSpeed,
	}}
	livesBefore := p.player.lives
	p.collideAlienBullets()
	if p.player.lives != livesBefore-1 {
		t.Errorf("lives=%d want %d", p.player.lives, livesBefore-1)
	}
	if p.state != psPlayerHit {
		t.Errorf("state=%v want psPlayerHit", p.state)
	}
	if len(p.alienBullets) != 0 {
		t.Errorf("alien bullets=%d want 0", len(p.alienBullets))
	}
}

func TestDiveLifecycle(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	var target *alien
	for _, a := range p.aliens {
		if a.kind == kindDrone {
			target = a
			break
		}
	}
	if target == nil {
		t.Fatal("no drone")
	}
	p.beginDive(target)
	if target.state != asPullout {
		t.Errorf("state=%v want asPullout", target.state)
	}
	// Run enough time for pullout → loop → descent → exit.
	const step = 0.016
	seenStates := map[alienState]bool{}
	for i := 0; i < 600; i++ { // 9.6 seconds max
		seenStates[target.state] = true
		p.tickDivingAlien(target, step)
		if target.state == asExited {
			break
		}
	}
	if !seenStates[asPullout] {
		t.Error("never saw asPullout")
	}
	if !seenStates[asLoop] {
		t.Error("never saw asLoop")
	}
	if !seenStates[asDescend] {
		t.Error("never saw asDescend")
	}
	if target.state != asExited {
		t.Errorf("final state=%v want asExited", target.state)
	}
}

func TestReturningCompletesBackToFormation(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	a := p.aliens[len(p.aliens)/2]
	p.startReturn(a)
	if a.state != asReturning {
		t.Fatalf("state=%v want asReturning", a.state)
	}
	// Step long enough to finish the return curve.
	step := 0.016
	steps := int(returnDur/step) + 10
	for i := 0; i < steps; i++ {
		p.tickDivingAlien(a, step)
	}
	if a.state != asFormation {
		t.Errorf("state=%v want asFormation", a.state)
	}
}

func TestWaveClearsTransition(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	for _, a := range p.aliens {
		a.alive = false
	}
	if err := p.Update(time.Millisecond * 16); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if p.state != psWaveCleared {
		t.Errorf("state=%v want psWaveCleared", p.state)
	}
}

func TestBonusLifeAt7000(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	livesBefore := p.player.lives
	p.score = bonusLifeAt
	if err := p.Update(time.Millisecond); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if p.player.lives != livesBefore+1 {
		t.Errorf("lives=%d want %d after crossing %d points",
			p.player.lives, livesBefore+1, bonusLifeAt)
	}
	if p.player.nextBonusAt != bonusLifeAt*2 {
		t.Errorf("nextBonusAt=%d want %d", p.player.nextBonusAt, bonusLifeAt*2)
	}
}

func TestSpaceFiresBullet(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	if p.player.bullet != nil {
		t.Fatal("expected no bullet at start")
	}
	p.handlePlayKey(engine.Key{Code: engine.KeyChar, Rune: ' '})
	if p.player.bullet == nil {
		t.Error("expected bullet after space")
	}
	// Second space while bullet exists must not replace it (single shot).
	first := p.player.bullet
	p.handlePlayKey(engine.Key{Code: engine.KeyChar, Rune: ' '})
	if p.player.bullet != first {
		t.Error("second space should not replace in-flight bullet")
	}
}

func TestEscQuits(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	p.handlePlayKey(engine.Key{Code: engine.KeyEsc})
	if !p.wantQuit {
		t.Error("expected wantQuit=true after ESC")
	}
}

func TestStarsScrollAndWrap(t *testing.T) {
	p := newTestPlayScene(t, 100, 80)
	if len(p.stars) != numStars {
		t.Errorf("stars=%d want %d", len(p.stars), numStars)
	}
	// Push one star nearly off the bottom; one tick of a big dt should
	// wrap it back to the top with a new x.
	p.stars[0].y = float64(p.h) - 0.5
	p.tickStars(1.0)
	if p.stars[0].y >= float64(p.h) {
		t.Errorf("star didn't wrap; y=%v", p.stars[0].y)
	}
}

func TestLoopMathGoesSouthFirst(t *testing.T) {
	// Both sides of a dive: starting position must move SOUTH (y
	// increases) within the first 1/16 second of the loop.
	for _, side := range []int{-1, +1} {
		p := newTestPlayScene(t, 100, 80)
		a := &alien{kind: kindDrone, state: asLoop, side: side, x: 50, y: 20}
		p.startLoop(a)
		startY := a.y
		p.tickDivingAlien(a, 0.05)
		if a.y <= startY {
			t.Errorf("side=%d: expected loop to descend, y went %v→%v", side, startY, a.y)
		}
	}
}
