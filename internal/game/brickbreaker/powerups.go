package brickbreaker

import "github.com/BenjaminBenetti/terminal-games/internal/engine"

// powerType is the kind of bonus a falling power-up grants on catch.
type powerType int

const (
	powerMultiBall  powerType = iota // split each ball into three
	powerWidePaddle                  // grow the paddle, timed
	powerExtraLife                   // +1 life, instant
	powerSlowBall                    // slow every ball, timed
)

// Power-up rendering dimensions in canvas pixels. The capsule is small
// enough to fit between brick rows yet large enough to land a one-cell
// glyph on top of it.
const (
	powerUpW         = 6
	powerUpH         = 4
	powerUpFallSpeed = 22.0 // px/s
)

// powerUpEntity is a falling capsule dropped by a destroyed brick. The
// paddle catches it by colliding with the rect; capsules that pass the
// floor without being caught are silently dropped.
type powerUpEntity struct {
	x, y float64
	kind powerType
	bobT float64 // animation timer, currently used for sparkle phase
}

// label is the single-character glyph drawn on top of the capsule and
// used in the HUD legend. M / W / L / S.
func (k powerType) label() string {
	switch k {
	case powerMultiBall:
		return "M"
	case powerWidePaddle:
		return "W"
	case powerExtraLife:
		return "L"
	case powerSlowBall:
		return "S"
	}
	return "?"
}

func (k powerType) color() engine.Color {
	switch k {
	case powerMultiBall:
		return engine.Color{R: 255, G: 220, B: 90, A: 255}
	case powerWidePaddle:
		return engine.Color{R: 90, G: 220, B: 255, A: 255}
	case powerExtraLife:
		return engine.Color{R: 255, G: 110, B: 150, A: 255}
	case powerSlowBall:
		return engine.Color{R: 180, G: 130, B: 255, A: 255}
	}
	return engine.Color{R: 200, G: 200, B: 200, A: 255}
}
