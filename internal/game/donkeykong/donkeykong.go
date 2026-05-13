// Package donkeykong registers a "donkeykong" game: a terminal recreation of
// the iconic 1981 barrel stage. Donkey Kong throws barrels from a top
// platform; Mario climbs slanted girders connected by ladders (some broken)
// to reach Pauline. A hammer power-up lets Mario smash barrels for a short
// window; an oil-drum flame spawns roaming fireballs as the clock ticks
// down. Clearing the stage advances the wave with a faster barrel cadence.
package donkeykong

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "donkeykong" }

func (game) Description() string {
	return "Climb the girders and rescue Pauline from a barrel-throwing ape"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
