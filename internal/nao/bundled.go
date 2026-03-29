package nao

import (
	"bytes"
	_ "embed"

	"github.com/pneumaticdeath/NH_Watcher/internal/ttyrec"
)

//go:embed bundled.ttyrec
var bundledTTYRec []byte

// ParseBundledTTYRec parses the embedded ttyrec recording.
func ParseBundledTTYRec() ([]ttyrec.Frame, error) {
	return ttyrec.Parse(bytes.NewReader(bundledTTYRec))
}
