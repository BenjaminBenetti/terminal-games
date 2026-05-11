// Package engine provides a fixed-step game loop and a pixel canvas
// for building games that draw coloured blocks to the terminal.
//
// The engine drives a Scene at a target frame rate (60 FPS by default)
// and renders the current canvas to the terminal using truecolor ANSI
// escape codes and the Unicode upper half block (▀): each terminal cell
// holds two vertically-stacked pixels (top half = foreground colour,
// bottom half = background colour). A 24-row terminal therefore exposes
// 48 rows of pixels.
//
// The renderer keeps a front buffer and only emits escape sequences for
// cells that changed since the previous frame, so a steady scene costs
// almost nothing to redraw.
//
// A minimal game:
//
//	type Demo struct{ t float64 }
//
//	func (d *Demo) Update(dt time.Duration) error {
//	    d.t += dt.Seconds()
//	    return nil
//	}
//
//	func (d *Demo) Draw(c *engine.Canvas) {
//	    c.Clear(engine.Black)
//	    x := int(40 + 20*math.Sin(d.t))
//	    c.FillCircle(x, 24, 6, engine.Red)
//	}
//
//	func main() {
//	    e, _ := engine.New(engine.Options{})
//	    _ = e.Run(&Demo{})
//	}
package engine
