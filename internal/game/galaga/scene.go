package galaga

import (
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level state — title vs gameplay. playScene owns
// its own gameplay sub-state (stage intro, playing, hit, captured, game
// over) so ESC can unwind cleanly from gameplay back to the title.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene. It owns the title screen and the
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

// drawTitle paints the title — game name, the enemy score guide, the
// controls, and a flashing start prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 0, G: 0, B: 12, A: 255})
	w := c.Width()

	// Subtle starfield drifting behind everything.
	s.drawTitleStars(c)

	// Big title.
	title := "GALAGA"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := 3
	titleCol := engine.Color{R: 255, G: 230, B: 90, A: 255}
	c.DrawText(tx, ty, title, titleCol)

	// Pulsing underline.
	pulse := 100 + int(120*pulse01(s.titleT, 1.6))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse / 2), B: 40, A: 255})

	// Enemy score guide. Two animation frames cycled by titleT.
	scoreboardY := ty + engine.FontHeight + 5
	frame := int(s.titleT*2) % 2
	rows := []struct {
		sprA, sprB sprite
		col        engine.Color
		label      string
	}{
		{bossA, bossB, enemyBoss.color(), "= 150 PTS"},
		{butterflyA, butterflyB, enemyButterfly.color(), "= 80 PTS"},
		{beeA, beeB, enemyBee.color(), "= 50 PTS"},
	}
	iconW := bossA.width()
	maxLabel := 0
	for _, r := range rows {
		if len(r.label) > maxLabel {
			maxLabel = len(r.label)
		}
	}
	baseX := (w - (iconW + 2 + maxLabel*1)) / 2
	if baseX < 1 {
		baseX = 1
	}
	rowGap := 6
	for i, r := range rows {
		spr := r.sprA
		if frame == 1 {
			spr = r.sprB
		}
		y := scoreboardY + i*rowGap
		drawSprite(c, baseX, y, spr, r.col)
		c.Print(baseX+iconW+2, y/2, r.label, engine.White)
	}

	// Controls.
	controlsRow := (scoreboardY + len(rows)*rowGap + 2) / 2
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
// It uses a deterministic-ish pattern keyed off titleT so we don't have
// to allocate a slice for the menu.
func (s *scene) drawTitleStars(c *engine.Canvas) {
	w := c.Width()
	h := c.Height()
	// 40 stars on a tiny pseudo-random grid.
	for i := 0; i < 40; i++ {
		x := (i*37 + 17) % w
		yBase := (i*23 + 9) % h
		yOff := int(s.titleT*4+float64(i)) % h
		y := (yBase + yOff) % h
		col := starPalette[i%len(starPalette)]
		// Twinkle on a slow phase per star.
		if int(s.titleT*3)+i&1 == 0 {
			col = engine.Color{R: col.R / 3, G: col.G / 3, B: col.B / 3, A: 255}
		}
		c.Set(x, y, col)
	}
}

// pulse01 returns a value in [0,1] that oscillates with `period` seconds.
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
