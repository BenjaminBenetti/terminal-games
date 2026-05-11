package engine_test

import (
	"testing"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

func TestRGB(t *testing.T) {
	c := engine.RGB(10, 20, 30)
	want := engine.Color{R: 10, G: 20, B: 30, A: 255}
	if c != want {
		t.Errorf("RGB(10,20,30) = %+v, want %+v", c, want)
	}
}

func TestRGBA(t *testing.T) {
	c := engine.RGBA(10, 20, 30, 128)
	want := engine.Color{R: 10, G: 20, B: 30, A: 128}
	if c != want {
		t.Errorf("RGBA(10,20,30,128) = %+v, want %+v", c, want)
	}
}

func TestPredefinedColors(t *testing.T) {
	cases := []struct {
		name string
		got  engine.Color
		want engine.Color
	}{
		{"Black", engine.Black, engine.Color{R: 0, G: 0, B: 0, A: 255}},
		{"White", engine.White, engine.Color{R: 255, G: 255, B: 255, A: 255}},
		{"Red", engine.Red, engine.Color{R: 255, G: 0, B: 0, A: 255}},
		{"Green", engine.Green, engine.Color{R: 0, G: 255, B: 0, A: 255}},
		{"Blue", engine.Blue, engine.Color{R: 0, G: 0, B: 255, A: 255}},
		{"Yellow", engine.Yellow, engine.Color{R: 255, G: 255, B: 0, A: 255}},
		{"Cyan", engine.Cyan, engine.Color{R: 0, G: 255, B: 255, A: 255}},
		{"Magenta", engine.Magenta, engine.Color{R: 255, G: 0, B: 255, A: 255}},
		{"Gray", engine.Gray, engine.Color{R: 128, G: 128, B: 128, A: 255}},
		{"Transparent", engine.Transparent, engine.Color{}},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %+v, want %+v", tc.name, tc.got, tc.want)
		}
	}
}
