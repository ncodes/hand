package tui

import (
	"strings"
)

type transcriptRenderer interface {
	RenderCell(transcriptCell, transcriptRenderContext) string
	RenderCells([]transcriptCell, transcriptRenderContext) string
}

type lipglossTranscriptRenderer struct{}

var defaultTranscriptRenderer transcriptRenderer = lipglossTranscriptRenderer{}

func (lipglossTranscriptRenderer) RenderCell(cell transcriptCell, ctx transcriptRenderContext) string {
	if cell == nil || cell.IsEmpty() {
		return ""
	}

	switch value := cell.(type) {
	case userTranscriptCell:
		return renderUserTranscriptCell(value.text, ctx.Width)
	case assistantTranscriptCell:
		return renderMarkdownForTranscript(value.text, ctx.Width)
	case reasoningTranscriptCell:
		return renderReasoningTranscriptCell(value.text, ctx.Width)
	case thoughtTranscriptCell:
		return renderThoughtTranscriptCell(formatToolTranscriptDuration(value.duration))
	case safetyTranscriptCell:
		return transcriptCellLabelStyle(transcriptCellSafety).Render("Safety:") + " " + value.safetyText()
	case errorTranscriptCell:
		return transcriptCellLabelStyle(transcriptCellError).Render("Error:") + " " + strings.TrimSpace(value.message)
	case systemTranscriptCell:
		return renderMarkdownForTranscript(value.text, ctx.Width)
	case toolTranscriptCell:
		group := toolTranscriptGroup{action: value.action}
		group.add(value)
		return renderToolTranscriptGroupWithContext(group, ctx)
	default:
		return ""
	}
}

func (renderer lipglossTranscriptRenderer) RenderCells(cells []transcriptCell, ctx transcriptRenderContext) string {
	rendered := make([]string, 0, len(cells))
	var toolGroup *toolTranscriptGroup
	for _, cell := range cells {
		if cell == nil || cell.IsEmpty() {
			continue
		}

		if toolCell, ok := cell.(toolTranscriptCell); ok {
			if toolGroup == nil || toolGroup.action != toolCell.action {
				flushToolTranscriptGroupWithContext(&rendered, &toolGroup, ctx)
			}
			if toolGroup == nil {
				toolGroup = &toolTranscriptGroup{action: toolCell.action}
			}
			toolGroup.add(toolCell)
			continue
		}

		flushToolTranscriptGroupWithContext(&rendered, &toolGroup, ctx)
		if renderedCell := renderer.RenderCell(cell, ctx); renderedCell != "" {
			rendered = append(rendered, renderedCell)
		}
	}
	flushToolTranscriptGroupWithContext(&rendered, &toolGroup, ctx)

	return strings.Join(rendered, "\n\n")
}
