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
)

const idleTimeout = 2 * time.Minute

// Viewer manages the terminal display and game watching lifecycle.
type Viewer struct {
	window     fyne.Window
	client     *nao.Client
	term       *terminal.Terminal
	status     *widget.Label
	container  *fyne.Container
	cols       int
	rows       int
	lastPlayer string
	quit       chan struct{}
}

// NewViewer creates a new screensaver viewer.
func NewViewer(w fyne.Window, client *nao.Client) *Viewer {
	t := terminal.New()
	status := widget.NewLabel("Connecting to nethack.alt.org...")

	v := &Viewer{
		window: w,
		client: client,
		term:   t,
		status: status,
		quit:   make(chan struct{}),
	}

	// Any keypress or mouse click exits, like a screensaver
	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		v.exit()
	})
	w.Canvas().SetOnTypedRune(func(r rune) {
		v.exit()
	})

	return v
}

func (v *Viewer) exit() {
	select {
	case <-v.quit:
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
			log.Printf("watch error: %v", err)
			fyne.Do(func() {
				v.status.SetText(fmt.Sprintf("Error: %v — retrying...", err))
			})
			time.Sleep(5 * time.Second)
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

	// Wait for either a switch signal, the terminal finishing, or quit
	select {
	case reason := <-monitor.SwitchCh():
		return reason, nil
	case <-done:
		return nao.SwitchEOF, nil
	case <-v.quit:
		return nao.SwitchEOF, nil
	}
}
