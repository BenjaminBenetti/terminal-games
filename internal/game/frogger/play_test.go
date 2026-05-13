package frogger

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// testPlayfieldW is the playfield width tests assume. Picking the
// minimum (80) keeps the per-lane counts at their original arcade
// values, so test invariants about counts/positions don't change with
// terminal size.
const testPlayfieldW = playfieldMinW

// newTestPlayScene constructs a play scene without going through the
// engine — the engine isn't headlessly testable in full, but a
// playScene with a deterministic RNG is enough to exercise all the
// logic paths.
func newTestPlayScene(t *testing.T, w, h int) *playScene {
	t.Helper()
	e, err := engine.New(engine.Options{Width: w, Height: h})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := &playScene{
		e:             e,
		w:             w,
		h:             h,
		playfieldW:    computePlayfieldW(w),
		lives:         3,
		level:         1,
		rng:           rand.New(rand.NewSource(1)),
		ladyLaneIdx:   -1,
		ladyEntityIdx: -1,
		flySlot:       -1,
		crocSlot:      -1,
		nextExtraLife: extraLifeAt,
	}
	p.offX = (p.w - p.playfieldW) / 2
	p.offY = (p.h - playfieldH) / 2
	if p.offX < 0 {
		p.offX = 0
	}
	if p.offY < 0 {
		p.offY = 0
	}
	p.startWave(false)
	return p
}

func TestGameRegistersInRegistry(t *testing.T) {
	if _, ok := registry.Get("frogger"); !ok {
		t.Error("frogger not registered")
	}
}

func TestPlayfieldDimensions(t *testing.T) {
	// HUD + home + river + median + road + start + time bar = 48.
	want := hudH + homeStripH + riverH + medianH + roadH + startH + 2
	if want != playfieldH {
		t.Errorf("playfield layout adds to %d, expected %d", want, playfieldH)
	}
}

func TestRowCenterYIsMonotonicForward(t *testing.T) {
	// Rows are indexed 0=home (top) to 12=start (bottom). Larger row
	// index must have a strictly larger y (further down the screen).
	prev := -1
	for r := rowHome; r < numFrogRows; r++ {
		y := rowCenterY(r)
		if y <= prev {
			t.Errorf("row %d y=%d not greater than previous %d", r, y, prev)
		}
		prev = y
	}
}

func TestHomeSlotXEvenlyDistributed(t *testing.T) {
	// Each slot must be inside the playfield, non-overlapping, and the
	// gaps between them must be roughly even (hedge dividers).
	for _, pw := range []int{playfieldMinW, playfieldTargetW} {
		prevX1 := 0
		for i := 0; i < numHomes; i++ {
			x0, x1 := homeSlotX(i, pw)
			if x0 < 0 || x1 > pw {
				t.Errorf("pw=%d home slot %d out of bounds: %d..%d", pw, i, x0, x1)
			}
			if x0 < prevX1 {
				t.Errorf("pw=%d home slot %d overlaps previous (%d < %d)", pw, i, x0, prevX1)
			}
			if x1-x0 != homeSlotW {
				t.Errorf("pw=%d home slot %d width = %d, want %d", pw, i, x1-x0, homeSlotW)
			}
			prevX1 = x1
		}
	}
}

func TestBuildLaneSpecsHasAllRows(t *testing.T) {
	specs := buildLaneSpecs(testPlayfieldW, 1.0)
	if len(specs) != 10 {
		t.Fatalf("expected 10 lanes (5 river + 5 road), got %d", len(specs))
	}
	for i, s := range specs[:5] {
		if s.kind != laneRiverWater {
			t.Errorf("lane %d expected river, got %v", i, s.kind)
		}
	}
	for i, s := range specs[5:] {
		if s.kind != laneRoad {
			t.Errorf("lane %d expected road, got %v", i+5, s.kind)
		}
	}
}

func TestLaneSpecPeriodFitsPlayfield(t *testing.T) {
	// Cycle period must be at least playfieldW + entityW so an entity
	// fully exits one side before the next one's slot reappears on the
	// other — otherwise we'd see overlapping copies visually. Verify at
	// both the minimum and the target playfield widths.
	for _, pw := range []int{playfieldMinW, playfieldTargetW} {
		specs := buildLaneSpecs(pw, 1.0)
		for i, s := range specs {
			period := s.entitySpan * s.count
			if period < pw+s.entityW {
				t.Errorf("pw=%d lane %d period %d < pw+entityW=%d",
					pw, i, period, pw+s.entityW)
			}
		}
	}
}

func TestLaneCountScalesWithPlayfield(t *testing.T) {
	// Doubling the playfield width should roughly double the per-lane
	// entity count (density stays constant).
	smallSpecs := buildLaneSpecs(playfieldMinW, 1.0)
	bigSpecs := buildLaneSpecs(playfieldTargetW, 1.0)
	for i := range smallSpecs {
		if bigSpecs[i].count <= smallSpecs[i].count {
			t.Errorf("lane %d count should grow with playfield width: small=%d big=%d",
				i, smallSpecs[i].count, bigSpecs[i].count)
		}
	}
}

func TestFrogSpawnsOnStartRow(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	if p.frog.row != rowStart {
		t.Errorf("frog row=%d, want rowStart (%d)", p.frog.row, rowStart)
	}
	if p.frog.state != fsAlive {
		t.Errorf("frog state=%v, want fsAlive", p.frog.state)
	}
	if p.timeLeft != timeBarDuration {
		t.Errorf("timeLeft=%v, want %v", p.timeLeft, timeBarDuration)
	}
}

func TestHopUpAdvancesRow(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	startRow := p.frog.row
	p.startHop(hopUp)
	if p.frog.row != startRow-1 {
		t.Errorf("after startHop(up), row=%d want %d", p.frog.row, startRow-1)
	}
	if p.frog.state != fsHopping {
		t.Errorf("after startHop, state=%v want fsHopping", p.frog.state)
	}
	for i := 0; i < 40; i++ {
		p.frogHop(0.02)
		if p.frog.state == fsAlive {
			break
		}
	}
	if p.frog.state != fsAlive {
		t.Errorf("hop didn't complete in time, state=%v", p.frog.state)
	}
}

func TestHopDownDoesNotPassStartRow(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	startRow := p.frog.row
	p.startHop(hopDown)
	if p.frog.row != startRow {
		t.Errorf("startHop(down) at rowStart shouldn't advance: row=%d", p.frog.row)
	}
	if p.frog.state != fsAlive {
		t.Errorf("startHop(down) at rowStart should be a no-op: state=%v", p.frog.state)
	}
}

func TestHopLeftClampsAtEdge(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	p.frog.x = 0
	p.startHop(hopLeft)
	if p.frog.state != fsAlive {
		t.Errorf("hop left at x=0 should be a no-op, state=%v", p.frog.state)
	}
}

func TestHopRightClampsAtEdge(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	p.frog.x = float64(p.playfieldW - frogW)
	p.startHop(hopRight)
	if p.frog.state != fsAlive {
		t.Errorf("hop right at right edge should be a no-op, state=%v", p.frog.state)
	}
}

func TestFrogDiesInWaterIfNoLog(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	p.frog.row = rowRiverL2
	p.frog.x = 5
	p.frog.y = float64(rowCenterY(rowRiverL2))
	laneIdx := laneSpecForRow(rowRiverL2)
	// Push the lane base so all of its entities sit far away from x=5.
	p.lanes[laneIdx].base = 60
	p.frogIdle(0.0)
	if p.frog.state != fsSplash {
		t.Errorf("frog in open water should drown, state=%v", p.frog.state)
	}
}

func TestFrogDeliveredToHomeSlot(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	x0, _ := homeSlotX(0, p.playfieldW)
	p.frog.x = float64(x0 + (homeSlotW-frogW)/2)
	p.frog.row = rowHome
	p.frog.y = float64(rowCenterY(rowHome))
	p.tryEnterHome()
	if !p.homes[0].occupied {
		t.Errorf("home 0 not marked occupied after delivery")
	}
	if p.score < pointsPerHome {
		t.Errorf("score didn't increase after delivery: %d", p.score)
	}
}

func TestFrogDiesOnHedge(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	_, x1 := homeSlotX(0, p.playfieldW)
	p.frog.x = float64(x1 + 1)
	p.frog.row = rowHome
	p.frog.y = float64(rowCenterY(rowHome))
	p.tryEnterHome()
	if p.frog.state != fsSplat {
		t.Errorf("frog should die on hedge, state=%v", p.frog.state)
	}
}

func TestFillingAllHomesTriggersWaveClear(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	for i := 0; i < numHomes-1; i++ {
		p.homes[i].occupied = true
	}
	last := numHomes - 1
	x0, _ := homeSlotX(last, p.playfieldW)
	p.frog.x = float64(x0 + (homeSlotW-frogW)/2)
	p.frog.row = rowHome
	p.tryEnterHome()
	if p.state != psWaveClear {
		t.Errorf("expected psWaveClear after filling all homes, got %v", p.state)
	}
}

func TestTimeBarRunsOut(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	p.timeLeft = 0.01
	p.advanceTimeBar(0.05)
	if p.frog.state == fsAlive {
		t.Errorf("frog should die when time bar runs out")
	}
}

func TestRespawnReducesLives(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	start := p.lives
	p.killFrog(false)
	if p.lives != start-1 {
		t.Errorf("lives not decremented: %d (was %d)", p.lives, start)
	}
}

func TestGameOverWhenOutOfLives(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	p.lives = 1
	p.killFrog(false)
	p.respawnOrGameOver()
	if p.state != psGameOver {
		t.Errorf("expected psGameOver, got %v", p.state)
	}
}

func TestTurtleDivePhasesProgress(t *testing.T) {
	ln := laneState{
		spec:  laneSpec{diveCycle: 10.0, isTurtle: true},
		diveT: 0,
	}
	if ln.turtleDivePhase() != 0 {
		t.Errorf("at t=0 expected surface phase")
	}
	ln.diveT = 6.0
	if ln.turtleDivePhase() != 1 {
		t.Errorf("at t=6.0 expected warning phase")
	}
	ln.diveT = 8.0
	if ln.turtleDivePhase() != 2 {
		t.Errorf("at t=8.0 expected submerged phase")
	}
}

func TestLaneVisibilityIndices(t *testing.T) {
	ln := laneState{
		spec: laneSpec{entityW: 6, entitySpan: 30, count: 3, dir: +1},
		base: 0,
	}
	lo, hi := ln.visibleEntityIndices(playfieldMinW)
	if lo > 0 || hi < 2 {
		t.Errorf("expected visibility ≥ {0..2}, got {%d..%d}", lo, hi)
	}
}

func TestUpdateProgressesGameState(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	if p.state != psPreStage {
		t.Fatalf("expected psPreStage at start, got %v", p.state)
	}
	for i := 0; i < 200; i++ {
		if err := p.Update(time.Millisecond * 16); err != nil {
			t.Fatalf("Update returned error: %v", err)
		}
		if p.state == psPlaying {
			break
		}
	}
	if p.state != psPlaying {
		t.Errorf("expected psPlaying after pre-stage banner, got %v", p.state)
	}
}

func TestDrawDoesNotPanic(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	c := p.e.Canvas()
	states := []playState{psPreStage, psPlaying, psWaveClear, psGameOver}
	for _, st := range states {
		p.state = st
		p.Draw(c)
	}
	for _, fs := range []frogState{fsSplat, fsSplash, fsHopping} {
		p.frog.state = fs
		p.Draw(c)
	}
}

func TestFrogRidesLogWithLane(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	p.frog.row = rowRiverL1
	p.frog.y = float64(rowCenterY(rowRiverL1))
	laneIdx := laneSpecForRow(rowRiverL1)
	ln := &p.lanes[laneIdx]
	ln.base = 20
	p.frog.x = ln.entityX(0) + float64(ln.spec.entityW/2-frogW/2)
	startX := p.frog.x
	dt := 0.5
	p.advanceLanes(dt)
	p.frogIdle(dt)
	if p.frog.state != fsAlive {
		t.Errorf("frog should ride the log, state=%v", p.frog.state)
	}
	expectDelta := float64(ln.spec.dir) * ln.spec.speed * dt
	if math.Abs(p.frog.x-startX-expectDelta) > 0.01 {
		t.Errorf("frog x didn't track log: got %v want %v (delta %v)",
			p.frog.x, startX+expectDelta, expectDelta)
	}
}

func TestFrogDrownsIfRiddenOffEdge(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	p.frog.row = rowRiverL1
	p.frog.y = float64(rowCenterY(rowRiverL1))
	p.frog.x = float64(p.playfieldW - 2)
	laneIdx := laneSpecForRow(rowRiverL1)
	p.lanes[laneIdx].base = float64(p.playfieldW - 10)
	p.frogIdle(0.0)
	if p.frog.state != fsSplash {
		t.Errorf("frog past right edge should drown, state=%v", p.frog.state)
	}
}

func TestSubmergedTurtleDrowns(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	p.frog.row = rowRiverT1
	p.frog.y = float64(rowCenterY(rowRiverT1))
	laneIdx := laneSpecForRow(rowRiverT1)
	ln := &p.lanes[laneIdx]
	ln.base = 0
	p.frog.x = ln.entityX(0) + 4
	ln.diveT = ln.spec.diveCycle * 0.85
	p.frogIdle(0.0)
	if p.frog.state != fsSplash {
		t.Errorf("frog on submerged turtle should drown, state=%v", p.frog.state)
	}
}

func TestScoringPerNewRow(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	if p.frog.highestRow != rowStart {
		t.Fatalf("expected highest row = start, got %d", p.frog.highestRow)
	}
	startScore := p.score
	p.frog.row = rowMedian
	p.frog.y = float64(rowCenterY(rowMedian))
	p.frog.x = float64(p.playfieldW / 2)
	p.landed()
	got := p.score - startScore
	want := int(rowStart-rowMedian) * pointsPerRow
	if got != want {
		t.Errorf("new-row scoring: got %d want %d", got, want)
	}
}

func TestExtraLifeAwarded(t *testing.T) {
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying
	startLives := p.lives
	p.score = extraLifeAt
	if err := p.Update(time.Millisecond * 16); err != nil {
		t.Fatalf("Update returned %v", err)
	}
	if p.lives != startLives+1 {
		t.Errorf("expected extra life at %d points, lives=%d (was %d)",
			extraLifeAt, p.lives, startLives)
	}
}

func TestSidewaysHopDoesNotHitAdjacentLane(t *testing.T) {
	// Regression: the hop arc lifts frog.y up to 2 px during the visual
	// hop. Previously the collision rect used that interpolated y, which
	// spilled into the lane above and let cars there hit the frog while
	// it was hopping sideways within its own lane. Collision should be
	// based on the frog's LOGICAL row, not its arc-lifted visual y.
	p := newTestPlayScene(t, testPlayfieldW, playfieldH)
	p.state = psPlaying

	// Park the frog mid-arc on rowRoad1.
	p.frog.row = rowRoad1
	p.frog.x = 30
	p.frog.y = float64(rowCenterY(rowRoad1)) - hopArcHeight // simulate peak of hop
	p.frog.state = fsHopping

	// Stuff a car directly above (rowRoad2) at the frog's x. With the
	// old buggy rect, this would have hit; with the fix it must not.
	upLaneIdx := laneSpecForRow(rowRoad2)
	upLane := &p.lanes[upLaneIdx]
	// Align entity 0 directly under the frog by working backwards from
	// entityX: entityX(0) = base mod period, so set base accordingly.
	upLane.base = 28 // car spans ~28..34, frog at 30..35
	p.resolveCollisions()
	if p.frog.state != fsHopping {
		t.Errorf("frog mid-sideways-hop must not be killed by a car in the lane above (state=%v)", p.frog.state)
	}

	// Sanity: a car in the frog's OWN lane at the same x SHOULD hit.
	p.frog.state = fsHopping
	ownLaneIdx := laneSpecForRow(rowRoad1)
	p.lanes[ownLaneIdx].base = 28
	p.resolveCollisions()
	if p.frog.state != fsSplat {
		t.Errorf("frog mid-hop should still die on a car in its own lane (state=%v)", p.frog.state)
	}
}

func TestPlayfieldAdaptsToCanvasWidth(t *testing.T) {
	// Wide canvas → playfield up to the target cap.
	// Narrow canvas → playfield matches canvas (down to the floor).
	cases := []struct {
		canvasW    int
		wantField  int
		wantLanger bool
	}{
		{canvasW: 60, wantField: playfieldMinW},      // clamped up to min
		{canvasW: 80, wantField: 80},                  // exact
		{canvasW: 120, wantField: 120},                // takes the canvas
		{canvasW: 160, wantField: 160},                // exact target
		{canvasW: 300, wantField: playfieldTargetW},   // clamped down to target
	}
	for _, tc := range cases {
		got := computePlayfieldW(tc.canvasW)
		if got != tc.wantField {
			t.Errorf("computePlayfieldW(%d) = %d, want %d", tc.canvasW, got, tc.wantField)
		}
	}
}
