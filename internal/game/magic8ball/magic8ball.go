// Package magic8ball registers a "magic8ball" game: the user types a
// yes-or-no question, the on-screen 8 ball shakes, and a short mystical
// answer is rendered inside the ball's triangle window. The answer text
// is produced by whichever AI coding-agent CLI the user has installed —
// see agent.go for the priority list (Claude, Codex, Gemini, Copilot).
package magic8ball

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "magic8ball" }

func (game) Description() string {
	return "Ask the Magic 8 Ball — powered by whichever AI CLI you have installed"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
