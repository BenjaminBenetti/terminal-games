// Package starcastle registers a "starcastle" game: a recreation of
// Cinematronics' 1980 vector arcade Star Castle. Pilot a small ship
// around three nested rotating rings of 12 segments each, blasting
// gaps through them to reach the spinning cannon at the center while
// dodging the homing fireballs it spits back. Segments regenerate, so
// the player has to keep up the pressure to carve an aligned channel
// before the rings repair themselves.
package starcastle

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "starcastle" }

func (game) Description() string {
	return "Carve a path through three rotating rings to destroy the cannon — Cinematronics 1980"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
