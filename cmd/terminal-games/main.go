// terminal-games is the command-line interface for the terminal-games collection.
//
// Usage:
//
//	terminal-games list          – list all available games
//	terminal-games <game>        – launch the named game
package main

import (
	"fmt"
	"os"

	// Blank-import game packages here so their init() functions register them.
	// Example:
	//   _ "github.com/BenjaminBenetti/terminal-games/internal/game/snake"

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
		printUsage()
		return nil
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

func printUsage() {
	fmt.Println(`terminal-games – play games in your terminal

Usage:
  terminal-games list       list all available games
  terminal-games <game>     launch the named game

Run 'terminal-games list' to see the available games.`)
}
