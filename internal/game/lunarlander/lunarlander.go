// Package lunarlander registers a "lunarlander" game: a recreation of
// the 1979 Atari Lunar Lander arcade cabinet. Pilot a small lunar
// module to a soft touchdown on one of several flat landing pads
// scattered across a jagged moonscape, balancing thrust against
// gravity on a finite fuel budget. Score is a function of touchdown
// softness multiplied by the pad's value (2x — 5x for the smallest).
package lunarlander

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "lunarlander" }

func (game) Description() string {
	return "Pilot the LM to a soft touchdown — Atari 1979 tribute"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
