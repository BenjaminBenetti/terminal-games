// Package scramble registers a "scramble" game: a terminal recreation of
// Konami's 1981 horizontally-scrolling shooter. The defining shape of
// Scramble is its six distinct sections — mountains with launchable
// rockets, the UFO fleet, the fireball storm, the narrow cavern, the
// city, and finally the enemy base with its reactor — looped at higher
// speed once you punch through the reactor. The player ship scrolls
// continuously to the right, fires a forward laser, drops gravity bombs,
// and must keep the fuel gauge topped up by bombing fuel depots.
package scramble

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "scramble" }

func (game) Description() string {
	return "Fly through six scrolling sectors and destroy the enemy base"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
