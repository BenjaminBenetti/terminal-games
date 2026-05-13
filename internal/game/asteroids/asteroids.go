// Package asteroids registers an "asteroids" game: a vector-graphics
// homage to Atari's 1979 arcade classic — a free-floating ship in a
// toroidal asteroid field, rocks that split when shot, and the two
// flying-saucer variants that show up to harass the player.
package asteroids

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "asteroids" }

func (game) Description() string {
	return "Drift through a wrapping asteroid field — shoot rocks and saucers"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
