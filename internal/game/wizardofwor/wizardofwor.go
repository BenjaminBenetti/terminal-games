// Package wizardofwor registers a "wizardofwor" game: a faithful terminal
// recreation of the 1980 Midway arcade classic. One Worrior fighter
// roams an edge-wall maze (the "dungeon") shooting Burwors, Garwors and
// Thorwors. Garwors and Thorwors phase in and out of visibility but
// always appear on the bottom radar. After the dungeon is cleared the
// fleeing Worluk may escape through the side tunnel — kill it for a
// doubled-score next dungeon. Occasionally the Wizard of Wor himself
// teleports in and hurls fireballs before retreating.
package wizardofwor

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "wizardofwor" }

func (game) Description() string {
	return "Hunt Burwors, Garwors, Thorwors and the Wizard in his dungeon"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
