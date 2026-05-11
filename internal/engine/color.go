package engine

// Color is a 32-bit RGBA colour. A zero-value Color (alpha 0) is treated
// as transparent by drawing primitives and rendered as black on screen.
type Color struct {
	R, G, B, A uint8
}

// RGB constructs an opaque colour (alpha 255).
func RGB(r, g, b uint8) Color {
	return Color{R: r, G: g, B: b, A: 255}
}

// RGBA constructs a colour with the given components.
func RGBA(r, g, b, a uint8) Color {
	return Color{R: r, G: g, B: b, A: a}
}

// Predefined opaque colours.
var (
	Black       = Color{R: 0, G: 0, B: 0, A: 255}
	White       = Color{R: 255, G: 255, B: 255, A: 255}
	Red         = Color{R: 255, G: 0, B: 0, A: 255}
	Green       = Color{R: 0, G: 255, B: 0, A: 255}
	Blue        = Color{R: 0, G: 0, B: 255, A: 255}
	Yellow      = Color{R: 255, G: 255, B: 0, A: 255}
	Cyan        = Color{R: 0, G: 255, B: 255, A: 255}
	Magenta     = Color{R: 255, G: 0, B: 255, A: 255}
	Gray        = Color{R: 128, G: 128, B: 128, A: 255}
	Transparent = Color{}
)
