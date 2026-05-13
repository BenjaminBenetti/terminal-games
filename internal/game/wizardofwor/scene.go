package wizardofwor

import (
	"math"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level state — title vs active match.
// playScene owns its own internal state machine (ready, playing,
// dying, …); separating the title here lets ESC unwind from gameplay
// back to the title without taking down the engine loop.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene for wizardofwor.
type scene struct {
	e       *engine.Engine
	state   sceneState
	play    *playScene
	hiScore int
	titleT  float64
}

func newScene(e *engine.Engine) *scene {
	return &scene{e: e, state: stateTitle}
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
			if s.play.score > s.hiScore {
				s.hiScore = s.play.score
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
		case engine.KeyEnter:
			s.start()
			return nil
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				return engine.ErrQuit
			case ' ', 'p', 'P':
				s.start()
				return nil
			}
		}
	}
	return nil
}

func (s *scene) start() {
	s.play = newPlayScene(s.e, s.hiScore)
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

// drawTitle paints the title screen: a big "WIZARD OF WOR" wordmark,
// a slow procession of monsters across the middle, the monster score
// reference, controls, and the flashing start prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 18, A: 255})
	w := c.Width()
	rows := c.Rows()

	// Title.
	titleA := "WIZARD"
	titleB := "OF WOR"
	twA := engine.TextWidth(titleA)
	twB := engine.TextWidth(titleB)
	tx := (w - twA) / 2
	ty := 2
	c.DrawText(tx, ty, titleA, wizardRobe)
	c.DrawText((w-twB)/2, ty+engine.FontHeight+2, titleB,
		engine.Color{R: 255, G: 230, B: 60, A: 255})

	// Glowing underline that breathes.
	pulse := 80 + int(120*pulse01(s.titleT, 1.6))
	c.FillRect(tx-2, ty+2*engine.FontHeight+5, twA+4, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse / 2), B: 255, A: 255})

	// Marching parade across the middle of the screen: Worrior chasing
	// monsters left-to-right. The position cycles across the maze on a
	// slow timer for some movement on an otherwise static menu.
	paradeY := ty + 2*engine.FontHeight + 12
	s.drawParade(c, paradeY)

	// Score reference for the three standard monsters.
	rowsAt := paradeY/2 + 6
	infoLines := []struct {
		col  engine.Color
		text string
	}{
		{burworBody, "BURWOR    100 PTS"},
		{garworBody, "GARWOR    200 PTS"},
		{thorworBody, "THORWOR   500 PTS"},
		{worlukBody, "WORLUK   1000 PTS  +DOUBLE NEXT"},
		{wizardRobe, "WIZARD   2500 PTS"},
	}
	maxLen := 0
	for _, ln := range infoLines {
		if len(ln.text) > maxLen {
			maxLen = len(ln.text)
		}
	}
	startCol := (c.Cols() - maxLen) / 2
	for i, ln := range infoLines {
		r := rowsAt + i
		if r >= rows-4 {
			break
		}
		c.Print(startCol, r, ln.text, ln.col)
	}

	// Controls.
	controlsRow := rows - 5
	lines := []string{
		"ARROWS / WASD     MOVE",
		"SPACE             FIRE",
		"ESC               QUIT",
	}
	for i, ln := range lines {
		r := controlsRow + i
		if r >= rows-1 {
			break
		}
		c.Print((c.Cols()-len(ln))/2, r, ln, engine.Gray)
	}

	// Flashing start hint.
	hint := "PRESS ENTER TO START"
	hintCol := engine.White
	if int(s.titleT*2)%2 == 0 {
		hintCol = engine.Yellow
	}
	c.Print((c.Cols()-len(hint))/2, rows-2, hint, hintCol)

	// Hi-score line, if any.
	if s.hiScore > 0 {
		hi := "HIGH SCORE  " + pad6(s.hiScore)
		c.Print((c.Cols()-len(hi))/2, rows-1, hi, worriorBody)
	}
}

// drawParade renders the slow procession of monsters across the title
// screen, with a Worrior bringing up the rear and animated walk cycles.
func (s *scene) drawParade(c *engine.Canvas, y int) {
	w := c.Width()
	loopLen := w + 80
	phase := math.Mod(s.titleT*22, float64(loopLen))
	startX := int(phase) - 60

	step := int(s.titleT * 6)

	// The four chase targets: Burwor, Garwor, Thorwor, Worluk.
	figs := []struct {
		body engine.Color
		offX int
	}{
		{burworBody, 0},
		{garworBody, 12},
		{thorworBody, 24},
		{worlukBody, 36},
	}
	for _, f := range figs {
		x := startX + f.offX
		if x < -8 || x >= w {
			continue
		}
		if f.body == worlukBody {
			pal := map[byte]engine.Color{
				'W': worlukBody, 'B': worlukWing,
			}
			drawSpriteScaled(c, x, y, sprite{rows: worlukSprite.rows, palette: pal}, 1)
			continue
		}
		drawMonster(c, x, y, f.body, step, 1)
	}

	// Worrior behind them, facing right.
	wx := startX + 50
	if wx >= -6 && wx < w {
		drawSpriteScaled(c, wx, y, worriorRight, 1)
	}
}

// -- shared helpers ----------------------------------------------------

// pad6 left-pads n with zeros to width 6.
func pad6(n int) string {
	if n < 0 {
		n = 0
	}
	var buf [6]byte
	for i := range buf {
		buf[i] = '0'
	}
	i := 5
	for n > 0 && i >= 0 {
		buf[i] = byte('0' + n%10)
		n /= 10
		i--
	}
	return string(buf[:])
}

// pulse01 returns a triangle wave in [0, 1] with the given period.
func pulse01(t, period float64) float64 {
	if period <= 0 {
		return 0
	}
	x := t / period
	x -= math.Floor(x)
	if x < 0.5 {
		return x * 2
	}
	return (1 - x) * 2
}
