package wizardofwor

import (
	"fmt"
)

// The dungeon is an EDGE-based maze: cells are wide-open squares and the
// walls live on the borders between them. This matches the original
// arcade much better than a cell-based wall scheme (Pac-Man style)
// because Wizard of Wor's monsters and players occupy full cell-sized
// sprites and walk through the empty cells, brushing up against the
// thin wall lines.
//
// The cage at the centre is a single cell with vertical walls on its
// left and right and a closed floor, but an open top — the cage door.
// Monsters spawn inside it one at a time and emerge upward; the
// player respawns there after death and steps out into the corridor.
//
// Row tunnelRow is the warp tunnel: the outer left/right walls of that
// one row are removed. Entities running off either side wrap around
// to the opposite end (see entity.advance).

const (
	mazeCols  = 13
	mazeRows  = 7
	tunnelRow = 3
	cageCol   = 6
	cageRow   = 3

	// Player spawn point — bottom-left corner of the playfield, matching
	// the arcade. (Player 2's slot would be the mirror at (mazeCols-2,
	// mazeRows-1); we're single-player here.) Worriors emerge from this
	// corner, NOT the central cage; the cage is monster-only.
	playerSpawnCol = 1
	playerSpawnRow = mazeRows - 1
)

// maze holds the edge-wall data plus the cosmetic decoration mask for
// the cage cell. The grids are fixed in size; we always run on a
// 13×7 dungeon regardless of which canonical layout was loaded.
type maze struct {
	// vwalls[r][c] reports a vertical wall to the LEFT of cell (c, r).
	// c ranges 0..mazeCols inclusive — vwalls[r][0] is the left outer
	// edge, vwalls[r][mazeCols] is the right outer edge.
	vwalls [mazeRows][mazeCols + 1]bool
	// hwalls[r][c] reports a horizontal wall above cell (c, r).
	// r ranges 0..mazeRows inclusive — hwalls[0][c] is the top edge,
	// hwalls[mazeRows][c] is the bottom edge.
	hwalls [mazeRows + 1][mazeCols]bool
}

// layoutSpec is the source format for a dungeon. Each layout is a
// (2*mazeRows+1)-line block, each (2*mazeCols+1) runes wide:
//
//	+-+-+...     line 0: top edge, with '-' for wall, ' ' for opening
//	|.|.|...     line 1: row-0 cell row, '|' for vertical wall, ' ' otherwise.
//	+-+-+...     line 2: between row 0 and row 1.
//	...          repeat for each row.
//
// '+' at corner positions is purely cosmetic — the parser ignores it.
// Cell content runes (at positions 2c+1 on odd lines) are also ignored
// here; the cage location is fixed by the cageCol/cageRow constants.
type layoutSpec [2*mazeRows + 1]string

// layouts is the table of canonical dungeon layouts. Each is symmetric
// left/right and top/bottom, and shares the canonical cage at
// (cageCol, cageRow) plus the side warp on row tunnelRow. Higher
// dungeons cycle through them; the original arcade also reused a
// small pool of dungeon layouts across many waves.
var layouts = []layoutSpec{
	// Dungeon 1 — the "lobby". Open corridors with light internal walls
	// shaping four roughly mirrored quadrants. The cage door opens up
	// the middle column.
	{
		"+-+-+-+-+-+-+-+-+-+-+-+-+-+",
		"|                         |",
		"+ + +-+ + + + + + + +-+ + +",
		"| |   | |   |   |   |   | |",
		"+ + + + +-+ + + + +-+ + + +",
		"|   |               |     |",
		"+ + + +-+-+-+ +-+-+-+ + + +",
		"            |C|            ",
		"+ + + +-+-+-+-+-+-+-+ + + +",
		"|   |               |     |",
		"+ + + + +-+ + + + +-+ + + +",
		"| |   | |   |   |   |   | |",
		"+ + +-+ + + + + + + +-+ + +",
		"|                         |",
		"+-+-+-+-+-+-+-+-+-+-+-+-+-+",
	},
	// Dungeon 2 — denser walls, narrower lanes, more dead-ends near the
	// outer rim. The cage corridor is now flanked by short stubs.
	{
		"+-+-+-+-+-+-+-+-+-+-+-+-+-+",
		"|     |   |       |   |   |",
		"+ +-+ + + + +-+-+ + + + +-+",
		"|   | |   | |   | |   | | |",
		"+ + + + +-+ + + + +-+ + + +",
		"| |     |           |     |",
		"+ + +-+ +-+-+ +-+-+ +-+ + +",
		"            |C|            ",
		"+ + +-+ +-+-+-+-+-+ +-+ + +",
		"| |     |           |     |",
		"+ + + + +-+ + + + +-+ + + +",
		"|   | |   | |   | |   | | |",
		"+-+ + + + + +-+-+ + + + +-+",
		"|     |   |       |   |   |",
		"+-+-+-+-+-+-+-+-+-+-+-+-+-+",
	},
	// Dungeon 3 — long lanes with vertical bars. Easier to line up shots
	// but the monsters can ambush from the perpendicular corridors.
	{
		"+-+-+-+-+-+-+-+-+-+-+-+-+-+",
		"|       |           |     |",
		"+ +-+ + + + + + + + + + +-+",
		"| | | |   | | | | |   | | |",
		"+ + + + + + + + + + + + + +",
		"| |   |   |       |   |   |",
		"+ + + + +-+-+ +-+-+ + + + +",
		"            |C|            ",
		"+ + + + +-+-+-+-+-+ + + + +",
		"| |   |   |       |   |   |",
		"+ + + + + + + + + + + + + +",
		"| | | |   | | | | |   | | |",
		"+-+ + + + + + + + + + + +-+",
		"|       |           |     |",
		"+-+-+-+-+-+-+-+-+-+-+-+-+-+",
	},
	// Dungeon 4 — the "shouldered" map. Two short outer walls pinch
	// row 0 and row 6 into three lobes; otherwise the floor plan
	// mirrors dungeon 1 so connectivity is guaranteed (the player can
	// always reach the corridor leading to the cage and to every
	// monster). Visually distinct, structurally safe.
	{
		"+-+-+-+-+-+-+-+-+-+-+-+-+-+",
		"|     |             |     |",
		"+ + +-+ + + + + + + +-+ + +",
		"| |   | |   |   |   |   | |",
		"+ + + + +-+ + + + +-+ + + +",
		"|   |               |     |",
		"+ + + +-+-+-+ +-+-+-+ + + +",
		"            |C|            ",
		"+ + + +-+-+-+-+-+-+-+ + + +",
		"|   |               |     |",
		"+ + + + +-+ + + + +-+ + + +",
		"| |   | |   |   |   |   | |",
		"+ + +-+ + + + + + + +-+ + +",
		"|     |             |     |",
		"+-+-+-+-+-+-+-+-+-+-+-+-+-+",
	},
}

// newMaze constructs the maze for the given dungeon number (1-based).
// Dungeon numbers larger than the layout table wrap around modulo,
// reusing the catalogue indefinitely — exactly what the arcade does.
func newMaze(dungeon int) *maze {
	idx := (dungeon - 1) % len(layouts)
	if idx < 0 {
		idx += len(layouts)
	}
	return parseMaze(layouts[idx])
}

// parseMaze walks the source layout strings and lifts them into the
// vwalls / hwalls grids. It panics on shape errors — these are
// developer mistakes in the layout literal, not runtime conditions.
func parseMaze(src layoutSpec) *maze {
	wantWidth := 2*mazeCols + 1
	m := &maze{}
	for i, line := range src {
		if len(line) != wantWidth {
			panic(fmt.Sprintf("wizardofwor: layout line %d width %d, want %d (%q)",
				i, len(line), wantWidth, line))
		}
		if i%2 == 0 {
			// Horizontal wall row. r = i/2 names the index in hwalls.
			r := i / 2
			for c := 0; c < mazeCols; c++ {
				ch := line[2*c+1]
				m.hwalls[r][c] = ch == '-'
			}
		} else {
			// Cell row. r = (i-1)/2 is the cell-row index.
			r := (i - 1) / 2
			for c := 0; c <= mazeCols; c++ {
				ch := line[2*c]
				m.vwalls[r][c] = ch == '|'
			}
		}
	}

	// Reinforce the cage geometry so a sloppy layout can't accidentally
	// leave it punctured. The cage must be 3-sided: floor + side walls
	// closed, ceiling open. Without this the spawn flow gets weird.
	m.vwalls[cageRow][cageCol] = true     // cage left wall
	m.vwalls[cageRow][cageCol+1] = true   // cage right wall
	m.hwalls[cageRow][cageCol] = false    // cage door (top open)
	m.hwalls[cageRow+1][cageCol] = true   // cage floor

	return m
}

// canMove asks whether an entity in cell (c, r) may step one tile in
// direction d. It handles the side warp on tunnelRow: tunnel-space
// cells (c < 0 or c >= mazeCols on that row) report all horizontal
// moves as allowed so the entity can keep walking off the playfield
// until entity.advance's wrap kicks in.
func (m *maze) canMove(c, r int, d direction) bool {
	switch d {
	case dirUp:
		if r <= 0 {
			return false
		}
		if c < 0 || c >= mazeCols {
			return false
		}
		return !m.hwalls[r][c]
	case dirDown:
		if r >= mazeRows-1 {
			return false
		}
		if c < 0 || c >= mazeCols {
			return false
		}
		return !m.hwalls[r+1][c]
	case dirLeft:
		if r == tunnelRow {
			// In tunnel space (c == -1) any horizontal move is allowed.
			if c < 0 {
				return true
			}
			if c == 0 {
				return !m.vwalls[r][0]
			}
			if c >= mazeCols {
				return true
			}
			return !m.vwalls[r][c]
		}
		if c <= 0 || c >= mazeCols {
			return false
		}
		return !m.vwalls[r][c]
	case dirRight:
		if r == tunnelRow {
			if c < 0 {
				return true
			}
			if c == mazeCols-1 {
				return !m.vwalls[r][mazeCols]
			}
			if c >= mazeCols {
				return true
			}
			return !m.vwalls[r][c+1]
		}
		if c < 0 || c >= mazeCols-1 {
			return false
		}
		return !m.vwalls[r][c+1]
	}
	return false
}

// hasWallTop reports whether the wall above cell (c, r) is solid.
// Used by drawing and by bullet-vs-wall intersections.
func (m *maze) hasWallTop(c, r int) bool {
	if r < 0 || r > mazeRows || c < 0 || c >= mazeCols {
		return true
	}
	return m.hwalls[r][c]
}

// hasWallLeft reports whether the wall to the left of cell (c, r) is
// solid. Out-of-range queries report true (treat as solid) so callers
// don't need bounds checks; the warp openings are recorded explicitly
// in the layout for r == tunnelRow.
func (m *maze) hasWallLeft(c, r int) bool {
	if r < 0 || r >= mazeRows || c < 0 || c > mazeCols {
		return true
	}
	return m.vwalls[r][c]
}

