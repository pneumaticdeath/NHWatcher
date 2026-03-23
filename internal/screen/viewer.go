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

// Viewer manages the terminal display and game watching lifecycle.
type Viewer struct {
	window fyne.Window
	client *nao.Client
	term   *terminal.Terminal
	status *widget.Label
}

// NewViewer creates a new screensaver viewer.
func NewViewer(w fyne.Window, client *nao.Client) *Viewer {
	t := terminal.New()
	status := widget.NewLabel("Connecting to nethack.alt.org...")

	return &Viewer{
		window: w,
		client: client,
		term:   t,
		status: status,
	}
}

// Content returns the Fyne canvas object for the viewer.
func (v *Viewer) Content() fyne.CanvasObject {
	return container.NewStack(
		v.term,
		container.NewBorder(nil, v.status, nil, nil),
	)
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

// Start connects to NAO and begins displaying a game.
func (v *Viewer) Start() error {
	v.waitForFullScreen()
	cols, rows := v.terminalSize()
	log.Printf("Terminal size: %dx%d", cols, rows)

	stdin, stdout, err := v.client.Connect(cols, rows)
	if err != nil {
		fyne.Do(func() {
			v.status.SetText(fmt.Sprintf("Connection failed: %v", err))
		})
		return fmt.Errorf("connect: %w", err)
	}

	fyne.Do(func() {
		v.status.SetText("Connected — selecting a game...")
	})
	log.Println("Connected to nethack.alt.org")

	player, err := v.client.WatchRandomGame()
	if err != nil {
		fyne.Do(func() {
			v.status.SetText(fmt.Sprintf("No games available: %v", err))
		})
		return fmt.Errorf("watch game: %w", err)
	}

	fyne.Do(func() {
		v.status.SetText(fmt.Sprintf("Watching %s", player))
	})
	log.Printf("Watching player: %s", player)

	// Feed the SSH session into the terminal widget
	v.term.RunWithConnection(stdin, stdout)

	return nil
}
