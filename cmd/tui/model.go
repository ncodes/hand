package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
	defaultStatus = "default session · ready · ctrl+c twice to quit"

	inputChromeHeight = 3

	exitConfirmationWindow = 2 * time.Second
)

var currentTime = time.Now

type exitConfirmationExpiredMsg struct {
	startedAt time.Time
}

// model is the root Bubble Tea application state for the interactive shell.
type model struct {
	transcript viewport.Model
	input      textarea.Model
	width      int
	height     int
	status     string
	modelName  string
	context    string
	messages   []string
	exitAt     time.Time
}

// newModel builds the initial TUI state and sizes child components.
func newModel() model {
	appModel := model{
		transcript: newTranscript(),
		input:      newInputComposer(),
		width:      defaultWidth,
		height:     defaultHeight,
		status:     defaultStatus,
		modelName:  "GPT 5.5",
		context:    "60,000 used · 65%",
	}
	appModel.resize()

	return appModel
}

// Init focuses the input composer when Bubble Tea starts the program.
func (m model) Init() tea.Cmd {
	return m.input.Focus()
}

// Update handles terminal events and delegates ordinary input to child models.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(msg.Width, 1)
		m.height = max(msg.Height, 1)
		m.resize()
		return m, nil
	case exitConfirmationExpiredMsg:
		return m.expireExitConfirmation(msg), nil
	case tea.KeyPressMsg:
		switch msg.Keystroke() {
		case "ctrl+c":
			return m.confirmExit()
		case "esc":
			return m, nil
		case "shift+enter":
			return m.insertInputNewline()
		case "enter":
			m.submitPrompt()
			return m, nil
		}
		if isInputLineDeleteKey(msg) {
			return m.deleteInputLine()
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	m.transcript, cmd = m.transcript.Update(msg)
	cmds = append(cmds, cmd)
	m.resize()

	return m, tea.Batch(cmds...)
}

// confirmExit quits only after a second Ctrl-C inside a short window.
func (m model) confirmExit() (tea.Model, tea.Cmd) {
	now := currentTime()
	if !m.exitAt.IsZero() && now.Sub(m.exitAt) <= exitConfirmationWindow {
		return m, tea.Quit
	}

	m.exitAt = now
	m.status = "Press Ctrl-C again to exit"
	startedAt := m.exitAt

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
	m.status = defaultStatus

	return m
}

// resize distributes terminal rows between transcript and composer.
func (m *model) resize() {
	inputHeight := m.getInputHeight()
	transcriptHeight := max(m.height-inputHeight-inputChromeHeight-m.getHeaderHeight(), 1)

	m.input.SetWidth(getInputInnerWidth(m.width))
	m.input.SetHeight(inputHeight)
	m.transcript.SetWidth(m.width)
	m.transcript.SetHeight(transcriptHeight)
}

// getInputHeight returns the visible composer height constrained by the screen.
func (m model) getInputHeight() int {
	availableHeight := max(m.height-inputChromeHeight-m.getHeaderHeight()-1, minInputHeight)
	contentHeight := getInputHeight(m.input.Value(), getInputInnerWidth(m.width))

	return min(contentHeight, availableHeight)
}

// insertInputNewline expands the composer before adding a newline.
func (m model) insertInputNewline() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	inputWidth := getInputInnerWidth(m.width)
	availableHeight := max(m.height-inputChromeHeight-m.getHeaderHeight()-1, minInputHeight)
	m.input.SetWidth(inputWidth)
	m.input.SetHeight(min(getInputHeight(m.input.Value()+"\n", inputWidth), availableHeight))
	m.input, cmd = m.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.resize()

	return m, cmd
}

// deleteInputLine clears the current logical composer line.
func (m model) deleteInputLine() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input.CursorEnd()
	m.input, cmd = m.input.Update(tea.KeyPressMsg(tea.Key{
		Code: 'u',
		Mod:  tea.ModCtrl,
	}))
	m.resize()

	return m, cmd
}

// submitPrompt appends a non-empty composer value to the transcript.
func (m *model) submitPrompt() bool {
	prompt := strings.TrimSpace(m.input.Value())
	if prompt == "" {
		return false
	}

	m.messages = append(m.messages, "You: "+prompt)
	m.transcript.SetContent(strings.Join(m.messages, "\n\n"))
	m.transcript.GotoBottom()
	m.input.SetValue("")
	m.resize()

	return true
}

// isInputLineDeleteKey reports whether a key should clear the current row.
func isInputLineDeleteKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	switch {
	case key.Code == 'u' && key.Mod.Contains(tea.ModCtrl):
		return true
	case key.Code == tea.KeyBackspace || key.Code == tea.KeyDelete:
		return key.Mod.Contains(tea.ModSuper) ||
			key.Mod.Contains(tea.ModMeta) ||
			key.Mod.Contains(tea.ModCtrl)
	default:
		return false
	}
}
