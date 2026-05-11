package engine

import (
	"bytes"
	"errors"
	"os"
	"strconv"
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

// Kitty keyboard protocol event types.
const (
	kittyEventPress   = 1
	kittyEventRepeat  = 2
	kittyEventRelease = 3
)

// Kitty keyboard protocol functional key codepoints (subset we care about).
// Reference: https://sw.kovidgoyal.net/kitty/keyboard-protocol/#functional-key-definitions
const (
	kittyKeyEsc       = 27
	kittyKeyEnter     = 13
	kittyKeyTab       = 9
	kittyKeyBackspace = 127
	kittyKeyLeft      = 57350
	kittyKeyRight     = 57351
	kittyKeyUp        = 57352
	kittyKeyDown      = 57353
)

// Sequences to enable and disable the Kitty keyboard progressive
// enhancement protocol. Flags 2 (report event types) + 8 (report all
// keys as escape codes) = 10. Terminals that don't understand the
// sequence silently ignore it, so this is safe to send unconditionally.
const (
	kittyEnableSequence  = "\x1b[>10u"
	kittyDisableSequence = "\x1b[<u"
)

// PollKey returns the next pending key press, or (Key{}, false) if none.
// It never blocks. Safe to call from any goroutine.
//
// Only press and auto-repeat events are queued — releases update the
// internal pressed-state map (used by IsKeyDown / IsCharDown) without
// being reported as discrete key events.
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

// startInput opens /dev/tty, sets non-canonical mode, and spawns a goroutine
// that decodes key events. Each parsed event is dispatched through a local
// emit closure that updates the Engine's pressed-state map and (for press /
// repeat events) queues the Key on e.inputCh for PollKey.
//
// The returned cleanup function stops the reader, restores the original
// terminal state, and closes the tty handle.
//
// Reading via /dev/tty (rather than os.Stdin) sidesteps the Go runtime's
// poller, which can hold stdin in non-blocking mode and silently break the
// VMIN/VTIME contract we rely on.
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
	_ = syscall.SetNonblock(fd, false)
	orig, err := makeRaw(uintptr(fd))
	if err != nil {
		tty.Close()
		return func() {}
	}

	emit := func(k Key, eventType int) {
		e.recordKey(k, eventType)
		if eventType == kittyEventPress || eventType == kittyEventRepeat {
			select {
			case e.inputCh <- k:
			default:
			}
		}
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
				pending = parseInput(data, emit)
			} else if len(pending) > 0 {
				flushPending(pending, emit)
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

// parseInput parses a chunk of raw terminal bytes into key events,
// dispatched via emit. The event-type argument is one of kittyEventPress,
// kittyEventRepeat, or kittyEventRelease.
//
// Trailing bytes that look like an incomplete escape sequence are returned
// as pending so the caller can prepend them to the next read.
func parseInput(data []byte, emit func(Key, int)) []byte {
	i := 0
	for i < len(data) {
		b := data[i]
		switch {
		case b == 0x1b:
			if i+1 >= len(data) {
				return data[i:]
			}
			if data[i+1] == '[' || data[i+1] == 'O' {
				// Scan for the CSI/SS3 final byte.
				j := i + 2
				for j < len(data) && !isCSIFinal(data[j]) {
					j++
				}
				if j >= len(data) {
					return data[i:]
				}
				final := data[j]
				params := data[i+2 : j]

				switch {
				case final == 'u' && data[i+1] == '[':
					parseKittyU(params, emit)
				case data[i+1] == 'O' || (data[i+1] == '[' && len(params) == 0):
					handleLegacyArrow(final, emit)
				}
				// Anything else: unknown CSI sequence, drop it.

				i = j + 1
				continue
			}
			emit(Key{Code: KeyEsc}, kittyEventPress)
			i++
		case b == '\r', b == '\n':
			emit(Key{Code: KeyEnter}, kittyEventPress)
			i++
		case b == '\t':
			emit(Key{Code: KeyTab}, kittyEventPress)
			i++
		case b == 0x7f, b == '\b':
			emit(Key{Code: KeyBackspace}, kittyEventPress)
			i++
		case b < 0x20:
			i++
		default:
			r, size := utf8.DecodeRune(data[i:])
			if size == 0 || r == utf8.RuneError {
				i++
				continue
			}
			emit(Key{Code: KeyChar, Rune: r}, kittyEventPress)
			i += size
		}
	}
	return nil
}

// handleLegacyArrow dispatches a press event for the legacy CSI/SS3 arrow
// final bytes A/B/C/D.
func handleLegacyArrow(final byte, emit func(Key, int)) {
	switch final {
	case 'A':
		emit(Key{Code: KeyUp}, kittyEventPress)
	case 'B':
		emit(Key{Code: KeyDown}, kittyEventPress)
	case 'C':
		emit(Key{Code: KeyRight}, kittyEventPress)
	case 'D':
		emit(Key{Code: KeyLeft}, kittyEventPress)
	}
}

// parseKittyU parses the parameters of a Kitty CSI u sequence and emits the
// resulting key event. The parameter format is
//
//	codepoint[:alt][;modifiers[:event-type]][;text]
//
// Only the codepoint, modifier-shift bit, and event type are interpreted.
// Codepoints in the Kitty functional-key range map to KeyUp/Down/etc.;
// ordinary Unicode codepoints become KeyChar.
func parseKittyU(params []byte, emit func(Key, int)) {
	cp, mods, event, ok := parseKittyParams(params)
	if !ok {
		return
	}

	var key Key
	switch cp {
	case kittyKeyEsc:
		key = Key{Code: KeyEsc}
	case kittyKeyEnter:
		key = Key{Code: KeyEnter}
	case kittyKeyTab:
		key = Key{Code: KeyTab}
	case kittyKeyBackspace:
		key = Key{Code: KeyBackspace}
	case kittyKeyUp:
		key = Key{Code: KeyUp}
	case kittyKeyDown:
		key = Key{Code: KeyDown}
	case kittyKeyLeft:
		key = Key{Code: KeyLeft}
	case kittyKeyRight:
		key = Key{Code: KeyRight}
	default:
		if cp < 0x20 || cp > 0x10FFFF {
			return
		}
		key = Key{Code: KeyChar, Rune: kittyCanonicalRune(cp, mods)}
	}
	emit(key, event)
}

// parseKittyParams pulls codepoint, modifiers, and event-type out of a
// Kitty CSI u parameter string. mods defaults to 1 (no modifiers) and
// event defaults to 1 (press) when their respective fields are missing.
func parseKittyParams(params []byte) (cp, mods, event int, ok bool) {
	mods = 1
	event = kittyEventPress

	rest := params
	semi := bytes.IndexByte(rest, ';')
	cpSection := rest
	if semi >= 0 {
		cpSection = rest[:semi]
		rest = rest[semi+1:]
	} else {
		rest = nil
	}
	// The codepoint section may include ":alt-codepoints" — only the part
	// before the first colon is the primary codepoint.
	if colon := bytes.IndexByte(cpSection, ':'); colon >= 0 {
		cpSection = cpSection[:colon]
	}
	if len(cpSection) == 0 {
		return 0, 0, 0, false
	}
	v, err := strconv.Atoi(string(cpSection))
	if err != nil {
		return 0, 0, 0, false
	}
	cp = v

	if rest == nil {
		return cp, mods, event, true
	}

	// Drop any trailing ;text section.
	if semi := bytes.IndexByte(rest, ';'); semi >= 0 {
		rest = rest[:semi]
	}
	modsPart := rest
	var eventPart []byte
	if colon := bytes.IndexByte(rest, ':'); colon >= 0 {
		modsPart = rest[:colon]
		eventPart = rest[colon+1:]
	}
	if len(modsPart) > 0 {
		if m, err := strconv.Atoi(string(modsPart)); err == nil {
			mods = m
		}
	}
	if len(eventPart) > 0 {
		if e, err := strconv.Atoi(string(eventPart)); err == nil {
			event = e
		}
	}
	return cp, mods, event, true
}

// kittyCanonicalRune produces the visible character for a Kitty-reported
// keypress. Kitty reports the *unshifted* codepoint plus a modifier mask,
// so we apply the shift bit ourselves for ASCII letters to keep "A"
// distinct from "a" in PollKey output. Other layout-dependent
// transformations (Shift+1 → "!", AltGr layers, etc.) require flag 16
// (text reporting) which we don't enable.
func kittyCanonicalRune(cp int, mods int) rune {
	flags := mods - 1
	if flags < 0 {
		flags = 0
	}
	shift := flags&1 != 0
	if shift && cp >= 'a' && cp <= 'z' {
		return rune(cp - 32)
	}
	return rune(cp)
}

// flushPending emits a bare KeyEsc for any pending bytes (which always
// start with ESC since that's the only sequence parseInput defers on).
// Trailing bytes after the ESC are discarded — they were part of an
// unrecognised or aborted escape sequence.
func flushPending(data []byte, emit func(Key, int)) {
	if len(data) > 0 && data[0] == 0x1b {
		emit(Key{Code: KeyEsc}, kittyEventPress)
	}
}

func isCSIFinal(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~'
}
