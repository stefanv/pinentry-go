// Package dialog implements the pinentry.Presenter interface using GTK4.
package dialog

import "github.com/stefanv/pinentry-go/internal/pinentry"

// Dialog is the GTK4-backed Presenter implementation.
// Use New() to create one.
type Dialog struct{}

// New returns a new Dialog ready to present GTK4 windows.
func New() *Dialog {
	return &Dialog{}
}

// GetPin shows a GTK4 password-entry dialog.
// Full implementation in Phase 4.
func (d *Dialog) GetPin(s pinentry.Settings) (string, error) {
	panic("dialog: not yet implemented")
}

// Confirm shows a GTK4 confirmation dialog.
// Full implementation in Phase 4.
func (d *Dialog) Confirm(s pinentry.Settings, oneButton bool) error {
	panic("dialog: not yet implemented")
}

// Message shows a GTK4 informational dialog.
// Full implementation in Phase 4.
func (d *Dialog) Message(s pinentry.Settings) error {
	panic("dialog: not yet implemented")
}
