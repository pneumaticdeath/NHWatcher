# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

NH_Watcher is a macOS screensaver-style application that spectates live NetHack games on nethack.alt.org (NAO). It connects via SSH to NAO's dgamelaunch server and displays the terminal output using a Fyne GUI terminal widget.

## Build & Run

```bash
# Build
go build ./cmd/nhwatcher/

# Run
./nhwatcher

# Run directly
go run ./cmd/nhwatcher/

# Test
go test ./internal/nao/
```

## Architecture

- **`cmd/nhwatcher/`** — Application entry point. Creates a fullscreen Fyne window, wires up the NAO client and viewer.
- **`internal/nao/`** — SSH client for nethack.alt.org. Handles connection, PTY setup, dgamelaunch menu navigation, game list parsing, and random non-idle game selection. PTY size is set dynamically based on the window.
- **`internal/screen/`** — Display layer. Wraps `github.com/fyne-io/terminal` widget, provides the dark theme, and manages the spectating lifecycle. Waits for fullscreen layout before measuring terminal dimensions.

### Key Dependencies

- **`github.com/fyne-io/terminal`** (FyneTerm) — Terminal emulator widget that handles VT100/xterm escape sequence rendering. The `RunWithConnection(in io.WriteCloser, out io.Reader)` method is the primary integration point — we feed it the SSH session's stdin/stdout.
- **`fyne.io/fyne/v2`** — GUI framework. Pinned to a pre-release (`v2.7.1-0.20251105...`) for compatibility with fyne-io/terminal.
- **`golang.org/x/crypto/ssh`** — SSH client for connecting to NAO.

### NAO Connection Flow

1. SSH to `nethack@alt.org` (no password required)
2. dgamelaunch presents a text menu
3. Send `w` to enter the "watch games" menu
4. Parse the game list (ANSI-stripped, entries run together without newlines)
5. Filter to games that fit the current PTY size, prefer non-idle games
6. Pick one at random, send its selector letter
7. Live game output streams as standard VT100 terminal data

### dgamelaunch Watch Menu Format

After ANSI stripping, game entries run together separated by watcher counts. Selectors are letters a-z (skipping q), then A-Z (skipping Q). Idle time is blank for active games (<= 4s idle), otherwise shows e.g. `45s`, `3m 42s`, `2h 15m`.

### NetHack Terminal Output

Games use varying terminal dimensions with xterm-256color, ANSI color codes, DEC Special Graphics line-drawing characters, and curses-based cursor positioning. FyneTerm handles all of this natively.
