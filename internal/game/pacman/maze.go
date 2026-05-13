package pacman

// The maze is the canonical 28-column × 31-row Pac-Man playfield. Each
// run-time tile is one of:
//
//   tileWall   — solid, blocks every entity
//   tileEmpty  — walkable, no pellet
//   tileDot    — walkable, holds a 10-point dot
//   tilePellet — walkable, holds a 50-point energizer pellet
//   tileDoor   — ghost-house door: ghosts pass through, Pac-Man can't
//   tileVoid   — outside the playfield (drawn as background)
//
// The layout below is the standard arcade map: four energizers in the
// outer corners, the ghost house dead-centre with its door on the top
// edge, and the side tunnel on row 14 that wraps left-edge to
// right-edge.
//
// Each row is exactly mazeCols runes wide; any drift between rows
// would break tile lookups, so the constructor verifies it.

const (
	mazeCols = 28
	mazeRows = 31
	// tunnelRow is the y-tile that exits via the side wraps. Both row
	// edges are tileVoid in the source map; the wrap is implemented in
	// the movement step.
	tunnelRow = 14
)

// tile is one cell of the maze.
type tile uint8

const (
	tileVoid       tile = iota // off-playfield; never walkable
	tileWall                   // solid wall
	tileEmpty                  // walkable corridor, no pellet
	tileDot                    // walkable, holds a 10-pt dot
	tilePellet                 // walkable, holds a 50-pt energizer
	tileDoor                   // ghost-house door; passable for ghosts
	tileGhostHouse             // ghost-house interior; ghosts only
)

// rawMaze is the source layout. Every row is exactly mazeCols runes.
// The runes encode:
//   '#' wall
//   '.' dot
//   'o' energizer pellet
//   ' ' walkable empty (no pellet)
//   '-' ghost-house door
//   'H' ghost-house interior (ghosts only)
//   'X' void (outside the maze; the dead corners next to the tunnel mouths).
//
// Layout follows the original 1980 arcade map. The energizers are the
// four 'o' tiles; the ghost house is the 7-wide × 4-tall room with the
// door at row 12; the tunnel row is row 14 (the row with two solitary
// '.' dots at the far edges that the maze wraps through).
var rawMaze = [mazeRows]string{
	"############################", // 0
	"#............##............#", // 1
	"#.####.#####.##.#####.####.#", // 2
	"#o####.#####.##.#####.####o#", // 3
	"#.####.#####.##.#####.####.#", // 4
	"#..........................#", // 5
	"#.####.##.########.##.####.#", // 6
	"#.####.##.########.##.####.#", // 7
	"#......##....##....##......#", // 8
	"######.##### ## #####.######", // 9
	"XXXXX#.##### ## #####.#XXXXX", // 10
	"XXXXX#.##          ##.#XXXXX", // 11
	"XXXXX#.## ###--### ##.#XXXXX", // 12
	"######.## #HHHHHH# ##.######", // 13
	"      .   #HHHHHH#   .      ", // 14  tunnel row (left/right cols are walkable mouths)
	"######.## #HHHHHH# ##.######", // 15
	"XXXXX#.## ######## ##.#XXXXX", // 16
	"XXXXX#.##          ##.#XXXXX", // 17
	"XXXXX#.## ######## ##.#XXXXX", // 18
	"######.## ######## ##.######", // 19
	"#............##............#", // 20
	"#.####.#####.##.#####.####.#", // 21
	"#.####.#####.##.#####.####.#", // 22
	"#o..##.......  .......##..o#", // 23  Pac-Man spawn between cols 13-14
	"###.##.##.########.##.##.###", // 24
	"###.##.##.########.##.##.###", // 25
	"#......##....##....##......#", // 26
	"#.##########.##.##########.#", // 27
	"#.##########.##.##########.#", // 28
	"#..........................#", // 29
	"############################", // 30
}

// maze is the runtime grid plus a few derived constants. Two parallel
// grids are kept: walls (immutable per game) and pellets (mutated as
// Pac-Man eats). Splitting them avoids re-walking the layout to ask
// "is there a pellet here right now?" on every dot collection.
type maze struct {
	walls   [mazeRows][mazeCols]tile // initial tile per cell; never mutated
	pellets [mazeRows][mazeCols]tile // tileDot / tilePellet / tileEmpty, mutated by eatPellet
	dotsTotal int
}

// newMaze parses rawMaze into a fresh maze instance.
func newMaze() *maze {
	m := &maze{}
	dots := 0
	for r, row := range rawMaze {
		if len(row) != mazeCols {
			panic("pacman: maze row " + itoa(r) + " is not exactly mazeCols wide")
		}
		for c, ch := range row {
			var t tile
			switch ch {
			case '#':
				t = tileWall
			case '.':
				t = tileDot
			case 'o':
				t = tilePellet
			case ' ':
				t = tileEmpty
			case '-':
				t = tileDoor
			case 'H':
				t = tileGhostHouse
			case 'X':
				t = tileVoid
			default:
				panic("pacman: unknown maze rune at row " + itoa(r) + " col " + itoa(c))
			}
			m.walls[r][c] = t
			switch t {
			case tileDot, tilePellet:
				m.pellets[r][c] = t
				dots++
			case tileEmpty, tileDoor:
				m.pellets[r][c] = tileEmpty
			default:
				m.pellets[r][c] = tileEmpty
			}
		}
	}
	m.dotsTotal = dots
	return m
}

// wallAt returns the structural tile (wall, door, void, or open) at
// (col, row). Out-of-range coordinates report as tileVoid so callers
// can treat off-maze access as "blocked but not a wall".
func (m *maze) wallAt(col, row int) tile {
	if row < 0 || row >= mazeRows || col < 0 || col >= mazeCols {
		return tileVoid
	}
	return m.walls[row][col]
}

// pelletAt returns the current pellet state at (col, row).
func (m *maze) pelletAt(col, row int) tile {
	if row < 0 || row >= mazeRows || col < 0 || col >= mazeCols {
		return tileEmpty
	}
	return m.pellets[row][col]
}

// eatPellet removes the pellet at (col, row), if any, and reports what
// was eaten (tileDot, tilePellet, or tileEmpty when nothing was there).
func (m *maze) eatPellet(col, row int) tile {
	if row < 0 || row >= mazeRows || col < 0 || col >= mazeCols {
		return tileEmpty
	}
	p := m.pellets[row][col]
	if p == tileDot || p == tilePellet {
		m.pellets[row][col] = tileEmpty
	}
	return p
}

// remainingDots reports how many pellets are still on the board.
func (m *maze) remainingDots() int {
	n := 0
	for r := 0; r < mazeRows; r++ {
		for c := 0; c < mazeCols; c++ {
			if p := m.pellets[r][c]; p == tileDot || p == tilePellet {
				n++
			}
		}
	}
	return n
}

// walkableForPac returns true if a Pac-Man-controlled entity may
// occupy (col, row). Walls, doors, void, and the ghost-house interior
// are off-limits; the tunnel-row mouths (cols -1 / mazeCols) are
// treated as walkable to let the tunnel wrap work.
func (m *maze) walkableForPac(col, row int) bool {
	if row == tunnelRow && (col < 0 || col >= mazeCols) {
		return true
	}
	t := m.wallAt(col, row)
	return t == tileEmpty || t == tileDot || t == tilePellet
}

// walkableForGhost returns true if a ghost may occupy (col, row).
// The ghost-house interior is always walkable for ghosts; the door is
// walkable only when permitDoor is true (i.e. when the ghost is either
// returning home as eyes or freshly emerging from the house).
func (m *maze) walkableForGhost(col, row int, permitDoor bool) bool {
	if row == tunnelRow && (col < 0 || col >= mazeCols) {
		return true
	}
	t := m.wallAt(col, row)
	switch t {
	case tileWall, tileVoid:
		return false
	case tileDoor:
		return permitDoor
	}
	return true
}

// itoa is a tiny strconv.Itoa replacement so the package can build
// without a strconv import — wrapping the panic message inside
// init-time validation. Pulled out so newMaze can use it.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
