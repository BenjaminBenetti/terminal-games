# CLAUDE.md

Project notes for Claude Code.

## Engine

This repo includes a small terminal-game engine under `internal/engine/`.
**Before touching the engine or building a new game on top of it, read the
docs under [`internal/engine/doc/`](internal/engine/doc/README.md).** They
cover the loop, canvas (pixel + native-text overlay), color, input, and
rendering. Start with `internal/engine/doc/README.md` — it has the
quick-start and an index to the per-module pages.

Reference game: `internal/game/enginedemo/` is the working example to
crib from. New games go in `internal/game/<name>/` and get registered by
adding a blank import to `cmd/terminal-games/main.go`.
