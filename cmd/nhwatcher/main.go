package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/pneumaticdeath/NH_Watcher/internal/nao"
	"github.com/pneumaticdeath/NH_Watcher/internal/screen"
)

// Preference keys for the menu-bar app. The screensaver and standalone
// modes ignore prefs; they use the CLI flags (or default to all servers).
const (
	prefServerNAO = "server.nao"
	prefServerHUS = "server.hdf-us"
	prefServerHEU = "server.hdf-eu"
	prefServerHAU = "server.hdf-au"
)

func main() {
	screensaverFlag := flag.Bool("screensaver", false, "screensaver-wrapper mode (PNG frames on stdout, any key/mouse exits)")
	standaloneFlag := flag.Bool("standalone", false, "single fullscreen window, ESC/Q to quit")
	serverFlag := flag.String("servers", "", "comma-separated server list: nao,hdf-us,hdf-eu,hdf-au (menu-bar: overrides Settings)")
	flag.Parse()

	// Auto-detect screensaver mode from executable path or env so the
	// ObjC .saver wrapper doesn't have to pass the flag explicitly.
	if !*screensaverFlag {
		exe, _ := os.Executable()
		if strings.Contains(exe, ".saver/") || os.Getenv("NHWATCHER_SCREENSAVER") == "1" {
			*screensaverFlag = true
		}
	}

	setupLog(*screensaverFlag)

	a := app.NewWithID("io.patenaude.nhwatcher")
	a.Settings().SetTheme(&screen.DarkTermTheme{})

	switch {
	case *screensaverFlag:
		runScreensaver(a, selectServers(*serverFlag))
	case *standaloneFlag:
		runStandalone(a, selectServers(*serverFlag))
	default:
		runMenuBar(a, *serverFlag)
	}
}

// setupLog routes log output to a file. In screensaver mode stdout is the
// frame pipe, so we must not log there.
func setupLog(screensaver bool) {
	logPath := filepath.Join(os.TempDir(), "nhwatcher_debug.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	log.SetOutput(f)
	log.Printf("=== nhwatcher started, PID=%d, PPID=%d, screensaver=%v, args=%v",
		os.Getpid(), os.Getppid(), screensaver, os.Args)
}

func selectServers(flagVal string) []nao.ServerConfig {
	servers := nao.AllServers
	if flagVal != "" {
		keys := strings.Split(flagVal, ",")
		if s := nao.ServersByKey(keys); len(s) > 0 {
			servers = s
		} else {
			log.Printf("Unknown server keys %q, using all servers", flagVal)
		}
	}
	names := make([]string, len(servers))
	for i, s := range servers {
		names[i] = s.Name
	}
	log.Printf("Servers: %v", names)
	return servers
}

// runScreensaver: hidden 1920x1080 window, PNG frames piped to stdout for
// the ObjC .saver wrapper to render.
func runScreensaver(a fyne.App, servers []nao.ServerConfig) {
	w := a.NewWindow("NH Watcher")
	w.Resize(fyne.NewSize(1920, 1080))

	viewer := screen.NewViewer(w, servers, true)
	viewer.SetOnQuit(func() {
		log.Println("onQuit: quitting Fyne app")
		fyne.Do(func() { a.Quit() })
	})
	viewer.SetFrameOutput(os.Stdout)
	w.SetContent(viewer.Content())
	w.SetCloseIntercept(func() { viewer.Exit() })

	installSignalHandler(viewer.Exit)

	// Exit if our parent (NSTask host) dies; the screensaver appex can fail
	// to deliver SIGTERM cleanly. Safety timeout caps runaway sessions.
	parentPID := os.Getppid()
	log.Printf("Watchdog: parentPID=%d", parentPID)
	go func() {
		for {
			time.Sleep(1 * time.Second)
			if os.Getppid() != parentPID {
				log.Println("Parent process died, shutting down")
				viewer.Exit()
				return
			}
		}
	}()
	go func() {
		time.Sleep(30 * time.Minute)
		log.Println("Safety timeout reached (30m), force exiting")
		viewer.Exit()
	}()

	go func() {
		if err := viewer.Start(); err != nil {
			log.Printf("viewer error: %v", err)
		}
	}()

	w.ShowAndRun()
}

// runStandalone: single fullscreen window, ESC/Q quits. Useful for `make run`
// and for manual testing of the viewer outside the screensaver path.
func runStandalone(a fyne.App, servers []nao.ServerConfig) {
	w := a.NewWindow("NH Watcher")
	w.SetFullScreen(true)

	viewer := screen.NewViewer(w, servers, false)
	viewer.SetOnQuit(func() { fyne.Do(func() { a.Quit() }) })
	w.SetContent(viewer.Content())
	w.SetCloseIntercept(func() { viewer.Exit() })

	installSignalHandler(viewer.Exit)

	go func() {
		if err := viewer.Start(); err != nil {
			log.Printf("viewer error: %v", err)
		}
	}()

	w.ShowAndRun()
}

// runMenuBar: tray icon "Watch now" launcher plus a Settings window for
// server selection. No idle detection — auto-activation is the OS
// screensaver's job. Settings changes apply on the next Watch now.
func runMenuBar(a fyne.App, cliServers string) {
	desk, ok := a.(desktop.App)
	if !ok {
		log.Fatal("menu-bar mode requires a desktop driver (system tray)")
	}

	m := &menuBar{
		app:        a,
		cliServers: cliServers,
		desk:       desk,
	}

	menu := fyne.NewMenu("NH Watcher",
		fyne.NewMenuItem("Watch now", func() { go m.activate() }),
		fyne.NewMenuItem("Settings…", func() { go m.showSettings() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() {
			m.deactivate()
			a.Quit()
		}),
	)
	desk.SetSystemTrayMenu(menu)

	if icon := loadTrayIcon(); icon != nil {
		desk.SetSystemTrayIcon(icon)
	}

	installSignalHandler(func() {
		m.deactivate()
		a.Quit()
	})

	a.Run()
}

type menuBar struct {
	app        fyne.App
	cliServers string // CLI --servers override ("" = use prefs)
	desk       desktop.App

	mu          sync.Mutex
	curWin      fyne.Window
	curView     *screen.Viewer
	settingsWin fyne.Window // open Settings window, nil if closed
}

// effectiveServers returns the live server list: CLI override if set,
// otherwise the union of enabled servers from prefs. If prefs leave nothing
// enabled, falls back to AllServers.
func (m *menuBar) effectiveServers() []nao.ServerConfig {
	if m.cliServers != "" {
		if s := nao.ServersByKey(strings.Split(m.cliServers, ",")); len(s) > 0 {
			return s
		}
		return nao.AllServers
	}
	prefs := m.app.Preferences()
	var keys []string
	if prefs.BoolWithFallback(prefServerNAO, true) {
		keys = append(keys, "nao")
	}
	if prefs.BoolWithFallback(prefServerHUS, true) {
		keys = append(keys, "hdf-us")
	}
	if prefs.BoolWithFallback(prefServerHEU, true) {
		keys = append(keys, "hdf-eu")
	}
	if prefs.BoolWithFallback(prefServerHAU, true) {
		keys = append(keys, "hdf-au")
	}
	if s := nao.ServersByKey(keys); len(s) > 0 {
		return s
	}
	return nao.AllServers
}

func (m *menuBar) activate() {
	m.mu.Lock()
	if m.curWin != nil {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	servers := m.effectiveServers()

	var w fyne.Window
	var v *screen.Viewer
	fyne.DoAndWait(func() {
		// LSUIElement apps don't auto-foreground from background goroutines.
		screen.ActivateApp()
		w = m.app.NewWindow("NH Watcher")
		w.SetFullScreen(true)
		v = screen.NewViewer(w, servers, true)
		v.SetOnQuit(m.deactivate)
		w.SetContent(v.Content())
		w.SetCloseIntercept(func() { v.Exit() })
		w.Show()
		w.RequestFocus()
	})

	m.mu.Lock()
	m.curWin = w
	m.curView = v
	m.mu.Unlock()

	go func() {
		if err := v.Start(); err != nil {
			log.Printf("viewer error: %v", err)
		}
	}()
	log.Println("Menu-bar: viewer activated")
}

func (m *menuBar) deactivate() {
	m.mu.Lock()
	w := m.curWin
	v := m.curView
	m.curWin = nil
	m.curView = nil
	m.mu.Unlock()
	if v != nil {
		v.Exit()
	}
	if w != nil {
		fyne.Do(func() { w.Close() })
	}
	log.Println("Menu-bar: viewer deactivated")
}

// showSettings opens the Settings window. If one is already open, it is
// raised. All construction is marshalled to the main goroutine.
func (m *menuBar) showSettings() {
	m.mu.Lock()
	existing := m.settingsWin
	m.mu.Unlock()
	if existing != nil {
		fyne.DoAndWait(func() {
			screen.ActivateApp()
			existing.Show()
			existing.RequestFocus()
		})
		return
	}

	prefs := m.app.Preferences()

	var win fyne.Window
	fyne.DoAndWait(func() {
		naoChk := widget.NewCheck("nethack.alt.org (NAO)", nil)
		hdfUSChk := widget.NewCheck("us.hardfought.org", nil)
		hdfEUChk := widget.NewCheck("eu.hardfought.org", nil)
		hdfAUChk := widget.NewCheck("au.hardfought.org", nil)
		naoChk.Checked = prefs.BoolWithFallback(prefServerNAO, true)
		hdfUSChk.Checked = prefs.BoolWithFallback(prefServerHUS, true)
		hdfEUChk.Checked = prefs.BoolWithFallback(prefServerHEU, true)
		hdfAUChk.Checked = prefs.BoolWithFallback(prefServerHAU, true)

		win = m.app.NewWindow("NH Watcher Settings")

		close := func() {
			m.mu.Lock()
			m.settingsWin = nil
			m.mu.Unlock()
			win.Close()
		}

		save := widget.NewButton("Save", func() {
			if !(naoChk.Checked || hdfUSChk.Checked || hdfEUChk.Checked || hdfAUChk.Checked) {
				naoChk.SetChecked(true)
			}
			prefs.SetBool(prefServerNAO, naoChk.Checked)
			prefs.SetBool(prefServerHUS, hdfUSChk.Checked)
			prefs.SetBool(prefServerHEU, hdfEUChk.Checked)
			prefs.SetBool(prefServerHAU, hdfAUChk.Checked)
			log.Println("Settings: saved")
			close()
		})
		save.Importance = widget.HighImportance
		cancel := widget.NewButton("Cancel", close)

		content := container.NewVBox(
			widget.NewLabelWithStyle("Servers", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			naoChk, hdfUSChk, hdfEUChk, hdfAUChk,
			layout.NewSpacer(),
			widget.NewSeparator(),
			container.NewHBox(layout.NewSpacer(), cancel, save),
		)
		win.SetContent(container.NewPadded(content))
		win.Resize(fyne.NewSize(320, 260))
		win.SetCloseIntercept(close)

		screen.ActivateApp()
		win.Show()
		win.RequestFocus()
	})

	m.mu.Lock()
	m.settingsWin = win
	m.mu.Unlock()
}

// loadTrayIcon searches likely paths for icon.png so dev builds and the
// packaged .app bundle both get a sensible tray icon. Falls back to a Fyne
// stock icon if nothing is found.
func loadTrayIcon() fyne.Resource {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "..", "Resources", "icon.png"),
			filepath.Join(dir, "icon.png"),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "icon.png"))
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil {
			return fyne.NewStaticResource("nhwatcher-tray", data)
		}
	}
	return theme.ComputerIcon()
}

func installSignalHandler(onSignal func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		log.Println("Signal received, shutting down")
		onSignal()
	}()
}
