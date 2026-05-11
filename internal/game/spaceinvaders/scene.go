package spaceinvaders

import (
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level state — title screen versus gameplay.
// playScene owns its own internal state machine (wave clear, player
// explosion, game over); this distinction lets ESC unwind from the
// game back to the title without disturbing the engine's loop.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene. It owns a title screen and the
// active playScene, forwarding Update/Draw to whichever is current.
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
		err := s.play.Update(dt)
		if err != nil {
			return err
		}
		// If the play scene was told to quit by the user, surface it.
		if s.play.wantQuit {
			if s.play.score > s.hiScore {
				s.hiScore = s.play.score
			}
			s.state = stateTitle
			s.play = nil
			s.titleT = 0
		}
		return nil
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
			s.play = newPlayScene(s.e, s.hiScore)
			s.state = statePlay
			return nil
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				return engine.ErrQuit
			case ' ', 'p', 'P':
				s.play = newPlayScene(s.e, s.hiScore)
				s.state = statePlay
				return nil
			}
		}
	}
	return nil
}

func (s *scene) Draw(c *engine.Canvas) {
	switch s.state {
	case stateTitle:
		s.drawTitle(c)
	case statePlay:
		s.play.Draw(c)
	}
}

// drawTitle paints the title screen — name, a small alien parade with
// point values, the controls, and a flashing start prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 12, A: 255})
	w := c.Width()

	// Title text in the big pixel font.
	title := "SPACE INVADERS"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := 3
	c.DrawText(tx, ty, title, engine.Color{R: 250, G: 250, B: 250, A: 255})

	// Decorative underline that pulses.
	pulse := 100 + int(80*pulse01(s.titleT, 1.3))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse / 2), B: uint8(pulse), A: 255})

	// Alien parade with point values. Sized so it fits a 48-pixel-tall canvas.
	scoreboardY := ty + engine.FontHeight + 5
	rowGap := 6
	frame := int(s.titleT*2) % 2
	rows := []struct {
		sprA, sprB sprite
		col        engine.Color
		label      string
	}{
		{ufoSprite, ufoSprite, engine.Color{R: 240, G: 80, B: 90, A: 255}, "= MYSTERY"},
		{alienTopA, alienTopB, alienTopKind.color(), "= 30 PTS"},
		{alienMidA, alienMidB, alienMidKind.color(), "= 20 PTS"},
		{alienBotA, alienBotB, alienBotKind.color(), "= 10 PTS"},
	}
	iconW := ufoSprite.width()
	maxLabel := 0
	for _, r := range rows {
		if len(r.label) > maxLabel {
			maxLabel = len(r.label)
		}
	}
	baseX := (w - (iconW + 2 + maxLabel)) / 2
	if baseX < 1 {
		baseX = 1
	}
	for i, r := range rows {
		spr := r.sprA
		if frame == 1 {
			spr = r.sprB
		}
		y := scoreboardY + i*rowGap
		// Centre narrower alien sprites within the UFO-width slot so the
		// labels line up.
		drawSprite(c, baseX+(iconW-spr.width())/2, y, spr, r.col)
		c.Print(baseX+iconW+2, y/2, r.label, engine.White)
	}

	// Controls — sit just below the parade in three cell rows.
	controlsRow := (scoreboardY + len(rows)*rowGap + 1) / 2
	lines := []string{
		"<- ->  MOVE",
		"SPACE  FIRE",
		"ESC    BACK",
	}
	for i, ln := range lines {
		row := controlsRow + i
		if row >= c.Rows()-2 {
			break
		}
		c.Print((w-len(ln))/2, row, ln, engine.Gray)
	}

	// Footer — flashing start prompt and (if set) hi-score.
	footer := "PRESS ENTER TO START"
	col := engine.White
	if int(s.titleT*2)%2 == 0 {
		col = engine.Yellow
	}
	c.Print((c.Cols()-len(footer))/2, c.Rows()-2, footer, col)
	if s.hiScore > 0 {
		hi := formatHi(s.hiScore)
		c.Print((c.Cols()-len(hi))/2, c.Rows()-1, hi, engine.Yellow)
	}
}

// pulse01 returns a value in [0,1] that oscillates with period seconds.
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

func formatHi(hi int) string {
	return "HI-SCORE " + zeroPad(hi, 5)
}

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
