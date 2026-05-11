# Input

The engine reads keyboard input from `/dev/tty` in raw mode, decodes
bytes (including ANSI escape sequences) into `Key` events, and queues
them on a buffered channel. Scenes drain the queue via `PollKey`.

## Reading keys

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
| Other CSI (`\x1b[…`) | Dropped silently |

## ESC is sticky

A lone `\x1b` byte is ambiguous — it might be a bare ESC press, or it
might be the start of an arrow-key sequence whose remaining bytes haven't
arrived yet. The parser holds incomplete escape sequences as "pending"
and only emits a `KeyEsc` once the ~100 ms inter-byte timeout fires.
This is why a fast arrow key works correctly but a bare ESC press feels
the tiniest bit laggy — that's the timeout.

Concrete consequence: ESC is reliable as a "back" key but don't try to
build a chord like `ESC + arrow` to mean something different from a
plain arrow — they look identical at this layer.

## Ctrl-C still works

The engine sets non-canonical mode but leaves `ISIG` enabled, so Ctrl-C
still generates SIGINT, which `Run` catches via its signal handler and
exits cleanly. Ctrl-C does **not** arrive as a `KeyChar` event.

## When the input reader is disabled

`startInput` is a no-op if `/dev/tty` can't be opened (tests, pipes,
containers without a controlling terminal). In that case `PollKey`
always returns `(Key{}, false)` and the engine still works — you just
have no input, and the only way to exit is `Engine.Stop()`, returning
`ErrQuit` from `Update`, or SIGTERM.

## Why `/dev/tty` instead of `os.Stdin`

Go's runtime can put `os.Stdin` into non-blocking mode and manage it
through its netpoller, which silently breaks the `VMIN`/`VTIME` termios
contract we depend on. Reading the controlling terminal directly via
`/dev/tty` plus `syscall.Read` bypasses the runtime poller entirely.
