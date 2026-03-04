// Command pinentry-go is a Wayland-native GTK4 pinentry replacement that
// shows a per-key accent color so you always know which key you are unlocking.
package main

import (
	"errors"
	"io"
	"log"
	"os"
	"strings"

	"github.com/stefanv/pinentry-go/internal/dialog"
	"github.com/stefanv/pinentry-go/internal/pinentry"
)

func main() {
	log.SetPrefix("pinentry-go: ")
	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	// Use the Cairo renderer so GTK4 doesn't attempt Vulkan/GPU initialisation,
	// which produces verbose loader messages and is unnecessary for a short-lived dialog.
	os.Setenv("GSK_RENDERER", "cairo")

	d := dialog.New()

	// Run the Assuan server in a background goroutine; the GTK main loop
	// must own the main goroutine (GTK requirement).
	go func() {
		if err := pinentry.Serve(os.Stdin, os.Stdout, d); err != nil && !errors.Is(err, io.EOF) {
			log.Print(err)
		}
		d.Quit()
	}()

	if code := d.Run(gtkArgs(os.Args)); code > 0 {
		os.Exit(code)
	}
}

// gtkArgs filters out command-line arguments that gpg-agent passes for
// historical GTK2/3 compatibility but that GTK4 no longer accepts.
// Specifically, gpg-agent passes "--display :0" which GTK4 rejects with
// exit code 1.  The display is already available via the DISPLAY environment
// variable, so the argument is redundant.
func gtkArgs(args []string) []string {
	// Arguments to drop, each consuming itself and optionally a following value.
	drop := map[string]bool{
		"--display": true,
		"--screen":  true, // also GTK2/3-era, not used by GTK4
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		// Handle --flag=value form.
		key := arg
		if idx := strings.IndexByte(arg, '='); idx != -1 {
			key = arg[:idx]
		}
		if drop[key] {
			// If no '=' and the value is a separate argument, skip that too.
			if !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
			}
			continue
		}
		out = append(out, arg)
	}
	return out
}
