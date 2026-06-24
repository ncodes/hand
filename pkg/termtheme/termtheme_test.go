package termtheme

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
)

type mockTerminal struct {
	fd         uintptr
	reads      []mockRead
	writeError error
	closeCount int
	writes     []string
}

type mockRead struct {
	value string
	err   error
	wait  chan struct{}
}

func (m *mockTerminal) Fd() uintptr {
	return m.fd
}

func (m *mockTerminal) Read(buffer []byte) (int, error) {
	if len(m.reads) == 0 {
		return 0, io.EOF
	}

	read := m.reads[0]
	m.reads = m.reads[1:]
	if read.wait != nil {
		<-read.wait
	}

	return copy(buffer, read.value), read.err
}

func (m *mockTerminal) WriteString(value string) (int, error) {
	m.writes = append(m.writes, value)
	if m.writeError != nil {
		return 0, m.writeError
	}

	return len(value), nil
}

func (m *mockTerminal) Close() error {
	m.closeCount++
	return nil
}

func withTerminalHooks(t *testing.T, tty terminal, openErr error, rawErr error) {
	t.Helper()

	oldOpenTTY := openTTY
	oldMakeRaw := makeRaw
	oldRestoreTerm := restoreTerm
	oldLookupEnv := lookupEnv
	oldSetNonblock := setNonblock
	t.Cleanup(func() {
		openTTY = oldOpenTTY
		makeRaw = oldMakeRaw
		restoreTerm = oldRestoreTerm
		lookupEnv = oldLookupEnv
		setNonblock = oldSetNonblock
	})

	lookupEnv = func(key string) (string, bool) {
		return "", false
	}
	setNonblock = func(int, bool) error {
		return nil
	}
	openTTY = func() (terminal, error) {
		return tty, openErr
	}
	makeRaw = func(uintptr) (*term.State, error) {
		if rawErr != nil {
			return nil, rawErr
		}

		return &term.State{}, nil
	}
	restoreTerm = func(uintptr, *term.State) error {
		return nil
	}
}

func withEnv(t *testing.T, values map[string]string) {
	t.Helper()

	oldLookupEnv := lookupEnv
	t.Cleanup(func() {
		lookupEnv = oldLookupEnv
	})

	lookupEnv = func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestDefaultOpenTTY(t *testing.T) {
	tty, err := openTTY()
	if err == nil {
		_ = tty.Close()
	}
}

func TestParseOSC11RGB(t *testing.T) {
	background, err := ParseOSC11("\x1b]11;rgb:0000/1111/ffff\x07")
	if err != nil {
		t.Fatal(err)
	}

	if background != "#0011ff" {
		t.Fatalf("expected #0011ff, got %s", background)
	}
}

func TestParseOSC11Hex(t *testing.T) {
	background, err := ParseOSC11("\x1b]11;#f7f7f7\x1b\\")
	if err != nil {
		t.Fatal(err)
	}

	if background != "#f7f7f7" {
		t.Fatalf("expected #f7f7f7, got %s", background)
	}
}

func TestParseOSC11MissingResponse(t *testing.T) {
	_, err := ParseOSC11("hello")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseOSC11UnsupportedFormat(t *testing.T) {
	_, err := ParseOSC11("\x1b]11;not-a-color\x07")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseOSC11InvalidRGB(t *testing.T) {
	tests := []string{
		"\x1b]11;rgb:0000/1111\x07",
		"\x1b]11;rgb:/1111/ffff\x07",
		"\x1b]11;rgb:xxxx/1111/ffff\x07",
	}

	for _, response := range tests {
		if _, err := ParseOSC11(response); err == nil {
			t.Fatalf("expected error for %q", response)
		}
	}
}

func TestDetectOpenTTYError(t *testing.T) {
	withTerminalHooks(t, nil, errors.New("missing tty"), nil)

	res := Detect(time.Millisecond)
	if res.Theme != "unknown" || res.Source != "tty" || !strings.Contains(res.Error, "missing tty") {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestDetectUsesExplicitBackgroundEnvironment(t *testing.T) {
	withEnv(t, map[string]string{"MORPH_TUI_BACKGROUND": "#1e1e2e"})

	res := Detect(time.Millisecond)
	if res.Theme != "dark" || res.Background != "#1e1e2e" || res.Source != "environment" || res.Error != "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestDetectPrefersOSC11OverCOLORFGBGEnvironment(t *testing.T) {
	terminal := &mockTerminal{
		reads: []mockRead{{value: "\x1b]11;#1e1e2e\x07"}},
	}
	withTerminalHooks(t, terminal, nil, nil)
	lookupEnv = func(key string) (string, bool) {
		return "15;0", key == "COLORFGBG"
	}

	res := Detect(time.Millisecond)
	if res.Theme != "dark" || res.Background != "#1e1e2e" || res.Source != "osc11" || res.Error != "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestDetectUsesCOLORFGBGEnvironmentFallback(t *testing.T) {
	terminal := &mockTerminal{}
	withTerminalHooks(t, terminal, nil, nil)
	lookupEnv = func(key string) (string, bool) {
		return "15;0", key == "COLORFGBG"
	}

	res := Detect(time.Millisecond)
	if res.Theme != "dark" || res.Background != "#000000" || res.Source != "environment" || res.Error != "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestDetectQueryError(t *testing.T) {
	terminal := &mockTerminal{}
	withTerminalHooks(t, terminal, nil, errors.New("raw failed"))

	res := Detect(time.Millisecond)
	if res.Theme != "unknown" || res.Source != "osc11" || !strings.Contains(res.Error, "raw failed") {
		t.Fatalf("unexpected result: %+v", res)
	}
	if terminal.closeCount != 1 {
		t.Fatalf("expected terminal to close once, got %d", terminal.closeCount)
	}
}

func TestDetectParseError(t *testing.T) {
	terminal := &mockTerminal{
		reads: []mockRead{{value: "\x1b]11;bad\x07"}},
	}
	withTerminalHooks(t, terminal, nil, nil)

	res := Detect(time.Millisecond)
	if res.Theme != "unknown" || res.Source != "osc11" || !strings.Contains(res.Error, "unsupported") {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestDetectSuccess(t *testing.T) {
	terminal := &mockTerminal{
		reads: []mockRead{{value: "\x1b]11;#ffffff\x07"}},
	}
	withTerminalHooks(t, terminal, nil, nil)

	res := Detect(time.Millisecond)
	if res.Theme != "light" || res.Background != "#ffffff" || res.Source != "osc11" || res.Error != "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestQueryBackgroundWriteError(t *testing.T) {
	terminal := &mockTerminal{writeError: errors.New("write failed")}
	withTerminalHooks(t, terminal, nil, nil)

	_, err := queryBackground(terminal, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestQueryBackgroundReadError(t *testing.T) {
	terminal := &mockTerminal{
		reads: []mockRead{{err: errors.New("read failed")}},
	}
	withTerminalHooks(t, terminal, nil, nil)

	_, err := queryBackground(terminal, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestQueryBackgroundTimeout(t *testing.T) {
	terminal := &mockTerminal{}
	withTerminalHooks(t, terminal, nil, nil)

	_, err := queryBackground(terminal, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not respond") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestReadResponseAccumulatesUntilTerminator(t *testing.T) {
	terminal := &mockTerminal{
		reads: []mockRead{
			{value: "\x1b]11;rgb:0000"},
			{value: "/0000/0000\x1b\\"},
		},
	}

	response, err := readResponse(terminal, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if response != "\x1b]11;rgb:0000/0000/0000\x1b\\" {
		t.Fatalf("unexpected response: %q", response)
	}
}

func TestThemeFromBackground(t *testing.T) {
	tests := map[string]string{
		"#000000": "dark",
		"#ffffff": "light",
		"#202124": "dark",
		"#f5f5f5": "light",
	}

	for background, want := range tests {
		if got := ThemeFromBackground(background); got != want {
			t.Fatalf("ThemeFromBackground(%q) = %q, want %q", background, got, want)
		}
	}
}
