package gorf

import (
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState — top-level state machine. The play scene owns its own
// inner machine (mission progression, player death, game over); this
// distinction lets the title screen and the game share an engine
// instance without leaking state between them.
type sceneState int

const (
	stateTitle sceneState = iota
	statePlay
)

// scene is the top-level engine.Scene. It paints a starry title screen
// with the GORF wordmark and a parade of mission icons, then hands off
// to playScene when the user starts a game.
type scene struct {
	e *engine.Engine

	state    sceneState
	play     *playScene
	hiScore  int
	hiCycle  int // best cycle reached, for the hi-score banner
	titleT   float64
	stars    []titleStar
	tauntIdx int
	tauntT   float64
	rng      *rand.Rand
}

// titleStar is a single twinkling point in the title-screen backdrop.
// Drifts down slowly; respawns at the top when it falls off the bottom.
type titleStar struct {
	x, y  float64
	speed float64
	phase float64
	tint  int
}

// Synthesised-speech taunts the original arcade cabinet would shout at
// the player. We cycle through these on the title screen, one at a time,
// the way the cabinet's attract mode does.
var titleTaunts = []string{
	"I AM GORF",
	"INSERT COIN",
	"GORF IS ROBOT",
	"PREPARE YOURSELF",
	"YOU WILL NOT SURVIVE",
	"HAHAHAHAHA",
}

func newScene(e *engine.Engine) *scene {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	s := &scene{
		e:     e,
		state: stateTitle,
		rng:   r,
	}
	s.spawnTitleStars()
	return s
}

func (s *scene) spawnTitleStars() {
	c := s.e.Canvas()
	w, h := c.Width(), c.Height()
	count := w / 2
	if count < 40 {
		count = 40
	}
	s.stars = make([]titleStar, count)
	for i := range s.stars {
		s.stars[i] = titleStar{
			x:     s.rng.Float64() * float64(w),
			y:     s.rng.Float64() * float64(h),
			speed: 2 + s.rng.Float64()*8,
			phase: s.rng.Float64(),
			tint:  s.rng.Intn(4),
		}
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
			if s.play.cycle > s.hiCycle {
				s.hiCycle = s.play.cycle
			}
			s.state = stateTitle
			s.play = nil
			s.titleT = 0
			s.spawnTitleStars()
		}
		return nil
	}
	return nil
}

func (s *scene) updateTitle(dt time.Duration) error {
	s.titleT += dt.Seconds()
	s.tauntT += dt.Seconds()
	if s.tauntT > 2.4 {
		s.tauntT = 0
		s.tauntIdx = (s.tauntIdx + 1) % len(titleTaunts)
	}
	c := s.e.Canvas()
	h := c.Height()
	for i := range s.stars {
		st := &s.stars[i]
		st.y += st.speed * dt.Seconds()
		st.phase += dt.Seconds() * 0.6
		if st.y >= float64(h) {
			st.y = -1
			st.x = s.rng.Float64() * float64(c.Width())
			st.speed = 2 + s.rng.Float64()*8
			st.tint = s.rng.Intn(4)
		}
	}

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
			case ' ', 'p', 'P', 'r', 'R':
				s.startPlay()
				return nil
			}
		}
	}
	return nil
}

func (s *scene) startPlay() {
	s.play = newPlayScene(s.e, s.hiScore, s.hiCycle)
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

// drawTitle composes the attract screen: starfield, GORF logo, a small
// parade of mission icons with point values, controls, and the cycling
// taunt + flashing PRESS ENTER prompt.
func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 16, A: 255})
	s.drawTitleStars(c)

	w := c.Width()

	// "GORF" wordmark in the chunky pixel font, with a flashing red glow.
	title := "GORF"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := 3
	glow := 200 + int(40*math.Sin(s.titleT*3))
	if glow > 255 {
		glow = 255
	}
	if glow < 0 {
		glow = 0
	}
	c.DrawText(tx, ty, title, engine.Color{R: uint8(glow), G: 60, B: 60, A: 255})

	// Sub-title — "THE GORFIAN EMPIRE INVADES" — small terminal-font line.
	sub := "THE GORFIAN EMPIRE INVADES"
	subRow := (ty+engine.FontHeight)/2 + 1
	c.Print((c.Cols()-len(sub))/2, subRow, sub, engine.Color{R: 200, G: 200, B: 240, A: 255})

	// Mission parade — one icon per mission with its name and points.
	parade := []struct {
		spr     sprite
		palette map[byte]engine.Color
		name    string
		pts     string
	}{
		{astroBirdA, astroBirdPalette, "ASTRO BATTLES", "= 100"},
		{laserShipA, laserShipPalette, "LASER ATTACK ", "= 200"},
		{galaxianA, galaxianPalette, "GALAXIANS    ", "= 300"},
		{warpShipMed, warpPaletteNear, "SPACE WARP   ", "= 400"},
		{flagshipShieldTile, flagshipShieldPalette, "FLAG SHIP    ", "=1000"},
	}
	frame := int(s.titleT*2) % 2
	startY := subRow*2 + 6
	rowGap := 5
	// Find the widest icon so labels line up.
	iconW := 0
	for _, p := range parade {
		if p.spr.width() > iconW {
			iconW = p.spr.width()
		}
	}
	baseX := (w - (iconW + 2 + 22)) / 2
	if baseX < 2 {
		baseX = 2
	}
	for i, p := range parade {
		spr := p.spr
		// Alternate frame for the two-frame enemies.
		if frame == 1 {
			switch p.name {
			case "ASTRO BATTLES":
				spr = astroBirdB
			case "LASER ATTACK ":
				spr = laserShipB
			case "GALAXIANS    ":
				spr = galaxianB
			}
		}
		y := startY + i*rowGap
		drawSprite(c, baseX+(iconW-spr.width())/2, y, spr, p.palette)
		c.Print(baseX+iconW+2, y/2, p.name+" "+p.pts, engine.White)
	}

	// Controls block, packed into the bottom strip.
	controls := []string{
		"ARROWS / WASD  MOVE",
		"SPACE          FIRE QUAD LASER",
		"ESC            ABANDON MISSION",
	}
	ctrlRow := c.Rows() - 5
	for i, ln := range controls {
		c.Print((c.Cols()-len(ln))/2, ctrlRow+i, ln, engine.Gray)
	}

	// Taunt — cycles, jitters slightly.
	taunt := titleTaunts[s.tauntIdx]
	tcol := engine.Color{R: 250, G: 90, B: 240, A: 255}
	// Hard-cut between taunts; fade in/out within each.
	u := s.tauntT / 2.4
	dim := 1.0
	if u < 0.15 {
		dim = u / 0.15
	} else if u > 0.85 {
		dim = (1 - u) / 0.15
	}
	if dim < 0 {
		dim = 0
	}
	tcol.R = uint8(float64(tcol.R) * dim)
	tcol.G = uint8(float64(tcol.G) * dim)
	tcol.B = uint8(float64(tcol.B) * dim)
	c.Print((c.Cols()-len(taunt))/2, c.Rows()-2, taunt, tcol)

	// Hi-score / prompt.
	if int(s.titleT*2)%2 == 0 {
		prompt := "PRESS ENTER TO BEGIN"
		c.Print((c.Cols()-len(prompt))/2, c.Rows()-1, prompt, engine.Yellow)
	}
	if s.hiScore > 0 {
		hi := "HI-SCORE " + zeroPad(s.hiScore, 6)
		c.Print(1, c.Rows()-1, hi, engine.Color{R: 250, G: 230, B: 110, A: 255})
	}
	if s.hiCycle > 0 {
		cy := "BEST CYCLE " + zeroPad(s.hiCycle, 2)
		c.Print(c.Cols()-len(cy)-1, c.Rows()-1, cy, engine.Color{R: 110, G: 240, B: 220, A: 255})
	}
}

func (s *scene) drawTitleStars(c *engine.Canvas) {
	for _, st := range s.stars {
		bri := 0.5 + 0.5*math.Sin(st.phase*2*math.Pi)
		var base engine.Color
		switch st.tint {
		case 0:
			base = engine.Color{R: 230, G: 230, B: 240, A: 255}
		case 1:
			base = engine.Color{R: 130, G: 220, B: 240, A: 255}
		case 2:
			base = engine.Color{R: 240, G: 230, B: 150, A: 255}
		default:
			base = engine.Color{R: 240, G: 180, B: 220, A: 255}
		}
		col := engine.Color{
			R: uint8(float64(base.R) * bri),
			G: uint8(float64(base.G) * bri),
			B: uint8(float64(base.B) * bri),
			A: 255,
		}
		c.Set(int(st.x), int(st.y), col)
	}
}

// zeroPad renders n as a width-character decimal with leading zeros.
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
