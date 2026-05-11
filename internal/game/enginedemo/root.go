package enginedemo

import (
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// demoScene is the interface every demo implements. It mirrors engine.Scene
// but is kept package-local so root can drive demo lifecycle (ESC unwinds
// back to the menu without disturbing the engine loop).
type demoScene interface {
	Update(dt time.Duration) error
	Draw(c *engine.Canvas)
}

type demoFactory struct {
	name  string
	build func() demoScene
}

// root is the top-level engine.Scene. It owns the menu and the currently
// selected demo, forwarding Update/Draw to whichever is active.
type root struct {
	engine *engine.Engine
	menu   *menuScene
	demos  []demoFactory

	inDemo bool
	demo   demoScene
}

func newRoot(e *engine.Engine) *root {
	r := &root{engine: e}
	r.demos = []demoFactory{
		{"COLOR PALETTE", newPaletteDemo},
		{"BOUNCING BALL", newBouncingBallDemo},
		{"SHAPES", newShapesDemo},
		{"PLASMA", newPlasmaDemo},
		{"MULTI-KEY", func() demoScene { return newKeysDemo(e) }},
		{"CAT", newCatDemo},
	}
	names := make([]string, len(r.demos))
	for i, d := range r.demos {
		names[i] = d.name
	}
	r.menu = &menuScene{items: names}
	return r
}

func (r *root) Update(dt time.Duration) error {
	for {
		k, ok := r.engine.PollKey()
		if !ok {
			break
		}
		if r.inDemo {
			// ESC always returns to the menu.
			if k.Code == engine.KeyEsc {
				r.inDemo = false
				r.demo = nil
				continue
			}
			// Anything else is ignored — demos are passive.
			continue
		}
		// In the menu.
		if k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q') {
			return engine.ErrQuit
		}
		if k.Code == engine.KeyEsc {
			return engine.ErrQuit
		}
		if r.menu.handleKey(k) == menuLaunch {
			idx := r.menu.selected
			if idx >= 0 && idx < len(r.demos) {
				r.demo = r.demos[idx].build()
				r.inDemo = true
			}
		}
	}
	if r.inDemo {
		return r.demo.Update(dt)
	}
	return nil
}

func (r *root) Draw(c *engine.Canvas) {
	if r.inDemo {
		r.demo.Draw(c)
		return
	}
	r.menu.draw(c)
}
