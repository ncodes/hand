package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/wordwrap"

	"github.com/wandxy/morph/internal/trace"
	"github.com/wandxy/morph/pkg/stringx"
)

type transcriptRenderer interface {
	RenderCell(transcriptCell, transcriptRenderContext) string
	RenderCells([]transcriptCell, transcriptRenderContext) string
}

type lipglossTranscriptRenderer struct{}

const (
	assistantTranscriptIndicatorGlyph = "◉ "
	assistantTranscriptWorkGlyph      = "◷ "
)

var defaultTranscriptRenderer transcriptRenderer = lipglossTranscriptRenderer{}

func (lipglossTranscriptRenderer) RenderCell(cell transcriptCell, ctx transcriptRenderContext) string {
	if cell == nil || cell.IsEmpty() {
		return ""
	}

	switch value := cell.(type) {
	case userTranscriptCell:
		return renderUserTranscriptCell(value.text, ctx.Width)
	case assistantTranscriptCell:
		return renderAssistantTranscriptCell(value, ctx.Width)
	case reasoningTranscriptCell:
		return renderReasoningTranscriptCell(value.text, ctx.Width)
	case thoughtTranscriptCell:
		return renderThoughtTranscriptCell(formatToolTranscriptDuration(value.duration))
	case safetyTranscriptCell:
		return transcriptCellLabelStyle(transcriptCellSafety).Render("Safety:") + " " + value.safetyText()
	case errorTranscriptCell:
		return renderErrorTranscriptCell(value.message, ctx.Width)
	case systemTranscriptCell:
		return renderMarkdownForTranscript(value.text, ctx.Width)
	case manualCompactionTranscriptCell:
		return renderManualCompactionCell(value, ctx)
	case toolTranscriptCell:
		group := toolTranscriptGroup{action: value.action}
		group.add(value)
		return renderToolTranscriptGroupWithContext(group, ctx)
	default:
		return ""
	}
}

func (renderer lipglossTranscriptRenderer) RenderCells(cells []transcriptCell, ctx transcriptRenderContext) string {
	cells = compactMatchedToolTranscriptCells(cells)
	cells = compactConsecutiveProcessToolAttemptCells(cells)
	cells = compactConsecutiveManualCompactionCells(cells)
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

func compactMatchedToolTranscriptCells(cells []transcriptCell) []transcriptCell {
	if len(cells) <= 1 {
		return cells
	}

	compacted := make([]transcriptCell, 0, len(cells))
	toolIndexes := map[string]int{}
	for _, cell := range cells {
		if _, ok := cell.(userTranscriptCell); ok {
			toolIndexes = map[string]int{}
		}

		toolCell, ok := cell.(toolTranscriptCell)
		if !ok {
			compacted = append(compacted, cell)
			continue
		}

		id := stringx.String(toolCell.id).Trim()
		if id == "" {
			compacted = append(compacted, cell)
			continue
		}

		if toolCell.completed {
			if index, ok := toolIndexes[id]; ok {
				if existing, existingOK := compacted[index].(toolTranscriptCell); existingOK {
					compacted[index] = mergeToolTranscriptCells(existing, toolCell)
					continue
				}
			}
		}

		toolIndexes[id] = len(compacted)
		compacted = append(compacted, cell)
	}

	return compacted
}

func compactConsecutiveManualCompactionCells(cells []transcriptCell) []transcriptCell {
	if len(cells) <= 1 {
		return cells
	}

	compacted := make([]transcriptCell, 0, len(cells))
	for _, cell := range cells {
		if _, ok := cell.(manualCompactionTranscriptCell); ok {
			if len(compacted) > 0 {
				if _, previousOK := compacted[len(compacted)-1].(manualCompactionTranscriptCell); previousOK {
					compacted[len(compacted)-1] = cell
					continue
				}
			}
		}
		compacted = append(compacted, cell)
	}

	return compacted
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
	return stringx.String(cell.action).Trim() == "Process" && cell.processState != nil
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

	target := stringx.String(cell.processState.ProcessID).Trim()
	if cell.processState.Operation == trace.ProcessToolOperationStart || target == "" {
		target = stringx.String(cell.processState.Command).Trim()
	}

	return processToolDetailGroupKey{operation: cell.processState.Operation, target: target}
}

func renderUserTranscriptCell(body string, width int) string {
	contentWidth := max(width, 1)
	wrapWidth := max(contentWidth-lipgloss.Width(userTranscriptPrompt), 1)

	lines := strings.Split(stringx.String(body).Trim(), "\n")
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, wrapped := range strings.Split(wordwrap.String(stringx.String(line).Trim(), wrapWidth), "\n") {
			if stringx.String(wrapped).Trim() != "" {
				rendered = append(rendered, renderUserTranscriptLine(wrapped, contentWidth, len(rendered) == 0))
			}
		}
	}
	if len(rendered) == 0 {
		return ""
	}

	rendered = append([]string{renderUserTranscriptVerticalPadding(contentWidth, "▄")}, rendered...)
	rendered = append(rendered, renderUserTranscriptVerticalPadding(contentWidth, "▀"))

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

func renderUserTranscriptVerticalPadding(width int, glyph string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.UserTranscriptBackground)).
		Render(strings.Repeat(glyph, max(width, 0)))
}

func renderAssistantTranscriptCell(cell assistantTranscriptCell, width int) string {
	rendered := stringx.String(renderMarkdownForTranscript(cell.text, width)).Trim()
	if rendered == "" {
		return ""
	}

	lines := strings.Split(rendered, "\n")
	for index, line := range lines {
		if index == 0 {
			lines[index] = renderAssistantTranscriptIndicator() + line
			continue
		}
		lines[index] = renderAssistantTranscriptContinuationPrefix() + line
	}

	if cell.duration > 0 {
		lines = append(lines, "", renderAssistantTranscriptWorkLabel(cell.duration))
	}

	return strings.Join(lines, "\n")
}

func renderAssistantTranscriptIndicator() string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.NoticeForeground)).
		Render(assistantTranscriptIndicatorGlyph)
}

func renderAssistantTranscriptContinuationPrefix() string {
	return strings.Repeat(" ", lipgloss.Width(assistantTranscriptIndicatorGlyph))
}

func renderAssistantTranscriptWorkLabel(duration time.Duration) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.MutedText)).
		Render(assistantTranscriptWorkGlyph + "Worked for " + formatToolTranscriptDuration(duration))
}

func renderReasoningTranscriptCell(body string, width int) string {
	contentWidth := max(width, 1)
	wrapWidth := max(contentWidth-4, 1)
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolTitle))
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolBranch))
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(defaultTUITheme.ToolDetail))

	lines := strings.Split(stringx.String(body).Trim(), "\n")
	reasoningLines := make([]string, 0, len(lines))
	for _, line := range lines {
		line = stringx.String(line).Trim()
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
			wrapped = stringx.String(wrapped).Trim()
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
	duration := stringx.String(body).Trim()
	if duration == "" {
		return ""
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.ToolBranch)).
		Render("Thought for " + duration)
}

func renderErrorTranscriptCell(message string, width int) string {
	message = stringx.String(message).Trim()
	if message == "" {
		return ""
	}

	contentWidth := max(width, 1)
	bodyWidth := max(contentWidth-2, 1)
	background := lipgloss.Color(defaultTUITheme.InputFrameBackground)
	titleStyle := transcriptCellLabelStyle(transcriptCellError).Background(background)
	descriptionStyle := lipgloss.NewStyle().
		Background(background).
		MaxWidth(bodyWidth).
		Foreground(lipgloss.Color(defaultTUITheme.MutedText))
	bodyStyle := lipgloss.NewStyle().
		Background(background).
		MaxWidth(bodyWidth).
		Foreground(lipgloss.Color(defaultTUITheme.ToolDetail))
	commandStyle := lipgloss.NewStyle().
		Background(background).
		MaxWidth(bodyWidth).
		Foreground(lipgloss.Color("15"))
	title := titleStyle.Render("Error")
	content := []string{title}
	if command, description, instruction, ok := getErrorTranscriptCommandInstruction(message); ok {
		content[0] = title + descriptionStyle.Render(" - "+description)
		content = append(
			content,
			"",
			commandStyle.Render(stringx.String(wordwrap.String(command, bodyWidth)).Trim()),
			"",
			bodyStyle.Render(stringx.String(wordwrap.String(instruction, bodyWidth)).Trim()),
		)
	} else {
		content = append(content, "", bodyStyle.Render(stringx.String(wordwrap.String(message, bodyWidth)).Trim()))
	}

	return lipgloss.NewStyle().
		Width(contentWidth).
		Background(background).
		Padding(1, 1).
		Render(strings.Join(content, "\n"))
}

func getErrorTranscriptCommandInstruction(message string) (string, string, string, bool) {
	message = stringx.String(message).Trim()
	const prefix = "run "
	const suffix = " in a new terminal"
	if !strings.HasPrefix(message, prefix) || !strings.HasSuffix(message, suffix) {
		return "", "", "", false
	}

	command := stringx.String(strings.TrimSuffix(strings.TrimPrefix(message, prefix), suffix)).Trim()
	if command == "" {
		return "", "", "", false
	}

	return command, "Model authentication is required.", "Run this command in a new terminal.", true
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
