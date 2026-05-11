# Canvas

The `Canvas` is the drawing surface every `Scene.Draw` paints into. It
holds a pixel grid plus an optional text overlay; both are reset by
`Clear`.

## Coordinate systems

There are **two** coordinate systems on the canvas:

- **Pixel coords** `(x, y)`: used by shape primitives. `x ∈ [0, Width)`,
  `y ∈ [0, Height)`. One pixel column maps to one terminal cell column,
  but two pixel rows pack into one terminal cell row (top half + bottom
  half of the cell). So a typical 80×24 terminal yields a 80×48 pixel
  canvas.
- **Cell coords** `(col, row)`: used by `Print`. `col ∈ [0, Cols())`,
  `row ∈ [0, Rows())`. One rune = one terminal cell. `Cols() == Width()`,
  `Rows() == Height()/2`.

| You want… | Use |
|---|---|
| Width in pixels | `Width()` |
| Height in pixels | `Height()` |
| Width in terminal cells | `Cols()` (same as `Width()`) |
| Height in terminal cells | `Rows()` |

## Shape primitives

All primitives clip to canvas bounds — out-of-range pixels are silently
ignored, never a panic.

```go
c.Clear(engine.Black)                          // fill + wipe text overlay
c.Set(x, y, engine.Red)                        // single pixel
got := c.Get(x, y)                             // returns Transparent if OOB
c.FillRect(x, y, w, h, engine.Blue)            // solid rectangle
c.DrawRect(x, y, w, h, engine.Green)           // outline only
c.DrawLine(x0, y0, x1, y1, engine.White)       // Bresenham line
c.FillCircle(cx, cy, r, engine.Yellow)         // solid disc
c.DrawCircle(cx, cy, r, engine.Cyan)           // circle outline (midpoint)
```

Notes:

- `NewCanvas(w, h)` rounds `h` up to the next even number so it divides
  evenly into terminal cells.
- `Set` on an out-of-bounds coordinate is a no-op. `Get` on an OOB coord
  returns `engine.Transparent`.
- `Draw` doesn't auto-clear. Most scenes call `c.Clear(...)` at the top
  of their `Draw` method.

## Images

```go
c.DrawImage(x, y int, img image.Image)
```

Blits a standard `image.Image` onto the canvas. `(x, y)` is the pixel
coordinate the image's top-left maps to. Source pixels are converted to
RGBA8 and copied one-for-one — **no scaling, no blending**. Pixels with
`alpha == 0` are skipped, so a transparent PNG behaves like a sprite over
whatever's already on the canvas.

Any `image.Image` works (PNG, JPEG, GIF, `*image.RGBA`, `*image.NRGBA`,
`*image.Paletted`, …). Subimages with non-zero `Bounds().Min` are
handled correctly: the image's logical top-left maps to `(x, y)`
regardless of where `Bounds().Min` is in source space.

Loading from disk:

```go
import (
    "image/png"
    "os"
)

f, _ := os.Open("sprite.png")
defer f.Close()
sprite, _ := png.Decode(f)

// in Draw:
c.DrawImage(20, 10, sprite)
```

Or embedded at build time (recommended for shipped assets):

```go
import (
    "bytes"
    _ "embed"
    "image"
    "image/png"
)

//go:embed sprite.png
var spriteBytes []byte
var sprite image.Image

func init() {
    sprite, _ = png.Decode(bytes.NewReader(spriteBytes))
}
```

Out-of-bounds destination pixels are clipped, so it's safe to draw an
image partially off-screen.

## Text

Two ways to put text on screen, with very different sizes:

### `Print` — native terminal font (small)

```go
c.Print(col, row, "score: 1234", engine.White)
```

Each rune takes exactly **one terminal cell**, rendered using the user's
configured terminal font. This is the right choice for menus, scores,
labels — anything where you want compact, readable text.

Coordinates are in cells, not pixels. The cell's background colour is
sampled from the underlying pixel pair (`averageColor(top, bottom)`), so:

```go
// Yellow highlight bar behind text.
c.FillRect(0, row*2, c.Cols(), 2, engine.Yellow)
c.Print(col, row, "selected", engine.Black)
```

`Print` writes into a separate text-overlay map; `Clear` wipes both the
overlay and the pixels. Out-of-bounds runes are clipped.

### `DrawText` — built-in pixel-art font (large, blocky)

```go
c.DrawText(x, y, "GAME OVER", engine.Red)
width := engine.TextWidth("GAME OVER")
```

Each rune is rendered as a 5×7 pixel bitmap glyph. Coordinates are in
**pixels**. The font supports `A-Z`, `0-9`, space, and common
punctuation; lowercase is folded to uppercase; unmapped runes render as
a hollow box. Constants: `engine.FontWidth = 5`, `engine.FontHeight = 7`,
`engine.FontAdvance = 6` (5 + 1 px spacing).

Use `DrawText` when you want a deliberate retro pixel-art aesthetic
(stylised titles, scoreboards inside a game world). Use `Print` for
everything else.

## What the demo game does

`internal/game/enginedemo` is the canonical reference. Specifically:

- The menu uses `Print` for the title, list items, and hint, with
  `FillRect` painting the highlight bar behind the selected item.
- Demos use shape primitives (`FillCircle`, `DrawRect`, `DrawLine`) for
  their content and `Print` for footer labels.
- The plasma demo writes per-pixel colours with `Set` in a tight loop,
  then calls `Clear` at the top of its `Draw` to wipe the menu's text
  overlay before painting.
