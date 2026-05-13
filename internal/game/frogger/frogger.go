// Package frogger registers a "frogger" game: a terminal recreation of
// the iconic 1981 Konami arcade. Hop a frog from the bottom safe-zone,
// across five lanes of traffic, over a median strip, and across a river
// where it must ride logs and turtles (some of which dive) to one of
// five home slots at the top. Fill all five homes to clear the wave;
// each new wave runs faster.
package frogger

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "frogger" }

func (game) Description() string {
	return "Hop across roads and rivers to five home slots — Konami 1981 tribute"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
