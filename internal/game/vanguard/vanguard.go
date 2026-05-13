// Package vanguard registers a "vanguard" game: a terminal recreation of
// the 1981 SNK arcade scrolling shooter. The defining mechanic over
// other shooters of its era is multi-directional firing — the player
// ship moves in eight directions and fires independently in four
// cardinal directions. The world auto-scrolls through five linked zones
// (Mountain, Stripe, Bleak, Rainbow, Styx) ending in a chamber fight
// against the Gond. Energy pods ("E") restore the constantly-draining
// energy meter and grant brief invulnerability with auto-fire.
package vanguard

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "vanguard" }

func (game) Description() string {
	return "Pilot a four-way shooter through five zones to destroy the Gond"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
