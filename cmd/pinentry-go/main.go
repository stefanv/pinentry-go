// Command pinentry-go is a Wayland-native GTK4 pinentry replacement that
// shows a per-key accent color so you always know which key you are unlocking.
package main

import (
	"log"
	"os"

	"github.com/stefanv/pinentry-go/internal/dialog"
	"github.com/stefanv/pinentry-go/internal/pinentry"
)

func main() {
	log.SetPrefix("pinentry-go: ")
	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	d := dialog.New()

	// Run the Assuan server in a background goroutine; the GTK main loop
	// must own the main goroutine (GTK requirement).
	go func() {
		if err := pinentry.Serve(os.Stdin, os.Stdout, d); err != nil {
			log.Print(err)
		}
		d.Quit()
	}()

	if code := d.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
