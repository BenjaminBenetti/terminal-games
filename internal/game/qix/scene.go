package qix

import (
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState distinguishes the title screen from a live match. The
// play scene owns its own sub-state machine (playing / dying /
// respawning / level-cleared / game-over); keeping the title outside
// of it lets ESC bounce a finished match back to the title without
// unwinding the engine.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

type scene struct {
	e     *engine.Engine
	state sceneState
	play  *playScene

	hiScore int
	titleT  float64
	rng     *rand.Rand
}

func newScene(e *engine.Engine) *scene {
	return &scene{
		e:     e,
		state: stateTitle,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
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
			s.startMatch()
			return nil
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				return engine.ErrQuit
			case ' ', 'p', 'P':
				s.startMatch()
				return nil
			}
		}
	}
	return nil
}

func (s *scene) startMatch() {
	s.play = newPlayScene(s.e, s.hiScore, s.rng)
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

// drawTitle paints the title screen — chunky pixel-font wordmark, a
// decorative miniature Qix-style tangle behind it, the controls, and
// a pulsing start prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Black)
	w := c.Width()
	rows := c.Rows()
	cx := w / 2
	cy := c.Height() / 2

	// Decorative tangled lines behind the title — a stylised Qix.
	for i := 0; i < 14; i++ {
		t := s.titleT*1.4 + float64(i)*0.6
		r := 14.0 + 3.0*math.Sin(t*0.7)
		x0 := cx + int(r*math.Cos(t))
		y0 := cy + int(r*math.Sin(t)*0.6)
		x1 := cx + int(r*math.Cos(t+2.4))
		y1 := cy + int(r*math.Sin(t+2.4)*0.6)
		shade := uint8(60 + 70*math.Abs(math.Sin(t*0.9)))
		c.DrawLine(x0, y0, x1, y1, engine.Color{R: shade, G: shade / 3, B: shade, A: 255})
	}

	title := "QIX"
	tx := (w - engine.TextWidth(title)) / 2
	ty := c.Height()/4 - engine.FontHeight/2
	if ty < 2 {
		ty = 2
	}
	c.DrawText(tx, ty, title, engine.White)
	// Pulsing underline.
	pulse := 80 + int(140*pulse01(s.titleT, 1.6))
	c.FillRect(tx, ty+engine.FontHeight+1, engine.TextWidth(title),
		1, engine.Color{R: uint8(pulse / 2), G: uint8(pulse), B: 255, A: 255})

	// Controls list — print rows of the explainer below the title.
	lines := []string{
		"ARROWS    MOVE / DRAW DIRECTION",
		"Z OR SPC  HOLD TO DRAW FAST  (1X)",
		"X         HOLD TO DRAW SLOW  (3X)",
		"",
		"CLAIM 75% OF THE FIELD TO ADVANCE.",
		"WATCH FOR THE QIX, SPARX, AND FUSE.",
	}
	maxLen := 0
	for _, ln := range lines {
		if len(ln) > maxLen {
			maxLen = len(ln)
		}
	}
	startRow := rows/2 - 1
	for i, ln := range lines {
		row := startRow + i
		if row >= rows-3 {
			break
		}
		col := (c.Cols() - maxLen) / 2
		c.Print(col, row, ln, engine.Gray)
	}

	footer := "PRESS ENTER TO START"
	col := engine.White
	if int(s.titleT*2)%2 == 0 {
		col = engine.Yellow
	}
	c.Print((c.Cols()-len(footer))/2, rows-2, footer, col)
	if s.hiScore > 0 {
		hi := "HI-SCORE " + zeroPad(s.hiScore, 6)
		c.Print((c.Cols()-len(hi))/2, rows-1, hi, engine.Yellow)
	}
}

// pulse01 is a triangle wave in [0, 1] with period seconds.
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

// zeroPad formats n as a fixed-width decimal padded with leading zeros.
func zeroPad(n, width int) string {
	if n < 0 {
		n = -n
	}
	digits := []byte{}
	if n == 0 {
		digits = []byte{'0'}
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	for len(digits) < width {
		digits = append([]byte{'0'}, digits...)
	}
	return string(digits)
}
