# Color

```go
type Color struct {
    R, G, B, A uint8
}
```

A 32-bit RGBA colour. `Color` is a comparable value type — `c1 == c2`
just works, which is what the renderer relies on when diffing cells.

## Constructors

```go
engine.RGB(255, 128, 0)        // opaque colour (A = 255)
engine.RGBA(255, 128, 0, 200)  // explicit alpha
```

## Predefined palette

```go
engine.Black       // {0,   0,   0,   255}
engine.White       // {255, 255, 255, 255}
engine.Red         // {255, 0,   0,   255}
engine.Green       // {0,   255, 0,   255}
engine.Blue        // {0,   0,   255, 255}
engine.Yellow      // {255, 255, 0,   255}
engine.Cyan        // {0,   255, 255, 255}
engine.Magenta     // {255, 0,   255, 255}
engine.Gray        // {128, 128, 128, 255}
engine.Transparent // {0,   0,   0,   0}    (A = 0)
```

## Transparency semantics

The terminal can't render "nothing", so transparency only affects how
pixels are *stored*, not what's displayed:

- `A == 0` (transparent) → renders as `Black` on screen, but is tracked
  distinctly in the diff. A transparent → opaque-black change still
  triggers a redraw of that cell.
- `A > 0` is treated as fully opaque. The engine doesn't currently
  blend.

So `c.Clear(Transparent)` clears the canvas to "logical empty" and the
display will be black; `c.Clear(Black)` does the same visually but the
two states differ for diffing purposes (rarely matters).

Drawing primitives don't treat `Transparent` as a no-op — `Set(x, y,
Transparent)` overwrites whatever was there with the transparent value.
If you want compositing, you build it on top.
