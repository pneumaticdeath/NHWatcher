package nao

import (
	"bytes"
	"embed"
	"math/rand/v2"

	"github.com/pneumaticdeath/NH_Watcher/internal/ttyrec"
)

//go:embed recordings/*.ttyrec
var recordingsFS embed.FS

// ParseBundledTTYRec parses a randomly selected embedded ttyrec recording.
func ParseBundledTTYRec() ([]ttyrec.Frame, error) {
	entries, err := recordingsFS.ReadDir("recordings")
	if err != nil {
		return nil, err
	}
	entry := entries[rand.IntN(len(entries))]
	data, err := recordingsFS.ReadFile("recordings/" + entry.Name())
	if err != nil {
		return nil, err
	}
	return ttyrec.Parse(bytes.NewReader(data))
}
