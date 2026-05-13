// Package gorf registers a terminal recreation of the 1981 Bally/Midway
// arcade game GORF.
//
// The hallmark of Gorf — and the reason it's worth recreating — is that
// it strings together five distinct shooter sub-games into a single
// loop, each with its own enemy behaviour. The player advances through:
//
//  1. ASTRO BATTLES — a Space Invaders-style march behind a curved
//     destructible force-field.
//  2. LASER ATTACK  — a two-row laser-ship formation that drifts toward
//     the player firing continuous beams.
//  3. GALAXIANS     — a swaying formation that peels off divers on
//     curving paths toward the player.
//  4. SPACE WARP    — enemies that emerge from a vanishing point at the
//     centre of the screen and spiral outward.
//  5. FLAG SHIP     — the Gorfian mothership boss with a sliding shield
//     wall and an exposed reactor.
//
// Defeating the Flag Ship loops the cycle with increased difficulty. The
// player has 4 "shields" instead of lives; movement is 2D within the
// lower half of the play field, and the Quad Laser fires one wide bolt
// at a time.
package gorf

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "gorf" }

func (game) Description() string {
	return "Battle Gorfian forces across five distinct missions"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
