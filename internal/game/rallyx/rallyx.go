// Package rallyx registers a "rallyx" game: a terminal port of the
// 1980 Namco arcade classic. The player drives a blue race car around
// a large scrolling maze, collecting ten yellow flags before running
// out of fuel while four red enemy cars give chase. Smoke screens
// dropped behind the car briefly stun pursuers and a radar mini-map
// shows the whole world at once — all reference points lifted from
// the original cabinet.
package rallyx

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func init() {
	registry.Register(game{})
}

type game struct{}

func (game) Name() string { return "rallyx" }

func (game) Description() string {
	return "The 1980 Namco arcade — race the maze, grab 10 flags, dodge red cars"
}

func (game) Run() error {
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	return e.Run(newScene(e))
}
