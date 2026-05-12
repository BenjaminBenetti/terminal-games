package pong

import (
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level state — title menu vs gameplay. playScene
// owns its own internal sub-states (serving, point scored, match over);
// keeping the title distinction here lets ESC unwind from a match back
// to the menu without exiting the engine.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// gameMode picks who controls the right paddle.
type gameMode int

const (
	modeVsCPU gameMode = iota
	modeTwoPlayer
)

func (m gameMode) label() string {
	switch m {
	case modeVsCPU:
		return "1P VS CPU"
	default:
		return "2 PLAYER"
	}
}

// scene is the top-level engine.Scene. It hands off to a playScene for
// gameplay and resumes title-screen control when the match ends or the
// user presses ESC.
type scene struct {
	e         *engine.Engine
	state     sceneState
	play      *playScene
	titleT    float64
	selected  int // index into menu
	menuModes []gameMode
}

func newScene(e *engine.Engine) *scene {
	return &scene{
		e:         e,
		state:     stateTitle,
		menuModes: []gameMode{modeVsCPU, modeTwoPlayer},
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
			s.moveSelection(-1)
		case engine.KeyDown:
			s.moveSelection(1)
		case engine.KeyEnter:
			s.startMatch()
			return nil
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				return engine.ErrQuit
			case 'w', 'W', 'k', 'K':
				s.moveSelection(-1)
			case 's', 'S', 'j', 'J':
				s.moveSelection(1)
			case '1':
				s.selected = 0
				s.startMatch()
				return nil
			case '2':
				s.selected = 1
				s.startMatch()
				return nil
			case ' ':
				s.startMatch()
				return nil
			}
		}
	}
	return nil
}

func (s *scene) moveSelection(delta int) {
	s.selected += delta
	if s.selected < 0 {
		s.selected = 0
	}
	if s.selected >= len(s.menuModes) {
		s.selected = len(s.menuModes) - 1
	}
}

func (s *scene) startMatch() {
	s.play = newPlayScene(s.e, s.menuModes[s.selected])
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

// drawTitle paints the menu: big "PONG" wordmark, a vertical menu of
// modes with a highlight bar on the selected row, and a controls hint
// at the bottom.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 12, A: 255})
	w := c.Width()
	rows := c.Rows()

	// Title wordmark in the chunky pixel font.
	title := "PONG"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := c.Height()/4 - engine.FontHeight/2
	if ty < 2 {
		ty = 2
	}
	c.DrawText(tx, ty, title, engine.White)

	// Pulsing underline so the title doesn't feel static.
	pulse := 80 + int(120*pulse01(s.titleT, 1.4))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse), B: 255, A: 255})

	// Menu — pad each label so the highlight bar is the same width.
	maxLabel := 0
	for _, m := range s.menuModes {
		if l := len(m.label()); l > maxLabel {
			maxLabel = l
		}
	}
	const sidePad = 3
	barWidth := maxLabel + sidePad*2
	if barWidth > c.Cols() {
		barWidth = c.Cols()
	}
	barCol := (c.Cols() - barWidth) / 2

	menuRow := rows/2 + 1
	for i, m := range s.menuModes {
		row := menuRow + i*2
		label := m.label()
		labelCol := (c.Cols() - len(label)) / 2
		colour := engine.Gray
		if i == s.selected {
			colour = engine.Black
			c.FillRect(barCol, row*2, barWidth, 2, engine.Yellow)
		}
		c.Print(labelCol, row, label, colour)
	}

	// Controls hint.
	hints := []string{
		"P1: W / S      P2: UP / DOWN",
		"ENTER START    ESC QUIT",
	}
	for i, ln := range hints {
		row := rows - len(hints) - 1 + i
		c.Print((c.Cols()-len(ln))/2, row, ln, engine.Gray)
	}
}

// pulse01 returns a triangle wave in [0,1] with the given period.
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
