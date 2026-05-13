package brickbreaker

import (
	"fmt"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level state — title (which doubles as level
// select) versus active play. The playScene owns its own internal
// state machine (serve / playing / ball-lost / cleared / game over);
// the split lets ESC unwind from gameplay back to the title without
// taking down the engine loop.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene. It owns the title / level-select
// screen and the active playScene, forwarding Update/Draw to whichever
// is current.
type scene struct {
	e        *engine.Engine
	state    sceneState
	play     *playScene
	titleT   float64
	selected int
	hiScores []int
}

func newScene(e *engine.Engine) *scene {
	return &scene{
		e:        e,
		state:    stateTitle,
		hiScores: make([]int, len(levels)),
	}
}

func (s *scene) Update(dt time.Duration) error {
	switch s.state {
	case stateTitle:
		return s.updateTitle(dt)
	case statePlay:
		if err := s.play.Update(dt); err != nil {
			return err
		}
		if s.play.wantQuit {
			idx := s.play.levelIndex
			if idx >= 0 && idx < len(s.hiScores) && s.play.score > s.hiScores[idx] {
				s.hiScores[idx] = s.play.score
			}
			s.state = stateTitle
			s.play = nil
			s.titleT = 0
		}
	}
	return nil
}

func (s *scene) updateTitle(dt time.Duration) error {
	s.titleT += dt.Seconds()
	for {
		k, ok := s.e.PollKey()
		if !ok {
			break
		}
		switch k.Code {
		case engine.KeyUp:
			s.selected = (s.selected + len(levels) - 1) % len(levels)
		case engine.KeyDown:
			s.selected = (s.selected + 1) % len(levels)
		case engine.KeyEnter:
			s.startSelected()
			return nil
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				return engine.ErrQuit
			case '1':
				s.selected = 0
				s.startSelected()
				return nil
			case '2':
				if len(levels) > 1 {
					s.selected = 1
					s.startSelected()
				}
				return nil
			case '3':
				if len(levels) > 2 {
					s.selected = 2
					s.startSelected()
				}
				return nil
			case ' ':
				s.startSelected()
				return nil
			}
		}
	}
	return nil
}

func (s *scene) startSelected() {
	hi := 0
	if s.selected >= 0 && s.selected < len(s.hiScores) {
		hi = s.hiScores[s.selected]
	}
	s.play = newPlayScene(s.e, s.selected, hi)
	s.state = statePlay
}

func (s *scene) Draw(c *engine.Canvas) {
	switch s.state {
	case stateTitle:
		s.drawTitle(c)
	case statePlay:
		s.play.Draw(c)
	}
}

// drawTitle paints the title screen — name, a rainbow brick parade for
// flavour, the selectable level list, and a controls block.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 8, G: 6, B: 22, A: 255})
	w := c.Width()

	title := "BRICK BREAKER"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := 3
	c.DrawText(tx, ty, title, engine.Color{R: 250, G: 220, B: 90, A: 255})

	// Pulsing underline.
	pulse := 80 + int(120*pulse01(s.titleT, 1.4))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse * 2 / 3), B: uint8(pulse / 2), A: 255})

	// Decorative rainbow row of bricks.
	bricksY := ty + engine.FontHeight + 4
	drawDecoBricks(c, bricksY, s.titleT)

	// Level list.
	listTop := c.Rows()/2 - 1
	hint := "CHOOSE A LEVEL"
	c.Print((c.Cols()-len(hint))/2, listTop-2, hint, engine.White)
	for i, lv := range levels {
		row := listTop + i*2
		label := fmt.Sprintf("%d  %-12s  %s", i+1, lv.name, lv.summary)
		col := engine.Color{R: 170, G: 170, B: 200, A: 255}
		if i == s.selected {
			c.FillRect(0, row*2, c.Cols(), 2, engine.Color{R: 60, G: 30, B: 100, A: 255})
			col = engine.Yellow
			// Pointer chevron.
			c.Print(2, row, ">", engine.Yellow)
		}
		startCol := (c.Cols() - len(label)) / 2
		if startCol < 4 {
			startCol = 4
		}
		c.Print(startCol, row, label, col)
		if i < len(s.hiScores) && s.hiScores[i] > 0 {
			hi := fmt.Sprintf("HI %05d", s.hiScores[i])
			hiCol := c.Cols() - len(hi) - 2
			if hiCol > startCol+len(label)+1 {
				c.Print(hiCol, row, hi, engine.Cyan)
			}
		}
	}

	// Controls.
	base := c.Rows() - 4
	lines := []string{
		"LEFT/RIGHT  MOVE PADDLE      SPACE  LAUNCH BALL",
		"UP/DOWN     PICK LEVEL       ENTER  START",
		"1-3         QUICK PICK       ESC    QUIT",
	}
	for i, ln := range lines {
		r := base + i
		if r >= c.Rows() {
			break
		}
		c.Print((c.Cols()-len(ln))/2, r, ln, engine.Gray)
	}
}

// drawDecoBricks paints a short rainbow row of bricks that pulses
// vertically. Pure decoration for the title screen.
func drawDecoBricks(c *engine.Canvas, y int, t float64) {
	w := c.Width()
	bw := 7
	gap := 1
	n := 8
	totalW := n*bw + (n-1)*gap
	if totalW >= w {
		bw = (w - (n-1)*gap - 4) / n
		if bw < 3 {
			bw = 3
		}
		totalW = n*bw + (n-1)*gap
	}
	x := (w - totalW) / 2
	colors := []engine.Color{
		{R: 255, G: 90, B: 90, A: 255},
		{R: 255, G: 160, B: 60, A: 255},
		{R: 255, G: 220, B: 80, A: 255},
		{R: 110, G: 220, B: 110, A: 255},
		{R: 80, G: 200, B: 255, A: 255},
		{R: 120, G: 130, B: 255, A: 255},
		{R: 200, G: 110, B: 255, A: 255},
		{R: 255, G: 120, B: 200, A: 255},
	}
	offset := int(2 * pulse01(t, 2.5))
	for i := 0; i < n; i++ {
		col := colors[i%len(colors)]
		c.FillRect(x+i*(bw+gap), y+offset, bw, 2, col)
		// Top highlight.
		hl := engine.Color{
			R: clampLighter(col.R, 50),
			G: clampLighter(col.G, 50),
			B: clampLighter(col.B, 50),
			A: 255,
		}
		c.FillRect(x+i*(bw+gap), y+offset, bw, 1, hl)
	}
}

func clampLighter(v uint8, by int) uint8 {
	n := int(v) + by
	if n > 255 {
		n = 255
	}
	return uint8(n)
}

// pulse01 returns a triangle wave in [0,1] with the given period in
// seconds. Used for blink / breathe effects on the title screen.
func pulse01(t, period float64) float64 {
	if period <= 0 {
		return 0
	}
	x := t / period
	x -= float64(int(x))
	if x < 0.5 {
		return x * 2
	}
	return (1 - x) * 2
}
