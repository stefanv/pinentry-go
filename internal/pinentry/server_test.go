package pinentry

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

// mockPresenter records calls made to it and returns pre-configured responses.
type mockPresenter struct {
	pin         string
	pinErr      error
	confirmErr  error
	messageErr  error
	lastGetPin  *Settings
	lastConfirm *Settings
	lastMessage *Settings
	oneButton   bool
}

func (m *mockPresenter) GetPin(s Settings) (string, error) {
	m.lastGetPin = &s
	return m.pin, m.pinErr
}

func (m *mockPresenter) Confirm(s Settings, oneButton bool) error {
	m.lastConfirm = &s
	m.oneButton = oneButton
	return m.confirmErr
}

func (m *mockPresenter) Message(s Settings) error {
	m.lastMessage = &s
	return m.messageErr
}

// session drives a fake client conversation over in-process pipes.
type session struct {
	t      *testing.T
	stdin  *io.PipeWriter  // we write commands here
	stdout *io.PipeReader  // we read responses here
	scnr   *bufio.Scanner
}

func newSession(t *testing.T, mock *mockPresenter) *session {
	t.Helper()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	go func() {
		if err := Serve(stdinR, stdoutW, mock); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			// Ignore pipe-closed errors that happen on BYE/test teardown.
		}
		stdoutW.Close()
	}()

	s := &session{t: t, stdin: stdinW, stdout: stdoutR}
	s.scnr = bufio.NewScanner(stdoutR)
	// Consume greeting.
	s.readLine()
	return s
}

func (s *session) send(cmd string) {
	s.t.Helper()
	if _, err := io.WriteString(s.stdin, cmd+"\n"); err != nil {
		s.t.Fatalf("send %q: %v", cmd, err)
	}
}

func (s *session) readLine() string {
	s.t.Helper()
	if !s.scnr.Scan() {
		s.t.Fatalf("unexpected end of server output")
	}
	return s.scnr.Text()
}

func (s *session) expectOK() {
	s.t.Helper()
	line := s.readLine()
	if line != "OK" && !strings.HasPrefix(line, "OK ") {
		s.t.Errorf("expected OK, got %q", line)
	}
}

func (s *session) expectERR(wantSubstr string) {
	s.t.Helper()
	line := s.readLine()
	if !strings.HasPrefix(line, "ERR") {
		s.t.Errorf("expected ERR, got %q", line)
		return
	}
	if wantSubstr != "" && !strings.Contains(line, wantSubstr) {
		s.t.Errorf("ERR line %q does not contain %q", line, wantSubstr)
	}
}

func (s *session) expectData(wantSubstr string) string {
	s.t.Helper()
	line := s.readLine()
	if !strings.HasPrefix(line, "D ") {
		s.t.Errorf("expected D line, got %q", line)
		return ""
	}
	if wantSubstr != "" && !strings.Contains(line, wantSubstr) {
		s.t.Errorf("D line %q does not contain %q", line, wantSubstr)
	}
	return strings.TrimPrefix(line, "D ")
}

func (s *session) close() {
	s.stdin.Close()
}

// ---- tests ----------------------------------------------------------------

func TestNOP(t *testing.T) {
	ses := newSession(t, &mockPresenter{})
	defer ses.close()
	ses.send("NOP")
	ses.expectOK()
}

func TestBYE(t *testing.T) {
	ses := newSession(t, &mockPresenter{})
	ses.send("BYE")
	ses.expectOK()
}

func TestUnknownCommand(t *testing.T) {
	ses := newSession(t, &mockPresenter{})
	defer ses.close()
	ses.send("FROBNICATOR")
	ses.expectERR("")
}

func TestGetinfo(t *testing.T) {
	ses := newSession(t, &mockPresenter{})
	defer ses.close()

	for _, tc := range []struct{ param, want string }{
		{"flavor", "gtk4"},
		{"version", version},
		{"ttyinfo", "- - -"},
	} {
		ses.send("GETINFO " + tc.param)
		got := ses.expectData(tc.want)
		ses.expectOK()
		if got != tc.want {
			t.Errorf("GETINFO %s = %q, want %q", tc.param, got, tc.want)
		}
	}
}

func TestGetinfo_PID(t *testing.T) {
	ses := newSession(t, &mockPresenter{})
	defer ses.close()
	ses.send("GETINFO pid")
	line := ses.expectData("")
	ses.expectOK()
	if line == "" {
		t.Error("expected non-empty PID")
	}
}

func TestGetinfo_Unknown(t *testing.T) {
	ses := newSession(t, &mockPresenter{})
	defer ses.close()
	ses.send("GETINFO nonsense")
	ses.expectERR("")
}

func TestSetcommands(t *testing.T) {
	ses := newSession(t, &mockPresenter{pin: "s3cr3t"})
	defer ses.close()

	commands := []string{
		"SETTITLE My Title",
		"SETDESC Please enter your passphrase",
		"SETPROMPT Passphrase:",
		"SETOK OK",
		"SETCANCEL Cancel",
		"SETNOTOK No",
		"SETERROR Wrong passphrase",
		"SETKEYINFO n/DEADBEEF",
		"SETREPEAT Repeat:",
		"SETTIMEOUT 30",
	}
	for _, cmd := range commands {
		ses.send(cmd)
		ses.expectOK()
	}
}

func TestGetpin_Success(t *testing.T) {
	mock := &mockPresenter{pin: "hunter2"}
	ses := newSession(t, mock)
	defer ses.close()

	ses.send("SETDESC Enter passphrase")
	ses.expectOK()
	ses.send("SETKEYINFO n/AABBCCDD")
	ses.expectOK()

	ses.send("GETPIN")
	data := ses.expectData("hunter2")
	ses.expectOK()

	if data != "hunter2" {
		t.Errorf("pin = %q, want hunter2", data)
	}
	if mock.lastGetPin == nil {
		t.Fatal("GetPin was not called")
	}
	if mock.lastGetPin.KeyID != "n/AABBCCDD" {
		t.Errorf("KeyID = %q, want n/AABBCCDD", mock.lastGetPin.KeyID)
	}
}

func TestGetpin_Canceled(t *testing.T) {
	ses := newSession(t, &mockPresenter{pinErr: ErrCanceled})
	defer ses.close()
	ses.send("GETPIN")
	ses.expectERR("cancelled")
}

func TestGetpin_ClearsError(t *testing.T) {
	// SETERROR should be cleared after GETPIN even on success.
	mock := &mockPresenter{pin: "abc"}
	ses := newSession(t, mock)
	defer ses.close()

	ses.send("SETERROR Bad passphrase")
	ses.expectOK()
	ses.send("GETPIN")
	ses.expectData("")
	ses.expectOK()

	if mock.lastGetPin.Error != "Bad passphrase" {
		t.Errorf("Error field not passed to GetPin: got %q", mock.lastGetPin.Error)
	}
}

func TestConfirm_OK(t *testing.T) {
	ses := newSession(t, &mockPresenter{confirmErr: nil})
	defer ses.close()
	ses.send("CONFIRM")
	ses.expectOK()
}

func TestConfirm_NotConfirmed(t *testing.T) {
	ses := newSession(t, &mockPresenter{confirmErr: ErrNotConfirmed})
	defer ses.close()
	ses.send("CONFIRM")
	ses.expectERR("confirmed")
}

func TestConfirm_OneButton(t *testing.T) {
	mock := &mockPresenter{confirmErr: nil}
	ses := newSession(t, mock)
	defer ses.close()
	ses.send("CONFIRM --one-button")
	ses.expectOK()
	if !mock.oneButton {
		t.Error("expected oneButton=true, got false")
	}
}

func TestMessage(t *testing.T) {
	mock := &mockPresenter{}
	ses := newSession(t, mock)
	defer ses.close()
	ses.send("SETDESC Hello world")
	ses.expectOK()
	ses.send("MESSAGE")
	ses.expectOK()
	if mock.lastMessage == nil {
		t.Fatal("Message was not called")
	}
}

func TestReset_ClearsState(t *testing.T) {
	mock := &mockPresenter{pin: "x"}
	ses := newSession(t, mock)
	defer ses.close()

	ses.send("SETKEYINFO n/AABB")
	ses.expectOK()
	ses.send("RESET")
	ses.expectOK()
	ses.send("GETPIN")
	ses.expectData("")
	ses.expectOK()

	if mock.lastGetPin.KeyID != "" {
		t.Errorf("KeyID should be empty after RESET, got %q", mock.lastGetPin.KeyID)
	}
}

func TestOption_Accepted(t *testing.T) {
	ses := newSession(t, &mockPresenter{})
	defer ses.close()
	ses.send("OPTION ttyname=/dev/pts/1")
	ses.expectOK()
	ses.send("OPTION lc-ctype=en_US.UTF-8")
	ses.expectOK()
}

func TestPassphraseEscaping(t *testing.T) {
	// Passphrase containing %, newline and carriage return must be escaped.
	mock := &mockPresenter{pin: "p%ss\nw\rrd"}
	ses := newSession(t, mock)
	defer ses.close()
	ses.send("GETPIN")
	data := ses.expectData("")
	ses.expectOK()

	if strings.Contains(data, "\n") || strings.Contains(data, "\r") {
		t.Errorf("unescaped newline in D line: %q", data)
	}
	if !strings.Contains(data, "%25") {
		t.Errorf("percent not escaped in D line: %q", data)
	}
}
