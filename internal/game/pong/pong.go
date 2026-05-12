// Package pong registers a "pong" game: the classic two-paddle
// ball-and-bounce arcade match. Supports a single-player mode against a
// simple tracking AI and a hot-seat two-player mode.
package pong

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "pong" }

func (game) Description() string {
	return "Classic paddle-and-ball — solo vs CPU or two-player hot seat"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
