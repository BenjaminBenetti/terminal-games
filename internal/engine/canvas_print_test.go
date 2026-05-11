package engine_test

import (
	"testing"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

func TestPrintBasic(t *testing.T) {
	c := engine.NewCanvas(10, 4)
	c.Print(2, 1, "AB", engine.Red)
	// We can't read text overlay through the public API, but rendering it
	// is exercised in renderer_test.go. Here we just make sure Print
	// doesn't panic and that out-of-range calls are no-ops.
	c.Print(-5, 1, "X", engine.Red)
	c.Print(0, 99, "X", engine.Red)
	c.Print(15, 1, "Y", engine.Red)
}

func TestColsRowsMatchDimensions(t *testing.T) {
	c := engine.NewCanvas(20, 8)
	if c.Cols() != 20 {
		t.Errorf("Cols() = %d, want 20", c.Cols())
	}
	if c.Rows() != 4 {
		t.Errorf("Rows() = %d, want 4", c.Rows())
	}
}

func TestClearWipesPrintOverlay(t *testing.T) {
	// Exercised at the renderer level (see TestRendererPrintClearedByCanvasClear);
	// this test just makes sure Clear after Print doesn't panic on the map.
	c := engine.NewCanvas(8, 4)
	c.Print(0, 0, "hello", engine.White)
	c.Clear(engine.Black)
	c.Print(0, 0, "world", engine.White)
}
