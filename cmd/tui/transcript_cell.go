package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type transcriptCellKind string

const (
	transcriptCellUser      transcriptCellKind = "user"
	transcriptCellAssistant transcriptCellKind = "assistant"
	transcriptCellTool      transcriptCellKind = "tool"
	transcriptCellSafety    transcriptCellKind = "safety"
	transcriptCellError     transcriptCellKind = "error"
	transcriptCellSystem    transcriptCellKind = "system"
)

func renderTranscriptCells(cells []string) string {
	rendered := make([]string, 0, len(cells))
	for _, cell := range cells {
		if cell = strings.TrimSpace(cell); cell != "" {
			rendered = append(rendered, renderTranscriptCell(cell))
		}
	}

	return strings.Join(rendered, "\n\n")
}

func renderTranscriptCell(cell string) string {
	kind, label, body := parseTranscriptCell(cell)
	if strings.TrimSpace(body) == "" {
		return ""
	}

	labelStyle := transcriptCellLabelStyle(kind)
	if label == "" {
		return body
	}

	return labelStyle.Render(label+":") + " " + body
}

func parseTranscriptCell(cell string) (transcriptCellKind, string, string) {
	cell = strings.TrimSpace(cell)
	label, body, ok := strings.Cut(cell, ":")
	if !ok {
		return transcriptCellSystem, "", cell
	}

	label = strings.TrimSpace(label)
	body = strings.TrimSpace(body)
	switch {
	case label == "You":
		return transcriptCellUser, label, body
	case label == "Hand":
		return transcriptCellAssistant, label, body
	case label == "Safety":
		return transcriptCellSafety, label, body
	case label == "Error":
		return transcriptCellError, label, body
	case strings.HasPrefix(label, "Tool"):
		return transcriptCellTool, label, body
	default:
		return transcriptCellSystem, label, body
	}
}

func transcriptCellLabelStyle(kind transcriptCellKind) lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true)
	switch kind {
	case transcriptCellUser:
		return style.Foreground(lipgloss.Color("39"))
	case transcriptCellAssistant:
		return style.Foreground(lipgloss.Color("83"))
	case transcriptCellTool:
		return style.Foreground(lipgloss.Color("214"))
	case transcriptCellSafety:
		return style.Foreground(lipgloss.Color("203"))
	case transcriptCellError:
		return style.Foreground(lipgloss.Color("196"))
	default:
		return style.Foreground(lipgloss.Color("244"))
	}
}
