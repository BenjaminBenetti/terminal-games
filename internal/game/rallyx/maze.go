package rallyx

import (
	"fmt"
	"strings"
)

// The Rally-X world is a single large scrolling maze. Unlike the
// Pac-Man playfield, the world is meaningfully larger than the visible
// viewport — the original cabinet only shows a fraction of the map at
// a time and the radar in the top-right tells you what's coming. We
// preserve that property here: the map is 32×24 tiles, the viewport
// renders ~14×10 tiles at most, and the radar reduces the whole world
// to a strip the player can glance at.
//
// Tile semantics:
//
//   tileWall  — solid; blocks every car
//   tileRoad  — drivable
//   tileRock  — drivable tile with a destructible obstacle; driving
//               into it destroys the car (and the rock)
//
// Flags, the special flag, the lucky flag, the player spawn, and the
// enemy spawns are NOT stored on the tile grid — they're parsed out
// of the layout into dedicated structs so the per-frame "did I drive
// over an item?" check is a small list scan instead of a 2D lookup
// for ~10 items.

const (
	mazeCols = 32
	mazeRows = 24

	// Panel grid dimensions: the world is a 5×4 lattice of 5×4-cell
	// "panel" blocks separated by 1-cell corridors. The numbers below
	// derive the rest of the geometry; changing them re-derives the
	// world size in buildMaze.
	panelCols   = 5
	panelRows   = 4
	panelHeight = 4
)

type tile uint8

const (
	tileRoad tile = iota
	tileWall
	tileRock
)

// A panel is a 4-row × 5-col block. Each entry is a 4-string array of
// 5-char rows. The leading character of every row is the leftmost
// wall edge of the block; the trailing character is the rightmost.
//
// '#' = wall, '.' = open road, plus the decoration characters used by
// parseMap.
type panel [panelHeight]string

var (
	// pSolid is an entirely walled-off block — used in the corners of
	// the layout to break sight lines.
	pSolid = panel{"#####", "#####", "#####", "#####"}

	// pV has a vertical corridor cut through the middle so cars can
	// drive top-to-bottom through this panel.
	pV = panel{"##.##", "##.##", "##.##", "##.##"}

	// pH has a horizontal corridor through the middle of the block;
	// cars can drive left-to-right but not top-to-bottom.
	pH = panel{"#####", ".....", ".....", "#####"}

	// pCross combines vertical + horizontal corridors — every approach
	// direction is open.
	pCross = panel{"##.##", ".....", "##.##", "##.##"}

	// pOpen is a chamber: the top and bottom edges have small notches
	// connecting to the corridor rows, and the middle two rows are
	// fully clear so it reads as a "courtyard" you can manoeuvre in.
	pOpen = panel{"##.##", ".....", ".....", "##.##"}

	// pFlagT is a dead-end pocket containing a flag, reachable only
	// from the top corridor.
	pFlagT = panel{"##.##", "##F##", "#####", "#####"}

	// pFlagB is the same dead-end, mirrored so it opens to the bottom.
	pFlagB = panel{"#####", "#####", "##F##", "##.##"}

	// pVRock is a vertical corridor with a rock parked at row 1 — the
	// player must brake or take another route to avoid wreckage.
	pVRock = panel{"##.##", "##R##", "##.##", "##.##"}

	// pSpecial is a flag-style dead-end containing the special flag.
	pSpecial = panel{"##.##", "##S##", "#####", "#####"}
)

// cellDeco overrides a single character in a corridor row — used to
// place flags, the player spawn, and enemy spawns into the otherwise
// uniformly-empty corridor between panels.
type cellDeco struct {
	col  int
	char rune
}

// stageMap is one named layout. The play scene picks one per stage
// so successive stages feel like fresh maps rather than re-runs.
type stageMap struct {
	name    string
	panels  [panelRows][panelCols]panel
	decos   map[int][]cellDeco
}

// stageMaps lists every distinct layout in the cabinet. The play
// scene cycles through them: stage n uses stageMaps[(n-1) % len].
// The total normal-flag count per map MUST be exactly 10 — panel
// flags (pFlagT/pFlagB) plus corridor 'F' decorations.
var stageMaps = []stageMap{
	{
		// Stage 1: the original "vertical maze" layout — five vertical
		// corridors with cross-junctions, panel flag-pockets at the
		// north and south edges, and two rock-blocked V corridors in
		// the upper-middle band.
		//
		// Flag accounting:
		//   Row 1: 3 flags                                  (corridor)
		//   Row 6: 1 flag                                   (corridor)
		//   Row 11: 1 flag + 2 enemy spawns                 (corridor)
		//   Row 16: 1 flag                                  (corridor)
		//   Row 21: 1 flag + player spawn + lucky flag      (corridor)
		//   pFlagT + 2× pFlagB                              (panels)
		//   Total: 7 corridor + 3 panel = 10 normal flags.
		name: "Vertical Maze",
		panels: [panelRows][panelCols]panel{
			{pV, pSolid, pFlagT, pSolid, pV},
			{pCross, pVRock, pH, pVRock, pCross},
			{pV, pOpen, pSpecial, pOpen, pV},
			{pCross, pFlagB, pV, pFlagB, pCross},
		},
		decos: map[int][]cellDeco{
			1:  {{4, 'F'}, {16, 'F'}, {26, 'F'}},
			6:  {{15, 'F'}},
			11: {{3, 'E'}, {16, 'F'}, {28, 'E'}},
			16: {{14, 'F'}},
			21: {{4, 'P'}, {13, 'F'}, {25, 'L'}},
			22: {},
		},
	},
	{
		// Stage 2: "Horizontal Speedway" — wide horizontal lanes (pH)
		// dominate the layout instead of vertical pillars, so the
		// chase tilts toward long left-right sprints. Flags sit
		// inside open chambers in the bottom band and in bottom-
		// opening pockets in the upper-middle band. The player
		// spawns at the top-centre and the enemies come up from the
		// bottom corners.
		//
		// Flag accounting:
		//   Row 1:  2 flags + player spawn                  (corridor)
		//   Row 6:  2 flags                                 (corridor)
		//   Row 11: 2 flags + lucky                         (corridor)
		//   Row 16: 2 flags                                 (corridor)
		//   Row 22: 2 enemy spawns                          (corridor)
		//   2× pFlagB                                       (panels)
		//   Total: 8 corridor + 2 panel = 10 normal flags.
		name: "Horizontal Speedway",
		panels: [panelRows][panelCols]panel{
			{pH, pV, pH, pV, pH},
			{pCross, pFlagB, pSpecial, pFlagB, pCross},
			{pH, pVRock, pH, pVRock, pH},
			{pCross, pOpen, pCross, pOpen, pCross},
		},
		decos: map[int][]cellDeco{
			1:  {{4, 'F'}, {16, 'P'}, {26, 'F'}},
			6:  {{6, 'F'}, {24, 'F'}},
			11: {{4, 'F'}, {13, 'L'}, {28, 'F'}},
			16: {{6, 'F'}, {24, 'F'}},
			21: {},
			22: {{4, 'E'}, {28, 'E'}},
		},
	},
}

// mapForStage picks the layout for the given 1-indexed stage,
// cycling through stageMaps when the stage count exceeds the number
// of distinct maps.
func mapForStage(stage int) *stageMap {
	if stage < 1 {
		stage = 1
	}
	return &stageMaps[(stage-1)%len(stageMaps)]
}

// buildMaze composes a stageMap into the final 24-string layout.
// Doing the composition in code instead of hand-counted strings
// means the row widths cannot drift — every row is guaranteed
// mazeCols runes long by construction.
func buildMaze(sm *stageMap) [mazeRows]string {
	var out [mazeRows]string
	wall := strings.Repeat("#", mazeCols)

	// Top and bottom borders.
	out[0] = wall
	out[mazeRows-1] = wall

	// Horizontal corridor rows. The default for each corridor row is
	// "#" + 30 dots + "#"; specific cells get overwritten with the
	// decorations from corridorDecos.
	corridorRowSet := map[int]bool{1: true, 6: true, 11: true, 16: true, 21: true, 22: true}
	defaultCorridor := "#" + strings.Repeat(".", mazeCols-2) + "#"
	for r := range corridorRowSet {
		row := []byte(defaultCorridor)
		for _, d := range sm.decos[r] {
			if d.col >= 0 && d.col < mazeCols {
				row[d.col] = byte(d.char)
			}
		}
		out[r] = string(row)
	}

	// Panel rows. The vertical layout is:
	//   row 0:       wall
	//   row 1:       corridor
	//   rows 2..5:   panel-row 0
	//   row 6:       corridor
	//   rows 7..10:  panel-row 1
	//   row 11:      corridor
	//   rows 12..15: panel-row 2
	//   row 16:      corridor
	//   rows 17..20: panel-row 3
	//   row 21:      corridor
	//   row 22:      corridor (2-wide bottom highway)
	//   row 23:      wall
	panelStartRows := [panelRows]int{2, 7, 12, 17}
	for pr := 0; pr < panelRows; pr++ {
		startRow := panelStartRows[pr]
		for innerR := 0; innerR < panelHeight; innerR++ {
			var sb strings.Builder
			sb.Grow(mazeCols)
			sb.WriteByte('#')
			for pc := 0; pc < panelCols; pc++ {
				sb.WriteByte('.')
				sb.WriteString(sm.panels[pr][pc][innerR])
			}
			sb.WriteByte('#')
			out[startRow+innerR] = sb.String()
		}
	}
	return out
}

// itemKind is the lookup key for non-wall, non-road game objects
// parsed out of the map layout on init.
type itemKind uint8

const (
	itemNone itemKind = iota
	itemFlag
	itemSpecialFlag
	itemLuckyFlag
)

// pickup is one collectable item on the map.
type pickup struct {
	col, row int
	kind     itemKind
	taken    bool
}

// maze is the runtime grid plus parsed metadata.
type maze struct {
	tiles       [mazeRows][mazeCols]tile
	flags       []pickup
	specialFlag *pickup // nil if none on this map
	luckyFlag   *pickup // nil if not assigned for this round
	playerSpawn [2]float64
	enemySpawns [][2]float64
	stageName   string
}

// newMaze parses the layout for the given stage into a fresh maze
// instance. The stage is 1-indexed; layouts cycle when stage exceeds
// the count in stageMaps.
func newMaze(stage int) *maze {
	sm := mapForStage(stage)
	rawMaze := buildMaze(sm)
	m := &maze{stageName: sm.name}
	for r, row := range rawMaze {
		if len(row) != mazeCols {
			panic(fmt.Sprintf("rallyx: maze row %d is %d runes wide, expected %d (line: %q)",
				r, len(row), mazeCols, row))
		}
		for c, ch := range row {
			switch ch {
			case '#':
				m.tiles[r][c] = tileWall
			case '.', ' ':
				m.tiles[r][c] = tileRoad
			case 'R':
				m.tiles[r][c] = tileRock
			case 'F':
				m.tiles[r][c] = tileRoad
				m.flags = append(m.flags, pickup{col: c, row: r, kind: itemFlag})
			case 'S':
				m.tiles[r][c] = tileRoad
				m.specialFlag = &pickup{col: c, row: r, kind: itemSpecialFlag}
			case 'L':
				m.tiles[r][c] = tileRoad
				m.luckyFlag = &pickup{col: c, row: r, kind: itemLuckyFlag}
			case 'P':
				m.tiles[r][c] = tileRoad
				m.playerSpawn = [2]float64{float64(c) + 0.5, float64(r) + 0.5}
			case 'E':
				m.tiles[r][c] = tileRoad
				m.enemySpawns = append(m.enemySpawns,
					[2]float64{float64(c) + 0.5, float64(r) + 0.5})
			default:
				panic(fmt.Sprintf("rallyx: unknown maze rune %q at row %d col %d", ch, r, c))
			}
		}
	}
	if m.playerSpawn[0] == 0 {
		// No 'P' in the map — drop the player on the first road tile we
		// find so the game is still playable while we're iterating.
		for r := 0; r < mazeRows; r++ {
			for c := 0; c < mazeCols; c++ {
				if m.tiles[r][c] == tileRoad {
					m.playerSpawn = [2]float64{float64(c) + 0.5, float64(r) + 0.5}
					return m
				}
			}
		}
	}
	return m
}

// at returns the tile at (col, row), or tileWall for out-of-range
// coordinates so the world edge reads as a solid border.
func (m *maze) at(col, row int) tile {
	if col < 0 || col >= mazeCols || row < 0 || row >= mazeRows {
		return tileWall
	}
	return m.tiles[row][col]
}

// drivable reports whether (col, row) is a tile a car may occupy.
// Walls are forbidden; rocks are drivable in the sense that motion
// can enter the tile (where the car then crashes against the rock).
func (m *maze) drivable(col, row int) bool {
	return m.at(col, row) != tileWall
}

// passable reports whether (col, row) is a tile the AI is willing to
// plan a path through. Walls and rocks both count as impassable for
// AI — the AI never deliberately drives into a rock.
func (m *maze) passable(col, row int) bool {
	return m.at(col, row) == tileRoad
}

// removeRock clears the rock at (col, row) if there is one, used when
// a car (player or enemy) hits a rock — the rock and the car both
// disappear in a wreck.
func (m *maze) removeRock(col, row int) {
	if m.at(col, row) == tileRock {
		m.tiles[row][col] = tileRoad
	}
}

// pickupAt returns a pointer to the first un-taken pickup whose tile
// matches (col, row), or nil. Used in the per-frame "did the player
// just drive over an item?" check.
func (m *maze) pickupAt(col, row int) *pickup {
	for i := range m.flags {
		p := &m.flags[i]
		if !p.taken && p.col == col && p.row == row {
			return p
		}
	}
	if m.specialFlag != nil && !m.specialFlag.taken &&
		m.specialFlag.col == col && m.specialFlag.row == row {
		return m.specialFlag
	}
	if m.luckyFlag != nil && !m.luckyFlag.taken &&
		m.luckyFlag.col == col && m.luckyFlag.row == row {
		return m.luckyFlag
	}
	return nil
}

// remainingFlags counts the un-taken normal flags. The round clears
// when this hits zero — the special and lucky flags are bonuses and
// don't gate level completion.
func (m *maze) remainingFlags() int {
	n := 0
	for _, p := range m.flags {
		if !p.taken {
			n++
		}
	}
	return n
}
