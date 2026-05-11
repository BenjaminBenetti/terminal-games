# Engine loop

The `Engine` owns the frame loop, the canvas, the input reader, and the
terminal state machine.

## Construction

```go
e, err := engine.New(engine.Options{
    Width:     0,           // 0 = auto-size to terminal
    Height:    0,           // 0 = auto-size to terminal (cols × 2*rows pixels)
    TargetFPS: 0,           // 0 = engine.DefaultFPS (60)
    Output:    nil,         // nil = os.Stdout
})
```

The zero `Options{}` value is valid. Width/Height are in **pixels** — the
canvas is `cols × rows*2` pixels because each terminal cell holds two
vertically-stacked pixels (see [rendering.md](rendering.md)). If the
terminal size can't be detected, the engine falls back to 80×48.

`Engine` is single-use. Don't call `Run` twice on the same instance.

## The Scene interface

```go
type Scene interface {
    Update(dt time.Duration) error
    Draw(canvas *Canvas)
}
```

- `Update` advances simulation state. `dt` is the wall-clock delta since
  the previous tick. Return `engine.ErrQuit` to stop the loop cleanly.
- `Draw` paints the current state into the canvas. The canvas is **not**
  auto-cleared between frames — call `canvas.Clear()` if you want a fresh
  background each frame. Both the pixel buffer and the text overlay
  persist until you clear them.

Drain keyboard events inside `Update` via `e.PollKey()` — see
[input.md](input.md).

## Run

```go
err := e.Run(scene)
```

`Run` blocks and returns when:

- `Update` returns `engine.ErrQuit` (returns `nil`),
- something calls `e.Stop()` from any goroutine (returns `nil`),
- the process receives SIGINT or SIGTERM (returns `nil`),
- `Update` returns any other error (`Run` propagates it),
- the renderer hits an I/O error (`Run` propagates it).

The first tick fires immediately at `dt=0` so the screen isn't blank
before the first ticker fire.

## Lifecycle (what `Run` does to your terminal)

On entry, in order:

1. `\x1b[?1049h` — switch to alternate screen buffer.
2. `\x1b[?25l` — hide the cursor.
3. `\x1b[?1l` — disable application cursor-key mode (arrows arrive as
   CSI `\x1b[A` style sequences, see [input.md](input.md)).
4. `\x1b[2J\x1b[H` — clear and home cursor.
5. Open `/dev/tty`, set non-canonical mode (raw mode minus `ISIG`, so
   Ctrl-C still SIGINTs), start the input goroutine.
6. Install SIGINT/SIGTERM handler.

On exit, all of the above is undone in reverse: signal handler stopped,
input goroutine joined, termios restored, `/dev/tty` closed, cursor shown,
colours reset, alt screen exited.

This means a panic inside `Update` or `Draw` will skip teardown and leave
the terminal in a bad state. Recover at scene level if you care.

## Stop / quit options

Three ways to exit cleanly:

```go
// 1. From inside Update — recommended for game-logic-driven quit.
func (s *scene) Update(dt time.Duration) error {
    if s.done {
        return engine.ErrQuit
    }
    return nil
}

// 2. From any goroutine — useful for async triggers.
e.Stop()

// 3. SIGINT / SIGTERM (Ctrl-C) — handled by the engine automatically.
```

`Stop` is idempotent and safe to call from multiple goroutines or before
`Run` is even called.

## Headless / piped use

When stdout isn't a real terminal, `TerminalSize` returns an error and
the engine falls back to 80×48. The input reader is a no-op if
`/dev/tty` can't be opened. This makes the engine testable: pass
`Output: &bytes.Buffer{}` and the engine will write escape sequences to
the buffer without touching the real terminal.

## Terminal size

```go
cols, rows, err := engine.TerminalSize()
```

Returns the current terminal size (in cells) for stdout via the
`TIOCGWINSZ` ioctl. Useful if you want to size the canvas yourself
instead of letting `Options.Width/Height = 0` do it.

The engine does not currently react to SIGWINCH — if the user resizes
the window mid-game, the canvas stays its original size.
