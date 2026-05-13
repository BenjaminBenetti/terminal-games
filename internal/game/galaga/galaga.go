// Package galaga registers a "galaga" game: a terminal recreation of the
// 1981 Namco fixed-shooter. The defining mechanic over Space Invaders is
// the enemy attack pattern — bee/butterfly/boss formations fly in along
// curved Bezier paths, sway as a single unit, then dive at the player
// dropping bombs and (in the case of bosses) attempting to capture the
// player with a tractor beam. A captured ship can be rescued to form a
// dual fighter with twice the firepower.
package galaga

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "galaga" }

func (game) Description() string {
	return "Battle insectoid invaders in curving formation attacks"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
