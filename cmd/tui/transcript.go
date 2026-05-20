package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// newTranscript creates the scrollable conversation viewport.
func newTranscript() viewport.Model {
	transcript := viewport.New()
	transcript.SoftWrap = true

	return transcript
}

// renderTranscript draws the conversation viewport.
func (m model) renderTranscript() string {
	horizontalPadding := getPanelHorizontalPadding(m.width)

	return lipgloss.NewStyle().
		Padding(0, horizontalPadding).
		Width(m.width).
		Height(max(m.transcript.Height(), 1)).
		Render(m.transcript.View())
}

func (m *model) setTranscriptContent() {
	m.clearTranscriptSelection()
	m.transcript.SetContent(m.renderTranscriptContent())
	m.transcript.GotoBottom()
}

func (m *model) setTranscriptContentForActiveTurn() {
	offset := m.transcript.YOffset()
	m.clearTranscriptSelection()
	m.transcript.SetContent(m.renderTranscriptContent())
	m.transcript.SetYOffset(offset)
}

func (m *model) renderTranscriptContent() string {
	cells := make([]string, 0, len(m.messages)+1)
	cells = append(cells, m.messages...)
	if strings.TrimSpace(m.live) != "" {
		cells = append(cells, m.live)
	}
	if len(cells) == 0 && m.showIntro {
		cells = append(cells, "Welcome to Hand TUI.\n\nThe interactive shell is ready.")
	}
	if len(cells) > 0 {
		m.showIntro = false
	}

	headerWidth := m.transcript.Width()
	if headerWidth <= 0 {
		headerWidth = getPanelContentWidth(m.width)
	}
	content := strings.TrimSpace(m.renderHeaderWithWidth(headerWidth))
	if cellsText := strings.TrimSpace(renderTranscriptCellsWithFrame(cells, headerWidth, m.toolAnimationFrame)); cellsText != "" {
		content = strings.Join([]string{content, cellsText}, "\n\n")
	}

	return content
}

func (m *model) updateTranscript(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.transcript, cmd = m.transcript.Update(msg)

	return *m, cmd
}

func (m *model) scrollTranscriptWithKey(msg tea.KeyPressMsg) bool {
	offset := m.transcript.YOffset()
	switch msg.Key().Code {
	case tea.KeyPgUp:
		m.transcript.PageUp()
	case tea.KeyPgDown:
		m.transcript.PageDown()
	case tea.KeyHome:
		m.transcript.GotoTop()
	case tea.KeyEnd:
		m.jumpTranscriptToBottom()
		return true
	default:
		return false
	}
	m.markResponseTranscriptScrolled(offset, true)

	return true
}

func (m *model) updateTranscriptWithScrollTracking(msg tea.Msg) (tea.Model, tea.Cmd) {
	offset := m.transcript.YOffset()
	_, cmd := m.updateTranscript(msg)
	m.markResponseTranscriptScrolled(offset, true)

	return *m, cmd
}

func (m *model) markResponseTranscriptScrolled(previousOffset int, scrollInput bool) {
	if !m.responding {
		return
	}
	if scrollInput || m.transcript.YOffset() != previousOffset {
		m.stopFollowingResponseTranscript()
	}
}

func (m *model) stopFollowingResponseTranscript() {
	m.responseTranscriptScrolled = true
	m.responseTranscriptFollow = false
}

func (m *model) applyTUIMessage(msg any) tea.Cmd {
	switch value := msg.(type) {
	case assistantTextDeltaMsg:
		if isReasoningDeltaChannel(value.Channel) {
			m.appendReasoningDelta(value.Text)
		} else {
			m.appendAssistantDelta(value.Text)
		}
	case assistantResponseCompletedMsg:
		m.completeAssistantResponse(value.Text)
	case sessionErrorMsg:
		m.addTranscriptMessage(value)
		return m.setStatus("response failed")
	case toolInvocationStartedMsg:
		m.addTranscriptMessage(value)
		return m.startToolAnimation()
	case toolInvocationCompletedMsg:
		m.addTranscriptMessage(value)
		return m.startThinkingComposer()
	case safetyEventMsg:
		m.addTranscriptMessage(value)
	}

	return nil
}

func (m *model) addTranscriptMessage(msg any) {
	if cell := tuiMessageToTranscriptCell(msg); cell != "" {
		m.messages = append(m.messages, cell)
		if m.responding {
			m.setTranscriptContentForResponseUpdate()
		} else {
			m.setTranscriptContent()
		}
		m.resize()
	}
}

func (m *model) appendReasoningDelta(delta string) {
	cell := reasoningTranscriptCell(delta)
	if cell == "" {
		return
	}
	if m.reasoningStartedAt.IsZero() {
		m.reasoningStartedAt = currentTime()
	}

	if len(m.messages) > 0 && isReasoningTranscriptCell(m.messages[len(m.messages)-1]) {
		m.messages[len(m.messages)-1] = appendReasoningTranscriptCell(m.messages[len(m.messages)-1], delta)
		m.reasoningMessageIndex = len(m.messages) - 1
	} else {
		m.messages = append(m.messages, cell)
		m.reasoningMessageIndex = len(m.messages) - 1
	}

	m.setTranscriptContentForResponseUpdate()
	m.resize()
}

func (m *model) appendAssistantDelta(delta string) {
	if delta == "" {
		return
	}

	m.stream.Add(delta)
	m.live = assistantTranscriptCell(m.stream.Render())
	m.setTranscriptContentForResponseUpdate()
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
		m.collapseReasoningTranscript()
		m.setTranscriptContentAfterResponseCompletion()
		m.resize()
		return
	}

	m.collapseReasoningTranscript()
	m.messages = append(m.messages, assistantTranscriptCell(reply))
	m.live = ""
	m.setTranscriptContentAfterResponseCompletion()
	m.resize()
}

func (m *model) setTranscriptContentAfterResponseCompletion() {
	m.setTranscriptContentForResponseUpdate()
}

func (m *model) setTranscriptContentForResponseUpdate() {
	if m.responding && m.responseTranscriptFollow && !m.responseTranscriptScrolled {
		m.setTranscriptContent()
		return
	}

	m.setTranscriptContentForActiveTurn()
}

func assistantTranscriptCell(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	return "Hand: " + text
}

func (m *model) collapseReasoningTranscript() {
	index := m.reasoningMessageIndex
	if index < 0 || index >= len(m.messages) || !isReasoningTranscriptCell(m.messages[index]) {
		return
	}

	duration := currentTime().Sub(m.reasoningStartedAt).Round(time.Second)
	if m.reasoningStartedAt.IsZero() || duration <= 0 {
		duration = time.Second
	}

	m.messages[index] = thoughtTranscriptCell(duration)
	m.clearReasoningTranscriptState()
}

func (m *model) clearReasoningTranscriptState() {
	m.reasoningStartedAt = time.Time{}
	m.reasoningMessageIndex = -1
}

func thoughtTranscriptCell(duration time.Duration) string {
	if duration <= 0 {
		duration = time.Second
	}

	return "Thought: " + formatToolTranscriptDuration(duration)
}

func reasoningTranscriptCell(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	return "Reasoning: " + text
}

func appendReasoningTranscriptCell(cell string, delta string) string {
	if strings.TrimSpace(delta) == "" {
		return cell
	}

	if !isReasoningTranscriptCell(cell) {
		return reasoningTranscriptCell(delta)
	}

	return cell + delta
}

func isReasoningTranscriptCell(cell string) bool {
	kind, _, _ := parseTranscriptCell(cell)
	return kind == transcriptCellReasoning
}

func isReasoningDeltaChannel(channel string) bool {
	return strings.TrimSpace(strings.ToLower(channel)) == "reasoning"
}
