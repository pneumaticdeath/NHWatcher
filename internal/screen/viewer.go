// Package screen manages the screensaver display using a Fyne terminal widget.
package screen

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	terminal "github.com/fyne-io/terminal"
	"github.com/pneumaticdeath/NH_Watcher/internal/nao"
	"github.com/pneumaticdeath/NH_Watcher/internal/ttyrec"
)

// mouseExitWidget is a transparent overlay that detects mouse movement
// and triggers the screensaver exit. Real screensavers exit on any
// mouse movement, not just clicks.
type mouseExitWidget struct {
	widget.BaseWidget
	onMouseMove func()
	exitOnce    sync.Once
	readyAt     time.Time // ignore mouse events before this time
	moveCount   int       // number of move events seen (ignore first few)
}

func newMouseExitWidget(onMove func()) *mouseExitWidget {
	w := &mouseExitWidget{
		onMouseMove: onMove,
		readyAt:     time.Now().Add(3 * time.Second), // 3s grace period
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *mouseExitWidget) CreateRenderer() fyne.WidgetRenderer {
	return &mouseExitRenderer{}
}

func (w *mouseExitWidget) MouseMoved(ev *desktop.MouseEvent) {
	// Ignore events during grace period (initial cursor position reports)
	if time.Now().Before(w.readyAt) {
		return
	}
	// Require a few move events to filter out jitter
	w.moveCount++
	if w.moveCount < 3 {
		return
	}
	if w.onMouseMove != nil {
		w.exitOnce.Do(func() {
			log.Println("Mouse moved, exiting screensaver")
			w.onMouseMove()
		})
	}
}

func (w *mouseExitWidget) MouseIn(ev *desktop.MouseEvent)  {}
func (w *mouseExitWidget) MouseOut()                        {}

// Ensure mouseExitWidget implements desktop.Hoverable
var _ desktop.Hoverable = (*mouseExitWidget)(nil)

type mouseExitRenderer struct{}

func (r *mouseExitRenderer) Layout(size fyne.Size) {}
func (r *mouseExitRenderer) MinSize() fyne.Size    { return fyne.NewSize(0, 0) }
func (r *mouseExitRenderer) Refresh()              {}
func (r *mouseExitRenderer) Objects() []fyne.CanvasObject { return nil }
func (r *mouseExitRenderer) Destroy()              {}

const idleTimeout = 2 * time.Minute

// Viewer manages the terminal display and game watching lifecycle.
type Viewer struct {
	window          fyne.Window
	servers         []nao.ServerConfig
	clients         []*nao.Client
	serverIdx       int // current server index
	term            *terminal.Terminal
	status          *widget.Label
	container       *fyne.Container
	cols            int
	rows            int
	lastPlayer      string
	quit            chan struct{}
	switchCh        chan struct{}
	screensaverMode bool
	frameOut        io.Writer // stdout pipe for frame output in screensaver mode
	onQuit          func()    // called to quit the Fyne app
}

// currentClient returns the client for the current server.
func (v *Viewer) currentClient() *nao.Client {
	return v.clients[v.serverIdx]
}

// rotateServer advances to the next server in the list.
func (v *Viewer) rotateServer() {
	v.serverIdx = (v.serverIdx + 1) % len(v.servers)
	v.lastPlayer = "" // reset avoid-player on server switch
}

// NewViewer creates a new screensaver viewer. If screensaverMode is true,
// any keypress or mouse movement exits. Otherwise, 's' switches games and Escape/Q quits.
func NewViewer(w fyne.Window, servers []nao.ServerConfig, screensaverMode bool) *Viewer {
	t := terminal.New()
	status := widget.NewLabel("Connecting...")

	clients := make([]*nao.Client, len(servers))
	for i, s := range servers {
		clients[i] = nao.NewClient(s)
	}

	v := &Viewer{
		window:          w,
		servers:         servers,
		clients:         clients,
		term:            t,
		status:          status,
		quit:            make(chan struct{}),
		switchCh:        make(chan struct{}, 1),
		screensaverMode: screensaverMode,
	}

	if screensaverMode {
		// Any keypress exits, like a screensaver
		w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
			log.Printf("Key pressed: %s, exiting", ev.Name)
			v.Exit()
		})
		w.Canvas().SetOnTypedRune(func(r rune) {
			log.Printf("Rune typed: %c, exiting", r)
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

// SetOnQuit sets a callback that will be called when the viewer exits
// to quit the Fyne application. This must be called before Start().
func (v *Viewer) SetOnQuit(f func()) {
	v.onQuit = f
}

func (v *Viewer) requestSwitch() {
	select {
	case v.switchCh <- struct{}{}:
	default:
	}
}

// Exit shuts down the viewer, closing all SSH connections and quitting the app.
// Safe to call from any goroutine.
func (v *Viewer) Exit() {
	select {
	case <-v.quit:
		return
	default:
		close(v.quit)
	}
	for _, c := range v.clients {
		c.Close()
	}
	if v.onQuit != nil {
		v.onQuit()
	}
}

// Content returns the Fyne canvas object for the viewer.
func (v *Viewer) Content() fyne.CanvasObject {
	if v.screensaverMode {
		// In screensaver mode, add a transparent mouse-detecting overlay
		// so mouse movement exits the screensaver (like a real screensaver).
		mouseOverlay := newMouseExitWidget(func() { v.Exit() })
		v.container = container.NewStack(
			v.term,
			container.NewBorder(nil, v.status, nil, nil),
			mouseOverlay,
		)
	} else {
		v.container = container.NewStack(
			v.term,
			container.NewBorder(nil, v.status, nil, nil),
		)
	}
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

// getNSWindow returns the native NSWindow handle, or 0 if not yet available.
func (v *Viewer) getNSWindow() uintptr {
	nw, ok := v.window.(driver.NativeWindow)
	if !ok {
		return 0
	}
	var handle uintptr
	nw.RunNative(func(ctx any) {
		switch c := ctx.(type) {
		case *driver.MacWindowContext:
			handle = c.NSWindow
		case driver.MacWindowContext:
			handle = c.NSWindow
		}
	})
	return handle
}

// waitForFullScreen blocks until the window canvas reports a size
// larger than the initial default, indicating fullscreen layout is done.
func (v *Viewer) waitForFullScreen() {
	deadline := time.After(5 * time.Second)
	// Phase 1: wait for canvas to be sized
	for {
		s := v.window.Canvas().Size()
		if s.Width > 200 && s.Height > 200 {
			log.Printf("Canvas ready: %v", s)
			break
		}
		select {
		case <-deadline:
			log.Printf("waitForFullScreen canvas timeout, size: %v", s)
			goto done
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// In screensaver mode, we need the NSWindow to exist for Canvas.Capture(),
	// but it must be hidden so the user only sees the ObjC screensaver view's
	// rendering of our piped frames.
	if v.screensaverMode {
		for {
			nsWindow := v.getNSWindow()
			if nsWindow != 0 {
				log.Printf("NSWindow ready: %v, hiding", nsWindow)
				done := make(chan struct{})
				fyne.Do(func() {
					HideNSWindow(nsWindow)
					close(done)
				})
				<-done
				break
			}
			select {
			case <-deadline:
				log.Println("waitForFullScreen NSWindow timeout")
				goto done
			default:
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
done:
}

// SetFrameOutput sets the writer where captured frames are sent
// (typically os.Stdout when launched from the ObjC screensaver wrapper).
// Each frame is written as: [4-byte big-endian length][PNG data]
func (v *Viewer) SetFrameOutput(w io.Writer) {
	v.frameOut = w
}

// captureFrames periodically captures the canvas and writes PNG frames
// to frameOut with a 4-byte big-endian length prefix.
func (v *Viewer) captureFrames() {
	time.Sleep(1 * time.Second)
	log.Println("Starting frame capture")
	for {
		select {
		case <-v.quit:
			return
		default:
		}
		img := v.window.Canvas().Capture()
		if img != nil {
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err == nil {
				length := uint32(buf.Len())
				if err := binary.Write(v.frameOut, binary.BigEndian, length); err != nil {
					log.Printf("Frame write error (length): %v", err)
					return
				}
				if _, err := v.frameOut.Write(buf.Bytes()); err != nil {
					log.Printf("Frame write error (data): %v", err)
					return
				}
			}
		}
		time.Sleep(100 * time.Millisecond) // ~10 fps
	}
}

// Start connects to NAO and loops, switching games on idle or game exit.
func (v *Viewer) Start() error {
	v.waitForFullScreen()
	v.cols, v.rows = v.terminalSize()
	log.Printf("Terminal size: %dx%d", v.cols, v.rows)

	// In screensaver mode, capture frames and send to the ObjC view via pipe
	if v.screensaverMode && v.frameOut != nil {
		go v.captureFrames()
	}

	failCount := 0
	for {
		select {
		case <-v.quit:
			return nil
		default:
		}

		server := v.servers[v.serverIdx]
		fyne.Do(func() {
			v.status.SetText(fmt.Sprintf("Connecting to %s...", server.Name))
		})

		reason, err := v.watchOne()
		if err != nil {
			log.Printf("[%s] No live game available: %v", server.Name, err)
			// Fall back to server ttyrec playback
			if ttyErr := v.playTTYRec(); ttyErr != nil {
				log.Printf("[%s] ttyrec fallback failed: %v", server.Name, ttyErr)
				failCount++
				v.rotateServer()

				// After cycling through all servers, fall back to bundled recording
				if failCount >= len(v.servers) {
					log.Println("All servers failed, falling back to bundled recording")
					if bundledErr := v.playBundledTTYRec(); bundledErr != nil {
						log.Printf("Bundled ttyrec failed: %v", bundledErr)
					}
					// Exponential backoff: 5s, 10s, 20s, 40s, capped at 60s
					delay := time.Duration(5<<min(failCount-len(v.servers), 3)) * time.Second
					if delay > 60*time.Second {
						delay = 60 * time.Second
					}
					fyne.Do(func() {
						v.status.SetText(fmt.Sprintf("No servers available — retrying in %s...", delay.Round(time.Second)))
					})
					time.Sleep(delay)
				} else {
					fyne.Do(func() {
						v.status.SetText(fmt.Sprintf("No games on %s — trying next server...", server.Name))
					})
					time.Sleep(3 * time.Second)
				}
			} else {
				failCount = 0 // ttyrec playback succeeded
			}
			continue
		}
		failCount = 0 // live game succeeded
		log.Printf("[%s] Switching game: %s", server.Name, reason)
		// Rotate server on each game switch for variety
		v.rotateServer()
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
	// Fresh terminal widget for each game. We must wait for Fyne to
	// lay out the widget before calling RunWithConnection, otherwise
	// FyneTerm's internal row/col counts won't match the grid size.
	v.term = terminal.New()
	layoutDone := make(chan struct{})
	fyne.Do(func() {
		v.container.Objects[0] = v.term
		v.container.Refresh()
		close(layoutDone)
	})
	<-layoutDone
	// Allow an extra layout cycle for the terminal to be fully sized
	time.Sleep(200 * time.Millisecond)

	client := v.currentClient()
	server := v.servers[v.serverIdx]
	client.Close()
	stdin, stdout, err := client.Connect(v.cols, v.rows)
	if err != nil {
		return 0, fmt.Errorf("connect to %s: %w", server.Name, err)
	}

	fyne.Do(func() {
		v.status.SetText(fmt.Sprintf("Connected to %s — selecting a game...", server.Name))
	})
	log.Printf("Connected to %s", server.Name)

	player, err := client.WatchRandomGame(v.lastPlayer)
	if err != nil {
		return 0, fmt.Errorf("select game: %w", err)
	}
	v.lastPlayer = player

	fyne.Do(func() {
		v.status.SetText(fmt.Sprintf("Watching %s on %s", player, server.Name))
	})
	log.Printf("Watching player: %s on %s", player, server.Name)

	// Wrap stdout with the monitor for idle/game-over detection
	monitor := nao.NewMonitoredReader(stdout, idleTimeout)
	defer monitor.Stop()

	// RunWithConnection blocks until the reader returns an error.
	// Run it in a goroutine so we can also wait on the switch channel.
	// Recover from FyneTerm panics (e.g. scrollUp index-out-of-range
	// when the widget hasn't been fully laid out).
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("FyneTerm panic (recovered): %v", r)
			}
			close(done)
		}()
		v.term.RunWithConnection(stdin, monitor)
	}()

	// FyneTerm skips Refresh() when there's leftover data in its read
	// buffer, which causes partial-update artifacts with curses games.
	// Periodically force a refresh to compensate.
	stopRefresh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fyne.Do(func() { v.term.Refresh() })
			case <-stopRefresh:
				return
			}
		}
	}()
	defer close(stopRefresh)

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

	client := v.currentClient()
	server := v.servers[v.serverIdx]

	// Get the player list to use as ttyrec candidates
	players, err := client.GetRecentPlayers(v.cols, v.rows)
	if err != nil {
		return fmt.Errorf("get players from %s: %w", server.Name, err)
	}
	if len(players) == 0 {
		return fmt.Errorf("no players found on %s", server.Name)
	}

	frames, label, err := nao.FetchRandomTTYRec(server, players)
	if err != nil {
		return fmt.Errorf("fetch ttyrec: %w", err)
	}
	log.Printf("Playing ttyrec: %s (%d frames)", label, len(frames))

	// Fresh terminal widget — wait for layout before starting playback
	v.term = terminal.New()
	layoutDone := make(chan struct{})
	fyne.Do(func() {
		v.container.Objects[0] = v.term
		v.container.Refresh()
		v.status.SetText(fmt.Sprintf("Replay: %s", label))
		close(layoutDone)
	})
	<-layoutDone
	time.Sleep(200 * time.Millisecond)

	player := ttyrec.NewPlayer(frames)
	defer player.Stop()

	// nopWriteCloser discards writes (ttyrec playback is read-only)
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("FyneTerm panic (recovered): %v", r)
			}
			close(done)
		}()
		v.term.RunWithConnection(nopWriteCloser{}, player)
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

// playBundledTTYRec plays the embedded ttyrec recording as a last-resort
// fallback when no servers are reachable.
func (v *Viewer) playBundledTTYRec() error {
	frames, err := nao.ParseBundledTTYRec()
	if err != nil {
		return fmt.Errorf("parse bundled ttyrec: %w", err)
	}
	log.Printf("Playing bundled ttyrec (%d frames)", len(frames))

	v.term = terminal.New()
	layoutDone := make(chan struct{})
	fyne.Do(func() {
		v.container.Objects[0] = v.term
		v.container.Refresh()
		v.status.SetText("Offline — playing bundled recording")
		close(layoutDone)
	})
	<-layoutDone
	time.Sleep(200 * time.Millisecond)

	player := ttyrec.NewPlayer(frames)
	defer player.Stop()

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("FyneTerm panic (recovered): %v", r)
			}
			close(done)
		}()
		v.term.RunWithConnection(nopWriteCloser{}, player)
	}()

	select {
	case <-done:
		log.Println("Bundled ttyrec playback finished")
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
