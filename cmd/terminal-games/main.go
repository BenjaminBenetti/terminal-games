// terminal-games is the command-line interface for the terminal-games collection.
//
// Usage:
//
//	terminal-games               – open the interactive game picker
//	terminal-games list          – list all available games
//	terminal-games <game>        – launch the named game directly
package main

import (
	"fmt"
	"os"

	// Blank-import game packages here so their init() functions register them.
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/brickbreaker"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/enginedemo"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/flappybird"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/pong"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/spaceinvaders"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
	"github.com/BenjaminBenetti/terminal-games/internal/registry"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return launchPicker()
	}

	switch args[0] {
	case "list":
		return listGames()
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return launchGame(args[0])
	}
}

func listGames() error {
	games := registry.List()
	if len(games) == 0 {
		fmt.Println("No games are currently available.")
		return nil
	}
	fmt.Println("Available games:")
	for _, g := range games {
		fmt.Printf("  %-16s %s\n", g.Name(), g.Description())
	}
	return nil
}

func launchGame(name string) error {
	g, ok := registry.Get(name)
	if !ok {
		return fmt.Errorf("unknown game %q – run 'terminal-games list' to see available games", name)
	}
	return g.Run()
}

// hiddenInPicker lists registered games that shouldn't appear in the
// interactive picker (they're still launchable by name and via `list`).
// Currently this is just the engine demo, which is a developer tool
// rather than a game.
var hiddenInPicker = map[string]bool{
	"enginedemo": true,
}

// launchPicker shows the interactive game picker. If the user picks a
// game, the picker's engine exits cleanly and we delegate to launchGame
// so the picked game gets its own fresh engine instance.
func launchPicker() error {
	all := registry.List()
	games := make([]registry.Game, 0, len(all))
	for _, g := range all {
		if hiddenInPicker[g.Name()] {
			continue
		}
		games = append(games, g)
	}
	if len(games) == 0 {
		fmt.Println("No games are currently available.")
		return nil
	}
	e, err := engine.New(engine.Options{})
	if err != nil {
		return err
	}
	picker := newPickerScene(e, games)
	if err := e.Run(picker); err != nil {
		return err
	}
	if picker.picked == "" {
		return nil
	}
	return launchGame(picker.picked)
}

func printUsage() {
	fmt.Println(`terminal-games – play games in your terminal

Usage:
  terminal-games             open the interactive game picker
  terminal-games list        list all available games
  terminal-games <game>      launch the named game directly`)
}
