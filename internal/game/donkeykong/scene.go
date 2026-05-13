package donkeykong

import (
	"fmt"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level split: title vs. active play. The play scene
// owns its own internal state machine; this one just routes input/draws
// and stays out of the way so ESC can unwind to the title cleanly.
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

// drawTitle paints the title screen — a wordmark, a tiny scene mock-up of
// DK with a stylised barrel, a controls block, and a flashing prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 6, G: 4, B: 18, A: 255})

	w := c.Width()
	title := "DONKEY KONG"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := 3
	c.DrawText(tx, ty, title, titleColor)

	// Pulsing underline.
	pulse := 80 + int(140*pulse01(s.titleT, 1.3))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse * 2 / 3), B: 30, A: 255})

	// Mini-stage: DK on a girder with Pauline beside him; one barrel beneath.
	miniY := ty + engine.FontHeight + 5
	miniGirderY := miniY + dkIdle.height() + 1
	dkX := (w - dkIdle.width() - paulineSprite.width() - 4) / 2
	pauX := dkX + dkIdle.width() + 4
	frame := dkIdle
	if int(s.titleT*1.4)%2 == 1 {
		frame = dkThrow
	}
	drawColorSprite(c, dkX, miniY, frame, false)
	drawColorSprite(c, pauX, miniY+dkIdle.height()-paulineSprite.height(),
		paulineSprite, false)
	// Girder under them.
	for x := dkX - 4; x < pauX+paulineSprite.width()+4; x++ {
		if x >= 0 && x < c.Width() {
			c.Set(x, miniGirderY, girderRed)
			c.Set(x, miniGirderY+1, girderDark)
		}
	}
	// One rolling barrel a little to the right, animated.
	bx := dkX - 8 + int(s.titleT*14)%(paulineSprite.width()+dkIdle.width()+12)
	by := miniGirderY - barrelH
	bspr := barrelA
	if int(s.titleT*6)%2 == 1 {
		bspr = barrelB
	}
	drawColorSprite(c, bx, by, bspr, false)

	// Controls.
	base := c.Rows()/2 + 1
	lines := []string{
		"<- ->     MOVE",
		"^   v     CLIMB LADDERS",
		"SPACE     JUMP",
		"ESC / Q   QUIT",
	}
	for i, ln := range lines {
		row := base + i
		if row >= c.Rows()-3 {
			break
		}
		c.Print((c.Cols()-len(ln))/2, row, ln, hintColor)
	}

	// Flashing prompt + hi-score.
	prompt := "PRESS ENTER TO START"
	pcol := engine.White
	if int(s.titleT*2)%2 == 0 {
		pcol = exclaimColor
	}
	c.Print((c.Cols()-len(prompt))/2, c.Rows()-2, prompt, pcol)
	if s.hiScore > 0 {
		hi := fmt.Sprintf("HI-SCORE %06d", s.hiScore)
		c.Print((c.Cols()-len(hi))/2, c.Rows()-1, hi, bonusColor)
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
