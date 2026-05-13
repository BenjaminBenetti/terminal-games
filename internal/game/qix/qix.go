// Package qix registers a recreation of Taito's 1981 arcade game Qix:
// the player steers a marker that travels along the borders of a
// rectangular playfield and can dive into the open area to draw a
// polyline back to the border, claiming the enclosed region that does
// not contain the chaotic line-creature ("Qix"). The level ends when
// the player has claimed at least targetPct percent of the field.
// Sparx travel along the borders and the Fuse chases the player along
// any unfinished draw — see entities.go for those.
package qix

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "qix" }

func (game) Description() string {
	return "Claim 75% of the field while dodging the Qix and Sparx"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
