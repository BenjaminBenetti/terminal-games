package galaxian

import (
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState distinguishes title screen from active play. ESC from
// gameplay returns to title; ESC from title quits the engine.
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
	// Animated title-screen stars — separate so they don't reset when
	// returning from gameplay.
	titleStars []star
}

func newScene(e *engine.Engine) *scene {
	s := &scene{
		e:     e,
		state: stateTitle,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	s.initTitleStars()
	return s
}

func (s *scene) initTitleStars() {
	c := s.e.Canvas()
	s.titleStars = make([]star, numStars)
	for i := range s.titleStars {
		s.titleStars[i] = star{
			x:     s.rng.Float64() * float64(c.Width()),
			y:     s.rng.Float64() * float64(c.Height()),
			phase: s.rng.Float64(),
			depth: s.rng.Intn(3),
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
			s.state = stateTitle
			s.play = nil
			s.titleT = 0
		}
	}
	return nil
}

func (s *scene) updateTitle(dt time.Duration) error {
	s.titleT += dt.Seconds()
	// Scroll & twinkle title stars.
	c := s.e.Canvas()
	w := float64(c.Width())
	h := float64(c.Height())
	for i := range s.titleStars {
		st := &s.titleStars[i]
		st.y += starScrollVy * dt.Seconds()
		st.phase += dt.Seconds() * starTwinkleHz
		if st.y >= h {
			st.y = -1
			st.x = s.rng.Float64() * w
			st.depth = s.rng.Intn(3)
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

func (s *scene) drawTitle(c *engine.Canvas) {
	c.Clear(engine.Color{R: 2, G: 2, B: 10, A: 255})
	s.drawTitleStars(c)

	w := c.Width()

	// Big "GALAXIAN" title in the pixel font, with a colour cycle on
	// the underline.
	title := "GALAXIAN"
	tw := engine.TextWidth(title)
	tx := (w - tw) / 2
	ty := 4
	c.DrawText(tx, ty, title, engine.Color{R: 250, G: 240, B: 220, A: 255})

	// Animated underline scanning across the title in rainbow.
	for x := 0; x < tw; x++ {
		hue := math.Mod(float64(x)/float64(tw)+s.titleT*0.4, 1.0)
		c.Set(tx+x, ty+engine.FontHeight+1, hsv(hue, 0.7, 1.0))
	}

	// Scoreboard parade — one of each alien type with its point value.
	parade := []struct {
		kind  alienKind
		label string
	}{
		{kindFlagship, "= 60 / 800 PTS"},
		{kindBoss, "= 50 / 100 PTS"},
		{kindBee, "= 40 / 80  PTS"},
		{kindDrone, "= 30 / 60  PTS"},
	}
	rowGap := 8
	scoreboardY := ty + engine.FontHeight + 6
	// Compute layout — widest sprite + space + longest label.
	maxSpriteW := 0
	for _, p := range parade {
		if pw := p.kind.spriteWidth(); pw > maxSpriteW {
			maxSpriteW = pw
		}
	}
	maxLabel := 0
	for _, p := range parade {
		if len(p.label) > maxLabel {
			maxLabel = len(p.label)
		}
	}
	baseX := (w - (maxSpriteW + 2 + maxLabel)) / 2
	if baseX < 1 {
		baseX = 1
	}
	frame := int(s.titleT*formationAnimHz) % 2
	for i, p := range parade {
		spA, spB := p.kind.frames()
		sp := spA
		if frame == 1 {
			sp = spB
		}
		y := scoreboardY + i*rowGap
		drawColorSprite(c, baseX+(maxSpriteW-sp.width())/2, y, sp, p.kind.palette())
		c.Print(baseX+maxSpriteW+2, y/2, p.label, engine.White)
	}

	// Controls block — three lines, just below parade.
	controlsRow := (scoreboardY + len(parade)*rowGap + 1) / 2
	lines := []struct {
		txt string
		col engine.Color
	}{
		{"<- ->  MOVE",  engine.Gray},
		{"SPACE  FIRE",  engine.Gray},
		{"ESC    BACK",  engine.Gray},
	}
	for i, ln := range lines {
		row := controlsRow + i
		if row >= c.Rows()-2 {
			break
		}
		c.Print((w-len(ln.txt))/2, row, ln.txt, ln.col)
	}

	// Flashing start prompt.
	prompt := "PRESS ENTER TO START"
	col := engine.White
	if int(s.titleT*2)%2 == 0 {
		col = engine.Color{R: 250, G: 220, B: 90, A: 255}
	}
	c.Print((c.Cols()-len(prompt))/2, c.Rows()-2, prompt, col)
	if s.hiScore > 0 {
		hi := formatHi(s.hiScore)
		c.Print((c.Cols()-len(hi))/2, c.Rows()-1, hi, engine.Yellow)
	}
}

func (s *scene) drawTitleStars(c *engine.Canvas) {
	for _, st := range s.titleStars {
		brightness := 0.6 + 0.4*math.Sin(st.phase*2*math.Pi)
		switch st.depth {
		case 0:
			brightness *= 0.35
		case 1:
			brightness *= 0.7
		}
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
			R: uint8(float64(base.R) * brightness),
			G: uint8(float64(base.G) * brightness),
			B: uint8(float64(base.B) * brightness),
			A: 255,
		}
		c.Set(int(st.x), int(st.y), col)
	}
}

// hsv converts HSV in [0,1] to engine.Color. Used for the rainbow
// underline on the title.
func hsv(h, sat, v float64) engine.Color {
	h6 := math.Mod(h, 1.0) * 6
	i := int(h6)
	f := h6 - float64(i)
	pp := v * (1 - sat)
	qq := v * (1 - f*sat)
	tt := v * (1 - (1-f)*sat)
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, tt, pp
	case 1:
		r, g, b = qq, v, pp
	case 2:
		r, g, b = pp, v, tt
	case 3:
		r, g, b = pp, qq, v
	case 4:
		r, g, b = tt, pp, v
	case 5:
		r, g, b = v, pp, qq
	}
	return engine.Color{R: uint8(r * 255), G: uint8(g * 255), B: uint8(b * 255), A: 255}
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
