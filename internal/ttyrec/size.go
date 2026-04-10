package ttyrec

import (
	"strconv"
	"strings"
)

// DetectSize scans ttyrec frames for escape sequences that reveal the
// terminal dimensions used during recording.
//
// For columns: uses the maximum column from explicit cursor positioning
// (CSI row;col H) and cursor horizontal absolute (CSI col G) sequences.
// Character printing is not tracked because games often print long
// continuous runs (e.g. box-drawing borders) that wrap at the terminal
// edge, which we can't detect without already knowing the width.
//
// For rows: uses both cursor positioning and scroll region
// (CSI top;bottom r) which directly encodes the number of rows.
//
// Returns (0, 0) if no relevant escape sequences are found.
func DetectSize(frames []Frame) (cols, rows int) {
	for _, f := range frames {
		data := f.Data
		for i := 0; i < len(data); i++ {
			if data[i] != '\x1b' {
				continue
			}
			// Parse escape sequence
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
			case 'H', 'f': // CUP / HVP — cursor position
				r, c := parseTwoParams(params)
				if c > cols {
					cols = c
				}
				if r > rows {
					rows = r
				}
			case 'r': // DECSTBM — scroll region
				if !strings.HasPrefix(params, "?") {
					_, bottom := parseTwoParams(params)
					if bottom > rows {
						rows = bottom
					}
				}
			case 'G': // CHA — cursor horizontal absolute
				n := parseSingleParam(params)
				if n > cols {
					cols = n
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
