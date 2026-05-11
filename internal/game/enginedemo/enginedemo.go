// Package enginedemo registers an "enginedemo" game that lets the user
// cycle through a set of small scenes to verify the engine's rendering
// and input handling.
package enginedemo

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "enginedemo" }

func (game) Description() string {
	return "Cycle through engine demos to verify rendering and input"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newRoot(e))
}
