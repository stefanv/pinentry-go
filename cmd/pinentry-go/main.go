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

	p := dialog.New()
	if err := pinentry.Serve(os.Stdin, os.Stdout, p); err != nil {
		log.Fatal(err)
	}
}
