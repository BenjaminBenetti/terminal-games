package engine

import (
	"os"
	"syscall"
	"unsafe"
)

// TerminalSize returns the current terminal size attached to stdout in
// columns and rows.
func TerminalSize() (cols, rows int, err error) {
	var ws struct {
		Row, Col, Xpix, Ypix uint16
	}
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		os.Stdout.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return 0, 0, errno
	}
	return int(ws.Col), int(ws.Row), nil
}
