package scramble

import (
	"math"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState selects between the title screen and an active run.
// playScene owns its own sub-state machine (intro / playing / hit /
// game-over / victory) so ESC unwinds gameplay back to the title.
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

// drawTitle paints the title — name, scrolling demo ship and a sample
// enemy roster, controls, and a flashing start prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(colBg)
	w := c.Width()
	h := c.Height()

	// Decorative star drift.
	for i := 0; i < 40; i++ {
		x := (i*37+int(s.titleT*8)+17)%w + 0
		y := (i*23+9)%h - 0
		col := starPalette[i%len(starPalette)]
		if int(s.titleT*3)+i&1 == 0 {
			col = engine.Color{R: col.R / 3, G: col.G / 3, B: col.B / 3, A: 255}
		}
		c.Set(x, y, col)
	}

	// Title banner.
	title := "SCRAMBLE"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := 3
	titleCol := engine.Color{R: 250, G: 220, B: 100, A: 255}
	c.DrawText(tx, ty, title, titleCol)

	// Pulsing underline.
	pulse := 100 + int(120*pulse01(s.titleT, 1.4))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse / 2), B: 40, A: 255})

	// Demo ship that flies across the title behind a small fuel tank.
	demoX := int(math.Mod(s.titleT*30, float64(w+20))) - 10
	demoY := ty + engine.FontHeight + 8
	drawSprite(c, demoX, demoY, playerSprite, colPlayer)
	// A handful of dot-pixels trailing the ship like exhaust.
	for i := 1; i <= 4; i++ {
		c.Set(demoX-i, demoY+2, engine.Color{
			R: uint8(80 + 40*i), G: uint8(180 - 20*i), B: 150, A: 255,
		})
	}

	// Score guide.
	guideY := demoY + 10
	rows := []struct {
		spr   sprite
		col   engine.Color
		label string
	}{
		{rocketIdle, colRocket, "= 50 PTS"},
		{ufoA, colUFO, "= 100 PTS"},
		{fireballA, colFire, "= 50 PTS"},
		{fuelTank, colFuel, "= REFUEL"},
		{baseTower, colTower, "= 200 PTS"},
	}
	iconW := 0
	for _, r := range rows {
		if r.spr.width() > iconW {
			iconW = r.spr.width()
		}
	}
	maxLabel := 0
	for _, r := range rows {
		if len(r.label) > maxLabel {
			maxLabel = len(r.label)
		}
	}
	rowGap := 6
	baseX := (w - (iconW + 2 + maxLabel)) / 2
	if baseX < 1 {
		baseX = 1
	}
	frame := int(s.titleT*2) % 2
	for i, r := range rows {
		y := guideY + i*rowGap
		if y/2+1 >= c.Rows()-3 {
			break
		}
		spr := r.spr
		if r.spr.width() == ufoA.width() && r.spr.height() == ufoA.height() && frame == 1 {
			spr = ufoB
		}
		if r.spr.width() == fireballA.width() && r.spr.height() == fireballA.height() && frame == 1 {
			spr = fireballB
		}
		drawSprite(c, baseX+(iconW-spr.width())/2, y, spr, r.col)
		c.Print(baseX+iconW+2, y/2, r.label, engine.White)
	}

	// Controls.
	lines := []string{
		"ARROWS  FLY",
		"SPACE   LASER",
		"B / Z   BOMB",
		"ESC     BACK",
	}
	controlsRow := c.Rows() - 2 - len(lines)
	if controlsRow < (guideY+len(rows)*rowGap)/2+1 {
		controlsRow = (guideY + len(rows)*rowGap) / 2 + 1
	}
	for i, ln := range lines {
		row := controlsRow + i
		if row >= c.Rows()-1 {
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

// pulse01 returns a triangular oscillation between 0 and 1 with period
// "period" seconds. Used for the underline pulse.
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
