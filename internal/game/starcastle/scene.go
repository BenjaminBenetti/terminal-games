package starcastle

import (
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState distinguishes the title screen from the gameplay scene.
// playScene owns its own internal state machine (playing, dying, level
// cleared, game over); the outer scene only flips between title and
// play so ESC unwinds back to the menu cleanly.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene. It owns a title screen and the
// currently active playScene, forwarding Update/Draw to whichever is
// current. A persistent hi-score survives matches within a session.
type scene struct {
	e       *engine.Engine
	state   sceneState
	play    *playScene
	hiScore int
	titleT  float64
	rng     *rand.Rand

	// Decorative spinning castle behind the title.
	demoRings    [3]ring
	demoCoreA    float64
	demoShipA    float64
	demoShipOrbT float64
}

func newScene(e *engine.Engine) *scene {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	c := e.Canvas()
	geom := computeGeometry(c.Width(), c.Height())
	s := &scene{
		e:     e,
		state: stateTitle,
		rng:   rng,
	}
	for i := 0; i < 3; i++ {
		s.demoRings[i] = newRing(i, geom)
	}
	return s
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
	dts := dt.Seconds()
	s.titleT += dts
	// Animate the demo rings so the title feels alive.
	for i := range s.demoRings {
		s.demoRings[i].angle += s.demoRings[i].spinRate * dts
	}
	s.demoCoreA += 1.5 * dts
	s.demoShipOrbT += dts
	s.demoShipA += 1.2 * dts

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

// drawTitle paints the attract-mode screen: castle behind the title,
// scoring table, controls, and a pulsing start prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Black)

	geom := computeGeometry(c.Width(), c.Height())

	// Demo castle pulled to the lower portion so the title sits clean.
	cx := float64(c.Width()) / 2
	cy := float64(c.Height()) * 0.62
	g := geom
	g.cx, g.cy = cx, cy
	// Shrink slightly so it doesn't crowd the title text.
	scale := 0.85
	g.outerOuterR *= scale
	g.outerInnerR *= scale
	g.middleOuterR *= scale
	g.middleInnerR *= scale
	g.innerOuterR *= scale
	g.innerInnerR *= scale
	g.coreR *= scale
	for i := range s.demoRings {
		drawRing(c, &s.demoRings[i], g, ringDemoColor(i))
	}
	drawCore(c, g, s.demoCoreA, true,
		engine.Color{R: 255, G: 200, B: 80, A: 255})

	// Orbiting demo ship.
	orbitR := g.outerOuterR + 5
	shipX := cx + math.Cos(s.demoShipOrbT*0.6)*orbitR
	shipY := cy + math.Sin(s.demoShipOrbT*0.6)*orbitR
	// Ship points along its orbit tangent.
	ang := s.demoShipOrbT*0.6 + math.Pi/2
	drawShipBody(c, shipX, shipY, ang,
		engine.Color{R: 200, G: 240, B: 255, A: 255})

	// Title text.
	title := "STAR CASTLE"
	tw := engine.TextWidth(title)
	tx := (c.Width() - tw) / 2
	ty := c.Height() / 6
	if ty < 2 {
		ty = 2
	}
	c.DrawText(tx, ty, title, engine.White)

	// Pulsing underline.
	pulse := 80 + int(140*pulse01(s.titleT, 1.4))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse / 2), G: uint8(pulse), B: 255, A: 255})

	cols := c.Cols()
	rows := c.Rows()

	// Scoring table.
	lines := []string{
		"OUTER RING   10 PTS",
		"MIDDLE RING  20 PTS",
		"INNER RING   30 PTS",
		"FIREBALL    100 PTS",
		"CANNON     1000 PTS",
	}
	maxLen := 0
	for _, ln := range lines {
		if len(ln) > maxLen {
			maxLen = len(ln)
		}
	}
	scoreRow := rows/2 - 2
	for i, ln := range lines {
		row := scoreRow + i
		if row >= rows-5 {
			break
		}
		c.Print((cols-maxLen)/2, row, ln, engine.Gray)
	}

	// Controls.
	controls := []string{
		"<- ->  ROTATE      UP / W  THRUST",
		"SPACE  FIRE        ESC     QUIT",
	}
	for i, ln := range controls {
		row := rows - len(controls) - 3 + i
		c.Print((cols-len(ln))/2, row, ln, engine.Gray)
	}

	// Footer — blinking prompt, hi-score below.
	footer := "PRESS ENTER TO START"
	col := engine.White
	if int(s.titleT*2)%2 == 0 {
		col = engine.Yellow
	}
	c.Print((cols-len(footer))/2, rows-2, footer, col)
	if s.hiScore > 0 {
		hi := "HI-SCORE " + zeroPad(s.hiScore, 6)
		c.Print((cols-len(hi))/2, rows-1, hi, engine.Yellow)
	}
}

// ringDemoColor — slightly dimmer palette for the title-screen castle so
// the text reads clean.
func ringDemoColor(idx int) engine.Color {
	switch idx {
	case 0:
		return engine.Color{R: 80, G: 200, B: 200, A: 255}
	case 1:
		return engine.Color{R: 80, G: 180, B: 220, A: 255}
	default:
		return engine.Color{R: 100, G: 160, B: 240, A: 255}
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

// zeroPad formats n as a fixed-width decimal padded with leading zeros.
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
