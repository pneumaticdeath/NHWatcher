# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

NH_Watcher is a macOS screensaver that spectates live NetHack games on nethack.alt.org (NAO). It connects via SSH to NAO's dgamelaunch server and displays the terminal output using a Fyne GUI terminal widget. When no live games are available, it falls back to playing ttyrec recordings.

## Build & Run

```bash
# Build standalone app
make app

# Build .saver bundle (includes the Go binary)
make saver

# Install screensaver to ~/Library/Screen Savers
make install

# Uninstall
make uninstall

# Test screensaver (opens it directly)
make test-saver

# Run standalone (no screensaver wrapper)
make run

# Run tests
go test ./internal/nao/
```

## Architecture

- **`cmd/nhwatcher/`** — Application entry point. Creates a fullscreen Fyne window, wires up the NAO client and viewer. Handles SIGTERM for clean shutdown when the screensaver wrapper terminates it.
- **`internal/nao/`** — SSH client for nethack.alt.org. Handles connection, PTY setup, dgamelaunch menu navigation, game list parsing, random non-idle game selection, and ttyrec fetching from NAO's userdata directories.
- **`internal/screen/`** — Display layer. Wraps `github.com/fyne-io/terminal` widget, provides the dark theme, and manages the spectating lifecycle. Auto-switches games on idle timeout or game exit. Falls back to ttyrec playback. Any keypress exits.
- **`internal/ttyrec/`** — ttyrec format parser and timed playback. Provides an `io.Reader` that delivers frames with realistic timing (capped at 2s delay between frames).
- **`screensaver/`** — Objective-C `.saver` bundle wrapper. Minimal `ScreenSaverView` subclass that launches the Go binary from the bundle's Resources and terminates it on deactivation.

### macOS Screensaver Bundle

The `.saver` bundle is a Mach-O loadable bundle (not an executable) that macOS loads via `legacyScreenSaver.appex`. Our wrapper (`NHWatcherView`) launches the embedded Go binary as a subprocess. The Go app renders in its own fullscreen window. On deactivation, the wrapper sends SIGTERM.

Bundle structure:
```
NHWatcher.saver/Contents/
  Info.plist          — NSPrincipalClass = NHWatcherView
  MacOS/NHWatcher     — ObjC bundle (ScreenSaverView subclass)
  Resources/nhwatcher — Go binary
```

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
6. Pick one at random (avoiding the previously watched player), send its selector letter
7. Live game output streams as standard VT100 terminal data
8. On 2min idle or game exit, switch to another game
9. If no live games: fetch a ttyrec from `alt.org/nethack/userdata/{letter}/{player}/ttyrec/` and play it back

### dgamelaunch Watch Menu Format

After ANSI stripping, game entries run together separated by watcher counts. Selectors are letters a-z (skipping q), then A-Z (skipping Q). Idle time is blank for active games (<= 4s idle), otherwise shows e.g. `45s`, `3m 42s`, `2h 15m`.

### NetHack Terminal Output

Games use varying terminal dimensions with xterm-256color, ANSI color codes, DEC Special Graphics line-drawing characters, and curses-based cursor positioning. FyneTerm handles all of this natively.
