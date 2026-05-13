package lunarlander

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level scene's two-mode toggle: title menu vs.
// active mission. The playScene owns its own internal state machine
// (flying / landed / crashed / game-over) — keeping the title
// distinction up here lets ESC out of a mission drop back to the menu
// instead of exiting the engine.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene implementation for Lunar Lander.
// Title-screen state lives directly on the scene; gameplay state is
// owned by an embedded *playScene that's created on demand each time
// the player starts a mission and discarded on return-to-menu.
type scene struct {
	e        *engine.Engine
	state    sceneState
	play     *playScene
	titleT   float64
	hiScore  int
	rng      *rand.Rand
	titleBG  *titleBackdrop
	starsSet bool
}

// titleBackdrop captures a frozen miniature landscape used purely for
// menu flavour: a star field and a terrain silhouette drawn at the
// foot of the title screen. Holding these as state means the menu's
// landscape doesn't re-shuffle every frame.
type titleBackdrop struct {
	stars   []star
	terrain *terrain
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
	if s.titleBG == nil {
		s.buildBackdrop()
	}
	for {
		k, ok := s.e.PollKey()
		if !ok {
			break
		}
		switch k.Code {
		case engine.KeyEnter:
			s.startMission()
			return nil
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyChar:
			switch k.Rune {
			case 'q', 'Q':
				return engine.ErrQuit
			case ' ', '\r', '\n':
				s.startMission()
				return nil
			}
		}
	}
	return nil
}

// buildBackdrop generates the static decoration shown behind the title.
// Stars are sized to the full canvas; the terrain silhouette is the
// same generator used in-game so the menu previews what gameplay looks
// like.
func (s *scene) buildBackdrop() {
	c := s.e.Canvas()
	s.titleBG = &titleBackdrop{
		stars:   generateStars(c.Width(), c.Height(), s.rng),
		terrain: generateTerrain(c.Width(), c.Height(), s.rng),
	}
}

func (s *scene) startMission() {
	s.play = newPlayScene(s.e)
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

// drawTitle paints the full title screen — backdrop, wordmark, an
// orbiting decorative lander, controls panel, and (once any mission
// has been played) a hi-score chip.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 14, A: 255})

	if s.titleBG != nil {
		for _, st := range s.titleBG.stars {
			if st.y >= s.titleBG.terrain.heightAt(st.x) {
				continue
			}
			c.Set(st.x, st.y, st.col)
		}
		// Terrain silhouette near the bottom — softer fill than in-game
		// so the menu reads as decoration rather than a playable scene.
		groundCol := engine.Color{R: 60, G: 60, B: 90, A: 255}
		rimCol := engine.Color{R: 110, G: 110, B: 150, A: 255}
		t := s.titleBG.terrain
		for x := 0; x < t.w; x++ {
			y := t.heightAt(x)
			c.FillRect(x, y, 1, t.h-y, groundCol)
			c.Set(x, y, rimCol)
		}
		for _, pad := range t.pads {
			c.FillRect(pad.xStart, pad.y, pad.width(), 1, padColor(pad.mult))
		}
	}

	w := c.Width()
	title := "LUNAR LANDER"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := c.Height()/5 - engine.FontHeight/2
	if ty < 2 {
		ty = 2
	}
	c.DrawText(tx, ty, title, engine.White)

	// Pulsing underline so the title feels alive on the menu.
	pulse := 80 + int(120*pulse01(s.titleT, 1.5))
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: uint8(pulse), G: uint8(pulse), B: 255, A: 255})

	subtitle := "ATARI 1979 TRIBUTE"
	c.Print((c.Cols()-len(subtitle))/2, (ty+engine.FontHeight+4)/2, subtitle, engine.Gray)

	// Decorative lander hovering above the menu, slowly rocking, with
	// a flickering thrust plume so it doesn't read as static art.
	s.drawDecoLander(c)

	// Controls panel.
	lines := []string{
		"LEFT / RIGHT  ROTATE",
		"UP / SPACE    THRUST",
		"ENTER         START MISSION",
		"ESC           QUIT",
	}
	startRow := c.Rows() - len(lines) - 3
	maxLen := 0
	for _, ln := range lines {
		if len(ln) > maxLen {
			maxLen = len(ln)
		}
	}
	col := (c.Cols() - maxLen) / 2
	for i, ln := range lines {
		c.Print(col, startRow+i, ln, engine.Gray)
	}

	if s.hiScore > 0 {
		hi := fmt.Sprintf("HI SCORE  %05d", s.hiScore)
		c.Print((c.Cols()-len(hi))/2, startRow-2, hi, engine.Yellow)
	}

	prompt := "PRESS ENTER TO LAUNCH"
	pulseHL := pulse01(s.titleT, 0.9)
	r := uint8(120 + 130*pulseHL)
	g := uint8(120 + 130*pulseHL)
	bb := uint8(80 + 40*pulseHL)
	c.Print((c.Cols()-len(prompt))/2, c.Rows()-2, prompt, engine.Color{R: r, G: g, B: bb, A: 255})
}

// drawDecoLander paints a single hovering lander centred above the
// title-screen terrain. It rocks gently on a sine and runs its thrust
// plume on a slower duty cycle so the menu has steady motion without
// the lander wandering offscreen.
func (s *scene) drawDecoLander(c *engine.Canvas) {
	local := standardLander()
	cx := float64(c.Width()) / 2
	cy := float64(c.Height())/3 + 4*math.Sin(s.titleT*0.9)
	angle := 0.18 * math.Sin(s.titleT*0.7)
	deco := &lander{
		pos:       vec2{x: cx, y: cy},
		angle:     angle,
		thrusting: math.Mod(s.titleT, 1.6) < 1.2,
		flameT:    s.titleT,
	}
	drawLander(c, deco, local)
}

// pulse01 returns a triangle wave in [0,1] with the given period.
// Borrowed pattern from the other titles in this repo: cheap visual
// motion that doesn't need a real animation system.
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
