package spaceinvaders

import (
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// newTestPlayScene constructs a play scene without going through the
// engine — the engine isn't testable headlessly anyway, and most of
// the logic operates on the playScene struct directly. This lets us
// drive Update at known dt values and inspect resulting state.
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
	p.computeLayout()
	p.startWave(1, true)
	return p
}

func TestStartWaveBuildsFormation(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	if p.grid.rows != 5 {
		t.Errorf("rows=%d, want 5", p.grid.rows)
	}
	if p.grid.cols < 4 || p.grid.cols > 11 {
		t.Errorf("cols=%d, want in [4,11]", p.grid.cols)
	}
	if p.grid.alive != p.grid.rows*p.grid.cols {
		t.Errorf("alive=%d, want %d", p.grid.alive, p.grid.rows*p.grid.cols)
	}
	if p.grid.total != p.grid.alive {
		t.Errorf("total=%d, want %d", p.grid.total, p.grid.alive)
	}
	// Row 0 should be top kind, rows 1-2 mid, rows 3-4 bottom.
	if p.grid.cells[0][0].kind != alienTopKind {
		t.Errorf("row 0 kind=%v, want alienTopKind", p.grid.cells[0][0].kind)
	}
	if p.grid.cells[2][0].kind != alienMidKind {
		t.Errorf("row 2 kind=%v, want alienMidKind", p.grid.cells[2][0].kind)
	}
	if p.grid.cells[4][0].kind != alienBotKind {
		t.Errorf("row 4 kind=%v, want alienBotKind", p.grid.cells[4][0].kind)
	}
}

func TestAlienScoreValues(t *testing.T) {
	cases := []struct {
		kind alienKind
		want int
	}{
		{alienTopKind, 30},
		{alienMidKind, 20},
		{alienBotKind, 10},
	}
	for _, c := range cases {
		if got := c.kind.score(); got != c.want {
			t.Errorf("kind %v score=%d, want %d", c.kind, got, c.want)
		}
	}
}

func TestPlayerBulletKillsAlienAndScores(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	// Aim a bullet at the top-left alien.
	ax, ay := p.grid.alienPos(0, 0)
	p.player.bullet = &bullet{
		x:          float64(ax + p.grid.spriteW/2),
		y:          float64(ay),
		vy:         0, // we'll just resolve a collision in place
		fromPlayer: true,
	}
	p.resolveCollisions()
	if p.grid.cells[0][0].alive {
		t.Error("top-left alien should be dead after collision")
	}
	if p.score != 30 {
		t.Errorf("score=%d, want 30 (top alien)", p.score)
	}
	if p.player.bullet != nil {
		t.Error("player bullet should be consumed on hit")
	}
	if p.grid.alive != p.grid.total-1 {
		t.Errorf("alive=%d, want %d", p.grid.alive, p.grid.total-1)
	}
}

func TestAlienBulletKillsPlayer(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	// Place an alien bullet directly on top of the player.
	p.alienBullets = []*bullet{{
		x:  p.player.x + 4,
		y:  float64(p.player.y),
		vy: alienBulletSpeed,
	}}
	livesBefore := p.player.lives
	p.resolveCollisions()
	if p.player.lives != livesBefore-1 {
		t.Errorf("lives=%d, want %d", p.player.lives, livesBefore-1)
	}
	if p.state != psPlayerHit {
		t.Errorf("state=%v, want psPlayerHit", p.state)
	}
	if len(p.alienBullets) != 0 {
		t.Errorf("alien bullets=%d, want 0", len(p.alienBullets))
	}
}

func TestMutualBulletDestruction(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.player.bullet = &bullet{x: 20, y: 20, fromPlayer: true}
	p.alienBullets = []*bullet{{x: 20, y: 20, vy: alienBulletSpeed}}
	p.collideBulletsVsBullets()
	if p.player.bullet != nil {
		t.Error("player bullet should be consumed")
	}
	if len(p.alienBullets) != 0 {
		t.Errorf("alien bullets=%d, want 0", len(p.alienBullets))
	}
}

func TestBunkerErodes(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	if len(p.bunkers) == 0 {
		t.Fatal("expected at least one bunker")
	}
	bk := p.bunkers[0]
	// Find a solid pixel near the centre top of the bunker.
	hitX := bk.x + bk.w/2
	hitY := bk.y + 1
	if !bk.solidAt(hitX, hitY) {
		t.Fatalf("expected pixel (%d,%d) to start solid", hitX, hitY)
	}
	bk.erode(hitX, hitY, false)
	if bk.solidAt(hitX, hitY) {
		t.Error("pixel should have been eroded")
	}
}

func TestPlayerBulletDestroyedByBunker(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	bk := p.bunkers[0]
	hitX := bk.x + bk.w/2
	hitY := bk.y // top of the bunker
	p.player.bullet = &bullet{
		x:          float64(hitX),
		y:          float64(hitY),
		vy:         -playerBulletSpeed,
		fromPlayer: true,
	}
	p.collidePlayerBullet()
	if p.player.bullet != nil {
		t.Error("player bullet should be consumed by bunker")
	}
}

func TestAlienMarchStepsHorizontally(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	startX := p.grid.originX
	// Force enough time to elapse for one step at full speed.
	p.tickAlienStep(alienBaseInterval + 0.01)
	dx := p.grid.originX - startX
	if dx == 0 {
		t.Errorf("expected horizontal step, got dx=%v", dx)
	}
}

func TestAlienFormationDropsAtEdge(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	// Push the formation right up against the right wall by setting
	// originX manually, then trigger a step.
	rightmost := p.grid.cols - 1
	// Make originX such that the rightmost alien sits 1 px from edge.
	p.grid.originX = float64(p.w - 1 - rightmost*p.grid.colPitch - p.grid.spriteW)
	p.grid.dir = 1
	startY := p.grid.originY
	// First step should queue a drop (no horizontal move).
	p.tickAlienStep(alienBaseInterval + 0.01)
	if p.grid.originY != startY {
		t.Errorf("first step at edge moved Y; got dy=%v, want 0", p.grid.originY-startY)
	}
	if !p.grid.pendingDrop {
		t.Error("expected pendingDrop=true after horizontal block")
	}
	// Second step should perform the drop and reverse direction.
	p.tickAlienStep(alienBaseInterval + 0.01)
	if p.grid.originY <= startY {
		t.Errorf("expected Y to increase after drop; got dy=%v", p.grid.originY-startY)
	}
	if p.grid.dir != -1 {
		t.Errorf("expected dir to flip to -1, got %d", p.grid.dir)
	}
}

func TestAlienFireRespectsLimit(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	for i := 0; i < maxAlienBullets+3; i++ {
		p.alienFire()
	}
	if len(p.alienBullets) != maxAlienBullets {
		t.Errorf("alien bullets=%d, want %d", len(p.alienBullets), maxAlienBullets)
	}
}

func TestWaveClearsTransition(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	// Kill every alien.
	for r := 0; r < p.grid.rows; r++ {
		for c := 0; c < p.grid.cols; c++ {
			p.grid.cells[r][c].alive = false
		}
	}
	p.grid.alive = 0
	if err := p.Update(time.Millisecond * 16); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if p.state != psWaveCleared {
		t.Errorf("state=%v, want psWaveCleared", p.state)
	}
}

func TestAliensReachingPlayerEndsGame(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	// Force the formation deep enough that the bottom row touches the
	// player line.
	p.grid.originY = float64(p.loseY - (p.grid.rows-1)*p.grid.rowPitch - p.grid.spriteH + 1)
	if !p.alienReachedPlayer() {
		t.Fatal("expected alienReachedPlayer to be true")
	}
	p.updatePlaying(0.016)
	if p.state != psPlayerHit {
		t.Errorf("state=%v, want psPlayerHit", p.state)
	}
	if p.player.lives != 0 {
		t.Errorf("lives=%d, want 0", p.player.lives)
	}
}

func TestStepIntervalAcceleratesAsAliensDie(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	full := p.grid.stepInterval(1.0)
	// Kill all but one alien.
	for r := 0; r < p.grid.rows; r++ {
		for c := 0; c < p.grid.cols; c++ {
			p.grid.cells[r][c].alive = false
		}
	}
	p.grid.cells[0][0].alive = true
	p.grid.alive = 1
	one := p.grid.stepInterval(1.0)
	if one >= full {
		t.Errorf("expected step interval to shrink with fewer aliens; full=%v one=%v", full, one)
	}
}

func TestPlayerBulletExitsTopOfScreen(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.player.bullet = &bullet{x: 40, y: 1, vy: -playerBulletSpeed, fromPlayer: true}
	// One second of movement should easily carry it past y=0.
	p.tickBullets(1.0)
	if p.player.bullet != nil {
		t.Errorf("expected bullet cleared after exiting top; bullet=%+v", p.player.bullet)
	}
}

func TestSpaceFiresBullet(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	if p.player.bullet != nil {
		t.Fatal("expected no bullet initially")
	}
	p.handlePlayKey(engine.Key{Code: engine.KeyChar, Rune: ' '})
	if p.player.bullet == nil {
		t.Error("expected bullet after space press")
	}
	// Holding space while one is in flight should not stack.
	p.handlePlayKey(engine.Key{Code: engine.KeyChar, Rune: ' '})
	if p.player.bullet == nil {
		t.Error("bullet should still exist (or have been replaced once)")
	}
}

func TestEscFromPlayWantsQuit(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.handlePlayKey(engine.Key{Code: engine.KeyEsc})
	if !p.wantQuit {
		t.Error("expected wantQuit=true after ESC")
	}
}

func TestGameRegistersInRegistry(t *testing.T) {
	// The init() in spaceinvaders.go should have registered the game.
	// We don't try to Run it (would require a real terminal), but the
	// registration must be present.
	if _, ok := registry.Get("spaceinvaders"); !ok {
		t.Error("spaceinvaders not registered")
	}
}
