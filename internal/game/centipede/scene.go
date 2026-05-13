package centipede

import (
	"fmt"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level state. The play scene owns its own
// sub-state (intro / playing / dying / wave-cleared / game-over) so that
// ESC can unwind cleanly from gameplay back to the title.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene — owns title + active playScene.
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

// drawTitle paints the title with the scoring guide and controls.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(colorBackground)
	w := c.Width()
	h := c.Height()

	// A scattering of static "mushrooms" decorating the title.
	s.drawTitleMushrooms(c)

	// Big title.
	title := "CENTIPEDE"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := 3
	pulse := pulse01(s.titleT, 1.5)
	titleCol := engine.Color{
		R: 120 + uint8(120*pulse),
		G: 230,
		B: 120,
		A: 255,
	}
	c.DrawText(tx, ty, title, titleCol)
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: 80, G: 200, B: 80, A: 255})

	// Score guide — one row per enemy type using actual sprites.
	guideY := ty + engine.FontHeight + 5
	rows := []struct {
		spr   sprite
		col   engine.Color
		label string
	}{
		{centipedeHeadA, colorCentipedeHead, "= 100 PTS"},
		{centipedeBodyA, colorCentipedeBody, "= 10 PTS"},
		{mushroomFull, colorMushroom, "= 1 PT"},
		{spiderA, colorSpider, "= 300-900 PTS"},
		{fleaA, colorFlea, "= 200 PTS"},
		{scorpionA, colorScorpion, "= 1000 PTS"},
	}
	maxIcon := 0
	maxLabel := 0
	for _, r := range rows {
		if r.spr.width() > maxIcon {
			maxIcon = r.spr.width()
		}
		if len(r.label) > maxLabel {
			maxLabel = len(r.label)
		}
	}
	baseX := (w - (maxIcon + 2 + maxLabel)) / 2
	if baseX < 1 {
		baseX = 1
	}
	rowGap := 5
	for i, r := range rows {
		y := guideY + i*rowGap
		drawSprite(c, baseX, y, r.spr, r.col)
		c.Print(baseX+maxIcon+2, y/2, r.label, engine.White)
	}

	// Controls.
	controlsRow := (guideY + len(rows)*rowGap + 3) / 2
	lines := []string{
		"ARROWS  MOVE",
		"SPACE   FIRE",
		"ESC     BACK",
	}
	for i, ln := range lines {
		row := controlsRow + i
		if row >= c.Rows()-2 {
			break
		}
		c.Print((w-len(ln))/2, row, ln, engine.Gray)
	}

	// Flashing start prompt.
	prompt := "PRESS ENTER TO START"
	col := engine.White
	if int(s.titleT*2)%2 == 0 {
		col = engine.Yellow
	}
	c.Print((c.Cols()-len(prompt))/2, c.Rows()-2, prompt, col)
	if s.hiScore > 0 {
		hi := fmt.Sprintf("HI-SCORE %06d", s.hiScore)
		c.Print((c.Cols()-len(hi))/2, c.Rows()-1, hi, engine.Yellow)
	}
	_ = h
}

// drawTitleMushrooms scatters a static decorative mushroom field below
// the title text. Positions are seeded deterministically so the field
// doesn't shimmer between frames.
func (s *scene) drawTitleMushrooms(c *engine.Canvas) {
	w := c.Width()
	h := c.Height()
	for i := 0; i < 22; i++ {
		// LCG-ish placement so things spread out without clustering.
		x := (i*53 + 11) % (w - cellW)
		y := h - 6 - ((i*37+7)%(h/3))
		col := colorMushroom
		if i%5 == 0 {
			col = colorMushroomPoisoned
		}
		drawSprite(c, x, y, mushroomFull, col)
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
