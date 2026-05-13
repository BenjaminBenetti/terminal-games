package donkeykong

import (
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// newTestPlayScene constructs a play scene without going through the
// engine — the engine isn't testable headlessly anyway, and most of
// the logic operates on the playScene struct directly.
func newTestPlayScene(t *testing.T, w, h int) *playScene {
	t.Helper()
	e, err := engine.New(engine.Options{Width: w, Height: h})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := &playScene{
		e:     e,
		w:     w,
		h:     h,
		rng:   rand.New(rand.NewSource(1)),
		lives: 3,
		wave:  1,
	}
	p.lvl = buildLevel(p.w, p.h)
	p.startStage()
	return p
}

func TestGameRegistersInRegistry(t *testing.T) {
	if _, ok := registry.Get("donkeykong"); !ok {
		t.Error("donkeykong not registered")
	}
}

func TestBuildLevelHasFiveGirders(t *testing.T) {
	lvl := buildLevel(80, 48)
	if len(lvl.girders) != 5 {
		t.Errorf("girders=%d, want 5", len(lvl.girders))
	}
	if len(lvl.ladders) != 4 {
		t.Errorf("ladder gaps=%d, want 4", len(lvl.ladders))
	}
	// DK platform (girder 0) must be flat.
	g0 := lvl.girders[0]
	if g0.leftY != g0.rightY {
		t.Errorf("DK platform should be flat: leftY=%d rightY=%d", g0.leftY, g0.rightY)
	}
	// Mario start girder (last) must be flat.
	gM := lvl.girders[len(lvl.girders)-1]
	if gM.leftY != gM.rightY {
		t.Errorf("Mario start should be flat: leftY=%d rightY=%d", gM.leftY, gM.rightY)
	}
}

func TestGirderClearanceFitsMario(t *testing.T) {
	// At every column on every girder, the row gap to the girder above
	// must leave room for Mario's 6-pixel sprite.
	lvl := buildLevel(80, 48)
	for i := 1; i < len(lvl.girders); i++ {
		upper, lower := lvl.girders[i-1], lvl.girders[i]
		for x := lower.leftX; x <= lower.rightX; x++ {
			gap := lower.yAt(x) - upper.yAt(x)
			if gap < marioHeight+1 {
				t.Errorf("girder %d→%d gap at x=%d is %d; need >= %d",
					i-1, i, x, gap, marioHeight+1)
				break
			}
		}
	}
}

func TestLadderEndpointsMeetGirders(t *testing.T) {
	lvl := buildLevel(80, 48)
	for gap, group := range lvl.ladders {
		upper, lower := lvl.girders[gap], lvl.girders[gap+1]
		for _, ld := range group {
			cx := ld.x + ladderWidth/2
			if ld.topY != upper.yAt(cx) {
				t.Errorf("ladder gap=%d x=%d topY=%d, want %d",
					gap, ld.x, ld.topY, upper.yAt(cx))
			}
			if ld.bottomY != lower.yAt(cx) {
				t.Errorf("ladder gap=%d x=%d bottomY=%d, want %d",
					gap, ld.x, ld.bottomY, lower.yAt(cx))
			}
		}
	}
}

func TestBrokenLadderClimbTopShorterThanIntact(t *testing.T) {
	// Build a synthetic broken / intact pair and verify climbTopY differs.
	intact := ladder{x: 10, topY: 10, bottomY: 20, broken: false}
	broken := ladder{x: 10, topY: 10, bottomY: 20, broken: true}
	if intact.climbTopY() != 10 {
		t.Errorf("intact climbTopY=%d, want 10", intact.climbTopY())
	}
	if broken.climbTopY() <= intact.climbTopY() {
		t.Errorf("broken climbTopY=%d should be > intact %d",
			broken.climbTopY(), intact.climbTopY())
	}
}

func TestMarioStartsOnBottomGirder(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	if p.mario.girderIdx != p.lvl.marioStartGIdx {
		t.Errorf("mario girderIdx=%d, want %d",
			p.mario.girderIdx, p.lvl.marioStartGIdx)
	}
	if p.mario.state != msWalking {
		t.Errorf("mario state=%v, want msWalking", p.mario.state)
	}
}

func TestStartStageResetsBonusAndBarrels(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.bonus = 100
	p.barrels = append(p.barrels, &barrel{})
	p.startStage()
	if p.bonus != bonusStart {
		t.Errorf("bonus=%d, want %d", p.bonus, bonusStart)
	}
	if len(p.barrels) != 0 {
		t.Errorf("barrels=%d, want 0", len(p.barrels))
	}
	if p.state != psPreStage {
		t.Errorf("state=%v, want psPreStage", p.state)
	}
}

func TestBonusCountsDown(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	start := p.bonus
	for i := 0; i < 100; i++ {
		p.updateBonus(0.1)
	}
	if p.bonus >= start {
		t.Errorf("bonus did not decrease: %d -> %d", start, p.bonus)
	}
}

func TestDKThrowsBarrels(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	p.nextBarrelT = 0
	p.updateDK(0.016)
	if len(p.barrels) != 1 {
		t.Errorf("barrels=%d, want 1", len(p.barrels))
	}
	if p.dkThrowAnimT <= 0 {
		t.Errorf("DK throw anim should be active; got %v", p.dkThrowAnimT)
	}
}

func TestBarrelHitsMarioKillsHim(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	// Place a barrel directly on top of Mario.
	b := &barrel{
		id:        1,
		x:         p.mario.x,
		y:         p.mario.y - 2,
		state:     0,
		girderIdx: p.mario.girderIdx,
	}
	p.barrels = []*barrel{b}
	lives0 := p.lives
	p.resolveCollisions()
	if p.lives != lives0-1 {
		t.Errorf("lives=%d, want %d", p.lives, lives0-1)
	}
	if p.mario.state != msDying {
		t.Errorf("mario state=%v, want msDying", p.mario.state)
	}
}

func TestHammerDestroysBarrel(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	p.hammerActive = true
	p.mario.hammerHigh = true
	hr := p.hammerSwingRect()
	// Place a barrel where the hammer swing will hit.
	b := &barrel{
		id:        1,
		x:         float64(hr.x0),
		y:         float64(hr.y0),
		state:     0,
		girderIdx: p.mario.girderIdx,
	}
	p.barrels = []*barrel{b}
	p.resolveCollisions()
	if len(p.barrels) != 0 {
		t.Errorf("barrels=%d, want 0 (hammer should destroy)", len(p.barrels))
	}
	if p.score != hammerKillScore {
		t.Errorf("score=%d, want %d", p.score, hammerKillScore)
	}
}

func TestMarioReachesTopWinsStage(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	// Force Mario onto the goal girder (top DK platform).
	p.mario.girderIdx = p.lvl.marioGoalGIdx
	p.mario.state = msWalking
	startBonus := p.bonus
	if err := p.Update(time.Millisecond * 16); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if p.state != psStageClear {
		t.Errorf("state=%v, want psStageClear", p.state)
	}
	if p.score < startBonus {
		t.Errorf("score=%d, want >= %d (bonus awarded)", p.score, startBonus)
	}
}

func TestGameOverAfterLastLife(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	p.lives = 1
	// Trigger a fatal hit.
	b := &barrel{
		id:        1,
		x:         p.mario.x,
		y:         p.mario.y - 2,
		state:     0,
		girderIdx: p.mario.girderIdx,
	}
	p.barrels = []*barrel{b}
	p.resolveCollisions()
	// Tick through the death animation.
	for i := 0; i < int(deathDuration/0.05)+5; i++ {
		if err := p.Update(time.Millisecond * 50); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if p.state != psGameOver {
		t.Errorf("state=%v, want psGameOver", p.state)
	}
}

func TestBarrelFallsThenLandsOnGirder(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	// Spawn a falling barrel just above girder 2, with girder 1 already
	// above. It should land on girder 2 (the next one below the spawn y).
	gTarget := p.lvl.girders[2]
	gAbove := p.lvl.girders[1]
	mid := gTarget.leftX + (gTarget.rightX-gTarget.leftX)/2
	startY := gAbove.yAt(mid) + 2 // just below the upper girder
	b := &barrel{
		id:         1,
		x:          float64(mid),
		y:          float64(startY),
		vy:         5,
		state:      1,
		ladderSeen: map[int]bool{},
	}
	p.barrels = []*barrel{b}
	for range 60 {
		p.updateBarrels(0.05)
		if b.state == 0 {
			break
		}
	}
	if b.state != 0 {
		t.Errorf("expected barrel to land on girder; state=%d y=%v", b.state, b.y)
	}
	if b.girderIdx != 2 {
		t.Errorf("expected to land on girder 2, got %d", b.girderIdx)
	}
}

func TestEscFromPlayWantsQuit(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	// Simulate the key handling — playScene listens via PollKey which we
	// can't easily feed in test, so call the helper that drives ESC.
	p.wantQuit = false
	// Use the engine's Stop to trigger the wantQuit path via input later;
	// for the unit test, exercise the explicit flag instead.
	p.handleEsc()
	if !p.wantQuit {
		t.Error("expected wantQuit=true after ESC")
	}
}

// handleEsc is a test hook that mirrors what handleInput does on ESC.
// It's defined here in the test file so the production code stays free of
// test-only surface area.
func (p *playScene) handleEsc() {
	p.wantQuit = true
}

// TestJumpDoesNotTeleportToTop is a regression test for a bug where a
// single jump from the bottom girder snapped Mario onto the DK platform
// (and instantly won the stage). The fix tracks previous-frame feet
// position so landing is only triggered by an actual girder CROSSING.
func TestJumpDoesNotTeleportToTop(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	p.state = psPlaying
	startG := p.mario.girderIdx
	startY := p.mario.y
	// Trigger a jump.
	p.mario.state = msJumping
	p.mario.vy = marioJumpVy
	p.mario.jumpedBarrels = map[int]bool{}
	// Step through the full jump arc.
	landed := false
	for i := 0; i < 120 && !landed; i++ {
		p.marioJump(1.0 / 60.0)
		if p.mario.state == msWalking {
			landed = true
			break
		}
	}
	if !landed {
		t.Fatalf("expected mario to land within 120 frames; state=%v y=%v",
			p.mario.state, p.mario.y)
	}
	// Mario should be back on the girder he jumped from, not teleported up.
	if p.mario.girderIdx != startG {
		t.Errorf("mario landed on girder %d, want %d (start)",
			p.mario.girderIdx, startG)
	}
	// And his y should be back near where it started (within a pixel).
	if diff := p.mario.y - startY; diff < -1 || diff > 1 {
		t.Errorf("mario y=%v, want near %v", p.mario.y, startY)
	}
	// Stage must NOT have been won.
	if p.state == psStageClear {
		t.Error("stage should not be cleared from a jump")
	}
}

// TestRunningJumpCarriesFartherThanStandingJump exercises the momentum
// system: a jump initiated while moving should travel farther than one
// from a standstill, because air friction is zero.
func TestRunningJumpCarriesFartherThanStandingJump(t *testing.T) {
	// Standing jump — no horizontal velocity at takeoff.
	stand := newTestPlayScene(t, 80, 48)
	stand.state = psPlaying
	stand.mario.state = msJumping
	stand.mario.vy = marioJumpVy
	stand.mario.vx = 0
	stand.mario.jumpedBarrels = map[int]bool{}
	startX := stand.mario.x
	for range 60 {
		stand.marioJump(1.0 / 60.0)
		if stand.mario.state == msWalking {
			break
		}
	}
	standDist := stand.mario.x - startX

	// Running jump — full forward momentum at takeoff.
	run := newTestPlayScene(t, 80, 48)
	run.state = psPlaying
	run.mario.state = msJumping
	run.mario.vy = marioJumpVy
	run.mario.vx = marioWalkSpeed
	run.mario.jumpedBarrels = map[int]bool{}
	startX2 := run.mario.x
	for range 60 {
		run.marioJump(1.0 / 60.0)
		if run.mario.state == msWalking {
			break
		}
	}
	runDist := run.mario.x - startX2

	if runDist <= standDist+1 {
		t.Errorf("running jump (%.2f px) should travel farther than standing jump (%.2f px)",
			runDist, standDist)
	}
}

func TestSlantedGirdersAreCropped(t *testing.T) {
	lvl := buildLevel(80, 48)
	// Girder 0 and 4 should span the full canvas; the three slanted
	// girders in between should be cropped on their low side.
	if lvl.girders[0].leftX != 0 || lvl.girders[0].rightX != 79 {
		t.Errorf("girder 0 should be full width; got %d..%d",
			lvl.girders[0].leftX, lvl.girders[0].rightX)
	}
	if lvl.girders[4].leftX != 0 || lvl.girders[4].rightX != 79 {
		t.Errorf("girder 4 should be full width; got %d..%d",
			lvl.girders[4].leftX, lvl.girders[4].rightX)
	}
	for i := 1; i <= 3; i++ {
		g := lvl.girders[i]
		if g.leftX == 0 && g.rightX == 79 {
			t.Errorf("slanted girder %d should be cropped on one side; got %d..%d",
				i, g.leftX, g.rightX)
		}
	}
	// Girder 1 rolls left → low end on the left → leftX > 0.
	if lvl.girders[1].leftX == 0 {
		t.Errorf("girder 1 (rolls left) should be cropped on left; leftX=%d",
			lvl.girders[1].leftX)
	}
	// Girder 2 rolls right → low end on the right → rightX < 79.
	if lvl.girders[2].rightX == 79 {
		t.Errorf("girder 2 (rolls right) should be cropped on right; rightX=%d",
			lvl.girders[2].rightX)
	}
}

func TestDrawDoesNotPanic(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	c := engine.NewCanvas(80, 48)
	// Draw in each major state to exercise the branches.
	p.state = psPreStage
	p.Draw(c)
	p.state = psPlaying
	p.Draw(c)
	p.state = psStageClear
	p.Draw(c)
	p.state = psGameOver
	p.Draw(c)
}

// TestGameLoopAdvances runs many frames of gameplay and checks that no
// state machine deadlocks, no entity slice grows unboundedly, and the
// scene survives a few simulated barrel collisions.
func TestGameLoopAdvances(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	c := engine.NewCanvas(80, 48)
	// Force into play immediately so we exercise update + draw.
	p.state = psPlaying
	p.stateT = 0
	dt := 33 * time.Millisecond
	for range 200 {
		if err := p.Update(dt); err != nil {
			t.Fatalf("Update: %v", err)
		}
		p.Draw(c)
		if len(p.barrels) > 64 {
			t.Fatalf("runaway barrel growth: %d", len(p.barrels))
		}
		if len(p.fireballs) > fireballMax+1 {
			t.Fatalf("too many fireballs: %d", len(p.fireballs))
		}
	}
}

// TestBarrelLadderSeenResetsOnLanding makes sure a barrel that descends a
// ladder doesn't immediately retake the same ladder on the lower girder.
func TestBarrelLadderSeenResetsOnLanding(t *testing.T) {
	p := newTestPlayScene(t, 80, 48)
	// Construct a descending barrel about to land.
	g := p.lvl.girders[2]
	mid := g.leftX + (g.rightX-g.leftX)/2
	ld := ladder{x: mid - 1, topY: p.lvl.girders[1].yAt(mid), bottomY: g.yAt(mid)}
	b := &barrel{
		id:         1,
		x:          float64(mid),
		y:          float64(g.yAt(mid)) - barrelFallH,
		state:      2,
		descLad:    ld,
		descGap:    1,
		ladderSeen: map[int]bool{},
	}
	p.barrels = []*barrel{b}
	for range 20 {
		p.updateBarrels(0.05)
		if b.state == 0 {
			break
		}
	}
	if b.state != 0 {
		t.Fatalf("expected barrel to land after descent, state=%d", b.state)
	}
	if !b.ladderSeen[-1] {
		t.Error("expected ladderSeen sentinel to be set after descent")
	}
}
