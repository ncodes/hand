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
	return renderTranscriptCellsWithWidth(cells, defaultWidth)
}

func renderTranscriptCellsWithWidth(cells []string, width int) string {
	rendered := make([]string, 0, len(cells))
	for _, cell := range cells {
		if cell = strings.TrimSpace(cell); cell != "" {
			rendered = append(rendered, renderTranscriptCellWithWidth(cell, width))
		}
	}

	return strings.Join(rendered, "\n\n")
}

func renderTranscriptCell(cell string) string {
	return renderTranscriptCellWithWidth(cell, defaultWidth)
}

func renderTranscriptCellWithWidth(cell string, width int) string {
	kind, label, body := parseTranscriptCell(cell)
	if strings.TrimSpace(body) == "" {
		return ""
	}

	labelStyle := transcriptCellLabelStyle(kind)
	if label == "" {
		return renderTranscriptCellBody(kind, body, width)
	}

	if rendered := renderTranscriptCellBody(kind, body, width); rendered != body {
		return labelStyle.Render(label+":") + "\n" + rendered
	}

	return labelStyle.Render(label+":") + " " + body
}

func renderTranscriptCellBody(kind transcriptCellKind, body string, width int) string {
	switch kind {
	case transcriptCellAssistant, transcriptCellSystem:
		return renderMarkdownForTranscript(body, width)
	default:
		return body
	}
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
