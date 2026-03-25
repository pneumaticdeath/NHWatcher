package ttyrec

import (
	"io"
	"time"
)

// maxFrameDelay caps the delay between frames so long idle periods
// in the recording don't stall playback.
const maxFrameDelay = 2 * time.Second

// Player plays back parsed ttyrec frames with realistic timing.
// It implements io.Reader for use with terminal.RunWithConnection.
type Player struct {
	frames  []Frame
	idx     int
	buf     []byte // remaining bytes from current frame
	started time.Time
	done    chan struct{}
}

// NewPlayer creates a player that will play back the given frames.
func NewPlayer(frames []Frame) *Player {
	return &Player{
		frames: frames,
		done:   make(chan struct{}),
	}
}

// Read implements io.Reader. It blocks according to frame timing,
// then returns frame data. Returns io.EOF after the last frame.
func (p *Player) Read(buf []byte) (int, error) {
	for {
		// If we have leftover data from the current frame, return it
		if len(p.buf) > 0 {
			n := copy(buf, p.buf)
			p.buf = p.buf[n:]
			return n, nil
		}

		// Check if playback is complete
		if p.idx >= len(p.frames) {
			return 0, io.EOF
		}

		frame := p.frames[p.idx]
		p.idx++

		if p.started.IsZero() {
			p.started = time.Now()
		}

		// Calculate how long to wait before delivering this frame
		elapsed := time.Since(p.started)
		wait := frame.Time - elapsed
		if wait > maxFrameDelay {
			// Fast-forward through long idle periods
			p.started = p.started.Add(wait - maxFrameDelay)
			wait = maxFrameDelay
		}
		if wait > 0 {
			select {
			case <-time.After(wait):
			case <-p.done:
				return 0, io.EOF
			}
		}

		p.buf = frame.Data
	}
}

// Stop terminates playback early.
func (p *Player) Stop() {
	select {
	case <-p.done:
	default:
		close(p.done)
	}
}

// Done returns a channel that is closed when playback should stop.
func (p *Player) Done() <-chan struct{} {
	return p.done
}
