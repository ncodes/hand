package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wandxy/morph/internal/trace"
	"github.com/wandxy/morph/pkg/stringx"
)

const autoCompactionLabel = "Automatic compaction"
const manualCompactionLabel = "Manual compaction"
const manualCompactionShimmerStep = 4

type manualCompactionState struct {
	Status string
	Error  string
	Label  string
}

func manualCompactionStateFromTraceEvent(eventType string, payload any) manualCompactionState {
	compaction, _ := payload.(trace.CompactionEventPayload)
	label := manualCompactionLabel
	if compaction.Auto {
		label = autoCompactionLabel
	}

	switch stringx.String(eventType).Trim() {
	case trace.EvtContextCompactionPending, trace.EvtContextCompactionRunning:
		return manualCompactionState{Status: "running", Label: label}
	case trace.EvtContextCompactionSucceeded:
		return manualCompactionState{Status: "succeeded", Label: label}
	case trace.EvtContextCompactionFailed:
		return manualCompactionState{Status: "failed", Error: compaction.Error, Label: label}
	default:
		return manualCompactionState{}
	}
}

func (state manualCompactionState) isVisible() bool {
	return stringx.String(state.Status).Trim() != ""
}

func (state manualCompactionState) isInProgress() bool {
	switch stringx.String(state.Status).Normalized() {
	case "pending", "running", "started":
		return true
	default:
		return false
	}
}

func (state manualCompactionState) displayText() string {
	label := stringx.String(state.Label).Trim()
	if label == "" {
		label = manualCompactionLabel
	}

	switch stringx.String(state.Status).Normalized() {
	case "pending", "running", "started":
		return label + " started"
	case "succeeded", "completed":
		return label + " completed"
	case "failed":
		if err := stringx.String(state.Error).Trim(); err != "" {
			return label + " failed: " + err
		}
		return label + " failed"
	default:
		return ""
	}
}

func (m *model) startManualCompactionStatus() tea.Cmd {
	m.manualCompactionActive = true
	cell := manualCompactionTranscriptCell{state: manualCompactionState{Status: "running", Label: manualCompactionLabel}}
	m.applyAction(appendTranscriptCellAction{Cell: cell})
	m.manualCompactionIndex = len(m.messages) - 1
	m.input.Blur()
	m.setTranscriptContent()

	return m.startToolAnimation()
}

func (m *model) completeManualCompactionStatus(err error) {
	state := manualCompactionState{Status: "succeeded", Label: manualCompactionLabel}
	if err != nil {
		state = manualCompactionState{Status: "failed", Error: err.Error(), Label: manualCompactionLabel}
	}

	if m.manualCompactionIndex >= 0 && m.manualCompactionIndex < len(m.messages) {
		m.applyAction(replaceTranscriptCellAction{
			Index: m.manualCompactionIndex,
			Cell:  manualCompactionTranscriptCell{state: state},
		})
	} else {
		m.applyAction(appendTranscriptCellAction{Cell: manualCompactionTranscriptCell{state: state}})
	}

	m.manualCompactionActive = false
	m.manualCompactionIndex = -1
	m.input.Focus()
	m.setTranscriptContent()
}

func renderManualCompactionCell(cell manualCompactionTranscriptCell, ctx transcriptRenderContext) string {
	text := cell.state.displayText()
	if text == "" {
		return ""
	}

	line := ""
	if !cell.state.isInProgress() {
		line += lipgloss.NewStyle().
			Foreground(lipgloss.Color(defaultTUITheme.CompactionText)).
			Render(text)
	} else {
		line += renderManualCompactionShimmer(text, ctx.Frame)
	}

	width := max(ctx.Width, 1)
	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.CompactionSeparator)).
		Render(strings.Repeat("─", width))

	return strings.Join([]string{
		separator,
		centerManualCompactionLine(line, width),
		separator,
	}, "\n")
}

func renderManualCompactionShimmer(text string, frame int) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}

	var builder strings.Builder
	shimmerFrame := frame * manualCompactionShimmerStep
	for index, char := range runes {
		color := getThinkingStatusColor(index, shimmerFrame, len(runes))
		builder.WriteString(lipgloss.NewStyle().
			Inline(true).
			Foreground(lipgloss.Color(color)).
			Render(string(char)))
	}

	return builder.String()
}

func centerManualCompactionLine(line string, width int) string {
	if width <= 0 {
		return line
	}

	pad := max((width-lipgloss.Width(line))/2, 0)
	return strings.Repeat(" ", pad) + line
}
