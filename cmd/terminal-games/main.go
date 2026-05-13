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
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/asteroids"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/brickbreaker"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/centipede"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/donkeykong"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/enginedemo"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/flappybird"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/galaga"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/galaxian"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/lunarlander"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/magic8ball"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/pacman"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/pong"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/scramble"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/spaceinvaders"
	_ "github.com/BenjaminBenetti/terminal-games/internal/game/wizardofwor"

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

// launchPicker shows the interactive game picker, then runs whichever
// game the user picks. When the game exits, control returns to the
// picker so the user can pick another game (or quit). The cursor
// defaults to the most recently played game on each re-entry.
//
// The picker and each game run on their own engine instances — fresh
// alt-screen + raw-mode setup each time — so any state from a previous
// game is fully cleared before the next one starts.
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

	var lastPicked string
	for {
		e, err := engine.New(engine.Options{})
		if err != nil {
			return err
		}
		picker := newPickerScene(e, games)
		if lastPicked != "" {
			for i, g := range games {
				if g.Name() == lastPicked {
					picker.selected = i
					break
				}
			}
		}
		if err := e.Run(picker); err != nil {
			return err
		}
		if picker.picked == "" {
			return nil
		}
		lastPicked = picker.picked
		if err := launchGame(picker.picked); err != nil {
			return err
		}
	}
}

func printUsage() {
	fmt.Println(`terminal-games – play games in your terminal

Usage:
  terminal-games             open the interactive game picker
  terminal-games list        list all available games
  terminal-games <game>      launch the named game directly`)
}
