package engine

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestRendererInitialFullDraw(t *testing.T) {
	c := NewCanvas(2, 2)
	c.Set(0, 0, Red)
	c.Set(1, 0, Green)
	c.Set(0, 1, Blue)
	c.Set(1, 1, White)

	var buf bytes.Buffer
	var r renderer
	if err := r.render(c, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"\x1b[1;1H",                   // cursor home for first cell
		"\x1b[38;2;255;0;0m",          // Red fg
		"\x1b[48;2;0;0;255m",          // Blue bg
		"\x1b[38;2;0;255;0m",          // Green fg
		"\x1b[48;2;255;255;255m",      // White bg
		upperHalfBlock,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output: %q", want, out)
		}
	}
	if !strings.HasSuffix(out, "\x1b[0m") {
		t.Errorf("expected trailing reset, got %q", out)
	}
}

func TestRendererNoChangeProducesNoOutput(t *testing.T) {
	c := NewCanvas(2, 2)
	c.Clear(Red)

	var buf bytes.Buffer
	var r renderer
	if err := r.render(c, &buf); err != nil {
		t.Fatalf("first render: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("first render produced no output")
	}

	buf.Reset()
	if err := r.render(c, &buf); err != nil {
		t.Fatalf("second render: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("steady-state render produced %d bytes: %q", buf.Len(), buf.String())
	}
}

func TestRendererPartialDiff(t *testing.T) {
	c := NewCanvas(4, 2)
	c.Clear(Black)

	var buf bytes.Buffer
	var r renderer
	if err := r.render(c, &buf); err != nil {
		t.Fatalf("first render: %v", err)
	}
	buf.Reset()

	// Only change one cell on the only cell-row.
	c.Set(2, 0, Red)
	c.Set(2, 1, Green)
	if err := r.render(c, &buf); err != nil {
		t.Fatalf("second render: %v", err)
	}
	out := buf.String()

	// Cursor should jump to col 3 (1-indexed) on row 1.
	wantPos := fmt.Sprintf("\x1b[%d;%dH", 1, 3)
	if !strings.Contains(out, wantPos) {
		t.Errorf("expected cursor move %q, got %q", wantPos, out)
	}
	// Exactly one half-block should be emitted for one changed cell.
	if got := strings.Count(out, upperHalfBlock); got != 1 {
		t.Errorf("expected exactly 1 half-block emitted, got %d in %q", got, out)
	}
}

func TestRendererTransparentRendersAsBlack(t *testing.T) {
	c := NewCanvas(1, 2) // transparent by default
	var buf bytes.Buffer
	var r renderer
	if err := r.render(c, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	// Both fg and bg should be black.
	black := "\x1b[38;2;0;0;0m"
	bgBlack := "\x1b[48;2;0;0;0m"
	if !strings.Contains(out, black) {
		t.Errorf("expected black fg %q in %q", black, out)
	}
	if !strings.Contains(out, bgBlack) {
		t.Errorf("expected black bg %q in %q", bgBlack, out)
	}
}

func TestRendererPrintEmitsRune(t *testing.T) {
	c := NewCanvas(4, 2)
	c.Clear(Black)
	c.Print(1, 0, "X", Yellow)

	var buf bytes.Buffer
	var r renderer
	if err := r.render(c, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "X") {
		t.Errorf("missing rune X in %q", out)
	}
	// Yellow fg, black bg from averageColor of two black pixels.
	if !strings.Contains(out, "\x1b[38;2;255;255;0m") {
		t.Errorf("missing yellow fg in %q", out)
	}
	// The X should be emitted while yellow is the active fg.
	yellowIdx := strings.Index(out, "\x1b[38;2;255;255;0m")
	xIdx := strings.Index(out, "X")
	if yellowIdx < 0 || xIdx < 0 || yellowIdx > xIdx {
		t.Errorf("expected yellow fg before X in %q", out)
	}
}

func TestRendererPrintDiffOnlyChangesTextCell(t *testing.T) {
	c := NewCanvas(4, 2)
	c.Clear(Black)
	var buf bytes.Buffer
	var r renderer
	_ = r.render(c, &buf)
	buf.Reset()

	c.Print(2, 0, "Q", Cyan)
	if err := r.render(c, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	// Only one cell changed → exactly one rune emitted, no half-block.
	if strings.Count(out, "Q") != 1 {
		t.Errorf("expected exactly 1 Q, got %q", out)
	}
	if strings.Contains(out, upperHalfBlock) {
		t.Errorf("expected no half-block in diff, got %q", out)
	}
}

func TestRendererPrintBgFromCanvas(t *testing.T) {
	c := NewCanvas(2, 2)
	c.FillRect(0, 0, 2, 2, Blue) // fill both pixel rows
	c.Print(0, 0, "A", White)

	var buf bytes.Buffer
	var r renderer
	if err := r.render(c, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	// Text cell should pick up blue (average of blue+blue = blue) as bg.
	if !strings.Contains(out, "\x1b[48;2;0;0;255m") {
		t.Errorf("expected blue bg from canvas, got %q", out)
	}
}

func TestRendererPrintClearedByCanvasClear(t *testing.T) {
	c := NewCanvas(2, 2)
	c.Clear(Black)
	c.Print(0, 0, "Z", Red)

	var buf bytes.Buffer
	var r renderer
	_ = r.render(c, &buf)
	if !strings.Contains(buf.String(), "Z") {
		t.Fatalf("first render missing Z: %q", buf.String())
	}

	// Clear should remove the text overlay; next render falls back to the
	// half-block character.
	c.Clear(Black)
	buf.Reset()
	if err := r.render(c, &buf); err != nil {
		t.Fatalf("render after clear: %v", err)
	}
	if strings.Contains(buf.String(), "Z") {
		t.Errorf("Z still present after Clear: %q", buf.String())
	}
}

func TestRendererResizeForcesFullRedraw(t *testing.T) {
	c := NewCanvas(2, 2)
	c.Clear(Red)
	var buf bytes.Buffer
	var r renderer
	_ = r.render(c, &buf)

	// Resize to a new canvas with the same colour and ensure the next render
	// emits cells again rather than skipping them.
	c2 := NewCanvas(4, 4)
	c2.Clear(Red)
	buf.Reset()
	if err := r.render(c2, &buf); err != nil {
		t.Fatalf("render after resize: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected resize to force a full redraw, got no output")
	}
}
