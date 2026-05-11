package engine

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// DefaultFPS is the target frame rate when none is specified in Options.
const DefaultFPS = 60

// ErrQuit can be returned from Scene.Update to stop the loop without
// surfacing an error from Run.
var ErrQuit = errors.New("engine: quit requested")

// Scene is the per-frame contract between a game and the Engine.
//
// Update advances simulation state by dt. Return ErrQuit to stop the loop.
//
// Draw paints the current state into canvas. The canvas is not cleared
// automatically — call canvas.Clear() if you want a blank background each
// frame.
type Scene interface {
	Update(dt time.Duration) error
	Draw(canvas *Canvas)
}

// Options configures a new Engine. The zero value is valid: the engine will
// auto-size to the terminal and run at DefaultFPS.
type Options struct {
	// Width and Height are the canvas size in pixels. If either is zero,
	// the engine auto-detects the terminal size (cols × 2*rows), falling
	// back to 80×48 if detection fails.
	Width, Height int

	// TargetFPS is the desired frame rate. Zero means DefaultFPS.
	TargetFPS int

	// Output is the writer the engine emits ANSI sequences to.
	// Nil means os.Stdout. Tests can pass a bytes.Buffer.
	Output io.Writer
}

// Engine drives a Scene at a fixed frame rate, rendering to the terminal.
// An Engine is single-use: do not call Run twice on the same instance.
type Engine struct {
	canvas   *Canvas
	fps      int
	out      io.Writer
	renderer renderer

	stopCh   chan struct{}
	stopOnce sync.Once

	inputCh chan Key

	pressedMu    sync.Mutex
	pressedKeys  map[KeyCode]*keyState
	pressedChars map[rune]*keyState
}

// keyState tracks how recently a key was reported as held. On terminals
// that support the Kitty keyboard protocol, released is flipped to true
// when a release event arrives. On legacy terminals (no release events),
// released stays false and a key is considered "up" only after no
// press/repeat event has been seen for keyHoldDecay.
type keyState struct {
	lastSeen time.Time
	released bool
}

// keyHoldDecay is the legacy-terminal fallback timeout: how long a key is
// considered "still held" after the last press/repeat event with no
// explicit release. The kernel's keyboard auto-repeat delay is typically
// 250–500 ms, so we pick a value comfortably above that so a freshly held
// key doesn't flicker to "released" before auto-repeat kicks in.
const keyHoldDecay = 600 * time.Millisecond

// New constructs an Engine from opts.
func New(opts Options) (*Engine, error) {
	w, h := opts.Width, opts.Height
	if w == 0 || h == 0 {
		cols, rows, err := TerminalSize()
		if err != nil || cols == 0 || rows == 0 {
			cols, rows = 80, 24
		}
		if w == 0 {
			w = cols
		}
		if h == 0 {
			h = rows * 2
		}
	}
	fps := opts.TargetFPS
	if fps <= 0 {
		fps = DefaultFPS
	}
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}
	return &Engine{
		canvas:  NewCanvas(w, h),
		fps:     fps,
		out:     out,
		stopCh:  make(chan struct{}),
		inputCh: make(chan Key, 64),
	}, nil
}

// Canvas returns the canvas the engine renders each frame. Useful for scenes
// that want to inspect dimensions during construction.
func (e *Engine) Canvas() *Canvas { return e.canvas }

// FPS returns the engine's target frame rate.
func (e *Engine) FPS() int { return e.fps }

// Stop requests the engine to exit before the next frame. It is safe to call
// from any goroutine and multiple times.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() { close(e.stopCh) })
}

// IsKeyDown reports whether the given non-character key (KeyUp, KeyEsc,
// KeyEnter, …) is currently held. On terminals that support the Kitty
// keyboard protocol the result is driven by explicit press/release events,
// so simultaneous keypresses (e.g. KeyUp + KeyRight for diagonal movement)
// are reported accurately. On older terminals the engine falls back to an
// auto-repeat heuristic: a key is "down" while press or repeat events keep
// arriving and decays to "up" after ~600 ms of silence.
//
// Use IsCharDown for printable characters like 'w' or '1'.
//
// Safe to call from any goroutine — typically from Scene.Update.
func (e *Engine) IsKeyDown(code KeyCode) bool {
	if code == KeyChar {
		return false
	}
	e.pressedMu.Lock()
	defer e.pressedMu.Unlock()
	return isStateDown(e.pressedKeys[code])
}

// IsCharDown reports whether the given printable character key is currently
// held. See IsKeyDown for the difference between Kitty-mode and legacy
// behaviour.
func (e *Engine) IsCharDown(r rune) bool {
	e.pressedMu.Lock()
	defer e.pressedMu.Unlock()
	return isStateDown(e.pressedChars[r])
}

func isStateDown(st *keyState) bool {
	if st == nil || st.lastSeen.IsZero() {
		return false
	}
	if st.released {
		return false
	}
	return time.Since(st.lastSeen) < keyHoldDecay
}

// recordKey updates the pressed-state map for a parsed key event. Called
// by the input goroutine for every press/repeat/release.
func (e *Engine) recordKey(k Key, eventType int) {
	e.pressedMu.Lock()
	defer e.pressedMu.Unlock()
	var st *keyState
	if k.Code == KeyChar {
		if e.pressedChars == nil {
			e.pressedChars = make(map[rune]*keyState)
		}
		st = e.pressedChars[k.Rune]
		if st == nil {
			st = &keyState{}
			e.pressedChars[k.Rune] = st
		}
	} else {
		if e.pressedKeys == nil {
			e.pressedKeys = make(map[KeyCode]*keyState)
		}
		st = e.pressedKeys[k.Code]
		if st == nil {
			st = &keyState{}
			e.pressedKeys[k.Code] = st
		}
	}
	if eventType == kittyEventRelease {
		st.released = true
		return
	}
	st.lastSeen = time.Now()
	st.released = false
}

// Run blocks, ticking the scene at the target frame rate. It enters the
// terminal's alternate screen buffer, hides the cursor, and restores both on
// return. Run exits cleanly when:
//   - the scene's Update returns ErrQuit,
//   - Stop is called,
//   - the process receives SIGINT or SIGTERM.
func (e *Engine) Run(scene Scene) error {
	if err := e.enter(); err != nil {
		return err
	}
	defer e.exit()

	inputCleanup := e.startInput()
	defer inputCleanup()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	frameDur := time.Second / time.Duration(e.fps)
	ticker := time.NewTicker(frameDur)
	defer ticker.Stop()

	// Render an initial frame at dt=0 so the screen is not blank before
	// the first tick fires.
	if done, err := e.tick(scene, 0); done || err != nil {
		return err
	}

	last := time.Now()
	for {
		select {
		case <-e.stopCh:
			return nil
		case <-sigCh:
			return nil
		case t := <-ticker.C:
			dt := t.Sub(last)
			last = t
			if done, err := e.tick(scene, dt); done || err != nil {
				return err
			}
		}
	}
}

// tick runs one Update/Draw/render cycle. It returns done=true when the loop
// should exit cleanly (ErrQuit), or err != nil on a real error.
func (e *Engine) tick(scene Scene, dt time.Duration) (done bool, err error) {
	if err := scene.Update(dt); err != nil {
		if errors.Is(err, ErrQuit) {
			return true, nil
		}
		return true, err
	}
	scene.Draw(e.canvas)
	if err := e.renderer.render(e.canvas, e.out); err != nil {
		return true, err
	}
	return false, nil
}

// enter switches to the alternate screen buffer, hides the cursor, disables
// application cursor-key mode (DECCKM) so arrows arrive as CSI sequences,
// clears the screen, and pushes the Kitty keyboard progressive-enhancement
// flags so we get press/release events on terminals that support them.
// Terminals that don't recognise the Kitty sequence silently ignore it.
func (e *Engine) enter() error {
	_, err := fmt.Fprint(e.out,
		"\x1b[?1049h\x1b[?25l\x1b[?1l\x1b[2J\x1b[H"+kittyEnableSequence)
	return err
}

// exit pops the Kitty keyboard stack, restores the previous screen, and
// shows the cursor.
func (e *Engine) exit() {
	fmt.Fprint(e.out, kittyDisableSequence+"\x1b[0m\x1b[?25h\x1b[?1049l")
}
