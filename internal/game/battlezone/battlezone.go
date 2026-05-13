// Package battlezone registers a "battlezone" game: a recreation of
// Atari's 1980 first-person tank arcade cabinet. The player drives a
// tank through a wireframe wasteland of cubes and pyramids, hunting
// enemy tanks, dodging guided missiles, and snipping the occasional
// saucer for bonus points — all seen through a periscope rendered in
// pure phosphor green.
package battlezone

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "battlezone" }

func (game) Description() string {
	return "Hunt tanks in a vector wasteland — Atari 1980 tribute"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
