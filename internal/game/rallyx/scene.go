package rallyx

import (
	"math"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level UI state — title screen vs active
// match. The playScene owns its own internal stage / death / clear
// state machine; we just route Update and Draw to whichever screen
// is current.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene. It owns the title screen, the
// active playScene (when running), and the carry-over hi-score.
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

// drawTitle paints the attract screen: a big "RALLY-X" wordmark, a
// pair of cars demoing the smoke trail, the carried hi-score, the
// controls list, and a blinking "PRESS ENTER" hint.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Black)
	w := c.Width()
	rows := c.Rows()

	title := "RALLY-X"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := c.Height()/5 - engine.FontHeight/2
	if ty < 2 {
		ty = 2
	}
	c.DrawText(tx, ty, title, engine.Color{R: 255, G: 200, B: 60, A: 255})
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: 100 + int8Pulse(s.titleT, 1.2), G: 100, B: 255, A: 255})

	subtitle := "NAMCO 1980"
	c.Print((c.Cols()-len(subtitle))/2, ty/2+engine.FontHeight+2, subtitle, engine.Gray)

	// Parade: a blue car chasing two red cars across the screen with
	// a smoke trail behind it.
	paradeY := ty + engine.FontHeight + 6
	s.drawTitleParade(c, paradeY)

	// Hi-score (only when set).
	if s.hiScore > 0 {
		hi := "HIGH SCORE  " + pad7(s.hiScore)
		c.Print((c.Cols()-len(hi))/2, rows/2+1, hi, engine.Cyan)
	}

	if int(s.titleT*2)%2 == 0 {
		hint := "PRESS ENTER TO START"
		c.Print((c.Cols()-len(hint))/2, rows/2+3, hint, engine.Yellow)
	}

	lines := []string{
		"ARROWS / WASD   DRIVE",
		"SPACE / Z       SMOKE SCREEN",
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

func (s *scene) drawTitleParade(c *engine.Canvas, y int) {
	w := c.Width()
	loopLen := w + 60
	phase := math.Mod(s.titleT*22, float64(loopLen))
	startX := int(phase) - 50

	// Smoke trail behind the player car.
	smokeCol := engine.Color{R: 200, G: 200, B: 200, A: 255}
	for i := 0; i < 6; i++ {
		sx := startX - 4 - i*3
		if sx < 0 || sx >= w {
			continue
		}
		shade := uint8(200 - i*25)
		c.FillCircle(sx, y, 1, engine.Color{R: shade, G: shade, B: shade, A: 255})
		_ = smokeCol
	}

	// Player car (blue, leading).
	if startX >= 0 && startX < w {
		drawTitleCar(c, startX, y, engine.Color{R: 50, G: 150, B: 255, A: 255}, dirRight)
	}

	// Two red enemies chasing.
	for i := 0; i < 2; i++ {
		ex := startX + 14 + i*8
		if ex < 0 || ex >= w {
			continue
		}
		drawTitleCar(c, ex, y, engine.Color{R: 230, G: 50, B: 50, A: 255}, dirRight)
	}
}

func drawTitleCar(c *engine.Canvas, x, y int, body engine.Color, _ direction) {
	// 5×3 car silhouette: rectangular body with a small windshield.
	c.FillRect(x-2, y-1, 5, 3, body)
	c.Set(x+1, y, engine.Color{R: 30, G: 30, B: 30, A: 255})
	c.Set(x-2, y-2, engine.Color{R: 30, G: 30, B: 30, A: 255})
	c.Set(x+2, y-2, engine.Color{R: 30, G: 30, B: 30, A: 255})
	c.Set(x-2, y+2, engine.Color{R: 30, G: 30, B: 30, A: 255})
	c.Set(x+2, y+2, engine.Color{R: 30, G: 30, B: 30, A: 255})
}

// pad7 left-pads n with zeros to width 7 — score formatting.
func pad7(n int) string {
	var buf [7]byte
	for i := range buf {
		buf[i] = '0'
	}
	i := len(buf) - 1
	for n > 0 && i >= 0 {
		buf[i] = byte('0' + n%10)
		n /= 10
		i--
	}
	return string(buf[:])
}

// int8Pulse returns an integer in [0,150] following a triangle wave —
// used by the title underline.
func int8Pulse(t, period float64) uint8 {
	if period <= 0 {
		return 0
	}
	x := t / period
	x -= math.Floor(x)
	if x > 0.5 {
		x = 1 - x
	}
	return uint8(x * 2 * 150)
}
