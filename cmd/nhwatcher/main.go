package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/pneumaticdeath/NH_Watcher/internal/nao"
	"github.com/pneumaticdeath/NH_Watcher/internal/screen"
)

func main() {
	screensaverMode := flag.Bool("screensaver", false, "run in screensaver mode (frames on stdout, any key exits)")
	serverFlag := flag.String("servers", "", "comma-separated server list: nao,hdf-us,hdf-eu,hdf-au (default: all)")
	flag.Parse()

	// Auto-detect screensaver mode from executable path or env
	if !*screensaverMode {
		exe, _ := os.Executable()
		if strings.Contains(exe, ".saver/") || os.Getenv("NHWATCHER_SCREENSAVER") == "1" {
			*screensaverMode = true
		}
	}

	// Log to file (stdout is used for frame data in screensaver mode)
	if f, err := os.OpenFile("/tmp/nhwatcher_debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		log.SetOutput(f)
		log.Printf("=== nhwatcher started, PID=%d, PPID=%d, screensaver=%v, args=%v", os.Getpid(), os.Getppid(), *screensaverMode, os.Args)
		for _, key := range []string{"HOME", "USER", "TMPDIR", "APP_SANDBOX_CONTAINER_ID"} {
			log.Printf("ENV %s=%s", key, os.Getenv(key))
		}
	}

	a := app.New()
	a.Settings().SetTheme(&screen.DarkTermTheme{})

	var w fyne.Window
	if *screensaverMode {
		drv := a.Driver().(desktop.Driver)
		w = drv.CreateSplashWindow()
		sw, sh := screen.ScreenSize()
		w.Resize(fyne.NewSize(float32(sw), float32(sh)))
	} else {
		w = a.NewWindow("NH Watcher")
		w.SetFullScreen(true)
	}

	// Select servers
	servers := nao.AllServers
	if *serverFlag != "" {
		keys := strings.Split(*serverFlag, ",")
		if s := nao.ServersByKey(keys); len(s) > 0 {
			servers = s
		} else {
			log.Printf("Unknown server keys %q, using all servers", *serverFlag)
		}
	}
	log.Printf("Servers: %v", func() []string {
		names := make([]string, len(servers))
		for i, s := range servers {
			names[i] = s.Name
		}
		return names
	}())

	viewer := screen.NewViewer(w, servers, *screensaverMode)

	// When the viewer exits, quit the Fyne app.
	viewer.SetOnQuit(func() {
		log.Println("onQuit: quitting Fyne app")
		fyne.Do(func() { a.Quit() })
	})

	// In screensaver mode, send captured frames to stdout
	if *screensaverMode {
		viewer.SetFrameOutput(os.Stdout)
	}

	w.SetContent(viewer.Content())

	w.SetCloseIntercept(func() {
		viewer.Exit()
	})

	// Handle SIGTERM/SIGINT
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		log.Println("Signal received, shutting down")
		viewer.Exit()
	}()

	// In screensaver mode, exit when parent dies (NSTask termination)
	// and add a safety timeout.
	if *screensaverMode {
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
	}

	go func() {
		if err := viewer.Start(); err != nil {
			log.Printf("viewer error: %v", err)
		}
	}()

	w.ShowAndRun()
}
