// Package spaceinvaders registers a "spaceinvaders" game: a small terminal
// homage to the classic fixed-shooter formula — march-and-drop alien
// formation, destructible bunkers, mystery UFO, and a player cannon at the
// bottom of the screen.
package spaceinvaders

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "spaceinvaders" }

func (game) Description() string {
	return "Defend Earth from a descending alien formation"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
