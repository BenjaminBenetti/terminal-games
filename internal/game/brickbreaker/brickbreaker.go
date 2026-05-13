// Package brickbreaker registers a "brickbreaker" game: a small terminal
// homage to the classic Breakout / Arkanoid formula — paddle at the
// bottom, bouncing ball, and a wall of bricks to clear. Three hand-built
// levels of escalating difficulty are selectable from the title screen.
package brickbreaker

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "brickbreaker" }

func (game) Description() string {
	return "Break the wall with a bouncing ball — three levels"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
