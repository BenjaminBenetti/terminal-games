package vanguard

import (
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level state — title vs gameplay. The gameplay
// scene owns its own sub-state machine inside playScene; this only
// chooses which one is currently driving Update/Draw.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the engine.Scene implementation for the Vanguard game. It
// shows a title screen with the score guide and the controls until the
// player presses Enter, then forwards Update/Draw to a fresh playScene.
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

// drawTitle paints the title — game name, the enemy score guide, the
// controls, and a flashing start prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 0, G: 0, B: 14, A: 255})
	w := c.Width()

	s.drawTitleStars(c)

	// Big title.
	title := "VANGUARD"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := 3
	titleCol := engine.Color{R: 240, G: 200, B: 90, A: 255}
	c.DrawText(tx, ty, title, titleCol)

	// Pulsing underline strip.
	pulse := 100 + int(120*pulse01(s.titleT, 1.6))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse / 2), B: 60, A: 255})

	// Score guide — three icons across a single row to keep the title
	// screen breathing on small terminals (we only have ~24 cells of
	// vertical space to play with on a default 80×24 setup).
	scoreboardY := ty + engine.FontHeight + 4
	cells := []struct {
		spr   sprite
		col   engine.Color
		label string
	}{
		{helmA, ekHelm.color(), "100"},
		{bearA, ekBear.color(), "200"},
		{floaterA, ekFloater.color(), "300"},
	}
	cellW := 18
	totalW := cellW * len(cells)
	baseX := (w - totalW) / 2
	if baseX < 1 {
		baseX = 1
	}
	for i, ent := range cells {
		x := baseX + i*cellW
		drawSprite(c, x, scoreboardY, ent.spr, ent.col)
		c.Print(x+ent.spr.width()+2, scoreboardY/2, ent.label, engine.White)
	}

	// Pod row — its own band beneath the enemy guide.
	podY := scoreboardY + 7
	podLabel := "ENERGY POD = 1000 PTS"
	podCol := engine.Color{R: 80, G: 240, B: 240, A: 255}
	podBaseX := (w - (energyPodA.width() + 2 + len(podLabel))) / 2
	if podBaseX < 1 {
		podBaseX = 1
	}
	drawSprite(c, podBaseX, podY, energyPodA, podCol)
	c.Print(podBaseX+energyPodA.width()+2, podY/2, podLabel, engine.White)

	// Controls.
	controlsRow := (podY + 7) / 2
	lines := []string{
		"ARROWS  MOVE 8-WAY",
		"W A S D  FIRE 4-WAY",
		"ESC      QUIT",
	}
	for i, ln := range lines {
		row := controlsRow + i
		if row >= c.Rows()-2 {
			break
		}
		c.Print((w-len(ln))/2, row, ln, engine.Gray)
	}

	// Footer — flashing prompt and (if set) hi-score.
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

// drawTitleStars sketches a quiet drifting starfield behind the title.
func (s *scene) drawTitleStars(c *engine.Canvas) {
	w := c.Width()
	h := c.Height()
	for i := 0; i < 40; i++ {
		x := (i*37 + 17) % w
		yBase := (i*23 + 9) % h
		yOff := int(s.titleT*4+float64(i)) % h
		y := (yBase + yOff) % h
		col := starPalette[i%len(starPalette)]
		if int(s.titleT*3)+i&1 == 0 {
			col = engine.Color{R: col.R / 3, G: col.G / 3, B: col.B / 3, A: 255}
		}
		c.Set(x, y, col)
	}
}

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
	return "HI-SCORE " + zeroPad(hi, 6)
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
