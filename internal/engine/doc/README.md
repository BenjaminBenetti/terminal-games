# Engine

A small fixed-step game loop with a pixel canvas for terminal games. The
engine drives a `Scene` at a target frame rate (60 FPS by default), renders
to the terminal using truecolor ANSI escape codes plus the Unicode upper
half-block (`▀`) for pixels, and supports a native-font text overlay on
top of the pixel grid.

## Getting it running

The shortest possible game:

```go
package main

import (
    "log"
    "math"
    "time"

    "github.com/BenjaminBenetti/terminal-games/internal/engine"
)

type demo struct {
    e *engine.Engine
    t float64
}

func (d *demo) Update(dt time.Duration) error {
    d.t += dt.Seconds()
    // Drain pending keys; quit on q or ESC.
    for {
        k, ok := d.e.PollKey()
        if !ok {
            break
        }
        if k.Code == engine.KeyEsc ||
            (k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
            return engine.ErrQuit
        }
    }
    return nil
}

func (d *demo) Draw(c *engine.Canvas) {
    c.Clear(engine.Black)
    cx := c.Width() / 2
    cy := c.Height() / 2
    x := cx + int(20*math.Sin(d.t))
    y := cy + int(10*math.Cos(d.t*1.3))
    c.FillCircle(x, y, 4, engine.Yellow)
    c.Print(2, 0, "press q to quit", engine.White)
}

func main() {
    e, err := engine.New(engine.Options{})
    if err != nil {
        log.Fatal(err)
    }
    d := &demo{e: e}
    if err := e.Run(d); err != nil {
        log.Fatal(err)
    }
}
```

What happens when you call `e.Run`:

1. The terminal switches to the alternate screen buffer and hides the
   cursor. `/dev/tty` is opened in raw mode for keyboard input.
2. The engine ticks at the target frame rate. Each tick calls
   `scene.Update(dt)`, then `scene.Draw(canvas)`, then writes the diff
   between the new canvas and the previous frame to the terminal.
3. The loop exits when `Update` returns `engine.ErrQuit`, `Engine.Stop()`
   is called, or the process receives SIGINT/SIGTERM.
4. On return, the alt screen, cursor, and termios state are restored.

Defaults — passing `Options{}` — auto-size the canvas to the current
terminal (`cols × 2*rows` pixels) and target 60 FPS. Override `Width`,
`Height`, `TargetFPS`, or `Output` (e.g. `bytes.Buffer` for tests) when
needed.

To register a game in this repo, see
[`internal/game/enginedemo`](../../game/enginedemo) — it implements the
`registry.Game` interface and blank-imports itself via
`cmd/terminal-games/main.go`.

## Modules

| Doc | Topic |
|---|---|
| [engine.md](engine.md) | Engine struct, `Run` loop, `Options`, `Scene` interface, lifecycle, quit semantics, terminal sizing |
| [canvas.md](canvas.md) | Canvas mental model, pixel vs cell coordinates, shape primitives, text overlay (`Print` and `DrawText`) |
| [color.md](color.md) | `Color` struct, `RGB`/`RGBA` constructors, predefined palette, transparency rules |
| [input.md](input.md) | Keyboard events, `Key` and `KeyCode`, `PollKey`, escape-sequence handling, Ctrl-C behaviour |
| [rendering.md](rendering.md) | How half-block rendering, truecolor ANSI, and the diffing front buffer fit together — read this if you need to understand the output format |
