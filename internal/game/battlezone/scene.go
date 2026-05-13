package battlezone

import (
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level toggle between the title screen and an
// active match. The playScene owns its own internal sub-state machine
// (waiting / playing / dying / game-over).
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene for Battlezone. It hosts a title
// screen with a spinning wireframe cube and a scoring table, and hands
// off to a playScene when the player starts a match. The hi-score
// survives between matches in one session.
type scene struct {
	e       *engine.Engine
	state   sceneState
	play    *playScene
	hiScore int
	titleT  float64
	rng     *rand.Rand
}

func newScene(e *engine.Engine) *scene {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &scene{
		e:     e,
		state: stateTitle,
		rng:   rng,
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
			case ' ':
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

// drawTitle paints the menu screen — a spinning wireframe tank
// silhouette, the BATTLEZONE wordmark, the scoring table, control
// hints, and a flashing start prompt. Pure green-on-black throughout.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Black)
	w := c.Width()
	rows := c.Rows()

	// Decorative spinning tank model behind the title.
	s.drawDecoTank(c)

	// Big wordmark — split into two words so it fits on narrow terms.
	title := "BATTLEZONE"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := c.Height()/6 - engine.FontHeight/2
	if ty < 2 {
		ty = 2
	}
	c.DrawText(tx, ty, title, hudBright)

	// Pulsing underline.
	pulse := 80 + int(140*pulse01(s.titleT, 1.3))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: 30, G: uint8(pulse), B: 60, A: 255})

	subtitle := "ATARI 1980 TRIBUTE"
	c.Print((c.Cols()-len(subtitle))/2, (ty+engine.FontHeight+5)/2, subtitle, hudDim)

	// Scoring table — aligns the dots so it reads as a marquee.
	lines := []string{
		"TANK . . . . . . . 1000",
		"SUPER TANK . . . . 3000",
		"MISSILE  . . . . . 2000",
		"SAUCER . . . . . . 5000",
	}
	maxLen := 0
	for _, ln := range lines {
		if len(ln) > maxLen {
			maxLen = len(ln)
		}
	}
	scoreboardRow := rows/2 + 1
	for i, ln := range lines {
		row := scoreboardRow + i
		if row >= rows-6 {
			break
		}
		c.Print((c.Cols()-maxLen)/2, row, ln, hudGreen)
	}

	// Controls hint.
	controls := []string{
		"<- ->  TURN          UP/DOWN  MOVE",
		"SPACE  FIRE          ESC      QUIT",
	}
	for i, ln := range controls {
		row := rows - len(controls) - 3 + i
		c.Print((c.Cols()-len(ln))/2, row, ln, hudDim)
	}

	prompt := "PRESS ENTER TO START"
	col := hudGreen
	if int(s.titleT*2)%2 == 0 {
		col = hudBright
	}
	c.Print((c.Cols()-len(prompt))/2, rows-2, prompt, col)
	if s.hiScore > 0 {
		hi := "HI " + zeroPad(s.hiScore, 6)
		c.Print((c.Cols()-len(hi))/2, rows-1, hi, hudGreen)
	}
}

// drawDecoTank spins a wireframe tank below the title using the same
// 3D pipeline as gameplay. The camera is fixed at a slight elevation
// looking at the origin so the tank reads as a centred trophy model.
func (s *scene) drawDecoTank(c *engine.Canvas) {
	cam := camera{
		pos:   vec3{x: 0, y: 1.5, z: -6},
		yaw:   0,
		focal: float64(c.Width()) * 0.6,
		cx:    c.Width() / 2,
		cy:    c.Height()/2 + 4,
	}
	tankYaw := s.titleT * 0.6
	if cachedTankModel == nil {
		cachedTankModel = tankEdges()
	}
	cam.drawModel(c, cachedTankModel, vec3{x: 0, y: 0, z: 0}, tankYaw, hudGreen)

	// A faint horizon under the tank for visual grounding.
	cam.drawWorldLine(c, vec3{x: -10, y: 0, z: 0}, vec3{x: 10, y: 0, z: 0}, hudDim)
}

// pulse01 returns a triangle wave in [0,1] with the given period.
// Lifted from the convention used by other games in this repo.
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
