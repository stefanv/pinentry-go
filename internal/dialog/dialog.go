// Package dialog implements the pinentry.Presenter interface using GTK4.
//
// Architecture: a gtk.Application runs in the main goroutine (via Run).
// GetPin/Confirm/Message are called from the Assuan goroutine; they post a
// request to a channel and block until the GTK main loop processes it and
// posts a response.
package dialog

import (
	"fmt"
	"strings"
	"sync"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/stefanv/pinentry-go/internal/config"
	"github.com/stefanv/pinentry-go/internal/pinentry"
)

// requestKind identifies which dialog to show.
type requestKind int

const (
	kindGetPin  requestKind = iota
	kindConfirm             // CONFIRM command
	kindMessage             // MESSAGE command
)

// request is sent from the Assuan goroutine to the GTK main loop.
type request struct {
	settings  pinentry.Settings
	kind      requestKind
	oneButton bool         // CONFIRM --one-button
	respCh    chan response // buffered(1); sender never blocks
}

type response struct {
	pin string
	err error
}

// Dialog is the GTK4-backed Presenter implementation.
// Create with New(), then call Run() from the main goroutine.
type Dialog struct {
	app   *gtk.Application
	reqCh chan request
}

// New creates a new Dialog.
func New() *Dialog {
	// ApplicationNonUnique: each invocation of pinentry-go is an independent
	// process; we must not use D-Bus single-instance enforcement, otherwise a
	// second call by gpg-agent would just activate the first instance and exit
	// without showing a dialog.
	app := gtk.NewApplication("com.github.stefanv.pinentry_go", gio.ApplicationNonUnique)
	d := &Dialog{
		app:   app,
		reqCh: make(chan request),
	}
	app.ConnectActivate(d.onActivate)
	return d
}

// Run starts the GTK main loop; blocks until Quit is called.
// Must be called from the main goroutine.
func (d *Dialog) Run(args []string) int {
	return d.app.Run(args)
}

// Quit signals the dialog to stop accepting requests and quit.
// Safe to call from any goroutine.
func (d *Dialog) Quit() {
	close(d.reqCh)
}

// onActivate is the GTK activate signal handler (main goroutine).
func (d *Dialog) onActivate() {
	// Hold the app so it doesn't auto-quit when no windows are open.
	d.app.Hold()

	// Relay requests from the Assuan goroutine into the GTK main loop.
	go func() {
		for req := range d.reqCh {
			req := req
			glib.IdleAdd(func() { d.handleRequest(req) })
		}
		// Channel closed (BYE received) — release the hold and quit.
		glib.IdleAdd(func() { d.app.Release() })
	}()
}

// dispatch sends a request to the GTK main loop and waits for a response.
func (d *Dialog) dispatch(req request) response {
	req.respCh = make(chan response, 1)
	d.reqCh <- req
	return <-req.respCh
}

func (d *Dialog) handleRequest(req request) {
	switch req.kind {
	case kindGetPin:
		d.showGetPin(req)
	case kindConfirm:
		d.showConfirm(req)
	case kindMessage:
		d.showMessage(req)
	}
}

// GetPin implements pinentry.Presenter.
func (d *Dialog) GetPin(s pinentry.Settings) (string, error) {
	resp := d.dispatch(request{settings: s, kind: kindGetPin})
	return resp.pin, resp.err
}

// Confirm implements pinentry.Presenter.
func (d *Dialog) Confirm(s pinentry.Settings, oneButton bool) error {
	return d.dispatch(request{settings: s, kind: kindConfirm, oneButton: oneButton}).err
}

// Message implements pinentry.Presenter.
func (d *Dialog) Message(s pinentry.Settings) error {
	return d.dispatch(request{settings: s, kind: kindMessage}).err
}

// --- helpers ----------------------------------------------------------------

// loadStyle resolves the display style for the given key ID from config.
// Falls back to default style on any error.
func loadStyle(keyID string) config.Style {
	cfg, err := config.Load()
	if err != nil {
		return config.Style{Color: "#888888", Name: "Unknown key"}
	}
	return cfg.FindStyle(keyID)
}

// stripMnemonic removes GTK mnemonic underscores (e.g. "_OK" → "OK").
func stripMnemonic(s string) string {
	return strings.ReplaceAll(s, "_", "")
}

// orDefault returns s if non-empty, otherwise def.
func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// accentCSS returns CSS that colors a headerbar with class pinentry-header.
func accentCSS(color string) string {
	return fmt.Sprintf(
		"headerbar.pinentry-header { background: %[1]s; background-color: %[1]s; }"+
			" headerbar.pinentry-header * { color: white; }",
		color,
	)
}

// buildWindow creates an ApplicationWindow with a colored header bar.
// Returns the window, the header's key-name label (so callers can update it,
// e.g. for a timeout countdown), and a destroyOnce function that calls
// win.Destroy() exactly once regardless of how many times it is invoked.
func (d *Dialog) buildWindow(title string, st config.Style) (
	win *gtk.ApplicationWindow,
	keyLabel *gtk.Label,
	destroyOnce func(),
) {
	win = gtk.NewApplicationWindow(d.app)
	win.SetModal(true)
	win.SetResizable(false)
	win.SetDefaultSize(420, -1)

	header := gtk.NewHeaderBar()
	header.SetShowTitleButtons(true)

	keyLabel = gtk.NewLabel(st.Name)
	header.SetTitleWidget(keyLabel)

	provider := gtk.NewCSSProvider()
	provider.LoadFromString(accentCSS(st.Color))
	header.AddCSSClass("pinentry-header")
	header.StyleContext().AddProvider(provider, 600) // APPLICATION priority

	win.SetTitlebar(header)
	if title != "" {
		win.SetTitle(title)
	}

	var once sync.Once
	destroyOnce = func() { once.Do(win.Destroy) }
	return win, keyLabel, destroyOnce
}

// errorLabel creates a small red italic label (for SETERROR or mismatch text).
func errorLabel(msg string) *gtk.Label {
	lbl := gtk.NewLabel(msg)
	lbl.SetWrap(true)
	lbl.SetXAlign(0)
	lbl.SetHAlign(gtk.AlignStart)
	lbl.AddCSSClass("pinentry-error")
	p := gtk.NewCSSProvider()
	p.LoadFromString(".pinentry-error { color: #cc0000; font-style: italic; }")
	lbl.StyleContext().AddProvider(p, 600)
	return lbl
}

// promptRow creates a horizontal box with a right-aligned label and a
// password entry, with entries sized consistently across rows.
func promptRow(label string, entry *gtk.PasswordEntry) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	lbl := gtk.NewLabel(label)
	lbl.SetXAlign(1)
	lbl.SetWidthChars(14)
	entry.SetHExpand(true)
	entry.SetShowPeekIcon(true)
	row.Append(lbl)
	row.Append(entry)
	return row
}

// startCountdown ticks every second, updating keyLabel with a "Name (Ns)"
// suffix and calling cancel when it reaches zero.
func startCountdown(keyName string, seconds int, keyLabel *gtk.Label, cancel func()) {
	remaining := seconds
	keyLabel.SetLabel(fmt.Sprintf("%s (%ds)", keyName, remaining))
	glib.TimeoutAdd(1000, func() bool {
		remaining--
		if remaining <= 0 {
			cancel()
			return false // remove the timeout source
		}
		keyLabel.SetLabel(fmt.Sprintf("%s (%ds)", keyName, remaining))
		return true // keep ticking
	})
}

// --- dialog builders --------------------------------------------------------

func (d *Dialog) showGetPin(req request) {
	s := req.settings
	st := loadStyle(s.KeyID)
	win, keyLabel, destroyOnce := d.buildWindow(s.Title, st)

	var responded bool
	respondOnce := func(resp response) {
		if !responded {
			responded = true
			destroyOnce()
			req.respCh <- resp
		}
	}

	outer := gtk.NewBox(gtk.OrientationVertical, 10)
	outer.SetMarginTop(14)
	outer.SetMarginBottom(14)
	outer.SetMarginStart(18)
	outer.SetMarginEnd(18)

	if s.Desc != "" {
		desc := gtk.NewLabel(s.Desc)
		desc.SetWrap(true)
		desc.SetXAlign(0)
		desc.SetHAlign(gtk.AlignStart)
		outer.Append(desc)
	}

	if s.Error != "" {
		outer.Append(errorLabel(s.Error))
	}

	entry := gtk.NewPasswordEntry()
	outer.Append(promptRow(orDefault(stripMnemonic(s.Prompt), "Passphrase:"), entry))

	var repeatEntry *gtk.PasswordEntry
	if s.RepeatPrompt != "" {
		repeatEntry = gtk.NewPasswordEntry()
		outer.Append(promptRow(stripMnemonic(s.RepeatPrompt), repeatEntry))
	}

	// Mismatch label — hidden until a repeat mismatch occurs.
	mismatchLbl := errorLabel("Passphrases do not match")
	mismatchLbl.SetVisible(false)
	outer.Append(mismatchLbl)

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(4)

	cancelBtn := gtk.NewButtonWithMnemonic(orDefault(s.CancelBtn, "_Cancel"))
	okBtn := gtk.NewButtonWithMnemonic(orDefault(s.OkBtn, "_OK"))
	okBtn.AddCSSClass("suggested-action")

	btnBox.Append(cancelBtn)
	btnBox.Append(okBtn)
	outer.Append(btnBox)

	win.SetChild(outer)
	win.SetDefaultWidget(okBtn)
	win.SetFocus(entry)

	cancelBtn.ConnectClicked(func() {
		respondOnce(response{err: pinentry.ErrCanceled})
	})

	okBtn.ConnectClicked(func() {
		pin := entry.Text()
		if repeatEntry != nil && repeatEntry.Text() != pin {
			mismatchLbl.SetVisible(true)
			return
		}
		respondOnce(response{pin: pin})
	})

	entry.ConnectActivate(func() {
		if repeatEntry == nil {
			okBtn.Activate()
		} else {
			repeatEntry.GrabFocus()
		}
	})
	if repeatEntry != nil {
		repeatEntry.ConnectActivate(func() { okBtn.Activate() })
	}

	win.ConnectCloseRequest(func() (ok bool) {
		respondOnce(response{err: pinentry.ErrCanceled})
		return false
	})

	if secs := int(s.Timeout.Seconds()); secs > 0 {
		startCountdown(st.Name, secs, keyLabel, func() {
			respondOnce(response{err: pinentry.ErrCanceled})
		})
	}

	win.Present()
}

func (d *Dialog) showConfirm(req request) {
	s := req.settings
	st := loadStyle(s.KeyID)
	win, _, destroyOnce := d.buildWindow(s.Title, st)

	var responded bool
	respond := func(resp response) {
		if !responded {
			responded = true
			destroyOnce()
			req.respCh <- resp
		}
	}

	outer := gtk.NewBox(gtk.OrientationVertical, 10)
	outer.SetMarginTop(14)
	outer.SetMarginBottom(14)
	outer.SetMarginStart(18)
	outer.SetMarginEnd(18)

	if s.Desc != "" {
		desc := gtk.NewLabel(s.Desc)
		desc.SetWrap(true)
		desc.SetXAlign(0)
		outer.Append(desc)
	}

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(4)

	okBtn := gtk.NewButtonWithMnemonic(orDefault(s.OkBtn, "_OK"))
	okBtn.AddCSSClass("suggested-action")

	if !req.oneButton {
		if s.NotOkBtn != "" {
			notOkBtn := gtk.NewButtonWithMnemonic(s.NotOkBtn)
			notOkBtn.ConnectClicked(func() {
				respond(response{err: pinentry.ErrNotConfirmed})
			})
			btnBox.Append(notOkBtn)
		}
		cancelBtn := gtk.NewButtonWithMnemonic(orDefault(s.CancelBtn, "_Cancel"))
		cancelBtn.ConnectClicked(func() {
			respond(response{err: pinentry.ErrNotConfirmed})
		})
		btnBox.Append(cancelBtn)
	}

	okBtn.ConnectClicked(func() { respond(response{}) })
	btnBox.Append(okBtn)
	outer.Append(btnBox)

	win.SetChild(outer)
	win.SetDefaultWidget(okBtn)

	win.ConnectCloseRequest(func() (ok bool) {
		if req.oneButton {
			respond(response{})
		} else {
			respond(response{err: pinentry.ErrNotConfirmed})
		}
		return false
	})

	win.Present()
}

func (d *Dialog) showMessage(req request) {
	s := req.settings
	st := loadStyle(s.KeyID)
	win, _, destroyOnce := d.buildWindow(s.Title, st)

	var responded bool
	respond := func(resp response) {
		if !responded {
			responded = true
			destroyOnce()
			req.respCh <- resp
		}
	}

	outer := gtk.NewBox(gtk.OrientationVertical, 10)
	outer.SetMarginTop(14)
	outer.SetMarginBottom(14)
	outer.SetMarginStart(18)
	outer.SetMarginEnd(18)

	if s.Desc != "" {
		desc := gtk.NewLabel(s.Desc)
		desc.SetWrap(true)
		desc.SetXAlign(0)
		outer.Append(desc)
	}

	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(4)

	okBtn := gtk.NewButtonWithMnemonic(orDefault(s.OkBtn, "_OK"))
	okBtn.AddCSSClass("suggested-action")
	okBtn.ConnectClicked(func() { respond(response{}) })
	btnBox.Append(okBtn)
	outer.Append(btnBox)

	win.SetChild(outer)
	win.SetDefaultWidget(okBtn)

	win.ConnectCloseRequest(func() (ok bool) {
		respond(response{})
		return false
	})

	win.Present()
}
