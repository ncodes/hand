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
	return lipgloss.NewStyle().
		Width(m.getMainPaneWidth()).
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

func (m *model) refreshTranscriptContentAfterResize() {
	offset := m.transcript.YOffset()
	wasAtBottom := m.transcript.AtBottom()
	m.clearTranscriptSelection()
	m.transcript.SetContent(m.renderTranscriptContent())
	if wasAtBottom {
		m.transcript.GotoBottom()
		return
	}

	m.transcript.SetYOffset(offset)
}

func (m *model) renderTranscriptContent() string {
	cells := make([]transcriptCell, 0, len(m.messages)+1)
	cells = append(cells, m.messages...)
	if m.live != nil && !m.live.IsEmpty() {
		cells = append(cells, m.live)
	}
	if len(cells) == 0 && m.showIntro {
		cells = append(cells, systemTranscriptCell{text: "Welcome to Hand TUI.\n\nThe interactive shell is ready."})
	}
	if len(cells) > 0 {
		m.showIntro = false
	}

	transcriptWidth := m.transcript.Width()
	if transcriptWidth <= 0 {
		transcriptWidth = m.getMainPaneWidth()
	}
	content := strings.Trim(m.renderHeaderWithWidth(transcriptWidth), "\n")
	if cellsText := strings.Trim(m.renderTranscriptBodyCells(cells), "\n"); strings.TrimSpace(cellsText) != "" {
		content = strings.Join([]string{content, cellsText}, "\n")
	}

	return content
}

func (m model) renderTranscriptBodyCells(cells []transcriptCell) string {
	width := max(m.transcript.Width(), m.getMainPaneWidth())
	contentWidth := getPanelContentWidth(width)
	if contentWidth <= 0 {
		contentWidth = max(width, 1)
	}
	cellsText := renderTranscriptCellsWithFrame(cells, contentWidth, m.toolAnimationFrame)
	if strings.TrimSpace(cellsText) == "" {
		return ""
	}

	padding := getPanelHorizontalPadding(width)
	if padding <= 0 {
		return cellsText
	}

	return indentTranscriptBodyCells(cellsText, padding)
}

func indentTranscriptBodyCells(content string, padding int) string {
	if padding <= 0 || content == "" {
		return content
	}

	prefix := strings.Repeat(" ", padding)
	lines := strings.Split(content, "\n")
	for index := range lines {
		if lines[index] != "" {
			lines[index] = prefix + lines[index]
		}
	}

	return strings.Join(lines, "\n")
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
			m.collapseCurrentReasoningTranscript()
			m.appendAssistantDelta(value.Text)
		}
	case assistantResponseCompletedMsg:
		m.completeAssistantResponse(value.Text)
	case reasoningCompletedMsg:
		m.completeReasoningTranscript(value.Duration)
	case sessionErrorMsg:
		m.collapseCurrentReasoningTranscript()
		m.addTranscriptMessage(value)
		return m.setStatus("response failed")
	case toolInvocationStartedMsg:
		m.collapseCurrentReasoningTranscript()
		m.addTranscriptMessage(value)
		return m.startToolAnimation()
	case toolInvocationCompletedMsg:
		m.addTranscriptMessage(value)
		return m.startThinkingComposer()
	case safetyEventMsg:
		m.addTranscriptMessage(value)
	case manualCompactionMsg:
		m.addTranscriptMessage(value)
		if value.State.isInProgress() {
			return m.startToolAnimation()
		}
	}

	return nil
}

func (m *model) addTranscriptMessage(msg any) {
	if cell := tuiMessageToTranscriptCell(msg); cell != nil && !cell.IsEmpty() {
		m.applyAction(appendTranscriptCellAction{Cell: cell})
		if m.responding {
			m.setTranscriptContentForResponseUpdate()
		} else {
			m.setTranscriptContent()
		}
		m.resize()
	}
}

func (m *model) appendReasoningDelta(delta string) {
	cell := newReasoningTranscriptCell(delta, currentTime())
	if cell == nil || cell.IsEmpty() {
		return
	}
	if m.reasoningStartedAt.IsZero() {
		m.reasoningStartedAt = currentTime()
	}

	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Kind() == transcriptCellReasoning {
		index := len(m.messages) - 1
		m.applyAction(replaceTranscriptCellAction{
			Index: index,
			Cell:  appendReasoningTranscriptCell(m.messages[index], delta),
		})
		m.trackReasoningTranscriptIndex(index)
	} else {
		m.applyAction(appendTranscriptCellAction{Cell: cell})
		m.trackReasoningTranscriptIndex(len(m.messages) - 1)
	}

	m.setTranscriptContentForResponseUpdate()
	m.resize()
}

func (m *model) appendAssistantDelta(delta string) {
	if delta == "" {
		return
	}

	m.stream.Add(delta)
	m.applyAction(setLiveTranscriptCellAction{Cell: assistantTranscriptCell{text: m.stream.Render()}})
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
		m.applyAction(setLiveTranscriptCellAction{})
		m.collapseCurrentReasoningTranscript()
		m.setTranscriptContentAfterResponseCompletion()
		m.resize()
		return
	}

	m.collapseCurrentReasoningTranscript()
	m.applyAction(appendTranscriptCellAction{Cell: assistantTranscriptCell{text: reply}})
	m.applyAction(setLiveTranscriptCellAction{})
	m.setTranscriptContentAfterResponseCompletion()
	m.resize()
}

func (m *model) setTranscriptContentAfterResponseCompletion() {
	m.setTranscriptContentForResponseUpdate()
}

func (m *model) setTranscriptContentForResponseUpdate() {
	m.resize()
	if m.responding && m.responseTranscriptFollow && !m.responseTranscriptScrolled {
		m.setTranscriptContent()
		return
	}

	m.setTranscriptContentForActiveTurn()
}

func (m *model) collapseReasoningTranscript() {
	m.collapseCurrentReasoningTranscript()
}

func (m *model) completeReasoningTranscript(duration time.Duration) {
	if duration <= 0 {
		duration = time.Second
	}

	index, ok := m.getCurrentReasoningTranscriptIndex()
	if !ok {
		return
	}

	m.replaceReasoningTranscriptCellWithThought(index, duration)
}

func (m *model) clearReasoningTranscriptState() {
	m.reasoningStartedAt = time.Time{}
	m.reasoningMessageIndex = -1
	m.reasoningMessageIndices = nil
}

func (m *model) trackReasoningTranscriptIndex(index int) {
	m.reasoningMessageIndex = index
	if index < 0 {
		return
	}
	for _, existing := range m.reasoningMessageIndices {
		if existing == index {
			return
		}
	}

	m.reasoningMessageIndices = append(m.reasoningMessageIndices, index)
}

func (m model) hasActiveReasoningTranscriptCells() bool {
	return len(m.getActiveReasoningTranscriptIndices()) > 0
}

func (m model) getCurrentReasoningTranscriptIndex() (int, bool) {
	if m.reasoningMessageIndex >= 0 &&
		m.reasoningMessageIndex < len(m.messages) &&
		m.messages[m.reasoningMessageIndex] != nil &&
		m.messages[m.reasoningMessageIndex].Kind() == transcriptCellReasoning {
		return m.reasoningMessageIndex, true
	}

	for index := len(m.reasoningMessageIndices) - 1; index >= 0; index-- {
		messageIndex := m.reasoningMessageIndices[index]
		if messageIndex < 0 || messageIndex >= len(m.messages) || m.messages[messageIndex] == nil {
			continue
		}
		if m.messages[messageIndex].Kind() == transcriptCellReasoning {
			return messageIndex, true
		}
	}

	return -1, false
}

func (m model) getActiveReasoningTranscriptIndices() []int {
	seen := map[int]bool{}
	indices := make([]int, 0, len(m.reasoningMessageIndices)+1)
	for _, index := range append(append([]int{}, m.reasoningMessageIndices...), m.reasoningMessageIndex) {
		if seen[index] || index < 0 || index >= len(m.messages) {
			continue
		}
		if m.messages[index] == nil || m.messages[index].Kind() != transcriptCellReasoning {
			continue
		}
		seen[index] = true
		indices = append(indices, index)
	}

	return indices
}

func (m *model) collapseCurrentReasoningTranscript() {
	index, ok := m.getCurrentReasoningTranscriptIndex()
	if !ok {
		return
	}

	duration := m.getReasoningTranscriptDuration(index, currentTime())
	m.replaceReasoningTranscriptCellWithThought(index, duration)
}

func (m model) getReasoningTranscriptDuration(index int, endedAt time.Time) time.Duration {
	if index < 0 || index >= len(m.messages) || m.messages[index] == nil {
		return time.Second
	}

	cell, ok := m.messages[index].(reasoningTranscriptCell)
	if !ok || cell.startedAt.IsZero() {
		if m.reasoningStartedAt.IsZero() {
			return time.Second
		}
		return normalizeThoughtDuration(endedAt.Sub(m.reasoningStartedAt).Round(time.Second))
	}

	return normalizeThoughtDuration(endedAt.Sub(cell.startedAt).Round(time.Second))
}

func (m *model) replaceReasoningTranscriptCellWithThought(index int, duration time.Duration) {
	m.applyAction(replaceTranscriptCellAction{
		Index: index,
		Cell:  thoughtTranscriptCell{duration: normalizeThoughtDuration(duration)},
	})
	m.untrackReasoningTranscriptIndex(index)
	if !m.hasActiveReasoningTranscriptCells() {
		m.clearReasoningTranscriptState()
	}
	if m.responding {
		m.setTranscriptContentForResponseUpdate()
	} else {
		m.setTranscriptContent()
	}
	m.resize()
}

func (m *model) untrackReasoningTranscriptIndex(index int) {
	next := m.reasoningMessageIndices[:0]
	for _, existing := range m.reasoningMessageIndices {
		if existing != index {
			next = append(next, existing)
		}
	}
	m.reasoningMessageIndices = next
	if m.reasoningMessageIndex == index {
		m.reasoningMessageIndex = -1
	}
}

func normalizeThoughtDuration(duration time.Duration) time.Duration {
	if duration <= 0 {
		return time.Second
	}

	return duration
}

func newReasoningTranscriptCell(text string, startedAt time.Time) transcriptCell {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	return reasoningTranscriptCell{text: text, startedAt: startedAt}
}

func appendReasoningTranscriptCell(cell transcriptCell, delta string) transcriptCell {
	if strings.TrimSpace(delta) == "" {
		return cell
	}

	reasoningCell, ok := cell.(reasoningTranscriptCell)
	if !ok {
		return newReasoningTranscriptCell(delta, currentTime())
	}

	reasoningCell.text += delta
	return reasoningCell
}

func isReasoningDeltaChannel(channel string) bool {
	return strings.TrimSpace(strings.ToLower(channel)) == "reasoning"
}
