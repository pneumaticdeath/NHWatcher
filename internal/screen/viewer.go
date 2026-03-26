// Package screen manages the screensaver display using a Fyne terminal widget.
package screen

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	terminal "github.com/fyne-io/terminal"
	"github.com/pneumaticdeath/NH_Watcher/internal/nao"
	"github.com/pneumaticdeath/NH_Watcher/internal/ttyrec"
)

const idleTimeout = 2 * time.Minute

// Viewer manages the terminal display and game watching lifecycle.
type Viewer struct {
	window         fyne.Window
	client         *nao.Client
	term           *terminal.Terminal
	status         *widget.Label
	container      *fyne.Container
	cols           int
	rows           int
	lastPlayer     string
	quit           chan struct{}
	switchCh       chan struct{}
	screensaverMode bool
}

// NewViewer creates a new screensaver viewer. If screensaverMode is true,
// any keypress exits. Otherwise, 's' switches games and Escape/Q quits.
func NewViewer(w fyne.Window, client *nao.Client, screensaverMode bool) *Viewer {
	t := terminal.New()
	status := widget.NewLabel("Connecting to nethack.alt.org...")

	v := &Viewer{
		window:          w,
		client:          client,
		term:            t,
		status:          status,
		quit:            make(chan struct{}),
		switchCh:        make(chan struct{}, 1),
		screensaverMode: screensaverMode,
	}

	if screensaverMode {
		// Any keypress exits, like a screensaver
		w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
			v.Exit()
		})
		w.Canvas().SetOnTypedRune(func(r rune) {
			v.Exit()
		})
	} else {
		w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
			if ev.Name == fyne.KeyEscape {
				v.Exit()
			}
		})
		w.Canvas().SetOnTypedRune(func(r rune) {
			switch r {
			case 's', 'S':
				v.requestSwitch()
			case 'q', 'Q':
				v.Exit()
			}
		})
	}

	return v
}

func (v *Viewer) requestSwitch() {
	select {
	case v.switchCh <- struct{}{}:
	default:
	}
}

// Exit shuts down the viewer, closing the SSH connection and window.
func (v *Viewer) Exit() {
	select {
	case <-v.quit:
		return
	default:
		close(v.quit)
	}
	v.client.Close()
	v.window.Close()
}

// Content returns the Fyne canvas object for the viewer.
func (v *Viewer) Content() fyne.CanvasObject {
	v.container = container.NewStack(
		v.term,
		container.NewBorder(nil, v.status, nil, nil),
	)
	return v.container
}

// terminalSize computes cols and rows that fit the current window.
func (v *Viewer) terminalSize() (int, int) {
	winSize := v.window.Canvas().Size()

	// Measure monospace cell size (same approach as FyneTerm)
	cell := canvas.NewText("M", color.White)
	cell.TextStyle.Monospace = true
	scale := v.term.Theme().Size(theme.SizeNameText) / theme.TextSize()
	min := cell.MinSize()
	cellW := float64(min.Width * scale)
	cellH := float64(min.Height * scale)

	cols := int(math.Floor(float64(winSize.Width) / cellW))
	rows := int(math.Floor(float64(winSize.Height) / cellH))
	if cols < 80 {
		cols = 80
	}
	if rows < 24 {
		rows = 24
	}
	return cols, rows
}

// waitForFullScreen blocks until the window canvas reports a size
// larger than the initial default, indicating fullscreen layout is done.
func (v *Viewer) waitForFullScreen() {
	for {
		s := v.window.Canvas().Size()
		if s.Width > 200 && s.Height > 200 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Start connects to NAO and loops, switching games on idle or game exit.
func (v *Viewer) Start() error {
	v.waitForFullScreen()
	v.cols, v.rows = v.terminalSize()
	log.Printf("Terminal size: %dx%d", v.cols, v.rows)

	for {
		select {
		case <-v.quit:
			return nil
		default:
		}

		reason, err := v.watchOne()
		if err != nil {
			log.Printf("No live game available: %v", err)
			// Fall back to ttyrec playback
			if ttyErr := v.playTTYRec(); ttyErr != nil {
				log.Printf("ttyrec fallback failed: %v", ttyErr)
				fyne.Do(func() {
					v.status.SetText("No games or recordings available — retrying...")
				})
				time.Sleep(10 * time.Second)
			}
			continue
		}
		log.Printf("Switching game: %s", reason)
		fyne.Do(func() {
			v.status.SetText(fmt.Sprintf("Switching game (%s)...", reason))
		})
		// Brief pause before reconnecting
		time.Sleep(1 * time.Second)
	}
}

// watchOne connects to NAO, watches a single game, and returns when
// the game should be switched (idle timeout, game over, or EOF).
func (v *Viewer) watchOne() (nao.SwitchReason, error) {
	// Fresh terminal widget for each game
	v.term = terminal.New()
	fyne.Do(func() {
		v.container.Objects[0] = v.term
		v.container.Refresh()
	})

	v.client.Close()
	stdin, stdout, err := v.client.Connect(v.cols, v.rows)
	if err != nil {
		return 0, fmt.Errorf("connect: %w", err)
	}

	fyne.Do(func() {
		v.status.SetText("Connected — selecting a game...")
	})
	log.Println("Connected to nethack.alt.org")

	player, err := v.client.WatchRandomGame(v.lastPlayer)
	if err != nil {
		return 0, fmt.Errorf("select game: %w", err)
	}
	v.lastPlayer = player

	fyne.Do(func() {
		v.status.SetText(fmt.Sprintf("Watching %s", player))
	})
	log.Printf("Watching player: %s", player)

	// Wrap stdout with the monitor for idle/game-over detection
	monitor := nao.NewMonitoredReader(stdout, idleTimeout)
	defer monitor.Stop()

	// RunWithConnection blocks until the reader returns an error.
	// Run it in a goroutine so we can also wait on the switch channel.
	done := make(chan struct{})
	go func() {
		v.term.RunWithConnection(stdin, monitor)
		close(done)
	}()

	// Wait for either a switch signal, the terminal finishing, user switch, or quit
	select {
	case reason := <-monitor.SwitchCh():
		return reason, nil
	case <-v.switchCh:
		return nao.SwitchManual, nil
	case <-done:
		return nao.SwitchEOF, nil
	case <-v.quit:
		return nao.SwitchEOF, nil
	}
}

// playTTYRec fetches and plays back a ttyrec recording as a fallback
// when no live games are available.
func (v *Viewer) playTTYRec() error {
	fyne.Do(func() {
		v.status.SetText("No live games — looking for recordings...")
	})

	// Get the player list to use as ttyrec candidates
	players, err := v.client.GetRecentPlayers(v.cols, v.rows)
	if err != nil {
		return fmt.Errorf("get players: %w", err)
	}
	if len(players) == 0 {
		return fmt.Errorf("no players found")
	}

	frames, label, err := nao.FetchRandomTTYRec(players)
	if err != nil {
		return fmt.Errorf("fetch ttyrec: %w", err)
	}
	log.Printf("Playing ttyrec: %s (%d frames)", label, len(frames))

	// Fresh terminal widget
	v.term = terminal.New()
	fyne.Do(func() {
		v.container.Objects[0] = v.term
		v.container.Refresh()
		v.status.SetText(fmt.Sprintf("Replay: %s", label))
	})

	player := ttyrec.NewPlayer(frames)
	defer player.Stop()

	// nopWriteCloser discards writes (ttyrec playback is read-only)
	done := make(chan struct{})
	go func() {
		v.term.RunWithConnection(nopWriteCloser{}, player)
		close(done)
	}()

	select {
	case <-done:
		log.Printf("ttyrec playback finished: %s", label)
		return nil
	case <-v.switchCh:
		player.Stop()
		return nil
	case <-v.quit:
		player.Stop()
		return nil
	}
}

// nopWriteCloser is an io.WriteCloser that discards all writes.
type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }
