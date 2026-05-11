package enginedemo

import (
	"testing"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

func TestMenuArrowNavigation(t *testing.T) {
	m := &menuScene{items: []string{"A", "B", "C"}}
	m.handleKey(engine.Key{Code: engine.KeyDown})
	if m.selected != 1 {
		t.Errorf("after KeyDown selected=%d, want 1", m.selected)
	}
	m.handleKey(engine.Key{Code: engine.KeyDown})
	m.handleKey(engine.Key{Code: engine.KeyDown}) // already at last
	if m.selected != 2 {
		t.Errorf("clamped down selected=%d, want 2", m.selected)
	}
	m.handleKey(engine.Key{Code: engine.KeyUp})
	if m.selected != 1 {
		t.Errorf("after KeyUp selected=%d, want 1", m.selected)
	}
	m.handleKey(engine.Key{Code: engine.KeyUp})
	m.handleKey(engine.Key{Code: engine.KeyUp}) // already at first
	if m.selected != 0 {
		t.Errorf("clamped up selected=%d, want 0", m.selected)
	}
}

func TestMenuEnterLaunches(t *testing.T) {
	m := &menuScene{items: []string{"A", "B"}, selected: 1}
	got := m.handleKey(engine.Key{Code: engine.KeyEnter})
	if got != menuLaunch {
		t.Errorf("KeyEnter returned %v, want menuLaunch", got)
	}
}

func TestMenuDigitShortcut(t *testing.T) {
	m := &menuScene{items: []string{"A", "B", "C"}}
	got := m.handleKey(engine.Key{Code: engine.KeyChar, Rune: '2'})
	if got != menuLaunch {
		t.Errorf("digit returned %v, want menuLaunch", got)
	}
	if m.selected != 1 {
		t.Errorf("after '2' selected=%d, want 1", m.selected)
	}
}

func TestMenuDigitOutOfRangeIgnored(t *testing.T) {
	m := &menuScene{items: []string{"A", "B"}, selected: 0}
	got := m.handleKey(engine.Key{Code: engine.KeyChar, Rune: '5'})
	if got != menuNone {
		t.Errorf("out-of-range digit returned %v, want menuNone", got)
	}
	if m.selected != 0 {
		t.Errorf("selected changed to %d, want 0", m.selected)
	}
}
