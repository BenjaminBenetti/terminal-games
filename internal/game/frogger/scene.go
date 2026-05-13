package frogger

import (
	"fmt"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level split: title vs. active play. The play
// scene owns its own internal state machine; this one just routes
// input/draws and stays out of the way so ESC can unwind to the title
// cleanly.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

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
			s.startPlay()
			return nil
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				return engine.ErrQuit
			case ' ', 'p', 'P':
				s.startPlay()
				return nil
			}
		}
	}
	return nil
}

func (s *scene) startPlay() {
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

// drawTitle paints the title screen — a wordmark above a stylised
// playfield mock-up with the frog hopping back and forth, the controls
// listed below, and a flashing prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 8, B: 28, A: 255})

	w := c.Width()
	h := c.Height()

	title := "FROGGER"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := 3
	c.DrawText(tx, ty, title, titleColor)

	// Subtitle.
	subtitle := "KONAMI 1981"
	c.Print((c.Cols()-len(subtitle))/2, (ty+engine.FontHeight+1)/2, subtitle, hiColor)

	// Stage mock-up: a single road lane with two cars, a river lane with
	// a log, a hedge with one home, and a hopping frog crossing.
	miniY := ty + engine.FontHeight + 6
	if miniY+24 > h {
		miniY = h - 26
	}

	// River row.
	c.FillRect(0, miniY, w, 4, riverColor)
	logX := int(s.titleT*8)%w - 18
	for dx := 0; dx < 24; dx++ {
		x := logX + dx
		if x >= 0 && x < w {
			c.Set(x, miniY+1, logLight)
			c.Set(x, miniY+2, logMid)
			c.Set(x, miniY+3, logDark)
		}
	}

	// Road row.
	c.FillRect(0, miniY+5, w, 4, roadColor)
	for x := 0; x < w; x += 6 {
		c.Set(x, miniY+5+2, roadStripe)
	}
	carX := w - (int(s.titleT*14)%(w+30)) - 6
	if carX >= 0 && carX+6 <= w {
		drawColorSprite(c, carX, miniY+6, carYellowSpr, false)
	}
	carX2 := (int(s.titleT*8)%(w+30)) - 6
	if carX2 >= 0 && carX2+6 <= w {
		drawColorSprite(c, carX2, miniY+6, carCyanSpr, true)
	}

	// Frog hopping between rows on the mockup — alternating between
	// "on log" and "on median" every second.
	hop := int(s.titleT) % 4
	frogX := w/2 - frogW/2
	switch hop {
	case 0:
		drawColorSprite(c, frogX, miniY+6, frogUpStand, false)
	case 1:
		drawColorSprite(c, frogX, miniY+3, frogUpHop, false)
	case 2:
		drawColorSprite(c, frogX, miniY+1, frogUpStand, false)
	case 3:
		drawColorSprite(c, frogX, miniY+4, frogUpHop, false)
	}

	// Controls block.
	baseRow := (miniY + 14) / 2
	lines := []string{
		"ARROWS / WASD  HOP",
		"ENTER / SPACE  START",
		"ESC / Q        QUIT",
	}
	for i, ln := range lines {
		row := baseRow + i
		if row >= c.Rows()-3 {
			break
		}
		c.Print((c.Cols()-len(ln))/2, row, ln, hintColor)
	}

	// Flashing prompt + hi-score.
	prompt := "PRESS ENTER TO START"
	pcol := engine.White
	if int(s.titleT*2)%2 == 0 {
		pcol = flashColor
	}
	c.Print((c.Cols()-len(prompt))/2, c.Rows()-2, prompt, pcol)
	if s.hiScore > 0 {
		hi := fmt.Sprintf("HI-SCORE %06d", s.hiScore)
		c.Print((c.Cols()-len(hi))/2, c.Rows()-1, hi, hiColor)
	}
}
