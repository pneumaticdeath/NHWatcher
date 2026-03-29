package nao

import (
	"testing"
)

func TestParseGameList(t *testing.T) {
	// Actual output from NAO after ANSI stripping (entries run together)
	input := ` a) Badger004        nh367  182x 35  2026-03-23 16:46:50  12m 24s  0b) BatBeefs         nh367   80x 24  2026-03-23 16:04:31  5m 19s   0c) BCKit            nh367  130x 26  2026-03-23 16:47:24           0d) crunchbubba      nh367  180x 50  2026-03-23 15:58:29  45s      0e) eosgorbash       nh367   80x 25  2026-03-23 16:05:01  26m 31s  0f) failbaka         nh367  127x 36  2026-03-23 14:29:48  19s      0g) h2g2             nh367  132x 43  2026-03-23 16:47:42           0h) IceCreamJones    nh367  150x 34  2026-03-23 16:54:05           0i) jtn              nh367  190x 41  2026-03-23 15:18:55           0j) Lanfaedhe        nh367   80x 24  2026-03-23 16:27:15           0k) LoveIsLove       nh367  238x 54  2026-03-23 16:48:49  2m 45s   0l) mday299          nh367   80x 24  2026-03-23 15:50:30  16s      0m) needler          nh367  112x 26  2026-03-23 15:06:52           0n) nnnet            nh367   90x 30  2026-03-23 16:52:47  5m 22s   0o) Sapphirejax      nh367  181x 38  2026-03-23 16:41:30           0(1-15 of 18)Watch which game? ('?' for help) =>`

	games := ParseGameList(input)

	if len(games) != 15 {
		t.Errorf("expected 15 games, got %d", len(games))
		for _, g := range games {
			t.Logf("  %s) %-16s idle=%q", g.Selector, g.Player, g.Idle)
		}
	}

	// Check a few specific entries
	checks := map[string]struct {
		player string
		cols   int
		rows   int
		idle   bool
		fits   bool // fits in 80x24
	}{
		"a": {"Badger004", 182, 35, true, false},  // 12m 24s, too big
		"b": {"BatBeefs", 80, 24, true, true},     // 5m 19s, fits
		"c": {"BCKit", 130, 26, false, false},     // active, too big
		"g": {"h2g2", 132, 43, false, false},      // active, too big
		"j": {"Lanfaedhe", 80, 24, false, true},   // active, fits!
		"m": {"needler", 112, 26, false, false},    // active, too big
	}
	gameMap := make(map[string]Game)
	for _, g := range games {
		gameMap[g.Selector] = g
	}
	for sel, want := range checks {
		g, ok := gameMap[sel]
		if !ok {
			t.Errorf("missing game with selector %q", sel)
			continue
		}
		if g.Player != want.player {
			t.Errorf("selector %s: player = %q, want %q", sel, g.Player, want.player)
		}
		if g.Cols != want.cols || g.Rows != want.rows {
			t.Errorf("selector %s: size = %dx%d, want %dx%d", sel, g.Cols, g.Rows, want.cols, want.rows)
		}
		if g.IsIdle() != want.idle {
			t.Errorf("selector %s (%s): IsIdle() = %v, want %v (idle=%q)", sel, g.Player, g.IsIdle(), want.idle, g.Idle)
		}
		if g.FitsIn(80, 24) != want.fits {
			t.Errorf("selector %s (%s): FitsIn(80,24) = %v, want %v", sel, g.Player, g.FitsIn(80, 24), want.fits)
		}
	}
}

func TestParseGameListHardfought(t *testing.T) {
	// Actual output from us.hardfought.org after ANSI stripping.
	// Hardfought has different game names, extra "W" and "Extra" columns,
	// and some games show "N/A" for size.
	input := ` a) Enigmic         nndnh0118  139x 29  2026-03-27 18:27:39            0  A Endb) griffs          nh370.132  162x 47  2026-03-27 17:42:54            0    S1c) somebody        evil092     80x 25  2026-03-27 18:00:00  2m 30s   0    M4d) tau             evil092    150x 47  2026-03-27 16:28:08            0  A S1e) gruefood        hackem132   80x 25  2026-03-27 17:53:33  1m 25s   0    M4f) Pullings        nh4        N/A      2026-03-27 18:18:01  17m 47s  0(1-6 of 6)Watch which game? ('?' for help) =>`

	games := ParseGameList(input)

	// N/A game (Pullings/nh4) should now be included with 0x0 size
	if len(games) != 6 {
		t.Errorf("expected 6 games (including N/A), got %d", len(games))
		for _, g := range games {
			t.Logf("  %s) %-16s %dx%d idle=%q", g.Selector, g.Player, g.Cols, g.Rows, g.Idle)
		}
	}

	checks := map[string]struct {
		player string
		cols   int
		rows   int
		idle   bool
	}{
		"a": {"Enigmic", 139, 29, false},
		"b": {"griffs", 162, 47, false},
		"c": {"somebody", 80, 25, true},   // 2m 30s
		"d": {"tau", 150, 47, false},
		"e": {"gruefood", 80, 25, true},   // 1m 25s
		"f": {"Pullings", 0, 0, true},     // N/A size, 17m 47s idle
	}
	gameMap := make(map[string]Game)
	for _, g := range games {
		gameMap[g.Selector] = g
	}
	for sel, want := range checks {
		g, ok := gameMap[sel]
		if !ok {
			t.Errorf("missing game with selector %q", sel)
			continue
		}
		if g.Player != want.player {
			t.Errorf("selector %s: player = %q, want %q", sel, g.Player, want.player)
		}
		if g.Cols != want.cols || g.Rows != want.rows {
			t.Errorf("selector %s: size = %dx%d, want %dx%d", sel, g.Cols, g.Rows, want.cols, want.rows)
		}
		if g.IsIdle() != want.idle {
			t.Errorf("selector %s (%s): IsIdle() = %v, want %v (idle=%q)", sel, g.Player, g.IsIdle(), want.idle, g.Idle)
		}
	}

	// N/A games should be treated as fitting any terminal size
	if na, ok := gameMap["f"]; ok {
		if !na.FitsIn(80, 24) {
			t.Error("N/A game should FitsIn(80,24)")
		}
	}
}
