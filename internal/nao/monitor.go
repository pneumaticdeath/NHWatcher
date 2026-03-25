package nao

import (
	"bytes"
	"io"
	"sync"
	"time"
)

// SwitchReason describes why the monitor triggered a game switch.
type SwitchReason int

const (
	SwitchIdle     SwitchReason = iota // No data received within timeout
	SwitchGameOver                     // Watched game ended, back at menu
	SwitchEOF                          // Connection closed
	SwitchManual                       // User requested switch
)

func (r SwitchReason) String() string {
	switch r {
	case SwitchIdle:
		return "idle timeout"
	case SwitchGameOver:
		return "game over"
	case SwitchEOF:
		return "connection closed"
	case SwitchManual:
		return "user request"
	default:
		return "unknown"
	}
}

// MonitoredReader wraps an io.Reader to detect inactivity and game exit.
// It passes all data through to the terminal while monitoring it.
type MonitoredReader struct {
	inner   io.Reader
	mu      sync.Mutex
	lastData time.Time
	timeout  time.Duration
	switchCh chan SwitchReason
	done     chan struct{}
	// Ring buffer of recent bytes for pattern matching
	recent []byte
}

// NewMonitoredReader creates a reader that monitors for idle timeout
// and game exit patterns.
func NewMonitoredReader(r io.Reader, idleTimeout time.Duration) *MonitoredReader {
	m := &MonitoredReader{
		inner:    r,
		lastData: time.Now(),
		timeout:  idleTimeout,
		switchCh: make(chan SwitchReason, 1),
		done:     make(chan struct{}),
		recent:   make([]byte, 0, 512),
	}
	go m.watchIdle()
	return m
}

// Read implements io.Reader, passing data through while monitoring it.
func (m *MonitoredReader) Read(p []byte) (int, error) {
	n, err := m.inner.Read(p)
	if n > 0 {
		m.mu.Lock()
		m.lastData = time.Now()
		// Append to recent buffer, keeping last 512 bytes
		m.recent = append(m.recent, p[:n]...)
		if len(m.recent) > 512 {
			m.recent = m.recent[len(m.recent)-512:]
		}
		// Check if we see the dgamelaunch watch menu prompt,
		// which means the game we were watching has ended.
		if bytes.Contains(m.recent, []byte("Watch which game?")) {
			m.mu.Unlock()
			m.signal(SwitchGameOver)
			return n, err
		}
		m.mu.Unlock()
	}
	if err != nil {
		m.signal(SwitchEOF)
	}
	return n, err
}

// SwitchCh returns a channel that receives the reason when a game
// switch should occur.
func (m *MonitoredReader) SwitchCh() <-chan SwitchReason {
	return m.switchCh
}

// Stop stops the idle watcher goroutine.
func (m *MonitoredReader) Stop() {
	select {
	case <-m.done:
	default:
		close(m.done)
	}
}

func (m *MonitoredReader) signal(reason SwitchReason) {
	select {
	case m.switchCh <- reason:
	default:
	}
}

func (m *MonitoredReader) watchIdle() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.mu.Lock()
			idle := time.Since(m.lastData)
			m.mu.Unlock()
			if idle >= m.timeout {
				m.signal(SwitchIdle)
				return
			}
		}
	}
}
