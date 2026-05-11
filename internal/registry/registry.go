// Package registry provides a simple mechanism for registering and looking up games.
// Adding a new game requires only a single call to Register, typically placed in an
// init() function inside the game package so that a blank import is sufficient.
package registry

import (
	"fmt"
	"sort"
)

// Game is the interface every game must implement.
type Game interface {
	// Name returns the unique identifier used on the command line (e.g. "snake").
	Name() string
	// Description returns a short human-readable description shown in the list.
	Description() string
	// Run starts the game and blocks until it exits.
	Run() error
}

var games = map[string]Game{}

// Register adds g to the global game registry.
// It panics if a game with the same name has already been registered.
func Register(g Game) {
	if _, exists := games[g.Name()]; exists {
		panic(fmt.Sprintf("registry: game %q is already registered", g.Name()))
	}
	games[g.Name()] = g
}

// List returns all registered games sorted by name.
func List() []Game {
	result := make([]Game, 0, len(games))
	for _, g := range games {
		result = append(result, g)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result
}

// Get returns the game registered under name and whether it was found.
func Get(name string) (Game, bool) {
	g, ok := games[name]
	return g, ok
}
