package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/wordwrap"

	"github.com/wandxy/hand/internal/trace"
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
	cells = compactConsecutiveProcessToolAttemptCells(cells)
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

func compactConsecutiveProcessToolAttemptCells(cells []transcriptCell) []transcriptCell {
	if len(cells) <= 1 {
		return cells
	}

	compacted := make([]transcriptCell, 0, len(cells))
	for index := 0; index < len(cells); {
		cell := cells[index]
		compacted = append(compacted, cell)

		toolCell, ok := cell.(toolTranscriptCell)
		if !ok || !isProcessToolTranscriptCell(toolCell) {
			index++
			continue
		}

		nextIndex := index + 1
		pending := make([]transcriptCell, 0, 1)
		for nextIndex < len(cells) {
			next := cells[nextIndex]
			if next == nil || next.IsEmpty() {
				nextIndex++
				continue
			}
			if _, ok := next.(thoughtTranscriptCell); ok {
				pending = append(pending, next)
				nextIndex++
				continue
			}

			nextToolCell, ok := next.(toolTranscriptCell)
			if !ok || !isEquivalentProcessToolAttempt(toolCell, nextToolCell) {
				break
			}

			compacted = append(compacted, nextToolCell)
			pending = pending[:0]
			nextIndex++
		}

		if nextIndex > index+1 {
			compacted = append(compacted, pending...)
			index = nextIndex
			continue
		}
		index++
	}

	return compacted
}

func isProcessToolTranscriptCell(cell toolTranscriptCell) bool {
	return strings.TrimSpace(cell.action) == "Process" && cell.processState != nil
}

func isEquivalentProcessToolAttempt(current toolTranscriptCell, next toolTranscriptCell) bool {
	if !isProcessToolTranscriptCell(current) || !isProcessToolTranscriptCell(next) {
		return false
	}
	if current.id != "" && current.id == next.id {
		return true
	}

	currentKey := getProcessToolCellGroupKey(current)
	nextKey := getProcessToolCellGroupKey(next)
	return currentKey.operation != "" &&
		currentKey.operation == nextKey.operation &&
		currentKey.target != "" &&
		currentKey.target == nextKey.target
}

func getProcessToolCellGroupKey(cell toolTranscriptCell) processToolDetailGroupKey {
	if cell.processState == nil {
		return processToolDetailGroupKey{}
	}

	target := strings.TrimSpace(cell.processState.ProcessID)
	if cell.processState.Operation == trace.ProcessToolOperationStart || target == "" {
		target = strings.TrimSpace(cell.processState.Command)
	}

	return processToolDetailGroupKey{operation: cell.processState.Operation, target: target}
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

	rendered = append([]string{renderUserTranscriptTopHeightStrip(contentWidth)}, rendered...)
	rendered = append(rendered, renderUserTranscriptBottomHeightStrip(contentWidth))

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
		Background(lipgloss.Color(defaultTUITheme.UserTranscriptBackground)).
		Foreground(lipgloss.Color(defaultTUITheme.UserTranscriptPrompt)).
		Render(userTranscriptPrompt)
}

func renderUserTranscriptContinuationPrefix() string {
	return renderUserTranscriptFiller(lipgloss.Width(userTranscriptPrompt))
}

func renderUserTranscriptText(text string) string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.UserTranscriptBackground)).
		Foreground(lipgloss.Color(defaultTUITheme.UserTranscriptText)).
		Render(text)
}

func renderUserTranscriptFiller(width int) string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(defaultTUITheme.UserTranscriptBackground)).
		Render(strings.Repeat(" ", max(width, 0)))
}

func renderUserTranscriptTopHeightStrip(width int) string {
	return renderUserTranscriptHeightStrip("▄", width)
}

func renderUserTranscriptBottomHeightStrip(width int) string {
	return renderUserTranscriptHeightStrip("▀", width)
}

func renderUserTranscriptHeightStrip(block string, width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.UserTranscriptBackground)).
		Render(strings.Repeat(block, max(width, 0)))
}

func renderReasoningTranscriptCell(body string, width int) string {
	contentWidth := max(width, 1)
	wrapWidth := max(contentWidth-4, 1)
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolTitle))
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolBranch))
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolDetail))

	lines := strings.Split(strings.TrimSpace(body), "\n")
	reasoningLines := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			reasoningLines = append(reasoningLines, line)
		}
	}
	if len(reasoningLines) == 0 {
		return ""
	}

	rendered := []string{titleStyle.Render("Thinking")}
	first := true
	for _, line := range reasoningLines {
		for _, wrapped := range strings.Split(wordwrap.String(line, wrapWidth), "\n") {
			wrapped = strings.TrimSpace(wrapped)
			if wrapped == "" {
				continue
			}

			branch := "  "
			if first {
				branch = "└ "
				first = false
			}
			rendered = append(rendered, "  "+branchStyle.Render(branch)+textStyle.Render(wrapped))
		}
	}

	return strings.Join(rendered, "\n")
}

func renderThoughtTranscriptCell(body string) string {
	duration := strings.TrimSpace(body)
	if duration == "" {
		return ""
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.ToolBranch)).
		Render("Thought for " + duration)
}

func transcriptCellLabelStyle(kind transcriptCellKind) lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true)
	switch kind {
	case transcriptCellUser:
		return style.Foreground(lipgloss.Color("39"))
	case transcriptCellAssistant:
		return style.Foreground(lipgloss.Color("83"))
	case transcriptCellReasoning:
		return style.Foreground(lipgloss.Color("246"))
	case transcriptCellThought:
		return style.Foreground(lipgloss.Color("244"))
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
