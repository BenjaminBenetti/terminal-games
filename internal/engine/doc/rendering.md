# Rendering

This is an explainer for how the engine turns a `Canvas` into bytes on
the terminal. You don't need to read this to use the engine, but if
you're wondering why the canvas is twice as tall as the terminal or why
text and pixels coexist on the same grid, this is the page.

## Half-block pixels

Each terminal cell renders the Unicode upper-half block `▀` (U+2580)
with two truecolor escapes — foreground for the top half, background
for the bottom half:

```
\x1b[38;2;<r>;<g>;<b>m   set foreground RGB
\x1b[48;2;<r>;<g>;<b>m   set background RGB
▀                       paint top=fg, bottom=bg in this cell
```

So **one terminal cell carries two stacked canvas pixels**, which is why
the canvas height is `rows * 2` pixels even though the terminal grid is
only `rows` cells tall. Width is unchanged: one pixel column = one cell
column.

## Text overlay

`Canvas.Print` adds entries to a per-cell text-overlay map keyed by
`row * width + col`. When the renderer visits a cell, it first checks
the overlay:

- If there's a `textCell{rune, fg}` for this position, the cell emits
  that rune (instead of `▀`), with `fg` as the foreground and
  `averageColor(topPixel, bottomPixel)` as the background. The two
  underlying pixels are visually replaced.
- Otherwise, the cell emits `▀` with `top` as fg and `bot` as bg.

This is why `FillRect(0, row*2, cols, 2, Yellow)` followed by
`Print(col, row, "x", Black)` gives black text on a yellow background —
both underlying pixels are yellow, their average is yellow, and the
text bg uses that average.

## Diff rendering

The renderer keeps a "front buffer" — one `cellState` per terminal cell
recording what it last drew there. Each frame:

1. Build the new `cellState` for every cell from the canvas (text
   overlay first, fall back to half-block).
2. Compare against the front buffer. Cells that match are skipped
   entirely — no bytes are emitted.
3. For changed cells, emit (if needed) a cursor-move (`\x1b[<row>;<col>H`),
   a foreground escape, a background escape, then the rune.
4. The cursor advances naturally between consecutive changed cells, so
   no move is emitted as long as `(row, col)` continues from where the
   previous changed cell ended.
5. Foreground and background escapes are only re-emitted when they
   actually change between cells, which keeps long runs of same-colour
   pixels cheap.
6. After the loop, a single `\x1b[0m` resets attributes.

A frame where nothing changed produces **zero bytes**. A frame where
one cell changed typically produces ~25 bytes (cursor move + two colour
escapes + the rune + reset).

## First-frame full redraw

The front buffer is initialised to a sentinel `cellState` (`fg = {1, 2,
3, 0}` — alpha 0 is unrenderable) that won't match any real cell, so
the first `render` call after construction or after a canvas resize
emits every cell. Subsequent frames diff normally.

## Performance budget

At 60 FPS on an 80×24 terminal:

- Total cells: 80 × 24 = 1920.
- Steady-state plasma demo (every pixel changes every frame): ~1920
  cells × ~25 bytes ≈ 48 KB/frame → ~3 MB/s. Modern terminals handle
  this fine but visible tearing depends on the terminal emulator.
- Mostly-static menu: a handful of cells change per frame → bytes per
  frame are in the tens.

If you're rendering full-screen animation every frame, consider whether
you can leave parts of the canvas unchanged. The diff renderer is your
friend.

## Terminal state side-effects

The renderer doesn't restore cursor position or attributes between
frames — the next `\x1b[<r>;<c>H` from the next frame's diff
re-establishes whatever it needs. It also doesn't track or restore the
user's previous colours; the trailing `\x1b[0m` is just a courtesy to
avoid leaving a colour bleeding into Ctrl-C output.
