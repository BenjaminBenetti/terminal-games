package pacman

import (
	"math"
	"testing"
)

// allOpen is a canPasser that treats every tile as walkable, used to
// isolate movement-loop behaviour from maze topology.
func allOpen(int, int) bool { return true }

// walledAt returns a canPasser that blocks exactly the listed tiles.
func walledAt(blocked ...[2]int) canPasser {
	set := make(map[[2]int]struct{}, len(blocked))
	for _, b := range blocked {
		set[b] = struct{}{}
	}
	return func(c, r int) bool {
		_, hit := set[[2]int{c, r}]
		return !hit
	}
}

// TestAdvanceMovesAlongAxis: an entity moving right covers
// speed * dt tiles per call when unobstructed.
func TestAdvanceMovesAlongAxis(t *testing.T) {
	e := entity{x: 5.5, y: 5.5, dir: dirRight, speed: 4}
	e.advance(1.0, allOpen)
	if math.Abs(e.x-9.5) > 1e-6 {
		t.Errorf("x = %g; want 9.5", e.x)
	}
	if math.Abs(e.y-5.5) > 1e-6 {
		t.Errorf("y = %g; want 5.5", e.y)
	}
}

// TestAdvanceStopsAtWall: a wall directly in front of the entity stops
// it at the centre of the current tile.
func TestAdvanceStopsAtWall(t *testing.T) {
	e := entity{x: 5.5, y: 5.5, dir: dirRight, speed: 4}
	canPass := walledAt([2]int{6, 5})
	e.advance(1.0, canPass)
	if math.Abs(e.x-5.5) > 1e-6 {
		t.Errorf("blocked entity moved past wall: x=%g; want 5.5", e.x)
	}
	if e.dir != dirNone {
		t.Errorf("blocked entity dir = %v; want dirNone", e.dir)
	}
}

// TestAdvanceBufferedTurnAtIntersection: a desired direction is
// consumed when the entity reaches a tile centre and the new
// direction is walkable. Starting mid-tile ensures the turn isn't
// consumed at the initial position.
func TestAdvanceBufferedTurnAtIntersection(t *testing.T) {
	e := entity{x: 5.7, y: 5.5, dir: dirRight, speed: 4, desired: dirDown}
	// Travel ≈1 tile from x=5.7. Expected path: continue right until
	// the next tile centre at x=6.5, turn down there, then continue
	// downward for the remaining motion.
	e.advance(1.0, allOpen)
	if math.Abs(e.x-6.5) > 1e-6 {
		t.Errorf("x = %g; want 6.5 (turn should snap x to centre)", e.x)
	}
	// Travelled 0.8 right + 3.2 down → y = 5.5 + 3.2 = 8.7.
	if math.Abs(e.y-8.7) > 1e-6 {
		t.Errorf("y = %g; want 8.7", e.y)
	}
	if e.dir != dirDown {
		t.Errorf("dir = %v; want dirDown", e.dir)
	}
}

// TestAdvanceTurnAtCurrentCentre: when the entity starts exactly at a
// tile centre with a buffered turn, it turns *immediately* — no need
// to ride to the next intersection first. This matches the arcade
// behaviour where pressing a direction while aligned commits at once.
func TestAdvanceTurnAtCurrentCentre(t *testing.T) {
	e := entity{x: 5.5, y: 5.5, dir: dirRight, speed: 4, desired: dirDown}
	e.advance(0.1, allOpen)
	if e.dir != dirDown {
		t.Errorf("dir = %v; want dirDown (instant turn at centre)", e.dir)
	}
	if math.Abs(e.x-5.5) > 1e-6 {
		t.Errorf("x = %g; should not have moved horizontally", e.x)
	}
}

// TestAdvance180Reversal: a 180° turn applies immediately, even
// mid-tile, without waiting for an intersection.
func TestAdvance180Reversal(t *testing.T) {
	e := entity{x: 5.7, y: 5.5, dir: dirRight, speed: 4, desired: dirLeft}
	e.advance(0.05, allOpen)
	if e.dir != dirLeft {
		t.Errorf("dir = %v; want dirLeft (instant 180°)", e.dir)
	}
	if e.x > 5.7-1e-6 {
		t.Errorf("x = %g; should have moved left from 5.7", e.x)
	}
}

// TestAdvanceTunnelWrap: an entity moving past the left tunnel mouth
// wraps to the right side. Wrap only happens on tunnelRow.
func TestAdvanceTunnelWrap(t *testing.T) {
	e := entity{x: -0.5, y: float64(tunnelRow) + 0.5, dir: dirLeft, speed: 4}
	e.advance(0.5, allOpen) // moves 2 tiles further left → x = -2.5, triggers wrap
	if e.x < float64(mazeCols)-3 {
		t.Errorf("wrap failed: x=%g; expected near right edge", e.x)
	}
}

// TestAdvanceBufferedTurnPersistsThroughCorner verifies that a buffered
// desired direction that wasn't legal at one intersection is still
// available at the next. This is the "hold-direction" feel the arcade
// player relies on.
func TestAdvanceBufferedTurnPersistsThroughCorner(t *testing.T) {
	// Layout: a 1-tile-wide corridor going right, with a turn south
	// available only at column 8.
	canPass := func(c, r int) bool {
		if r == 5 {
			return c >= 0 && c <= 9
		}
		if c == 8 && r >= 5 && r <= 9 {
			return true
		}
		return false
	}
	e := entity{x: 5.5, y: 5.5, dir: dirRight, speed: 4, desired: dirDown}
	// At (6.5, 5.5) the down neighbour is wall — turn fails.
	// At (7.5, 5.5) likewise.
	// At (8.5, 5.5) down is open — turn succeeds.
	e.advance(1.0, canPass)
	if math.Abs(e.x-8.5) > 0.5 {
		t.Errorf("expected to arrive at column 8; got x=%g", e.x)
	}
	if e.dir != dirDown {
		t.Errorf("dir = %v; want dirDown (turn should have been consumed at the open corner)", e.dir)
	}
}

// TestDirectionOpposites sanity-checks the direction.opposite() table.
func TestDirectionOpposites(t *testing.T) {
	cases := []struct {
		in, want direction
	}{
		{dirUp, dirDown},
		{dirDown, dirUp},
		{dirLeft, dirRight},
		{dirRight, dirLeft},
		{dirNone, dirNone},
	}
	for _, tc := range cases {
		if got := tc.in.opposite(); got != tc.want {
			t.Errorf("(%v).opposite() = %v; want %v", tc.in, got, tc.want)
		}
	}
}
