package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/wordwrap"
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

const userTranscriptPrompt = inputPrompt
const userTranscriptBackground = "#151515"

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

	if kind == transcriptCellUser {
		return renderUserTranscriptCell(body, width)
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

func renderUserTranscriptCell(body string, width int) string {
	contentWidth := max(width, 1)
	wrapWidth := max(contentWidth-lipgloss.Width(userTranscriptPrompt), 1)

	lines := strings.Split(strings.TrimSpace(body), "\n")
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, wrapped := range strings.Split(wordwrap.String(strings.TrimSpace(line), wrapWidth), "\n") {
			if strings.TrimSpace(wrapped) != "" {
				rendered = append(rendered, renderUserTranscriptLine(wrapped, contentWidth, len(rendered) == 0))
			}
		}
	}
	if len(rendered) == 0 {
		return ""
	}

	return strings.Join(rendered, "\n")
}

func renderUserTranscriptLine(text string, width int, showPrompt bool) string {
	prefix := renderUserTranscriptContinuationPrefix()
	if showPrompt {
		prefix = renderUserTranscriptPrompt()
	}
	message := renderUserTranscriptText(text)
	usedWidth := lipgloss.Width(userTranscriptPrompt) + lipgloss.Width(text)
	filler := renderUserTranscriptFiller(max(width-usedWidth, 0))

	return prefix + message + filler
}

func renderUserTranscriptPrompt() string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(userTranscriptBackground)).
		Foreground(lipgloss.Color("245")).
		Render(userTranscriptPrompt)
}

func renderUserTranscriptContinuationPrefix() string {
	return renderUserTranscriptFiller(lipgloss.Width(userTranscriptPrompt))
}

func renderUserTranscriptText(text string) string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(userTranscriptBackground)).
		Foreground(lipgloss.Color("252")).
		Render(text)
}

func renderUserTranscriptFiller(width int) string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(userTranscriptBackground)).
		Render(strings.Repeat(" ", max(width, 0)))
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
