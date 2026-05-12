# Input

The engine reads keyboard input from `/dev/tty` in raw mode, decodes
bytes (including ANSI escape sequences) into `Key` events, and queues
them on a buffered channel. Scenes drain the queue via `PollKey` for
discrete events, or call `IsKeyDown`/`IsCharDown` for held-key state
(useful for diagonal movement and other multi-key situations).

## Discrete events: `PollKey`

```go
for {
    k, ok := e.PollKey()
    if !ok {
        break
    }
    switch k.Code {
    case engine.KeyUp:    /* … */
    case engine.KeyEsc:   return engine.ErrQuit
    case engine.KeyChar:
        if k.Rune == 'q' { return engine.ErrQuit }
    }
}
```

`PollKey` is non-blocking and safe to call from any goroutine. Inside
`Update`, drain it in a loop — multiple keys can arrive per frame and
unprocessed ones get dropped if the 64-event buffer fills.

Only **press** and **auto-repeat** events flow through `PollKey`.
Release events update internal state for `IsKeyDown`/`IsCharDown` but
are never queued — most game logic doesn't want to react to a release
as a discrete event.

## Held-key state: `IsKeyDown` / `IsCharDown`

```go
func (s *playerScene) Update(dt time.Duration) error {
    speed := 50.0 * dt.Seconds()
    if s.e.IsKeyDown(engine.KeyUp)    { s.y -= speed }
    if s.e.IsKeyDown(engine.KeyDown)  { s.y += speed }
    if s.e.IsKeyDown(engine.KeyLeft)  { s.x -= speed }
    if s.e.IsKeyDown(engine.KeyRight) { s.x += speed }
    if s.e.IsCharDown(' ')            { s.fire() }
    return nil
}
```

This is how to handle "multiple keys held at once" — two `IsKeyDown`
calls can both return `true` in the same frame, so diagonal movement
and chord inputs work naturally.

`IsKeyDown` takes a `KeyCode` (must NOT be `KeyChar` — it always
returns false for that). `IsCharDown` takes a `rune` for printable
characters.

### How accurate is it?

That depends on the terminal:

- **Kitty keyboard protocol** terminals report explicit press / repeat /
  release events. `IsKeyDown` becomes a direct lookup of true key state
  and is exact. Confirmed working on Kitty, WezTerm, foot, recent xterm,
  and modern VTE-based terminals (Ptyxis, GNOME Terminal — libvte 0.78+,
  shipping in Fedora 40+ and similar).
- **Legacy terminals** only send press events; releases never arrive.
  The engine falls back to an auto-repeat heuristic: a key is "down"
  while press or repeat events keep arriving, decaying to "up" after
  ~250 ms of silence (`keyHoldDecay`). The kernel's initial auto-repeat
  delay is typically 250–500 ms, so holding a key briefly may register
  as "up" for a moment before auto-repeat kicks in, and a released key
  may register as "down" for up to ~250 ms after release — short enough
  to feel responsive but long enough that auto-repeating keys don't
  flicker.

The engine asks for the Kitty protocol on entry by sending
`\x1b[>10u` (flags 2 + 8: report event types + report all keys as
escape codes). Terminals that don't recognise the sequence silently
ignore it, so this is safe to send unconditionally. On exit, the
engine sends `\x1b[<u` to pop the keyboard-mode stack.

## Key codes

```go
type KeyCode int

const (
    KeyUnknown KeyCode = iota
    KeyChar      // printable rune in Key.Rune
    KeyEsc
    KeyEnter
    KeyTab
    KeyBackspace
    KeyUp
    KeyDown
    KeyLeft
    KeyRight
)
```

`KeyChar` carries the actual rune in `Key.Rune`. Decoded sequences:

| Input bytes | Result |
|---|---|
| Printable byte / valid UTF-8 | `KeyChar` with rune |
| `\r` or `\n` | `KeyEnter` |
| `\t` | `KeyTab` |
| `\x7f` or `\b` | `KeyBackspace` |
| `\x1b` followed by no more bytes within ~100 ms | `KeyEsc` |
| `\x1b[A`, `B`, `C`, `D` | `KeyUp`/`Down`/`Right`/`Left` (CSI form) |
| `\x1bOA`, `B`, `C`, `D` | Same (SS3 form, application keypad mode) |
| `\x1b[<codepoint>[;mods[:event]]u` | Kitty CSI u — arrows from codepoints 57350–57353, special keys by codepoint, printable chars from their Unicode codepoint |
| Other CSI (`\x1b[…`) | Dropped silently |

## ESC is sticky

A lone `\x1b` byte is ambiguous — it might be a bare ESC press, or it
might be the start of an arrow-key sequence whose remaining bytes haven't
arrived yet. The parser holds incomplete escape sequences as "pending"
and only emits a `KeyEsc` once the ~100 ms inter-byte timeout fires.
This is why a fast arrow key works correctly but a bare ESC press feels
the tiniest bit laggy — that's the timeout.

On terminals with Kitty mode active, ESC arrives as `\x1b[27u` (no
ambiguity), so the timeout doesn't affect it there.

## Ctrl-C still works

The engine sets non-canonical mode but leaves `ISIG` enabled, so Ctrl-C
still generates SIGINT, which `Run` catches via its signal handler and
exits cleanly. Ctrl-C does **not** arrive as a `KeyChar` event.

## When the input reader is disabled

`startInput` is a no-op if `/dev/tty` can't be opened (tests, pipes,
containers without a controlling terminal). In that case `PollKey`
always returns `(Key{}, false)`, `IsKeyDown`/`IsCharDown` always return
`false`, and the engine still works — you just have no input, and the
only way to exit is `Engine.Stop()`, returning `ErrQuit` from `Update`,
or SIGTERM.

## Why `/dev/tty` instead of `os.Stdin`

Go's runtime can put `os.Stdin` into non-blocking mode and manage it
through its netpoller, which silently breaks the `VMIN`/`VTIME` termios
contract we depend on. Reading the controlling terminal directly via
`/dev/tty` plus `syscall.Read` bypasses the runtime poller entirely.
