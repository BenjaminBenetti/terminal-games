package asteroids

import (
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState distinguishes the title screen from the gameplay scene.
// playScene owns its own internal state machine (playing, dying, wave
// cleared, game over); keeping the title distinction here lets ESC
// unwind from a match back to the menu without exiting the engine.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene. It owns a title screen and the
// currently active playScene, forwarding Update/Draw to whichever is
// current. A persistent hi-score survives matches within a single
// session (no on-disk persistence — terminal games re-launch fresh).
type scene struct {
	e       *engine.Engine
	state   sceneState
	play    *playScene
	hiScore int
	titleT  float64
	rng     *rand.Rand
	// Decorative asteroid that drifts behind the title.
	bgAst *asteroid
}

func newScene(e *engine.Engine) *scene {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	c := e.Canvas()
	bg := newAsteroid(rng, float64(c.Width())/2, float64(c.Height())/2+8, sizeLarge)
	// Slow the background asteroid down — it's decoration, not action.
	bg.vx *= 0.4
	bg.vy *= 0.4
	bg.spin *= 0.5
	return &scene{
		e:     e,
		state: stateTitle,
		rng:   rng,
		bgAst: bg,
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
	// Advance the background asteroid so the title isn't perfectly static.
	c := s.e.Canvas()
	s.bgAst.x = wrapF(s.bgAst.x+s.bgAst.vx*dt.Seconds(), float64(c.Width()))
	s.bgAst.y = wrapF(s.bgAst.y+s.bgAst.vy*dt.Seconds(), float64(c.Height()))
	s.bgAst.angle += s.bgAst.spin * dt.Seconds()

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

// drawTitle paints the title screen — big wordmark, a single drifting
// asteroid for life, the controls, and a pulsing start prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Black)
	w := c.Width()
	rows := c.Rows()

	// Background asteroid drifts behind the title. Dim so the text reads.
	drawWrapped(c, s.bgAst.x, s.bgAst.y, asteroidRadii[s.bgAst.size]+1, func(ox, oy int) {
		drawAsteroidAt(c, s.bgAst, ox, oy, engine.Color{R: 60, G: 60, B: 90, A: 255})
	})

	// Title in the chunky pixel font.
	title := "ASTEROIDS"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := c.Height()/4 - engine.FontHeight/2
	if ty < 2 {
		ty = 2
	}
	c.DrawText(tx, ty, title, engine.White)

	// Pulsing underline.
	pulse := 80 + int(140*pulse01(s.titleT, 1.4))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse / 2), B: 255, A: 255})

	// Score table — point values for each target so the player knows what to shoot.
	lines := []string{
		"LARGE ROCK   20 PTS",
		"MEDIUM ROCK  50 PTS",
		"SMALL ROCK  100 PTS",
		"LARGE UFO   200 PTS",
		"SMALL UFO  1000 PTS",
	}
	scoreboardRow := rows/2
	maxLen := 0
	for _, ln := range lines {
		if len(ln) > maxLen {
			maxLen = len(ln)
		}
	}
	for i, ln := range lines {
		row := scoreboardRow + i
		if row >= rows-5 {
			break
		}
		c.Print((c.Cols()-maxLen)/2, row, ln, engine.Gray)
	}

	// Controls hint.
	controls := []string{
		"<- ->  ROTATE      UP  THRUST",
		"SPACE  FIRE        H   HYPERSPACE",
	}
	for i, ln := range controls {
		row := rows - len(controls) - 3 + i
		c.Print((c.Cols()-len(ln))/2, row, ln, engine.Gray)
	}

	// Footer — flashing start prompt and (if set) hi-score.
	footer := "PRESS ENTER TO START"
	col := engine.White
	if int(s.titleT*2)%2 == 0 {
		col = engine.Yellow
	}
	c.Print((c.Cols()-len(footer))/2, rows-2, footer, col)
	if s.hiScore > 0 {
		hi := "HI-SCORE " + zeroPad(s.hiScore, 5)
		c.Print((c.Cols()-len(hi))/2, rows-1, hi, engine.Yellow)
	}
}

// pulse01 returns a triangle wave in [0,1] with the given period in seconds.
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
