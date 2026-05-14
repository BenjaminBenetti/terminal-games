package qix

import (
	"math/rand"
	"testing"
)

// newDeterministicRng returns an RNG that always produces the same
// sequence within a test, so tests don't flake on random spawn
// positions or joint headings.
func newDeterministicRng(t *testing.T) *rand.Rand {
	t.Helper()
	return rand.New(rand.NewSource(1))
}

// newField is the public constructor for the playfield; its result is
// what the rest of the tests poke at. Verify the basic shape: every
// cell on the rectangle's edge is cellBorder, every interior cell is
// cellOpen, and the cached count matches the interior area.
func TestNewFieldRectangleBorder(t *testing.T) {
	f := newField(10, 8)
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			onEdge := x == 0 || x == f.w-1 || y == 0 || y == f.h-1
			got := f.at(x, y)
			want := cellOpen
			if onEdge {
				want = cellBorder
			}
			if got != want {
				t.Fatalf("cell (%d,%d): got %v, want %v", x, y, got, want)
			}
		}
	}
	wantOpen := (10 - 2) * (8 - 2)
	if f.openCount != wantOpen {
		t.Fatalf("openCount: got %d, want %d", f.openCount, wantOpen)
	}
	if f.totalCells != wantOpen {
		t.Fatalf("totalCells: got %d, want %d", f.totalCells, wantOpen)
	}
}

// percentClaimed should be 0 at start and reflect cells flipped to
// cellClaimed thereafter.
func TestPercentClaimedTracksCellSets(t *testing.T) {
	f := newField(12, 12)
	if got := f.percentClaimed(); got != 0 {
		t.Fatalf("fresh field: got %d%%, want 0%%", got)
	}
	total := f.totalCells
	// Mark exactly half of the interior cellOpen cells as cellClaimed.
	flipped := 0
	for y := 1; y < f.h-1; y++ {
		for x := 1; x < f.w-1; x++ {
			if flipped*2 >= total {
				break
			}
			f.set(x, y, cellClaimed)
			flipped++
		}
		if flipped*2 >= total {
			break
		}
	}
	got := f.percentClaimed()
	if got < 49 || got > 51 {
		t.Fatalf("half-claimed field: got %d%%, want ~50%%", got)
	}
}

// resolveClaim splits the open area on a horizontal mid-line and
// claims the side not containing the Qix probe.
func TestResolveClaimPicksNonQixRegion(t *testing.T) {
	f := newField(10, 10)
	// Trail starts at the left-edge border cell (0,5), walks across
	// the interior as cellDraw, and terminates at the right-edge
	// border cell (9,5). The endpoints stay border; the interior
	// cells become cellDraw before resolveClaim flips them to border.
	trail := []point{{0, 5}}
	for x := 1; x <= 8; x++ {
		f.set(x, 5, cellDraw)
		trail = append(trail, point{x, 5})
	}
	trail = append(trail, point{9, 5})

	// Qix probe placed below the line — that region should stay open
	// and the *above* region should be claimed.
	probes := []point{{4, 7}}
	claimed := f.resolveClaim(trail, probes)
	if claimed == 0 {
		t.Fatalf("resolveClaim: expected nonzero claim count, got 0")
	}
	// The cell directly above the line at (4, 4) should now be claimed;
	// (4, 7) which contains the Qix should still be open.
	if !f.isOpen(4, 7) {
		t.Fatalf("(4,7) should remain open (qix region), got %v", f.at(4, 7))
	}
	if f.at(4, 4) != cellClaimed {
		t.Fatalf("(4,4) should be claimed (non-qix region), got %v", f.at(4, 4))
	}
	// And every cell on the drawn line should now be border.
	for x := 1; x <= 8; x++ {
		if f.at(x, 5) != cellBorder {
			t.Fatalf("(%d,5) trail cell should be border, got %v", x, f.at(x, 5))
		}
	}
}

// A trail that doesn't actually split the open area shouldn't claim
// anything. Here we draw a one-cell "spike" out from the border and
// back — degenerate; the trail becomes border but nothing is claimed.
func TestResolveClaimDegenerateNoClaim(t *testing.T) {
	f := newField(10, 10)
	// Spike straight up from bottom border (5,9) → (5,8) → (5,7).
	// The trail's two endpoints (5,9) and (5,9 again, hypothetically)
	// must be distinct border cells for a real claim; here the spike
	// returns to the same border cell — degenerate.
	// Easier degenerate case: just paint one cellDraw with no closure.
	f.set(5, 8, cellDraw)
	trail := []point{{5, 9}, {5, 8}, {5, 9}}
	claimed := f.resolveClaim(trail, []point{{4, 4}})
	if claimed != 0 {
		t.Fatalf("degenerate trail: expected 0 claim, got %d", claimed)
	}
}

// Verify that the sparx wall-walker actually advances around a clean
// rectangular border without getting stuck.
func TestSparxWalksRectangleBorder(t *testing.T) {
	f := newField(20, 12)
	// Start the sparx at top centre heading right with right-hand rule.
	sp := newSparx(10, 0, 1, 0, 100, true)
	// Run 200 forced steps and verify it visits a variety of cells
	// (i.e. doesn't get pinned). The exact path doesn't matter for the
	// test; we just want non-degenerate motion.
	seen := map[point]bool{}
	for i := 0; i < 200; i++ {
		if !sp.step(f) {
			t.Fatalf("sparx step failed at iteration %d", i)
		}
		seen[point{sp.x, sp.y}] = true
	}
	if len(seen) < 20 {
		t.Fatalf("sparx only visited %d distinct cells; expected ≥20", len(seen))
	}
}

// Smoke-test the Qix initial state — every joint should spawn inside
// an open cell. Uses the real constructor so we exercise its
// fallback-to-centre behaviour as well.
func TestQixJointsStayInOpenArea(t *testing.T) {
	f := newField(20, 20)
	q := newQix(newDeterministicRng(t), f, 8, 6)
	for i, p := range q.pts {
		if !f.isOpen(int(p.x), int(p.y)) {
			t.Fatalf("joint %d at (%d,%d) not open at spawn", i,
				int(p.x), int(p.y))
		}
	}
}
