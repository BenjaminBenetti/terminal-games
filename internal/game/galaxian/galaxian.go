// Package galaxian registers a "galaxian" game: a terminal recreation of
// Namco's 1979 fixed-shooter Galaxian. The hallmark over Space Invaders
// is the swooping attack — aliens peel off the stationary formation in
// looping arcs, dive at the player, and (if they survive the run) curve
// back into their slot. A flagship at the top dives with up to two
// escorts for the famous 800-point convoy kill.
package galaxian

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "galaxian" }

func (game) Description() string {
	return "Namco's 1979 swoop-attack shooter — dive bombers and flagships"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
