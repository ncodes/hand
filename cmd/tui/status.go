package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	defaultSessionTitle  = "default session"
	defaultStatus        = statusReadySuffix
	statusReadySuffix    = "enter to send · ctrl+c to quit"
	statusAutoHideWindow = 3 * time.Second

	exitConfirmationWindow = 2 * time.Second
)

var currentTime = time.Now

type statusExpiredMsg struct {
	startedAt time.Time
}

type exitConfirmationExpiredMsg struct {
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

	return "enter to send · ctrl+c to quit"
}

func (s statusModel) hasTransient() bool {
	return !s.startedAt.IsZero()
}

func (s *statusModel) setDefault(text string) {
	s.defaultText = strings.TrimSpace(text)
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

func (m *model) setStatus(text string) tea.Cmd {
	return m.status.setTransient(text)
}

func (m *model) setDefaultStatus(text string) {
	m.status.setDefault(text)
}

func (m *model) setSessionTitle(text string) {
	m.applyAction(setSessionTitleAction{Title: text})
}

func (m model) statusExpireCmd() tea.Cmd {
	if !m.status.hasTransient() {
		return nil
	}

	startedAt := m.status.startedAt
	hideAfter := m.status.hideAfter
	if hideAfter <= 0 {
		hideAfter = statusAutoHideWindow
	}

	return tea.Tick(hideAfter, func(time.Time) tea.Msg {
		return statusExpiredMsg{startedAt: startedAt}
	})
}

// confirmExit quits only after a second Ctrl-C inside a short window.
func (m model) confirmExit() (tea.Model, tea.Cmd) {
	now := currentTime()
	if !m.exitAt.IsZero() && now.Sub(m.exitAt) <= exitConfirmationWindow {
		return m, tea.Quit
	}

	m.exitAt = now
	startedAt := m.exitAt
	m.status.text = "Press Ctrl-C again to exit"
	m.status.startedAt = startedAt

	return m, tea.Tick(exitConfirmationWindow, func(time.Time) tea.Msg {
		return exitConfirmationExpiredMsg{startedAt: startedAt}
	})
}

// hasPendingExitConfirmation reports whether Ctrl-C is awaiting confirmation.
func (m model) hasPendingExitConfirmation() bool {
	return !m.exitAt.IsZero()
}

// expireExitConfirmation clears a stale Ctrl-C exit confirmation.
func (m model) expireExitConfirmation(msg exitConfirmationExpiredMsg) tea.Model {
	if m.exitAt.IsZero() || !m.exitAt.Equal(msg.startedAt) {
		return m
	}

	m.exitAt = time.Time{}
	m.status.expire(statusExpiredMsg{startedAt: msg.startedAt})

	return m
}
