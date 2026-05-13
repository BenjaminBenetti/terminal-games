// Package centipede registers a terminal recreation of Atari's 1981
// arcade Centipede. The defining mechanics are reproduced here:
//
//   - A multi-segment centipede snakes down the playfield, dropping one
//     row and reversing direction each time it hits a mushroom or the
//     screen edge.
//   - Shooting a segment kills it (10 pts body, 100 pts head), plants a
//     mushroom in its cell, and splits the centipede into two halves
//     that continue independently.
//   - Mushrooms take four hits to destroy (1 pt) and form the obstacles
//     that steer the centipede toward the player.
//   - A spider bounces in the player's zone, eating mushrooms and
//     awarding 300/600/900 pts based on proximity when shot.
//   - A flea drops straight down planting mushrooms when the lower field
//     gets sparse, and takes two hits to kill (first hit speeds it up,
//     200 pts on the second).
//   - A scorpion walks across the upper field poisoning every mushroom
//     it touches (1000 pts). Centipedes that hit a poisoned mushroom
//     dive straight down to the player zone instead of dropping
//     normally.
//
// The player ("bug blaster") moves with the arrow keys inside the
// bottom band of the field, fires a single bullet at a time with space,
// and has three lives.
package centipede

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "centipede" }

func (game) Description() string {
	return "Blast a snaking centipede through a mushroom forest"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
