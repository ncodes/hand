package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	defaultStatus        = "default session · ready · ctrl+c twice to quit"
	statusAutoHideWindow = 3 * time.Second
)

type statusExpiredMsg struct {
	startedAt time.Time
}

type statusModel struct {
	defaultText string
	text        string
	startedAt   time.Time
	hideAfter   time.Duration
}

func newStatusModel() statusModel {
	return statusModel{
		defaultText: defaultStatus,
		hideAfter:   statusAutoHideWindow,
	}
}

func (s statusModel) Text() string {
	if text := strings.TrimSpace(s.text); text != "" {
		return text
	}
	if text := strings.TrimSpace(s.defaultText); text != "" {
		return text
	}

	return "ready"
}

func (s statusModel) hasTransient() bool {
	return !s.startedAt.IsZero()
}

func (s *statusModel) setTransient(text string) tea.Cmd {
	s.text = strings.TrimSpace(text)
	if s.text == "" {
		s.startedAt = time.Time{}
		return nil
	}

	s.startedAt = currentTime()
	startedAt := s.startedAt
	hideAfter := s.hideAfter
	if hideAfter <= 0 {
		hideAfter = statusAutoHideWindow
	}

	return tea.Tick(hideAfter, func(time.Time) tea.Msg {
		return statusExpiredMsg{startedAt: startedAt}
	})
}

func (s *statusModel) expire(msg statusExpiredMsg) {
	if s.startedAt.IsZero() || !s.startedAt.Equal(msg.startedAt) {
		return
	}

	s.text = ""
	s.startedAt = time.Time{}
}
