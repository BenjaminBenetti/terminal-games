package pacman

import "testing"

// TestMazeDimensions asserts that the canonical layout in rawMaze has
// the right shape — every row must be exactly mazeCols runes wide, or
// every tile lookup downstream is off by one.
func TestMazeDimensions(t *testing.T) {
	if len(rawMaze) != mazeRows {
		t.Fatalf("rawMaze has %d rows; want %d", len(rawMaze), mazeRows)
	}
	for r, row := range rawMaze {
		if len(row) != mazeCols {
			t.Errorf("rawMaze row %d is %d cols wide; want %d", r, len(row), mazeCols)
		}
	}
}

// TestMazeEnergizerCount confirms the four-pellet rule: the corners
// each hold a single energizer, total 4.
func TestMazeEnergizerCount(t *testing.T) {
	m := newMaze()
	n := 0
	for r := 0; r < mazeRows; r++ {
		for c := 0; c < mazeCols; c++ {
			if m.wallAt(c, r) == tilePellet {
				n++
			}
		}
	}
	if n != 4 {
		t.Errorf("energizer count = %d; want 4", n)
	}
}

// TestMazeGhostHouseDoor verifies that row 12 has the two-cell door at
// the expected columns and that the rest of the row is composed of
// walls / void surrounding the house top.
func TestMazeGhostHouseDoor(t *testing.T) {
	m := newMaze()
	if got := m.wallAt(13, 12); got != tileDoor {
		t.Errorf("wallAt(13,12) = %v; want tileDoor", got)
	}
	if got := m.wallAt(14, 12); got != tileDoor {
		t.Errorf("wallAt(14,12) = %v; want tileDoor", got)
	}
	// Cells immediately outside the door should be ghost-house wall.
	if got := m.wallAt(12, 12); got != tileWall {
		t.Errorf("wallAt(12,12) = %v; want tileWall (door pillar)", got)
	}
	if got := m.wallAt(15, 12); got != tileWall {
		t.Errorf("wallAt(15,12) = %v; want tileWall (door pillar)", got)
	}
}

// TestPacWalkability checks that Pac-Man is blocked from walls, the
// door, the ghost-house interior, and void corners, but can walk into
// open corridor tiles.
func TestPacWalkability(t *testing.T) {
	m := newMaze()
	tests := []struct {
		col, row int
		want     bool
		name     string
	}{
		{1, 1, true, "open corridor"},
		{0, 0, false, "top-left wall"},
		{13, 12, false, "ghost-house door"},
		{13, 14, false, "ghost-house interior"},
		{0, 10, false, "void corner above tunnel"},
		{-1, 14, true, "tunnel mouth wrap (left)"},
		{mazeCols, 14, true, "tunnel mouth wrap (right)"},
		{0, 14, true, "tunnel row open corridor"},
	}
	for _, tc := range tests {
		if got := m.walkableForPac(tc.col, tc.row); got != tc.want {
			t.Errorf("%s: walkableForPac(%d,%d) = %v; want %v",
				tc.name, tc.col, tc.row, got, tc.want)
		}
	}
}

// TestGhostWalkability checks that ghosts may enter the house
// interior, may pass the door only when permitted, and otherwise
// share Pac-Man's walkability rules.
func TestGhostWalkability(t *testing.T) {
	m := newMaze()
	if !m.walkableForGhost(13, 14, false) {
		t.Errorf("ghost cannot enter house interior; should be walkable")
	}
	if m.walkableForGhost(13, 12, false) {
		t.Errorf("ghost passed locked door (permitDoor=false)")
	}
	if !m.walkableForGhost(13, 12, true) {
		t.Errorf("ghost can't pass door with permitDoor=true")
	}
}

// TestEatPellet asserts the eatPellet bookkeeping: a real pellet is
// returned and zeroed; eating again yields tileEmpty; walls and empty
// tiles never report a pellet eaten.
func TestEatPellet(t *testing.T) {
	m := newMaze()
	// (1, 1) is a dot.
	if got := m.eatPellet(1, 1); got != tileDot {
		t.Errorf("first eatPellet(1,1) = %v; want tileDot", got)
	}
	if got := m.eatPellet(1, 1); got != tileEmpty {
		t.Errorf("second eatPellet(1,1) = %v; want tileEmpty", got)
	}
	// (1, 3) is an energizer.
	if got := m.eatPellet(1, 3); got != tilePellet {
		t.Errorf("eatPellet(1,3) = %v; want tilePellet", got)
	}
	// (0, 0) is a wall — eating should be a no-op returning empty.
	if got := m.eatPellet(0, 0); got != tileEmpty {
		t.Errorf("eatPellet(0,0) wall = %v; want tileEmpty", got)
	}
}

// TestRemainingDotsDecreases sanity-checks the dot counter as pellets
// are eaten.
func TestRemainingDotsDecreases(t *testing.T) {
	m := newMaze()
	start := m.remainingDots()
	if start < 200 {
		t.Errorf("starting dot count = %d; suspiciously low", start)
	}
	m.eatPellet(1, 1)
	if got := m.remainingDots(); got != start-1 {
		t.Errorf("after one eat: remainingDots=%d; want %d", got, start-1)
	}
}
