// Package pinentry implements the pinentry-go Assuan server.
//
// It uses github.com/foxcpp/go-assuan/server for all protocol framing (I/O,
// line parsing, BYE, NOP, OPTION, HELP, RESET) and registers its own handlers
// for the pinentry-specific commands (SET*, GETINFO, GETPIN, CONFIRM, MESSAGE).
//
// The state type is our own Settings struct (which embeds the library's
// pinentry.Settings and adds KeyID) so that all handlers share a single,
// consistent type.
package pinentry

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/foxcpp/go-assuan/common"
	"github.com/foxcpp/go-assuan/pinentry"
	"github.com/foxcpp/go-assuan/server"
)

// Presenter is the interface implemented by the GTK4 dialog layer.
// All methods are called from the Assuan goroutine; implementations that run
// a GUI must marshal to the GUI thread themselves.
type Presenter interface {
	// GetPin shows a password-entry dialog and returns the passphrase.
	// Returns ("", ErrCanceled) when the user cancels.
	GetPin(s Settings) (string, error)

	// Confirm shows a yes/no (or one-button) dialog.
	// Returns nil on confirmation, ErrNotConfirmed on decline/cancel.
	Confirm(s Settings, oneButton bool) error

	// Message shows an informational dialog with a single OK button.
	Message(s Settings) error
}

// ErrCanceled is returned by Presenter.GetPin when the user cancels.
var ErrCanceled = errors.New("operation cancelled")

// ErrNotConfirmed is returned by Presenter.Confirm when the user declines.
var ErrNotConfirmed = errors.New("not confirmed")

// Settings extends the library's pinentry.Settings with the key identifier
// received via SETKEYINFO, which we use to look up the per-key accent color.
type Settings struct {
	pinentry.Settings
	// KeyID is the raw value from SETKEYINFO, e.g. "n/DEADBEEF".
	// Use this with config.FindStyle to get the accent color.
	KeyID string
}

// version is set via -ldflags at build time; falls back to "dev".
var version = "dev"

// Serve starts the Assuan server, reading from r and writing to w, delegating
// UI presentation to p.  It blocks until the client sends BYE or EOF.
func Serve(r io.Reader, w io.Writer, p Presenter) error {
	rw := struct {
		io.Reader
		io.Writer
	}{r, w}
	return server.Serve(rw, buildProtoInfo(p))
}

// assuanErr is a convenience constructor for common.Error values.
func assuanErr(src common.ErrorSource, code common.ErrorCode, msg string) *common.Error {
	return &common.Error{Src: src, Code: code, SrcName: "pinentry", Message: msg}
}

// buildProtoInfo constructs the server.ProtoInfo for our pinentry.
// The library's server.Serve handles BYE, NOP, OPTION (via SetOption),
// HELP, and RESET (via our "RESET" handler entry).
func buildProtoInfo(p Presenter) server.ProtoInfo {
	h := make(map[string]server.CommandHandler)

	// -- Simple SET* commands: store the unescaped parameter in state. --

	setter := func(assign func(*Settings, string)) server.CommandHandler {
		return func(_ io.ReadWriter, state interface{}, params string) *common.Error {
			assign(state.(*Settings), params)
			return nil
		}
	}

	h["SETTITLE"] = setter(func(s *Settings, v string) { s.Title = v })
	h["SETDESC"] = setter(func(s *Settings, v string) { s.Desc = v })
	h["SETPROMPT"] = setter(func(s *Settings, v string) { s.Prompt = v })
	h["SETERROR"] = setter(func(s *Settings, v string) { s.Error = v })
	h["SETOK"] = setter(func(s *Settings, v string) { s.OkBtn = v })
	h["SETCANCEL"] = setter(func(s *Settings, v string) { s.CancelBtn = v })
	h["SETNOTOK"] = setter(func(s *Settings, v string) { s.NotOkBtn = v })
	h["SETQUALITYBAR"] = setter(func(s *Settings, v string) { s.QualityBar = v })
	h["SETQUALITYBAR_TT"] = setter(func(_ *Settings, _ string) {}) // acknowledged, ignored
	h["SETGENPIN"] = setter(func(_ *Settings, _ string) {})
	h["SETGENPIN_TT"] = setter(func(_ *Settings, _ string) {})

	h["SETREPEAT"] = func(_ io.ReadWriter, state interface{}, params string) *common.Error {
		label := params
		if label == "" {
			label = "Repeat passphrase:"
		}
		state.(*Settings).RepeatPrompt = label
		return nil
	}

	h["SETREPEATERROR"] = setter(func(s *Settings, v string) { s.RepeatError = v })

	h["SETTIMEOUT"] = func(_ io.ReadWriter, state interface{}, params string) *common.Error {
		var secs int
		if _, err := fmt.Sscanf(params, "%d", &secs); err != nil {
			return assuanErr(common.ErrSrcPinentry, common.ErrAssInvValue, "invalid timeout value")
		}
		state.(*Settings).Timeout = time.Duration(secs) * time.Second
		return nil
	}

	h["SETKEYINFO"] = setter(func(s *Settings, v string) { s.KeyID = v })

	h["RESET"] = func(_ io.ReadWriter, state interface{}, _ string) *common.Error {
		*state.(*Settings) = Settings{}
		return nil
	}

	// -- GETINFO --

	h["GETINFO"] = func(pipe io.ReadWriter, _ interface{}, params string) *common.Error {
		var reply string
		switch strings.TrimSpace(params) {
		case "flavor":
			reply = "gtk4"
		case "version":
			reply = version
		case "ttyinfo":
			reply = "- - -"
		case "pid":
			reply = strconv.Itoa(os.Getpid())
		default:
			return assuanErr(common.ErrSrcPinentry, common.ErrAssInvValue,
				"unknown GETINFO parameter: "+params)
		}
		common.WriteData(pipe, []byte(reply))
		return nil
	}

	// -- Action commands --

	h["GETPIN"] = func(pipe io.ReadWriter, state interface{}, _ string) *common.Error {
		s := state.(*Settings)
		pin, err := p.GetPin(*s)
		s.Error = "" // clear error after each attempt (standard pinentry behaviour)
		if errors.Is(err, ErrCanceled) {
			return assuanErr(common.ErrSrcPinentry, common.ErrCanceled, "Operation cancelled")
		}
		if err != nil {
			return assuanErr(common.ErrSrcPinentry, common.ErrCanceled, err.Error())
		}
		common.WriteData(pipe, []byte(pin))
		return nil
	}

	h["CONFIRM"] = func(_ io.ReadWriter, state interface{}, params string) *common.Error {
		oneButton := strings.Contains(params, "--one-button")
		err := p.Confirm(*state.(*Settings), oneButton)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrNotConfirmed) {
			return assuanErr(common.ErrSrcPinentry, common.ErrNotConfirmed, "Not confirmed")
		}
		return assuanErr(common.ErrSrcPinentry, common.ErrCanceled, "Operation cancelled")
	}

	h["MESSAGE"] = func(_ io.ReadWriter, state interface{}, _ string) *common.Error {
		if err := p.Message(*state.(*Settings)); err != nil {
			return assuanErr(common.ErrSrcPinentry, common.ErrCanceled, err.Error())
		}
		return nil
	}

	return server.ProtoInfo{
		Greeting: fmt.Sprintf("Pleased to meet you, process %d", os.Getpid()),
		Handlers: h,
		Help:     map[string][]string{},
		GetDefaultState: func() interface{} {
			return &Settings{}
		},
		// Accept all OPTION values silently; we don't need them for display.
		SetOption: func(_ interface{}, _, _ string) *common.Error { return nil },
	}
}

