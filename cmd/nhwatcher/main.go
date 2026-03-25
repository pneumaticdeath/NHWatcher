package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"fyne.io/fyne/v2/app"
	"github.com/pneumaticdeath/NH_Watcher/internal/nao"
	"github.com/pneumaticdeath/NH_Watcher/internal/screen"
)

func main() {
	screensaverMode := flag.Bool("screensaver", false, "run in screensaver mode (any key exits)")
	flag.Parse()

	a := app.New()
	a.Settings().SetTheme(&screen.DarkTermTheme{})
	w := a.NewWindow("NH Watcher")

	// Start fullscreen for screensaver-like behavior
	w.SetFullScreen(true)

	client := nao.NewClient()
	viewer := screen.NewViewer(w, client, *screensaverMode)

	w.SetContent(viewer.Content())

	w.SetCloseIntercept(func() {
		client.Close()
		w.Close()
	})

	// Handle SIGTERM (sent by screensaver wrapper on deactivation)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		log.Println("Signal received, shutting down")
		client.Close()
		a.Quit()
	}()

	go func() {
		if err := viewer.Start(); err != nil {
			log.Printf("viewer error: %v", err)
		}
	}()

	w.ShowAndRun()
}
