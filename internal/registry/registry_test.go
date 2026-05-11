package registry_test

import (
	"errors"
	"testing"

	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

// stub implements registry.Game for testing purposes.
type stub struct {
	name        string
	description string
	runErr      error
}

func (s stub) Name() string        { return s.name }
func (s stub) Description() string { return s.description }
func (s stub) Run() error          { return s.runErr }

func TestRegisterAndGet(t *testing.T) {
	registry.Reset()
	registry.Register(stub{name: "pong", description: "Classic pong"})

	g, ok := registry.Get("pong")
	if !ok {
		t.Fatal("expected to find game 'pong'")
	}
	if g.Name() != "pong" {
		t.Errorf("got name %q, want %q", g.Name(), "pong")
	}
	if g.Description() != "Classic pong" {
		t.Errorf("got description %q, want %q", g.Description(), "Classic pong")
	}
}

func TestGetMissing(t *testing.T) {
	registry.Reset()

	_, ok := registry.Get("nonexistent")
	if ok {
		t.Fatal("expected game 'nonexistent' to be absent")
	}
}

func TestList(t *testing.T) {
	registry.Reset()
	registry.Register(stub{name: "tetris", description: "Classic tetris"})
	registry.Register(stub{name: "asteroids", description: "Classic asteroids"})
	registry.Register(stub{name: "pong", description: "Classic pong"})

	list := registry.List()

	if len(list) != 3 {
		t.Fatalf("List() returned %d games, want 3", len(list))
	}

	// Verify alphabetical order.
	names := make([]string, len(list))
	for i, g := range list {
		names[i] = g.Name()
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("List() not sorted: %v", names)
			break
		}
	}

	// Verify all expected games are present.
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	for _, want := range []string{"pong", "tetris", "asteroids"} {
		if !found[want] {
			t.Errorf("List() missing game %q", want)
		}
	}
}

func TestListEmpty(t *testing.T) {
	registry.Reset()

	if list := registry.List(); len(list) != 0 {
		t.Errorf("List() = %v, want empty", list)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	registry.Reset()
	registry.Register(stub{name: "snake", description: "Classic snake"})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when registering duplicate game name")
		}
	}()
	registry.Register(stub{name: "snake", description: "Duplicate"})
}

func TestRunError(t *testing.T) {
	registry.Reset()
	want := errors.New("boom")
	registry.Register(stub{name: "crash", runErr: want})

	g, ok := registry.Get("crash")
	if !ok {
		t.Fatal("expected to find game 'crash'")
	}
	if err := g.Run(); !errors.Is(err, want) {
		t.Errorf("Run() = %v, want %v", err, want)
	}
}
