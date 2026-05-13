package rallyx

import (
	"strings"
	"testing"
)

// TestMazeShape verifies every defined stage layout is the expected
// size and that the composition pipeline didn't drop or insert
// characters anywhere.
func TestMazeShape(t *testing.T) {
	for i := range stageMaps {
		raw := buildMaze(&stageMaps[i])
		if got, want := len(raw), mazeRows; got != want {
			t.Errorf("stage %d (%q) has %d rows, want %d",
				i+1, stageMaps[i].name, got, want)
			continue
		}
		for r, row := range raw {
			if got, want := len(row), mazeCols; got != want {
				t.Errorf("stage %d (%q) row %d is %d cols wide, want %d (%q)",
					i+1, stageMaps[i].name, r, got, want, row)
			}
		}
	}
}

// TestMazeInventory makes sure every stage layout contains the
// expected number of normal flags (10) plus a player spawn and at
// least one enemy spawn. Without this guard a typo in a panelLayout
// or in a corridor decoration could silently lose a flag and make
// the round impossible to clear.
func TestMazeInventory(t *testing.T) {
	for i := range stageMaps {
		stage := i + 1
		m := newMaze(stage)
		if got := len(m.flags); got != 10 {
			t.Errorf("stage %d (%q): normal flags = %d, want 10",
				stage, stageMaps[i].name, got)
		}
		if m.specialFlag == nil {
			t.Errorf("stage %d (%q): special flag missing", stage, stageMaps[i].name)
		}
		if m.playerSpawn == [2]float64{0, 0} {
			t.Errorf("stage %d (%q): player spawn missing", stage, stageMaps[i].name)
		}
		if len(m.enemySpawns) < 2 {
			t.Errorf("stage %d (%q): only %d enemy spawns; want at least 2",
				stage, stageMaps[i].name, len(m.enemySpawns))
		}
	}
}

// TestMazeConnected runs a flood-fill from the player spawn for
// every stage and verifies every flag tile is reachable. A dead-end
// pocket with a flag inside is fine; an island of road that the
// player can't physically reach is a bug.
func TestMazeConnected(t *testing.T) {
	for i := range stageMaps {
		stage := i + 1
		m := newMaze(stage)
		startC := int(m.playerSpawn[0])
		startR := int(m.playerSpawn[1])

		visited := make([][]bool, mazeRows)
		for j := range visited {
			visited[j] = make([]bool, mazeCols)
		}
		queue := [][2]int{{startC, startR}}
		visited[startR][startC] = true
		for len(queue) > 0 {
			next := queue[0]
			queue = queue[1:]
			c0, r0 := next[0], next[1]
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nc, nr := c0+d[0], r0+d[1]
				if nc < 0 || nc >= mazeCols || nr < 0 || nr >= mazeRows {
					continue
				}
				if visited[nr][nc] {
					continue
				}
				if m.at(nc, nr) == tileWall {
					continue
				}
				visited[nr][nc] = true
				queue = append(queue, [2]int{nc, nr})
			}
		}
		for _, f := range m.flags {
			if !visited[f.row][f.col] {
				t.Errorf("stage %d (%q): flag at (%d,%d) unreachable from player spawn",
					stage, stageMaps[i].name, f.col, f.row)
			}
		}
		if m.specialFlag != nil && !visited[m.specialFlag.row][m.specialFlag.col] {
			t.Errorf("stage %d (%q): special flag at (%d,%d) unreachable",
				stage, stageMaps[i].name, m.specialFlag.col, m.specialFlag.row)
		}
		for _, sp := range m.enemySpawns {
			c, r := int(sp[0]), int(sp[1])
			if !visited[r][c] {
				t.Errorf("stage %d (%q): enemy spawn at (%d,%d) unreachable from player",
					stage, stageMaps[i].name, c, r)
			}
		}
	}
}

// TestStageCycle verifies stages cycle through stageMaps so stage
// (N+len) reuses the same layout as stage N — this is what gives
// the wrap-around progression after every map has been seen.
func TestStageCycle(t *testing.T) {
	if len(stageMaps) < 2 {
		t.Skip("need at least 2 stage maps to test cycling")
	}
	if mapForStage(1).name != mapForStage(1+len(stageMaps)).name {
		t.Errorf("stage %d should re-use the stage 1 layout", 1+len(stageMaps))
	}
	if mapForStage(1).name == mapForStage(2).name {
		t.Error("stages 1 and 2 must use distinct layouts")
	}
}

// TestMazeRender prints every stage's composed maze so a developer
// can eyeball them via `go test -run TestMazeRender -v`.
func TestMazeRender(t *testing.T) {
	for i := range stageMaps {
		raw := buildMaze(&stageMaps[i])
		t.Logf("\nStage %d: %s\n%s", i+1, stageMaps[i].name,
			strings.Join(raw[:], "\n"))
	}
}
