package flappybird

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// hiScoreRelPath is the on-disk location of the persistent best-score
// file, relative to the user's data home. Saving across runs matches
// the original game's behavior — the "BEST" badge wouldn't mean much
// if it reset every time you launched.
const hiScoreRelPath = "terminal-games/flappybird.hi"

// hiScoreFilePath returns the absolute path of the hi-score file
// (creating no directories — that's a write-time concern). XDG_DATA_HOME
// wins if set, otherwise we fall back to ~/.local/share, matching the
// XDG Base Directory Specification.
func hiScoreFilePath() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, hiScoreRelPath), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", hiScoreRelPath), nil
}

// loadHiScore reads the persisted best score, returning 0 if anything
// goes wrong (no file, permission error, corrupt content). Hi-score
// loading must never break the game, so all errors are swallowed.
func loadHiScore() int {
	p, err := hiScoreFilePath()
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// saveHiScore writes score to the persistent hi-score file. Like
// loadHiScore, this is best-effort: any I/O failure is silently
// dropped because failing a write should never crash the game.
func saveHiScore(score int) {
	p, err := hiScoreFilePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(p, []byte(strconv.Itoa(score)), 0o644)
}
