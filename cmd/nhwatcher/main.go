package main

import (
	"log"

	"fyne.io/fyne/v2/app"
	"github.com/pneumaticdeath/NH_Watcher/internal/nao"
	"github.com/pneumaticdeath/NH_Watcher/internal/screen"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(&screen.DarkTermTheme{})
	w := a.NewWindow("NH Watcher")

	// Start fullscreen for screensaver-like behavior
	w.SetFullScreen(true)

	client := nao.NewClient()
	viewer := screen.NewViewer(w, client)

	w.SetContent(viewer.Content())

	w.SetCloseIntercept(func() {
		client.Close()
		w.Close()
	})

	go func() {
		if err := viewer.Start(); err != nil {
			log.Printf("viewer error: %v", err)
		}
	}()

	w.ShowAndRun()
}
