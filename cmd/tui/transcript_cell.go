package tui

import (
	"fmt"
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
	return renderTranscriptCellsWithFrame(cells, width, 0)
}

func renderTranscriptCellsWithFrame(cells []string, width int, frame int) string {
	rendered := make([]string, 0, len(cells))
	var toolGroup *toolTranscriptGroup
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}

		if toolCell, ok := parseToolTranscriptCell(cell); ok {
			if toolGroup == nil || toolGroup.action != toolCell.action {
				flushToolTranscriptGroup(&rendered, &toolGroup, frame)
			}
			if toolGroup == nil {
				toolGroup = &toolTranscriptGroup{action: toolCell.action}
			}
			toolGroup.add(toolCell)
			continue
		}

		flushToolTranscriptGroup(&rendered, &toolGroup, frame)
		if renderedCell := renderTranscriptCellWithWidth(cell, width); renderedCell != "" {
			rendered = append(rendered, renderedCell)
		}
	}
	flushToolTranscriptGroup(&rendered, &toolGroup, frame)

	return strings.Join(rendered, "\n\n")
}

func renderTranscriptCell(cell string) string {
	return renderTranscriptCellWithWidth(cell, defaultWidth)
}

func renderTranscriptCellWithWidth(cell string, width int) string {
	if toolCell, ok := parseToolTranscriptCell(cell); ok {
		group := toolTranscriptGroup{action: toolCell.action}
		group.add(toolCell)
		return renderToolTranscriptGroup(group, 0)
	}

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

func renderUserTranscriptTopHeightStrip(width int) string {
	return renderUserTranscriptHeightStrip("▄", width)
}

func renderUserTranscriptBottomHeightStrip(width int) string {
	return renderUserTranscriptHeightStrip("▀", width)
}

func renderUserTranscriptHeightStrip(block string, width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(userTranscriptBackground)).
		Render(strings.Repeat(block, max(width, 0)))
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

type toolTranscriptCell struct {
	id        string
	action    string
	detail    string
	completed bool
}

type toolTranscriptGroup struct {
	action       string
	details      []string
	seenIDs      map[string]bool
	completedIDs map[string]bool
	completed    bool
}

func toolOperationTranscriptCell(id string, name string, detail string, completed ...bool) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	detail = normalizeToolTranscriptDetail(detail)
	if detail == "" {
		detail = name
	}
	statusLine := ""
	if len(completed) > 0 && completed[0] {
		statusLine = "\nstatus: completed"
	}
	if id = strings.TrimSpace(id); id != "" {
		return fmt.Sprintf("Tool %s:\nid: %s\ndetail: %s%s", getToolActionName(name), id, detail, statusLine)
	}

	return fmt.Sprintf("Tool %s:\ndetail: %s%s", getToolActionName(name), detail, statusLine)
}

func parseToolTranscriptCell(cell string) (toolTranscriptCell, bool) {
	kind, label, body := parseTranscriptCell(cell)
	if kind != transcriptCellTool {
		return toolTranscriptCell{}, false
	}

	label = strings.TrimSpace(strings.TrimPrefix(label, "Tool"))
	action := strings.Trim(strings.TrimSpace(label), ":")
	if action == "" {
		action = "Tool"
	}

	result := toolTranscriptCell{action: action, detail: strings.TrimSpace(body)}
	for _, line := range strings.Split(body, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "id":
			result.id = strings.TrimSpace(value)
		case "detail":
			result.detail = normalizeToolTranscriptDetail(value)
		case "status":
			result.completed = strings.EqualFold(strings.TrimSpace(value), "completed")
		}
	}
	if strings.TrimSpace(result.detail) == "" {
		result.detail = strings.TrimSpace(action)
	}

	return result, true
}

func (group *toolTranscriptGroup) add(cell toolTranscriptCell) {
	if group == nil {
		return
	}
	if id := strings.TrimSpace(cell.id); id != "" {
		if group.seenIDs == nil {
			group.seenIDs = map[string]bool{}
		}
		if cell.completed {
			if group.completedIDs == nil {
				group.completedIDs = map[string]bool{}
			}
			group.completedIDs[id] = true
		}
		if group.seenIDs[id] {
			return
		}
		group.seenIDs[id] = true
	} else if cell.completed {
		group.completed = true
	}

	detail := strings.TrimSpace(cell.detail)
	if detail == "" {
		detail = strings.TrimSpace(cell.action)
	}
	if detail != "" {
		group.details = append(group.details, detail)
	}
}

func flushToolTranscriptGroup(rendered *[]string, group **toolTranscriptGroup, frame int) {
	if group == nil || *group == nil {
		return
	}
	if cell := renderToolTranscriptGroup(**group, frame); cell != "" {
		*rendered = append(*rendered, cell)
	}
	*group = nil
}

func renderToolTranscriptGroup(group toolTranscriptGroup, frame int) string {
	action := strings.TrimSpace(group.action)
	if action == "" {
		action = "Tool"
	}
	if action == "Run" {
		return renderRunTranscriptGroup(group, frame)
	}
	completed := group.isCompleted()

	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(getToolTranscriptDotColor(completed))).
		Bold(true).
		Render(getToolTranscriptDot(completed, frame)) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Render(" "+getToolTranscriptTitle(action, completed))

	details := make([]string, 0, len(group.details))
	for _, detail := range group.details {
		if detail = strings.TrimSpace(detail); detail != "" {
			details = append(details, detail)
		}
	}
	if len(details) == 0 {
		return header
	}

	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	lines := []string{header}
	for index, detail := range details {
		branch := "├"
		if index == len(details)-1 {
			branch = "└"
		}
		lines = append(lines, "  "+branchStyle.Render(branch)+" "+detailStyle.Render(detail))
	}

	return strings.Join(lines, "\n")
}

func renderRunTranscriptGroup(group toolTranscriptGroup, frame int) string {
	count := len(group.details)
	if count == 0 {
		count = 1
	}

	noun := "shell command"
	if count != 1 {
		noun = "shell commands"
	}
	verb := "Running"
	suffix := "…"
	completed := group.isCompleted()
	if completed {
		verb = "Ran"
		suffix = ""
	}
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(getToolTranscriptDotColor(completed))).
		Bold(true).
		Render(getToolTranscriptDot(completed, frame)) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Render(" "+verb+" ") +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true).
			Render(fmt.Sprintf("%d", count)) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Render(" "+noun+suffix)

	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	lines := []string{header}
	for index, detail := range group.details {
		branch := "├"
		if index == len(group.details)-1 {
			branch = "└"
		}
		lines = append(lines, "  "+branchStyle.Render(branch)+" "+detailStyle.Render("$ "+detail))
	}

	return strings.Join(lines, "\n")
}

func (group toolTranscriptGroup) isCompleted() bool {
	if len(group.seenIDs) == 0 {
		return group.completed
	}

	for id := range group.seenIDs {
		if !group.completedIDs[id] {
			return false
		}
	}

	return true
}

func getToolActionName(name string) string {
	normalized := strings.TrimSpace(strings.ToLower(name))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "read", "read_file", "view_file", "open_file", "cat":
		return "Read"
	case "write", "write_file", "edit_file", "apply_patch", "create_file":
		return "Write"
	case "web_search", "search_web", "search", "web":
		return "Web Search"
	case "memory_search", "search_memory", "memory":
		return "Memory Search"
	case "exec", "exec_command", "run", "run_command", "shell", "bash", "process":
		return "Run"
	default:
		return humanizeToolActionName(name)
	}
}

func getToolTranscriptDotColor(completed bool) string {
	if completed {
		return "83"
	}

	return "250"
}

func getToolTranscriptDot(completed bool, frame int) string {
	if completed {
		return "●"
	}

	frames := []string{"●", "◖", "◐", "◗", "●", "◔"}
	index := frame % len(frames)
	if index < 0 {
		index += len(frames)
	}

	return frames[index]
}

func getToolTranscriptTitle(action string, completed bool) string {
	if strings.TrimSpace(action) == "Memory Search" {
		if completed {
			return "Searched Memory"
		}

		return "Searching Memory"
	}

	if !completed {
		return action
	}

	switch strings.TrimSpace(action) {
	case "Run":
		return "Ran"
	case "Write":
		return "Wrote"
	case "Web Search":
		return "Searched"
	case "Read":
		return "Read"
	default:
		return strings.TrimSpace(action)
	}
}

func normalizeToolTranscriptDetail(detail string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(detail)), " ")
}

func humanizeToolActionName(name string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(name), func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for index, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		parts[index] = string(runes)
	}

	return strings.Join(parts, " ")
}
