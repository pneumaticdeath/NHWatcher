package ttyrec

import (
	"strconv"
	"strings"
)

// DetectSize scans ttyrec frames to determine the terminal dimensions
// used during recording. It simulates cursor position through control
// characters (CR, LF, BS), CSI movement sequences (CUP/HVP/CHA/CUF/CUB/
// CUU/CUD), and printable-character advancement, recording the maximum
// row and column ever reached. Scroll-region bottom (DECSTBM) is also
// considered for rows.
//
// Returns (0, 0) if no relevant escape sequences or printables are found.
func DetectSize(frames []Frame) (cols, rows int) {
	col, row := 1, 1
	for _, f := range frames {
		data := f.Data
		for i := 0; i < len(data); i++ {
			c := data[i]
			switch c {
			case '\r':
				col = 1
			case '\n':
				row++
				if row > rows {
					rows = row
				}
			case '\b':
				if col > 1 {
					col--
				}
			case '\x1b':
				if i+1 >= len(data) {
					break
				}
				i++
				if data[i] != '[' {
					if data[i] == '(' || data[i] == ')' {
						i++ // skip charset selector
					}
					continue
				}
				// CSI sequence — collect parameters and final byte
				i++
				paramStart := i
				for i < len(data) && (data[i] >= '0' && data[i] <= '9' || data[i] == ';' || data[i] == '?') {
					i++
				}
				if i >= len(data) {
					break
				}
				params := string(data[paramStart:i])
				final := data[i]
				switch final {
				case 'H', 'f': // CUP / HVP — cursor position (1-based)
					row, col = parseTwoParams(params)
				case 'G': // CHA — cursor horizontal absolute
					col = parseSingleParam(params)
				case 'C': // CUF — cursor forward
					col += parseSingleParam(params)
				case 'D': // CUB — cursor back
					col -= parseSingleParam(params)
					if col < 1 {
						col = 1
					}
				case 'A': // CUU — cursor up
					row -= parseSingleParam(params)
					if row < 1 {
						row = 1
					}
				case 'B', 'e': // CUD / VPR — cursor down
					row += parseSingleParam(params)
				case 'd': // VPA — vertical position absolute
					row = parseSingleParam(params)
				case 'r': // DECSTBM — scroll region
					if !strings.HasPrefix(params, "?") {
						_, bottom := parseTwoParams(params)
						if bottom > rows {
							rows = bottom
						}
					}
				}
				if col > cols {
					cols = col
				}
				if row > rows {
					rows = row
				}
			default:
				// Printable character (including DEC line-drawing glyphs and
				// 8-bit/UTF-8 bytes) advances the cursor by one cell. We
				// record the column the glyph occupies before advancing.
				if c >= 0x20 {
					if col > cols {
						cols = col
					}
					col++
				}
			}
		}
	}
	return cols, rows
}

// parseTwoParams extracts two semicolon-separated integer parameters,
// defaulting to 1 for missing values (per ECMA-48).
func parseTwoParams(s string) (int, int) {
	a, b := 1, 1
	for i, c := range s {
		if c == ';' {
			if i > 0 {
				a, _ = strconv.Atoi(s[:i])
			}
			if i+1 < len(s) {
				b, _ = strconv.Atoi(s[i+1:])
			}
			return a, b
		}
	}
	if len(s) > 0 {
		a, _ = strconv.Atoi(s)
	}
	return a, b
}

// parseSingleParam extracts a single integer parameter, defaulting to 1.
func parseSingleParam(s string) int {
	if len(s) == 0 {
		return 1
	}
	n, _ := strconv.Atoi(s)
	if n == 0 {
		n = 1
	}
	return n
}
