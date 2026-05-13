package flappybird

import (
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level state — title menu vs gameplay. playScene
// owns its own internal sub-states; this top-level distinction lets ESC
// unwind from a round back to the title without quitting the engine.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene. It owns the title screen and hands
// off to a playScene for gameplay; on quit-back-to-menu the title resumes.
// Persistent best-score lives here so it survives across rounds in a
// single session (the save file is touched only on death).
type scene struct {
	e *engine.Engine

	state sceneState
	play  *playScene

	titleT  float64
	hiScore int
	theme   theme
	variant birdVariant

	// titleBirdY is the bobbing bird's vertical pixel position; it just
	// follows a sine wave for visual life on the title screen.
	titleBirdY float64

	rng *rand.Rand
}

func newScene(e *engine.Engine) *scene {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	s := &scene{
		e:       e,
		state:   stateTitle,
		hiScore: loadHiScore(),
		rng:     rng,
	}
	s.rollCosmetics()
	return s
}

// rollCosmetics randomly picks a fresh theme + bird color, matching the
// original game's behavior of swapping these between runs so the player
// occasionally sees a different bird against a different sky.
func (s *scene) rollCosmetics() {
	s.theme = themeDay
	if s.rng.Intn(2) == 0 {
		s.theme = themeNight
	}
	s.variant = birdVariant(s.rng.Intn(3))
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
			// Persist hi-score before unwinding — playScene also writes on
			// death, but covers the case where the user ESCs out mid-round
			// after beating their best.
			if s.play.hiScore > s.hiScore {
				s.hiScore = s.play.hiScore
				saveHiScore(s.hiScore)
			}
			// Roll fresh cosmetics for the next round so day/night and bird
			// colors actually rotate instead of getting stuck.
			s.rollCosmetics()
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
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				return engine.ErrQuit
			case ' ', 'w', 'W', 'k', 'K':
				s.startRound()
				return nil
			}
		case engine.KeyEnter, engine.KeyUp:
			s.startRound()
			return nil
		}
	}
	return nil
}

func (s *scene) startRound() {
	s.play = newPlayScene(s.e, s.hiScore, s.theme, s.variant)
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

// drawTitle paints the start screen: scrolling clouds/stars background,
// the "FLAPPY BIRD" wordmark, a bobbing bird, best-score readout, and a
// flashing start hint. The visual is meant to evoke the original title
// card — chunky pixel-font wordmark with the bird floating above the
// game-start instruction.
func (s *scene) drawTitle(c *engine.Canvas) {
	w := c.Width()
	h := c.Height()
	rows := c.Rows()

	drawSkyBackground(c, s.theme, s.titleT)
	drawSkyline(c, s.theme, s.titleT)
	groundTop := h - groundHeight
	drawGround(c, w, groundTop, groundHeight, s.theme, s.titleT*pipeSpeed)

	// Title wordmark. The pixel-font is 5 wide × 7 tall per glyph, so
	// "FLAPPY BIRD" takes about 66 pixels — fits in 80-wide canvas with
	// room to breathe.
	title := "FLAPPY BIRD"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := h/4 - engine.FontHeight/2
	if ty < 2 {
		ty = 2
	}
	drawOutlinedPixelText(c, tx, ty, title,
		engine.Color{R: 255, G: 220, B: 80, A: 255},
		engine.Color{R: 40, G: 30, B: 0, A: 255})

	// Bobbing bird, centered horizontally, just below the wordmark.
	bobY := float64(ty+engine.FontHeight+8) + bobAmplitude*math.Sin(s.titleT*bobFrequency*2*math.Pi)
	s.titleBirdY = bobY
	birdX := (w - birdSpriteWidth) / 2
	tilt := 0
	if math.Sin(s.titleT*bobFrequency*2*math.Pi) < -0.5 {
		tilt = -1
	} else if math.Sin(s.titleT*bobFrequency*2*math.Pi) > 0.5 {
		tilt = 1
	}
	wing := int(s.titleT/wingPeriod) % 3
	drawBird(c, birdX, int(bobY), tilt, wing, s.variant, false)

	// Best-score plate.
	bestText := "BEST 0"
	if s.hiScore > 0 {
		bestText = "BEST " + intToStr(s.hiScore)
	}
	bestCol := (c.Cols() - len(bestText)) / 2
	bestRow := rows/2 + 2
	c.Print(bestCol, bestRow, bestText, engine.White)

	// Flashing start hint.
	if int(s.titleT*2)%2 == 0 {
		hint := "PRESS SPACE TO PLAY"
		c.Print((c.Cols()-len(hint))/2, rows-4, hint, engine.Yellow)
	}
	quit := "ESC QUIT"
	c.Print((c.Cols()-len(quit))/2, rows-2, quit, engine.Gray)
}
