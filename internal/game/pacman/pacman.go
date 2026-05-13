// Package pacman registers a "pacman" game: a faithful terminal port
// of the 1980 arcade classic. The 28×31 maze, four ghosts with their
// distinct chase behaviours, scatter / chase / frightened mode timing,
// the side tunnel, and the four corner energizers are all modelled
// after the original.
package pacman

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "pacman" }

func (game) Description() string {
	return "The 1980 arcade maze chase — four ghosts, four energizers, one tunnel"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
