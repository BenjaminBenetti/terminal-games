// Package defender registers a "defender" game: a terminal recreation
// of the 1981 Williams Electronics side-scrolling shooter. The defining
// mechanics over its contemporaries are the toroidal scrolling world,
// the scanner/radar showing the entire planet, and the humanoid-rescue
// loop — Landers descend, grab humans, and mutate into hostile Mutants
// if they reach the top of the screen carrying one. Letting every
// humanoid die explodes the planet and turns the wave into a chaotic
// mutant rush.
package defender

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "defender" }

func (game) Description() string {
	return "Patrol a wrapping planet — rescue humans, kill landers, dodge mutants"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
