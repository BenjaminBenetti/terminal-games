package pacman

import (
	"math/rand"
	"testing"
)

// TestBlinkyChaseTargetsPacMan: Blinky's chase target is just
// Pac-Man's current tile.
func TestBlinkyChaseTargetsPacMan(t *testing.T) {
	g := newGhost(blinky, rand.New(rand.NewSource(1)), 0)
	in := aiInputs{
		pacTileX: 10,
		pacTileY: 17,
		pacDir:   dirRight,
		phase:    modeChase,
	}
	got := g.targetTile(in)
	if got != [2]int{10, 17} {
		t.Errorf("Blinky chase target = %v; want {10,17}", got)
	}
}

// TestPinkyChaseTargetsAheadAndUpBug: Pinky's chase target is four
// tiles ahead. When Pac-Man faces up the famous "up bug" also shifts
// the target four tiles left.
func TestPinkyChaseTargetsAheadAndUpBug(t *testing.T) {
	g := newGhost(pinky, rand.New(rand.NewSource(1)), 0)
	cases := []struct {
		dir  direction
		want [2]int
	}{
		{dirRight, [2]int{14, 10}},     // +4 X
		{dirLeft, [2]int{6, 10}},       // -4 X
		{dirDown, [2]int{10, 14}},      // +4 Y
		{dirUp, [2]int{6, 6}},          // -4 Y AND -4 X (the up-bug)
	}
	for _, tc := range cases {
		in := aiInputs{pacTileX: 10, pacTileY: 10, pacDir: tc.dir, phase: modeChase}
		got := g.targetTile(in)
		if got != tc.want {
			t.Errorf("Pinky %v target = %v; want %v", tc.dir, got, tc.want)
		}
	}
}

// TestInkyChaseDoublesVectorThroughBlinky: Inky's chase target is the
// pivot (2 ahead of Pac-Man, with the same up bug) reflected through
// Blinky's tile.
func TestInkyChaseDoublesVectorThroughBlinky(t *testing.T) {
	g := newGhost(inky, rand.New(rand.NewSource(1)), 0)
	in := aiInputs{
		pacTileX:    10,
		pacTileY:    10,
		pacDir:      dirRight,
		blinkyTileX: 8,
		blinkyTileY: 10,
		phase:       modeChase,
	}
	// Pivot = (12, 10). Target = 2*pivot - blinky = (16, 10).
	got := g.targetTile(in)
	if got != [2]int{16, 10} {
		t.Errorf("Inky target = %v; want {16,10}", got)
	}
}

// TestClydeChaseSwitchesNearPacMan: Clyde targets Pac-Man directly
// when 8+ tiles away, but falls back to his scatter corner when
// within 8 tiles.
func TestClydeChaseSwitchesNearPacMan(t *testing.T) {
	g := newGhost(clyde, rand.New(rand.NewSource(1)), 0)
	g.x, g.y = 1.5, 28.5 // ghost-house corner, close to bottom-left

	// Far Pac-Man: chase target = Pac-Man.
	in := aiInputs{pacTileX: 26, pacTileY: 1, pacDir: dirLeft, phase: modeChase}
	got := g.targetTile(in)
	if got != [2]int{26, 1} {
		t.Errorf("Clyde far target = %v; want {26,1}", got)
	}

	// Near Pac-Man: revert to scatter corner.
	in = aiInputs{pacTileX: 3, pacTileY: 28, pacDir: dirLeft, phase: modeChase}
	got = g.targetTile(in)
	if got != scatterTarget[clyde] {
		t.Errorf("Clyde near target = %v; want %v", got, scatterTarget[clyde])
	}
}

// TestScatterIgnoresPacMan: in scatter mode every ghost returns its
// own corner regardless of Pac-Man state.
func TestScatterIgnoresPacMan(t *testing.T) {
	for k := blinky; k <= clyde; k++ {
		g := newGhost(k, rand.New(rand.NewSource(1)), 0)
		in := aiInputs{pacTileX: 10, pacTileY: 10, pacDir: dirUp, phase: modeScatter}
		got := g.targetTile(in)
		if got != scatterTarget[k] {
			t.Errorf("%s scatter target = %v; want %v", k, got, scatterTarget[k])
		}
	}
}

// TestChooseDirectionPicksNearestNeighbour: at a 4-way intersection
// the AI should head toward the neighbour closest to the target,
// ignoring the forbidden reversal direction.
func TestChooseDirectionPicksNearestNeighbour(t *testing.T) {
	g := newGhost(blinky, rand.New(rand.NewSource(1)), 0)
	// canPass: every tile in a 5x5 area is walkable.
	canPass := func(c, r int) bool { return c >= 0 && c < 5 && r >= 0 && r < 5 }

	// At (2,2), target (0,0). Forbidden = down (came from above).
	// Up neighbour (2,1) → dist^2 = 4+1 = 5
	// Left neighbour (1,2) → dist^2 = 1+4 = 5  → tied!
	// Tie-break: up beats left (arcade order).
	got := g.chooseDirection(2, 2, dirDown, [2]int{0, 0}, canPass)
	if got != dirUp {
		t.Errorf("tie-break expected dirUp; got %v", got)
	}

	// Asymmetric target should pick the unique nearest.
	got = g.chooseDirection(2, 2, dirDown, [2]int{4, 2}, canPass)
	if got != dirRight {
		t.Errorf("target (4,2) expected dirRight; got %v", got)
	}
}

// TestChooseDirectionForbidsReverse confirms the AI never picks the
// 180° reversal, even when it'd be the optimal move.
func TestChooseDirectionForbidsReverse(t *testing.T) {
	g := newGhost(blinky, rand.New(rand.NewSource(1)), 0)
	canPass := func(c, r int) bool { return c >= 0 && c < 5 && r >= 0 && r < 5 }
	// Came from above (was moving down). Target above. Reversing would
	// be optimal but is forbidden.
	got := g.chooseDirection(2, 2, dirUp, [2]int{2, 0}, canPass)
	if got == dirUp {
		t.Errorf("AI chose forbidden reversal (dirUp)")
	}
}

// TestNewGhostStartingPositions checks each ghost spawns at its
// canonical slot with the expected initial mode.
func TestNewGhostStartingPositions(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	cases := []struct {
		kind  ghostKind
		x, y  float64
		mode  ghostMode
	}{
		{blinky, 13.5, 11.5, modeScatter},
		{pinky, 13.5, 14.5, modeInHouse},
		{inky, 11.5, 14.5, modeInHouse},
		{clyde, 15.5, 14.5, modeInHouse},
	}
	for _, tc := range cases {
		g := newGhost(tc.kind, rng, 0)
		if g.x != tc.x || g.y != tc.y {
			t.Errorf("%s spawn = (%g, %g); want (%g, %g)", tc.kind, g.x, g.y, tc.x, tc.y)
		}
		if g.mode != tc.mode {
			t.Errorf("%s spawn mode = %v; want %v", tc.kind, g.mode, tc.mode)
		}
	}
}
