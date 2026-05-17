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
	status     statusModel
	modelName  string
	context    string
	messages   []string
	live       string
	stream     markdownStreamCollector
	history    []string
	historyAt  int
	draft      string
	exitAt     time.Time
	allowShell bool
}

// newModel builds the initial TUI state and sizes child components.
func newModel() model {
	history, err := loadPromptHistory()
	appModel := model{
		transcript: newTranscript(),
		input:      newInputComposer(),
		width:      defaultWidth,
		height:     defaultHeight,
		status:     newStatusModel(),
		modelName:  "GPT 5.5",
		context:    "60,000 used · 65%",
		history:    history,
	}
	appModel.historyAt = len(appModel.history)
	if err != nil {
		appModel.status.setTransient("prompt history unavailable")
	}
	appModel.resize()

	return appModel
}

// Init focuses the input composer when Bubble Tea starts the program.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.input.Focus(), m.statusExpireCmd())
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
	case statusExpiredMsg:
		m.status.expire(msg)
		return m, nil
	case assistantTextDeltaMsg:
		m.appendAssistantDelta(msg.Text)
		return m, nil
	case assistantResponseCompletedMsg:
		m.completeAssistantResponse(msg.Text)
		return m, nil
	case tea.PasteMsg:
		m.input, _ = m.input.Update(msg)
		m.resize()
		return m, nil
	case tea.KeyPressMsg:
		switch msg.Keystroke() {
		case "ctrl+c":
			return m.confirmExit()
		case "esc":
			return m, nil
		case "ctrl+p":
			m.showPreviousPrompt()
			return m, nil
		case "ctrl+n":
			m.showNextPrompt()
			return m, nil
		case "shift+enter":
			return m.insertInputNewline()
		case "enter":
			cmd := m.submitPrompt()
			return m, cmd
		}
		if isInputLineDeleteKey(msg) {
			return m.deleteInputLine()
		}
		if m.shouldUseHistoryKey(msg) {
			switch msg.Key().Code {
			case tea.KeyUp:
				m.showPreviousPrompt()
			case tea.KeyDown:
				m.showNextPrompt()
			}
			return m, nil
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

// submitPrompt routes a non-empty composer value to prompt or command handling.
func (m *model) submitPrompt() tea.Cmd {
	input := parseComposerInput(m.input.Value())
	if input.Kind == composerInputEmpty {
		return nil
	}

	cmd := m.addPromptHistory(input.Text)
	switch input.Kind {
	case composerInputPrompt:
		m.messages = append(m.messages, "You: "+input.Text)
	case composerInputCommand:
		cmd = tea.Batch(cmd, m.handleSlashCommand(input))
	case composerInputLocalCommand:
		cmd = tea.Batch(cmd, m.handleLocalCommand(input))
	}
	m.setTranscriptContent()
	m.clearComposer()
	m.resize()

	return cmd
}

func (m *model) handleSlashCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	switch input.Name {
	case "clear":
		m.messages = nil
		m.live = ""
		m.stream.Reset()
		cmd = m.setStatus("transcript cleared")
	case "help":
		m.messages = append(m.messages, "Commands: /clear, /help")
	case "":
		cmd = m.setStatus("empty command")
	default:
		cmd = m.setStatus("unknown command: /" + input.Name)
	}

	m.setTranscriptContent()
	return cmd
}

func (m *model) handleLocalCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	if !m.allowShell {
		cmd = m.setStatus("local commands are disabled")
		m.messages = append(m.messages, "Local command blocked: !"+input.Args)
		m.setTranscriptContent()
		return cmd
	}

	cmd = m.setStatus("local command execution is not connected yet")
	m.messages = append(m.messages, "Local command queued: !"+input.Args)
	m.setTranscriptContent()
	return cmd
}

func (m *model) clearComposer() {
	m.input.SetValue("")
	m.historyAt = len(m.history)
	m.draft = ""
}

func (m *model) addPromptHistory(value string) tea.Cmd {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(m.history) > 0 && m.history[len(m.history)-1] == value {
		m.historyAt = len(m.history)
		return nil
	}

	m.history = normalizePromptHistory(append(m.history, value))
	m.historyAt = len(m.history)
	if err := savePromptHistory(m.history); err != nil {
		return m.setStatus("prompt history unavailable")
	}

	return nil
}

func (m *model) setStatus(text string) tea.Cmd {
	return m.status.setTransient(text)
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

func (m *model) showPreviousPrompt() {
	if len(m.history) == 0 {
		return
	}
	if m.historyAt == len(m.history) {
		m.draft = m.input.Value()
	}
	if m.historyAt > 0 {
		m.historyAt--
	}

	m.input.SetValue(m.history[m.historyAt])
	m.input.CursorEnd()
	m.resize()
}

func (m *model) showNextPrompt() {
	if len(m.history) == 0 || m.historyAt >= len(m.history) {
		return
	}

	m.historyAt++
	if m.historyAt == len(m.history) {
		m.input.SetValue(m.draft)
		m.draft = ""
	} else {
		m.input.SetValue(m.history[m.historyAt])
	}
	m.input.CursorEnd()
	m.resize()
}

func (m model) shouldUseHistoryKey(msg tea.KeyPressMsg) bool {
	switch msg.Key().Code {
	case tea.KeyUp, tea.KeyDown:
		return !strings.Contains(m.input.Value(), "\n")
	default:
		return false
	}
}

func (m *model) appendAssistantDelta(delta string) {
	if delta == "" {
		return
	}

	m.stream.Add(delta)
	m.live = assistantTranscriptCell(m.stream.Render())
	m.setTranscriptContent()
	m.resize()
}

func (m *model) completeAssistantResponse(text string) {
	reply := text
	if strings.TrimSpace(reply) == "" {
		reply = m.stream.Finalize()
	} else {
		m.stream.Reset()
	}
	if strings.TrimSpace(reply) == "" {
		m.live = ""
		m.setTranscriptContent()
		m.resize()
		return
	}

	m.messages = append(m.messages, assistantTranscriptCell(reply))
	m.live = ""
	m.setTranscriptContent()
	m.resize()
}

func assistantTranscriptCell(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	return "Hand: " + text
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
