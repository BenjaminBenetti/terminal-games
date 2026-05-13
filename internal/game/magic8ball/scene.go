package magic8ball

import (
	"context"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// sceneState is the top-level state machine driving the game.
//
//	stateInput  → user is typing a question
//	stateShake  → ball jitters; AI request is in flight; we wait at
//	              least minShakeSecs and for the result to arrive
//	stateReveal → triangle window scales in over revealSecs
//	stateAnswer → answer is displayed; ENTER returns to input
type sceneState int

const (
	stateInput sceneState = iota
	stateShake
	stateReveal
	stateAnswer
)

const (
	// maxQuestionLen caps user input. Longer questions just stop
	// accepting more characters; the cap also avoids passing a giant
	// argv to the agent CLI.
	maxQuestionLen = 80
	// minShakeSecs is the minimum time the ball must shake before the
	// answer can be revealed, even if the agent replies instantly. This
	// preserves the toy-like ritual of "shake and watch".
	minShakeSecs = 1.5
	// revealSecs is how long the triangle takes to scale from 0 to full
	// size when transitioning shake → answer.
	revealSecs = 0.5
	// cursorBlinkHz controls the input-prompt cursor blink rate (Hz).
	cursorBlinkHz = 2.0
)

// scene is the engine.Scene for the magic 8 ball game.
type scene struct {
	e *engine.Engine

	state  sceneState
	stateT float64 // seconds since entering current state
	t      float64 // seconds since scene start (continuous; for pulses)

	question []rune
	cursorT  float64

	// Async agent request state. resultCh receives exactly one value;
	// cancel aborts the in-flight CLI if the user quits during a shake.
	resultCh      chan agentResult
	cancel        context.CancelFunc
	pendingResult *agentResult
	shakeT        float64
	revealT       float64

	// Final answer fields filled in once the agent replies.
	answer    string
	agent     string
	answerErr string

	// detected is the agent we'll *probably* ask, computed at scene
	// construction so the input screen can advertise it. The real call
	// re-detects in case PATH changes during the session.
	detected      agentSpec
	detectedFound bool
}

func newScene(e *engine.Engine) *scene {
	d, ok := detectAgent()
	return &scene{
		e:             e,
		state:         stateInput,
		detected:      d,
		detectedFound: ok,
	}
}

// Update — drains keys, advances per-state timers, and supervises the
// async agent request.
func (s *scene) Update(dt time.Duration) error {
	ds := dt.Seconds()
	s.stateT += ds
	s.t += ds
	s.cursorT += ds

	switch s.state {
	case stateInput:
		return s.updateInput()
	case stateShake:
		return s.updateShake(ds)
	case stateReveal:
		return s.updateReveal(ds)
	case stateAnswer:
		return s.updateAnswer()
	}
	return nil
}

func (s *scene) transitionTo(st sceneState) {
	s.state = st
	s.stateT = 0
}

func (s *scene) updateInput() error {
	for {
		k, ok := s.e.PollKey()
		if !ok {
			break
		}
		switch k.Code {
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyEnter:
			if strings.TrimSpace(string(s.question)) == "" {
				continue
			}
			s.startAsk()
			return nil
		case engine.KeyBackspace:
			if n := len(s.question); n > 0 {
				s.question = s.question[:n-1]
			}
		case engine.KeyChar:
			if len(s.question) >= maxQuestionLen {
				continue
			}
			r := k.Rune
			// Printable ASCII only — the canvas font is uppercase Latin
			// + digits + a few punctuation marks, and we want to avoid
			// passing control codes through to the agent CLI.
			if r >= 32 && r < 127 {
				s.question = append(s.question, r)
			}
		}
	}
	return nil
}

// startAsk kicks off the goroutine that runs the agent CLI and switches
// to the shake state.
func (s *scene) startAsk() {
	s.answer = ""
	s.agent = ""
	s.answerErr = ""
	s.pendingResult = nil
	s.shakeT = 0
	s.revealT = 0

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	ch := make(chan agentResult, 1)
	s.resultCh = ch
	q := strings.TrimSpace(string(s.question))
	go func() {
		ch <- askAgent(ctx, q)
	}()
	s.transitionTo(stateShake)
}

func (s *scene) updateShake(ds float64) error {
	s.shakeT += ds

	// Non-blocking poll of the result channel; cache the value so we
	// don't keep racing select once it has arrived.
	if s.pendingResult == nil && s.resultCh != nil {
		select {
		case r := <-s.resultCh:
			s.pendingResult = &r
		default:
		}
	}

	// Only transition once we've shaken long enough *and* have a result
	// — the shake masks the agent's latency rather than the user
	// staring at a still ball.
	if s.shakeT >= minShakeSecs && s.pendingResult != nil {
		s.applyResult(*s.pendingResult)
		s.pendingResult = nil
		s.resultCh = nil
		s.transitionTo(stateReveal)
	}

	for {
		k, ok := s.e.PollKey()
		if !ok {
			break
		}
		if k.Code == engine.KeyEsc {
			if s.cancel != nil {
				s.cancel()
				s.cancel = nil
			}
			return engine.ErrQuit
		}
	}
	return nil
}

// applyResult moves an agentResult into the visible answer fields,
// translating errors into a UI-friendly message.
func (s *scene) applyResult(r agentResult) {
	if r.err != nil {
		if r.err == errNoAgent {
			s.answerErr = "NO AGENT FOUND"
		} else {
			s.answerErr = "SPIRITS ARE SILENT"
		}
		s.answer = ""
		s.agent = ""
		return
	}
	s.answer = r.answer
	s.agent = r.agent
}

func (s *scene) updateReveal(ds float64) error {
	s.revealT += ds
	if s.revealT >= revealSecs {
		s.transitionTo(stateAnswer)
	}
	for {
		k, ok := s.e.PollKey()
		if !ok {
			break
		}
		if k.Code == engine.KeyEsc {
			return engine.ErrQuit
		}
	}
	return nil
}

func (s *scene) updateAnswer() error {
	for {
		k, ok := s.e.PollKey()
		if !ok {
			break
		}
		switch k.Code {
		case engine.KeyEsc:
			return engine.ErrQuit
		case engine.KeyEnter:
			s.resetForNext()
			return nil
		}
	}
	return nil
}

func (s *scene) resetForNext() {
	s.question = s.question[:0]
	s.answer = ""
	s.agent = ""
	s.answerErr = ""
	s.pendingResult = nil
	s.resultCh = nil
	s.shakeT = 0
	s.revealT = 0
	s.transitionTo(stateInput)
}

// Draw paints one frame.
func (s *scene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 14, A: 255})
	s.drawTitle(c)
	cx, cy, r := s.ballGeometry(c)
	s.drawBall(c, cx, cy, r)
	s.drawBottom(c)
}

// ballGeometry picks the ball's pixel-space center and radius, scaling
// to fit alongside the title (top) and the input/hint rows (bottom).
func (s *scene) ballGeometry(c *engine.Canvas) (cx, cy, r int) {
	w := c.Width()
	h := c.Height()
	cx = w / 2
	// Centre slightly above midpoint so the input prompt below has
	// breathing room.
	cy = h*9/20 + 4
	// Reserve roughly 14 px at top (title) and 16 px at bottom (input + hint).
	maxByH := (h - 30) / 2
	maxByW := w/2 - 4
	r = maxByH
	if maxByW < r {
		r = maxByW
	}
	if r < 8 {
		r = 8
	}
	if r > 22 {
		r = 22
	}
	return cx, cy, r
}

func (s *scene) drawTitle(c *engine.Canvas) {
	title := "MAGIC 8 BALL"
	tw := engine.TextWidth(title)
	tx := (c.Width() - tw) / 2
	if tx < 0 {
		tx = 0
	}
	ty := 1
	c.DrawText(tx, ty, title, engine.White)

	// Soft pulsing underline so the title doesn't feel static.
	p := 0.5 + 0.5*math.Sin(s.t*1.8)
	shade := uint8(80 + 120*p)
	c.FillRect(tx, ty+engine.FontHeight+1, tw, 1,
		engine.Color{R: shade / 3, G: shade / 3, B: shade, A: 255})
}

// drawBall paints the ball body, glint, and the per-state inner content
// (idle "8", shaking "8" with jitter, or the answer triangle).
func (s *scene) drawBall(c *engine.Canvas, cx, cy, r int) {
	ox, oy := 0, 0
	if s.state == stateShake {
		// Random jitter each frame. Amplitude grows briefly then
		// settles, but a flat 2-3 px feels punchy enough for the
		// terminal canvas.
		amp := 2.5
		ox = int(amp * (rand.Float64()*2 - 1))
		oy = int(amp * (rand.Float64()*2 - 1))
	}

	body := engine.Color{R: 10, G: 10, B: 20, A: 255}
	rim := engine.Color{R: 40, G: 40, B: 60, A: 255}

	c.FillCircle(cx+ox, cy+oy, r, body)
	c.DrawCircle(cx+ox, cy+oy, r, rim)
	c.DrawCircle(cx+ox, cy+oy, r-1, rim)

	// Specular glint top-left.
	glintR := r / 5
	if glintR < 2 {
		glintR = 2
	}
	c.FillCircle(
		cx+ox-r*3/5+glintR,
		cy+oy-r*3/5+glintR,
		glintR,
		engine.Color{R: 220, G: 220, B: 235, A: 255},
	)

	switch s.state {
	case stateInput:
		s.drawBig8(c, cx+ox, cy+oy, r)
	case stateShake:
		// Keep the "8" visible so the ball still reads as an 8 ball
		// while it's shaking — only the offset jitter conveys motion.
		s.drawBig8(c, cx+ox, cy+oy, r)
	case stateReveal:
		scale := s.revealT / revealSecs
		if scale < 0 {
			scale = 0
		}
		if scale > 1 {
			scale = 1
		}
		s.drawAnswerWindow(c, cx+ox, cy+oy, r, scale, false)
	case stateAnswer:
		s.drawAnswerWindow(c, cx+ox, cy+oy, r, 1, true)
	}
}

// drawBig8 paints a white disc containing two stacked ring outlines to
// suggest a chunky "8". Using two rings instead of the 5x7 font glyph
// keeps the digit readable at ball-sized scale.
func (s *scene) drawBig8(c *engine.Canvas, cx, cy, ballR int) {
	discR := ballR / 2
	if discR < 5 {
		discR = 5
	}
	c.FillCircle(cx, cy, discR, engine.White)

	ringR := discR/2 - 1
	if ringR < 3 {
		ringR = 3
	}
	top := cy - ringR + 1
	bot := cy + ringR - 1
	ink := engine.Color{R: 10, G: 10, B: 20, A: 255}
	// Two passes per ring to thicken the outline so it reads as an "8"
	// rather than two thin circles.
	c.DrawCircle(cx, top, ringR, ink)
	c.DrawCircle(cx, top, ringR-1, ink)
	c.DrawCircle(cx, bot, ringR, ink)
	c.DrawCircle(cx, bot, ringR-1, ink)
}

// drawAnswerWindow paints the blue triangle window plus the answer text
// (only when showText is true — during the reveal animation we hold
// off on text until the triangle is at full size).
func (s *scene) drawAnswerWindow(c *engine.Canvas, cx, cy, ballR int, scale float64, showText bool) {
	if scale <= 0 {
		return
	}
	baseW := ballR * 8 / 5
	baseH := ballR * 5 / 6
	triW := int(float64(baseW) * scale)
	triH := int(float64(baseH) * scale)
	if triW < 2 || triH < 2 {
		return
	}

	apexY := cy - triH/2
	baseY := cy + triH/2

	blue := engine.Color{R: 30, G: 70, B: 220, A: 255}
	deep := engine.Color{R: 18, G: 40, B: 140, A: 255}

	// Filled triangle with apex at top, base at bottom. A row's half-width
	// scales linearly with the vertical progress from apex to base.
	for y := apexY; y <= baseY; y++ {
		progress := float64(y-apexY) / float64(triH)
		hw := int(progress * float64(triW) / 2)
		c.FillRect(cx-hw, y, hw*2+1, 1, blue)
	}
	// Darker outline along the base for a subtle 3D feel.
	c.FillRect(cx-triW/2, baseY, triW, 1, deep)

	if !showText {
		return
	}

	var text string
	switch {
	case s.answerErr != "":
		text = s.answerErr
	case s.answer != "":
		text = s.answer
	default:
		text = "..."
	}

	// Lines fit roughly within ~60% of the triangle's base width so the
	// triangle's slant doesn't clip them.
	maxCols := triW * 3 / 5
	if maxCols < 4 {
		maxCols = 4
	}
	lines := wrapText(text, maxCols)
	if len(lines) > 4 {
		lines = lines[:4]
	}

	// Position lines along the bottom of the triangle where it's
	// widest — text near the apex would be squeezed by the slope. The
	// bottom line sits one cell row above the base; further lines
	// stack upward from there.
	baseRow := baseY/2 - 1
	apexRow := apexY / 2
	startRow := baseRow - len(lines) + 1
	if startRow < apexRow+1 {
		startRow = apexRow + 1
	}

	for i, ln := range lines {
		row := startRow + i
		col := cx - len(ln)/2
		if col < 0 {
			col = 0
		}
		c.Print(col, row, ln, engine.White)
	}
}

// drawBottom paints whichever footer UI matches the current state:
// the question input + hint, the shaking status, the reveal pause, or
// the "answered by X" credit + next-question hint.
func (s *scene) drawBottom(c *engine.Canvas) {
	cols := c.Cols()
	rows := c.Rows()

	statusRow := rows - 4
	inputRow := rows - 3
	hintRow := rows - 1

	gray := engine.Gray
	soft := engine.Color{R: 170, G: 170, B: 220, A: 255}

	switch s.state {
	case stateInput:
		// Label above the input line.
		label := "ASK YOUR QUESTION"
		c.Print((cols-len(label))/2, statusRow, label, soft)

		// Input line: "> question_" with blinking cursor.
		q := string(s.question)
		cursor := " "
		if int(s.cursorT*cursorBlinkHz)%2 == 0 {
			cursor = "_"
		}
		line := "> " + q + cursor
		if len(line) > cols {
			// Keep the tail visible so the user sees what they're typing.
			line = line[len(line)-cols:]
		}
		col := (cols - len(line)) / 2
		if col < 0 {
			col = 0
		}
		c.Print(col, inputRow, line, engine.White)

		// Agent badge — tell the user who'll be answering.
		badge := "NO AGENT INSTALLED"
		badgeColour := engine.Color{R: 200, G: 120, B: 120, A: 255}
		if s.detectedFound {
			badge = "POWERED BY " + strings.ToUpper(s.detected.name)
			badgeColour = engine.Color{R: 130, G: 200, B: 140, A: 255}
		}
		c.Print((cols-len(badge))/2, 0, badge, badgeColour)

		hint := "ENTER ASK     ESC QUIT"
		c.Print((cols-len(hint))/2, hintRow, hint, gray)

	case stateShake:
		who := "THE SPIRITS"
		if s.detectedFound {
			who = strings.ToUpper(s.detected.name)
		}
		// Alternate between two messages so the row visibly ticks even
		// while we wait on the (potentially slow) agent.
		dots := strings.Repeat(".", 1+int(s.t*3)%4)
		status := "CONSULTING " + who + dots
		c.Print((cols-len(status))/2, statusRow, status, soft)

		hint := "ESC QUIT"
		c.Print((cols-len(hint))/2, hintRow, hint, gray)

	case stateReveal:
		c.Print((cols-3)/2, statusRow, "...", soft)

	case stateAnswer:
		switch {
		case s.answerErr == "NO AGENT FOUND":
			line := "INSTALL CLAUDE, CODEX, GEMINI, OR COPILOT"
			if len(line) > cols {
				line = "INSTALL AN AI AGENT CLI"
			}
			c.Print((cols-len(line))/2, statusRow, line,
				engine.Color{R: 230, G: 130, B: 130, A: 255})
		case s.answerErr != "":
			line := "TRY AGAIN"
			c.Print((cols-len(line))/2, statusRow, line,
				engine.Color{R: 230, G: 180, B: 130, A: 255})
		case s.agent != "":
			line := "ANSWERED BY " + strings.ToUpper(s.agent)
			c.Print((cols-len(line))/2, statusRow, line,
				engine.Color{R: 130, G: 200, B: 140, A: 255})
		}

		// Echo the question above the status so the player can see
		// what they asked alongside the answer.
		if q := strings.TrimSpace(string(s.question)); q != "" {
			line := "> " + q
			if len(line) > cols {
				line = line[:cols-1] + "…"
			}
			col := (cols - len(line)) / 2
			if col < 0 {
				col = 0
			}
			c.Print(col, statusRow-2, line, gray)
		}

		hint := "ENTER ASK ANOTHER     ESC QUIT"
		c.Print((cols-len(hint))/2, hintRow, hint, gray)
	}
}

// wrapText word-wraps s into lines of at most maxCols runes each. A
// word longer than maxCols is hard-cut to maxCols so it doesn't bleed
// out of the triangle.
func wrapText(s string, maxCols int) []string {
	if maxCols < 1 {
		maxCols = 1
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := ""
	for _, w := range words {
		if len(w) > maxCols {
			// Flush any current line and hard-cut the long word.
			if cur != "" {
				lines = append(lines, cur)
				cur = ""
			}
			lines = append(lines, w[:maxCols])
			continue
		}
		switch {
		case cur == "":
			cur = w
		case len(cur)+1+len(w) <= maxCols:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}
