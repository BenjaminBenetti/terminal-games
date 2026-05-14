package defender

import (
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState distinguishes the title from the gameplay scene. The
// gameplay scene has its own internal state machine for wave / game
// over transitions; this layer just lets ESC unwind to the title
// without exiting the engine.
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
	rng     *rand.Rand
}

func newScene(e *engine.Engine) *scene {
	return &scene{
		e:     e,
		state: stateTitle,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
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
			s.startMatch()
			return nil
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				return engine.ErrQuit
			case ' ', 'p', 'P':
				s.startMatch()
				return nil
			}
		}
	}
	return nil
}

func (s *scene) startMatch() {
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

// drawTitle paints the title screen — the wordmark, a small enemy
// legend so newcomers can identify everything before getting shot at,
// the control list, and a pulsing start prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 0, G: 0, B: 10, A: 255})
	w := c.Width()
	rows := c.Rows()

	// Drifting starfield behind the title — keeps the screen from
	// feeling static while idle.
	for i := 0; i < 60; i++ {
		x := (i*53 + 13 + int(s.titleT*8)) % w
		y := (i*29 + 7) % c.Height()
		if x < 0 {
			x += w
		}
		col := colStarBri
		if (i+int(s.titleT*4))%3 == 0 {
			col = colStarDim
		}
		c.Set(x, y, col)
	}

	// Title in the chunky pixel font.
	title := "DEFENDER"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := rows*2/8
	if ty < 2 {
		ty = 2
	}
	titleCol := engine.Color{R: 240, G: 240, B: 255, A: 255}
	c.DrawText(tx, ty, title, titleCol)

	// Pulsing underline (red — the lander signature colour).
	pulse := 80 + int(140*pulse01(s.titleT, 1.4))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: 50, B: 50, A: 255})

	// Enemy legend — animate the wing-flap frame so the title isn't
	// totally static.
	legendRow := ty/2 + engine.FontHeight + 4
	frame := int(s.titleT*3) % 2
	legend := []struct {
		a, b sprite
		col  engine.Color
		text string
	}{
		{landerA, landerB, colLander, "LANDER  150"},
		{mutantA, mutantB, colMutant, "MUTANT  150"},
		{bomberA, bomberB, colBomber, "BOMBER  250"},
		{podA, podB, colPod, "POD     1000"},
		{baiterA, baiterB, colBaiter, "BAITER  200"},
	}
	maxLabel := 0
	for _, l := range legend {
		if len(l.text) > maxLabel {
			maxLabel = len(l.text)
		}
	}
	iconW := landerA.width()
	baseX := (w - (iconW + 2 + maxLabel)) / 2
	if baseX < 1 {
		baseX = 1
	}
	for i, l := range legend {
		sx := baseX
		spr := l.a
		if frame == 1 {
			spr = l.b
		}
		// Each row gets ~3 cell rows of vertical space.
		py := (legendRow + i*3) * 2
		drawSprite(c, sx, py, spr, l.col)
		c.Print(sx+iconW+2, legendRow+i*3, l.text, engine.White)
	}

	// Controls hint.
	controls := []string{
		"<- ->  THRUST/FACING",
		"UP/DN  ALTITUDE",
		"SPACE  FIRE",
		"R      REVERSE",
		"B      SMART BOMB",
		"H      HYPERSPACE",
		"ESC    BACK",
	}
	ctrlRow := legendRow + len(legend)*3 + 2
	for i, ln := range controls {
		row := ctrlRow + i
		if row >= rows-3 {
			break
		}
		c.Print((c.Cols()-len(ln))/2, row, ln, engine.Gray)
	}

	// Flashing start prompt and (if set) hi-score.
	footer := "PRESS ENTER TO START"
	col := engine.White
	if int(s.titleT*2)%2 == 0 {
		col = engine.Yellow
	}
	c.Print((c.Cols()-len(footer))/2, rows-2, footer, col)
	if s.hiScore > 0 {
		hi := "HI-SCORE " + zeroPad(s.hiScore, 6)
		c.Print((c.Cols()-len(hi))/2, rows-1, hi, engine.Yellow)
	}
}

// pulse01 returns a triangle wave in [0,1] with the given period.
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
