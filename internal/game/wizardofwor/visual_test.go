package wizardofwor

import (
	"strings"
	"testing"
)

// TestMazeAsciiDump renders each canonical layout back out as ASCII
// art for eyeball verification. The test always passes; failure is
// purely about whether the printed maze looks like Wizard of Wor when
// you run `go test -v -run TestMazeAsciiDump`.
func TestMazeAsciiDump(t *testing.T) {
	for li := range layouts {
		m := newMaze(li + 1)
		var sb strings.Builder
		// Top wall row.
		for c := 0; c < mazeCols; c++ {
			sb.WriteByte('+')
			if m.hwalls[0][c] {
				sb.WriteByte('-')
			} else {
				sb.WriteByte(' ')
			}
		}
		sb.WriteByte('+')
		sb.WriteByte('\n')
		for r := 0; r < mazeRows; r++ {
			// Cell row with vertical walls.
			for c := 0; c <= mazeCols; c++ {
				if m.vwalls[r][c] {
					sb.WriteByte('|')
				} else {
					sb.WriteByte(' ')
				}
				if c < mazeCols {
					if r == cageRow && c == cageCol {
						sb.WriteByte('C')
					} else {
						sb.WriteByte(' ')
					}
				}
			}
			sb.WriteByte('\n')
			// Horizontal wall row below this cell row.
			for c := 0; c < mazeCols; c++ {
				sb.WriteByte('+')
				if m.hwalls[r+1][c] {
					sb.WriteByte('-')
				} else {
					sb.WriteByte(' ')
				}
			}
			sb.WriteByte('+')
			sb.WriteByte('\n')
		}
		t.Logf("=== Dungeon %d ===\n%s", li+1, sb.String())
	}
}
