// Package flappybird registers a "flappybird" game: a faithful homage to
// the 2013 Dong Nguyen mobile original — tap to flap, dodge the endless
// stream of green pipes, score one point for every pipe you clear.
package flappybird

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "flappybird" }

func (game) Description() string {
	return "Tap to flap past endless pipes — homage to the 2013 mobile original"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
