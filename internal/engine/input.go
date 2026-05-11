package engine

import (
	"errors"
	"os"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

// KeyCode classifies a Key event.
type KeyCode int

const (
	KeyUnknown KeyCode = iota
	// KeyChar is a printable character. The decoded rune is in Key.Rune.
	KeyChar
	KeyEsc
	KeyEnter
	KeyTab
	KeyBackspace
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
)

// Key is a single keyboard event read from the terminal.
type Key struct {
	Code KeyCode
	Rune rune
}

// PollKey returns the next pending key, or (Key{}, false) if none. It never
// blocks. Safe to call from any goroutine.
func (e *Engine) PollKey() (Key, bool) {
	select {
	case k := <-e.inputCh:
		return k, true
	default:
		return Key{}, false
	}
}

// isTerminal reports whether fd refers to a TTY.
func isTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL, fd, syscall.TCGETS,
		uintptr(unsafe.Pointer(&t)), 0, 0, 0,
	)
	return errno == 0
}

// makeRaw switches fd into non-canonical, non-echo mode with VMIN=0 and
// VTIME=1 so reads return after a ~100ms timeout when no data is available.
// ISIG is left enabled so Ctrl-C still produces SIGINT for the engine to
// handle via its signal channel.
func makeRaw(fd uintptr) (orig syscall.Termios, err error) {
	if _, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL, fd, syscall.TCGETS,
		uintptr(unsafe.Pointer(&orig)), 0, 0, 0,
	); errno != 0 {
		return orig, errno
	}
	raw := orig
	raw.Lflag &^= syscall.ECHO | syscall.ICANON
	raw.Cc[syscall.VMIN] = 0
	raw.Cc[syscall.VTIME] = 1
	if _, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL, fd, syscall.TCSETS,
		uintptr(unsafe.Pointer(&raw)), 0, 0, 0,
	); errno != 0 {
		return orig, errno
	}
	return orig, nil
}

// restoreMode sets fd's termios back to orig.
func restoreMode(fd uintptr, orig syscall.Termios) {
	_, _, _ = syscall.Syscall6(
		syscall.SYS_IOCTL, fd, syscall.TCSETS,
		uintptr(unsafe.Pointer(&orig)), 0, 0, 0,
	)
}

// startInput opens the controlling terminal (/dev/tty), puts it in
// non-canonical mode, and spawns a goroutine that decodes key events into
// e.inputCh. The returned cleanup function stops the reader, restores the
// original terminal state, and closes the tty handle.
//
// Reading via /dev/tty (rather than os.Stdin) sidesteps the Go runtime's
// poller, which can hold stdin in non-blocking mode and silently break the
// VMIN/VTIME contract we rely on.
//
// If /dev/tty cannot be opened (e.g. stdin/stdout aren't a real terminal),
// startInput is a no-op and cleanup is a harmless noop.
func (e *Engine) startInput() func() {
	tty, err := os.OpenFile("/dev/tty", os.O_RDONLY|syscall.O_NOCTTY, 0)
	if err != nil {
		return func() {}
	}
	fd := int(tty.Fd())
	if !isTerminal(uintptr(fd)) {
		tty.Close()
		return func() {}
	}
	// Belt-and-suspenders: make sure the descriptor is blocking so VMIN/VTIME
	// take effect.
	_ = syscall.SetNonblock(fd, false)
	orig, err := makeRaw(uintptr(fd))
	if err != nil {
		tty.Close()
		return func() {}
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 64)
		var pending []byte
		for {
			select {
			case <-stop:
				return
			default:
			}
			n, err := syscall.Read(fd, buf)
			if err != nil {
				if errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.EAGAIN) {
					continue
				}
				return
			}
			if n > 0 {
				data := append(pending, buf[:n]...)
				pending = parseInput(data, e.inputCh)
			} else if len(pending) > 0 {
				flushPending(pending, e.inputCh)
				pending = nil
			}
		}
	}()
	return func() {
		close(stop)
		<-done
		restoreMode(uintptr(fd), orig)
		tty.Close()
	}
}

// parseInput decodes a chunk of raw terminal bytes into Key events.
//
// If data ends mid-escape-sequence (e.g. just "\x1b" or "\x1b["), the
// trailing bytes are returned as pending so the caller can prepend them
// to the next read. Otherwise the return value is nil.
func parseInput(data []byte, out chan<- Key) []byte {
	i := 0
	for i < len(data) {
		b := data[i]
		switch {
		case b == 0x1b:
			// Could be CSI ("ESC ["), SS3 ("ESC O"), or a bare ESC.
			if i+1 >= len(data) {
				return data[i:]
			}
			if data[i+1] == '[' || data[i+1] == 'O' {
				if i+2 >= len(data) {
					return data[i:]
				}
				switch data[i+2] {
				case 'A':
					sendKey(out, Key{Code: KeyUp})
					i += 3
					continue
				case 'B':
					sendKey(out, Key{Code: KeyDown})
					i += 3
					continue
				case 'C':
					sendKey(out, Key{Code: KeyRight})
					i += 3
					continue
				case 'D':
					sendKey(out, Key{Code: KeyLeft})
					i += 3
					continue
				}
				if data[i+1] == '[' {
					// Unknown CSI: skip until a final byte.
					j := i + 2
					for j < len(data) && !isCSIFinal(data[j]) {
						j++
					}
					if j >= len(data) {
						return data[i:]
					}
					i = j + 1
					continue
				}
				// Unknown SS3 sequence.
				i += 3
				continue
			}
			// ESC followed by something that isn't [ or O: emit a bare ESC
			// and let the next byte be parsed on its own.
			sendKey(out, Key{Code: KeyEsc})
			i++
		case b == '\r', b == '\n':
			sendKey(out, Key{Code: KeyEnter})
			i++
		case b == '\t':
			sendKey(out, Key{Code: KeyTab})
			i++
		case b == 0x7f, b == '\b':
			sendKey(out, Key{Code: KeyBackspace})
			i++
		case b < 0x20:
			// Other control bytes (handled separately, e.g. Ctrl-C via SIGINT).
			i++
		default:
			r, size := utf8.DecodeRune(data[i:])
			if size == 0 || r == utf8.RuneError {
				i++
				continue
			}
			sendKey(out, Key{Code: KeyChar, Rune: r})
			i += size
		}
	}
	return nil
}

// flushPending emits a bare KeyEsc for any pending bytes (which always
// start with ESC since that's the only sequence parseInput defers on).
// Trailing bytes after the ESC are discarded — they were part of an
// unrecognised or aborted escape sequence.
func flushPending(data []byte, out chan<- Key) {
	if len(data) > 0 && data[0] == 0x1b {
		sendKey(out, Key{Code: KeyEsc})
	}
}

func sendKey(out chan<- Key, k Key) {
	select {
	case out <- k:
	default:
	}
}

func isCSIFinal(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~'
}
