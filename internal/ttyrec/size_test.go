package ttyrec_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pneumaticdeath/NH_Watcher/internal/ttyrec"
)

func TestDetectSizeRecordings(t *testing.T) {
	files, _ := filepath.Glob("../nao/recordings/*.ttyrec")
	if len(files) == 0 {
		t.Skip("no ttyrec recordings found")
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		frames, err := ttyrec.Parse(bytes.NewReader(data))
		if err != nil {
			t.Errorf("%s: parse error: %v", path, err)
			continue
		}
		cols, rows := ttyrec.DetectSize(frames)
		t.Logf("%s: %d frames, detected size %dx%d", filepath.Base(path), len(frames), cols, rows)
		if cols == 0 || rows == 0 {
			t.Errorf("%s: failed to detect size", path)
		}
	}
}
