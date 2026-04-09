package ttyrec

import (
	"regexp"
	"strconv"
)

// cursorPosRe matches CSI row;col H cursor positioning sequences.
// Also matches CSI row;col f (HVP) which is equivalent.
var cursorPosRe = regexp.MustCompile(`\x1b\[(\d+);(\d+)[Hf]`)

// DetectSize scans ttyrec frames for cursor positioning escape sequences
// and returns the maximum column and row values observed. This gives a
// good estimate of the terminal dimensions used during recording.
// Returns (0, 0) if no cursor positioning sequences are found.
func DetectSize(frames []Frame) (cols, rows int) {
	for _, f := range frames {
		for _, m := range cursorPosRe.FindAllSubmatch(f.Data, -1) {
			r, _ := strconv.Atoi(string(m[1]))
			c, _ := strconv.Atoi(string(m[2]))
			if c > cols {
				cols = c
			}
			if r > rows {
				rows = r
			}
		}
	}
	return cols, rows
}
