// Package ttyrec parses and plays back ttyrec terminal recordings.
//
// The ttyrec format is a sequence of frames, each consisting of:
//   - sec:  4 bytes little-endian uint32 (seconds since epoch)
//   - usec: 4 bytes little-endian uint32 (microseconds)
//   - len:  4 bytes little-endian uint32 (data length)
//   - data: `len` bytes of terminal output
package ttyrec

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// Frame represents a single ttyrec frame.
type Frame struct {
	Time time.Duration // offset from the start of the recording
	Data []byte
}

// Parse reads all frames from a ttyrec stream.
func Parse(r io.Reader) ([]Frame, error) {
	var frames []Frame
	var startTime uint64
	header := make([]byte, 12)

	for {
		_, err := io.ReadFull(r, header)
		if err == io.EOF {
			break
		}
		if err != nil {
			return frames, fmt.Errorf("read header: %w", err)
		}

		sec := binary.LittleEndian.Uint32(header[0:4])
		usec := binary.LittleEndian.Uint32(header[4:8])
		dataLen := binary.LittleEndian.Uint32(header[8:12])

		ts := uint64(sec)*1_000_000 + uint64(usec)
		if len(frames) == 0 {
			startTime = ts
		}

		data := make([]byte, dataLen)
		if _, err := io.ReadFull(r, data); err != nil {
			return frames, fmt.Errorf("read data (%d bytes): %w", dataLen, err)
		}

		offset := time.Duration(ts-startTime) * time.Microsecond
		frames = append(frames, Frame{
			Time: offset,
			Data: data,
		})
	}

	return frames, nil
}
