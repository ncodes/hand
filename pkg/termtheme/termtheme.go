package termtheme

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

type Result struct {
	Theme      string `json:"theme"`
	Background string `json:"background,omitempty"`
	Source     string `json:"source"`
	Error      string `json:"error,omitempty"`
}

type terminal interface {
	Fd() uintptr
	Read([]byte) (int, error)
	WriteString(string) (int, error)
	Close() error
}

var (
	openTTY = func() (terminal, error) {
		return os.OpenFile("/dev/tty", os.O_RDWR, 0)
	}
	makeRaw     = term.MakeRaw
	restoreTerm = term.Restore
)

func Detect(timeout time.Duration) Result {
	terminal, err := openTTY()
	if err != nil {
		return Result{Theme: "unknown", Source: "tty", Error: err.Error()}
	}
	defer terminal.Close()

	response, err := queryBackground(terminal, timeout)
	if err != nil {
		return Result{Theme: "unknown", Source: "osc11", Error: err.Error()}
	}

	background, err := ParseOSC11(response)
	if err != nil {
		return Result{Theme: "unknown", Source: "osc11", Error: err.Error()}
	}

	return Result{
		Theme:      ThemeFromBackground(background),
		Background: background,
		Source:     "osc11",
	}
}

func ParseOSC11(response string) (string, error) {
	_, after, ok := strings.Cut(response, "]11;")
	if !ok {
		return "", fmt.Errorf("OSC 11 response not found: %q", response)
	}

	value := after
	value = strings.TrimSuffix(value, "\x07")
	value = strings.TrimSuffix(value, "\x1b\\")
	value = strings.TrimSpace(value)

	if after0, ok0 := strings.CutPrefix(value, "rgb:"); ok0 {
		return rgbToHex(after0)
	}

	if strings.HasPrefix(value, "#") && len(value) == 7 {
		return strings.ToLower(value), nil
	}

	return "", fmt.Errorf("unsupported OSC 11 color format: %q", value)
}

func ThemeFromBackground(background string) string {
	red, _ := strconv.ParseInt(background[1:3], 16, 32)
	green, _ := strconv.ParseInt(background[3:5], 16, 32)
	blue, _ := strconv.ParseInt(background[5:7], 16, 32)
	luminance := 0.2126*float64(red)/255 + 0.7152*float64(green)/255 + 0.0722*float64(blue)/255

	if luminance > 0.5 {
		return "light"
	}

	return "dark"
}

func queryBackground(terminal terminal, timeout time.Duration) (string, error) {
	fd := terminal.Fd()
	state, err := makeRaw(fd)
	if err != nil {
		return "", err
	}
	defer restoreTerm(fd, state)

	if _, err := terminal.WriteString("\x1b]11;?\x07"); err != nil {
		return "", err
	}

	response := make(chan string, 1)
	readErr := make(chan error, 1)

	go func() {
		value, err := readResponse(terminal)
		if err != nil {
			readErr <- err
			return
		}
		response <- value
	}()

	select {
	case value := <-response:
		return value, nil
	case err := <-readErr:
		return "", err
	case <-time.After(timeout):
		return "", errors.New("terminal did not respond to OSC 11")
	}
}

func readResponse(terminal terminal) (string, error) {
	var builder strings.Builder
	buffer := make([]byte, 64)

	for {
		n, err := terminal.Read(buffer)
		if n > 0 {
			builder.Write(buffer[:n])
			value := builder.String()
			if strings.Contains(value, "\x07") || strings.Contains(value, "\x1b\\") {
				return value, nil
			}
		}
		if err != nil {
			return "", err
		}
	}
}

func rgbToHex(value string) (string, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid rgb color: %q", value)
	}

	values := make([]int, 3)
	for i, part := range parts {
		if len(part) == 0 {
			return "", fmt.Errorf("invalid rgb component: %q", value)
		}

		parsed, err := strconv.ParseUint(part, 16, 16)
		if err != nil {
			return "", err
		}

		maxValue := uint64(1<<(len(part)*4)) - 1
		values[i] = int((parsed*255 + maxValue/2) / maxValue)
	}

	return fmt.Sprintf("#%02x%02x%02x", values[0], values[1], values[2]), nil
}
