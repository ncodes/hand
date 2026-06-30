package status

import (
	"time"

	"github.com/wandxy/morph/pkg/stringx"
)

const (
	DefaultSessionTitle    = "default session"
	ReadySuffix            = "enter to send · ctrl+c to quit"
	DefaultText            = ReadySuffix
	AutoHideWindow         = 3 * time.Second
	ExitConfirmationWindow = 2 * time.Second
)

// Model describes the status bar text rendered by the tui.
type Model struct {
	defaultText string
	text        string
	startedAt   time.Time
	hideAfter   time.Duration
}

// New returns a status model with the supplied initial text.
func New() Model {
	return Model{
		defaultText: DefaultText,
		hideAfter:   AutoHideWindow,
	}
}

func (m Model) Text() string {
	if text := stringx.String(m.text).Trim(); text != "" {
		return text
	}
	if text := stringx.String(m.defaultText).Trim(); text != "" {
		return text
	}

	return ReadySuffix
}

func (m Model) HasTransient() bool {
	return !m.startedAt.IsZero()
}

func (m Model) StartedAt() time.Time {
	return m.startedAt
}

func (m Model) HideAfter() time.Duration {
	if m.hideAfter <= 0 {
		return AutoHideWindow
	}

	return m.hideAfter
}

func (m *Model) SetHideAfter(duration time.Duration) {
	m.hideAfter = duration
}

func (m *Model) SetDefault(text string) {
	m.defaultText = stringx.String(text).Trim()
}

func (m *Model) SetTransient(text string, now time.Time) bool {
	m.text = stringx.String(text).Trim()
	if m.text == "" {
		m.startedAt = time.Time{}
		return false
	}

	m.startedAt = now

	return true
}

func (m *Model) Expire(startedAt time.Time) {
	if m.startedAt.IsZero() || !m.startedAt.Equal(startedAt) {
		return
	}

	m.text = ""
	m.startedAt = time.Time{}
}
