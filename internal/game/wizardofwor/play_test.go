package wizardofwor

import (
	"bytes"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// TestPlaySceneSmoke runs a few seconds of simulation against a fresh
// scene to make sure no Update or Draw call panics or deadlocks. The
// engine is configured headless — Output is a bytes.Buffer so no real
// terminal is touched.
func TestPlaySceneSmoke(t *testing.T) {
	e, err := engine.New(engine.Options{
		Width:  120,
		Height: 80,
		Output: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	s := newScene(e)

	c := engine.NewCanvas(120, 80)
	dt := time.Second / 60

	// 10 seconds at 60 FPS — well past the READY hold, the first
	// cage emerges, and likely a Worluk phase if the player cleans
	// out the spawns.
	for i := 0; i < 10*60; i++ {
		if err := s.Update(dt); err != nil && err != engine.ErrQuit {
			t.Fatalf("scene.Update at frame %d: %v", i, err)
		}
		s.Draw(c)
	}
}

// TestAllMazesFullyConnected: every non-cage cell must be reachable
// from the player spawn (with the side tunnels wrapping). If a layout
// has an isolated section, the player simply can't get to monsters
// stranded there and the round is unwinnable.
func TestAllMazesFullyConnected(t *testing.T) {
	for li := range layouts {
		m := newMaze(li + 1)
		visited := map[[2]int]bool{}
		start := [2]int{playerSpawnCol, playerSpawnRow}
		visited[start] = true
		queue := [][2]int{start}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, d := range allMoves {
				if !m.canMove(cur[0], cur[1], d) {
					continue
				}
				next := [2]int{cur[0] + d.dx(), cur[1] + d.dy()}
				// Tunnel wrap: map off-playfield horizontal cells back
				// to the opposite side of the same row.
				if next[1] == tunnelRow {
					if next[0] < 0 {
						next[0] += mazeCols
					} else if next[0] >= mazeCols {
						next[0] -= mazeCols
					}
				}
				if next[0] < 0 || next[0] >= mazeCols ||
					next[1] < 0 || next[1] >= mazeRows {
					continue
				}
				if visited[next] {
					continue
				}
				visited[next] = true
				queue = append(queue, next)
			}
		}
		// The cage itself is monster-only territory; everything else
		// must be reachable from the spawn.
		for r := 0; r < mazeRows; r++ {
			for c := 0; c < mazeCols; c++ {
				if c == cageCol && r == cageRow {
					continue
				}
				if !visited[[2]int{c, r}] {
					t.Errorf("dungeon %d: cell (%d,%d) unreachable from spawn (%d,%d)",
						li+1, c, r, playerSpawnCol, playerSpawnRow)
				}
			}
		}
	}
}

// TestMazeParseAllLayouts verifies every canonical layout parses
// without panic and constructs the cage geometry correctly.
func TestMazeParseAllLayouts(t *testing.T) {
	for i := range layouts {
		m := newMaze(i + 1)
		// Cage walls must be there regardless of source layout.
		if !m.vwalls[cageRow][cageCol] || !m.vwalls[cageRow][cageCol+1] {
			t.Errorf("layout %d: cage side walls missing", i+1)
		}
		if !m.hwalls[cageRow+1][cageCol] {
			t.Errorf("layout %d: cage floor missing", i+1)
		}
		if m.hwalls[cageRow][cageCol] {
			t.Errorf("layout %d: cage door is closed", i+1)
		}
		// Side warps must be present on the tunnel row.
		if m.vwalls[tunnelRow][0] {
			t.Errorf("layout %d: left tunnel mouth is sealed", i+1)
		}
		if m.vwalls[tunnelRow][mazeCols] {
			t.Errorf("layout %d: right tunnel mouth is sealed", i+1)
		}
	}
}

// TestEntityWallStop confirms an entity walking into a wall halts at
// some cell centre instead of phasing through. The exact stopping
// row depends on the layout (interior corridors may be open or
// walled); the invariant is "dir becomes dirNone and y lands on a
// cell centre".
func TestEntityWallStop(t *testing.T) {
	m := newMaze(1)
	e := entity{x: 1.5, y: 5.5, dir: dirDown, desired: dirDown, speed: 4.0}
	canPass := func(c, r int, d direction) bool { return m.canMove(c, r, d) }
	for i := 0; i < 60; i++ {
		e.advance(time.Second.Seconds()/30, canPass)
	}
	if e.dir != dirNone {
		t.Errorf("entity did not stop at wall; dir=%d, y=%v", e.dir, e.y)
	}
	frac := e.y - float64(int(e.y))
	if frac < 0.45 || frac > 0.55 {
		t.Errorf("entity y not aligned with cell centre: y=%v (frac=%v)", e.y, frac)
	}
}

// TestTunnelWrap walks an entity off the left tunnel mouth and
// confirms it wraps to the right side.
func TestTunnelWrap(t *testing.T) {
	m := newMaze(1)
	e := entity{
		x: 0.5, y: float64(tunnelRow) + 0.5,
		dir: dirLeft, desired: dirLeft, speed: 8.0,
	}
	canPass := func(c, r int, d direction) bool { return m.canMove(c, r, d) }
	for i := 0; i < 30; i++ {
		e.advance(time.Second.Seconds()/30, canPass)
		if e.x > float64(mazeCols)-1 {
			break
		}
	}
	if e.x <= float64(mazeCols)-1 {
		t.Errorf("entity did not wrap through left tunnel; x=%v", e.x)
	}
}

// TestPlayerFiringSpawnsBullet — pressing fire should add exactly one
// bullet, and a second press while it's alive should not.
func TestPlayerFiringSpawnsBullet(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 80, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	p.state = psPlaying
	p.stateT = 0
	p.lastPlayerDir = dirUp

	p.firePlayerBullet()
	if got := p.bulletCount(shooterPlayer); got != 1 {
		t.Fatalf("expected 1 player bullet, got %d", got)
	}
	p.firePlayerBullet()
	if got := p.bulletCount(shooterPlayer); got != 1 {
		t.Errorf("second fire should be ignored while bullet is live; got %d", got)
	}
}

// TestMonsterEmergeFromCage simulates enough time for the first
// monster to emerge and verifies the cage queue shrinks.
func TestMonsterEmergeFromCage(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 80, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	p.state = psPlaying
	p.stateT = 0
	queueLen := len(p.spawnQueue)
	if queueLen == 0 {
		t.Fatal("expected non-empty spawn queue at dungeon 1")
	}

	for i := 0; i < 180; i++ { // 3 seconds
		p.updatePlaying(time.Second.Seconds() / 60)
		if len(p.monsters) > 0 {
			return
		}
	}
	t.Errorf("no monster emerged from the cage after 3 seconds; queue still %d",
		len(p.spawnQueue))
}

// TestBulletKillsMonster places a stationary monster on the same
// open row as the player, fires, and confirms the monster dies.
// We use row 0 — its cells are guaranteed to be horizontally
// connected by the layout.
func TestBulletKillsMonster(t *testing.T) {
	e, err := engine.New(engine.Options{Width: 120, Height: 80, Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	p.state = psPlaying
	p.stateT = 0
	p.player.x = 1.5
	p.player.y = 0.5
	p.player.dir = dirRight
	p.player.desired = dirNone
	p.lastPlayerDir = dirRight
	p.playerInvulnT = 0

	m := newMonster(mkBurwor, 1.0, p.rng)
	m.state = msHunting
	m.x = 6.5
	m.y = 0.5
	m.dir = dirNone
	m.desired = dirNone
	m.speed = 0 // keep it stationary so the test is deterministic
	p.monsters = []*monster{m}
	p.spawnQueue = nil

	startScore := p.score
	p.firePlayerBullet()

	for i := 0; i < 60; i++ {
		p.updatePlaying(time.Second.Seconds() / 60)
		if m.state == msDying {
			break
		}
	}
	if m.state != msDying {
		t.Fatalf("bullet did not kill monster after 1s")
	}
	if p.score <= startScore {
		t.Errorf("score did not increase; %d <= %d", p.score, startScore)
	}
}
