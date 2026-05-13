package pacman

import (
	"math"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level state — title screen vs active match.
// The playScene owns its own internal state machine (ready / playing
// / dying / level-clear / game-over); separating the title here lets
// ESC unwind from gameplay back to the title without taking down the
// engine loop.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene. It owns the title screen and
// the active playScene; play returns control here on death or quit.
type scene struct {
	e       *engine.Engine
	state   sceneState
	play    *playScene
	titleT  float64
	hiScore int
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
			case ' ':
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

// drawTitle paints the title screen: a big "PAC-MAN" wordmark, a
// pulsing underline, a small parade of ghost icons + Pac-Man, the
// carried hi-score, the controls, and a "PRESS ENTER" hint.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 0, G: 0, B: 0, A: 255})
	w := c.Width()
	rows := c.Rows()

	title := "PAC-MAN"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := c.Height()/4 - engine.FontHeight/2
	if ty < 2 {
		ty = 2
	}
	c.DrawText(tx, ty, title, engine.Color{R: 255, G: 240, B: 0, A: 255})

	pulse := 80 + int(120*pulse01(s.titleT, 1.4))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse), B: 255, A: 255})

	// Marching parade: Pac-Man chased by the four ghosts. Position
	// loops across the screen on a slow timer.
	paradeY := ty + engine.FontHeight + 6
	s.drawParade(c, paradeY)

	// Optional hi-score line.
	if s.hiScore > 0 {
		hi := "HIGH SCORE  "
		score := pad6(s.hiScore)
		text := hi + score
		c.Print((c.Cols()-len(text))/2, rows/2+1, text, engine.Cyan)
	}

	// "PRESS ENTER" blinker.
	if int(s.titleT*2)%2 == 0 {
		hint := "PRESS ENTER TO START"
		c.Print((c.Cols()-len(hint))/2, rows/2+3, hint, engine.Yellow)
	}

	// Static controls block at the bottom.
	lines := []string{
		"ARROWS / WASD   MOVE PAC-MAN",
		"ENTER           START",
		"ESC / Q         QUIT",
	}
	base := rows - len(lines) - 1
	for i, ln := range lines {
		r := base + i
		if r >= rows {
			break
		}
		c.Print((c.Cols()-len(ln))/2, r, ln, engine.Gray)
	}
}

// drawParade renders a small animation of Pac-Man being chased by the
// four ghosts across the title screen.
func (s *scene) drawParade(c *engine.Canvas, y int) {
	w := c.Width()
	// Position cycles across the canvas every 6 seconds.
	loopLen := w + 40
	phase := math.Mod(s.titleT*16, float64(loopLen))
	startX := int(phase) - 36

	// Pac-Man icon: yellow filled circle with mouth.
	pacX := startX
	if pacX >= 0 && pacX < w {
		c.FillCircle(pacX, y, 2, engine.Color{R: 255, G: 240, B: 0, A: 255})
		// Open mouth — wedge in the +X direction.
		c.Set(pacX+2, y, engine.Color{R: 0, G: 0, B: 0, A: 255})
	}

	colours := []engine.Color{
		{R: 255, G: 0, B: 0, A: 255},
		{R: 255, G: 184, B: 222, A: 255},
		{R: 0, G: 222, B: 222, A: 255},
		{R: 255, G: 184, B: 71, A: 255},
	}
	for i, col := range colours {
		gx := startX + 8 + i*6
		if gx < 0 || gx >= w {
			continue
		}
		c.FillCircle(gx, y, 2, col)
		c.FillRect(gx-2, y, 5, 3, col)
		c.Set(gx-1, y-1, engine.White)
		c.Set(gx+1, y-1, engine.White)
		c.Set(gx-1, y, engine.Color{R: 33, G: 33, B: 222, A: 255})
		c.Set(gx+1, y, engine.Color{R: 33, G: 33, B: 222, A: 255})
	}
}

// pad6 left-pads n with zeros to width 6.
func pad6(n int) string {
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

// pulse01 returns a triangle wave in [0,1] with the given period in
// seconds. Used by the title screen for breathing accents.
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
