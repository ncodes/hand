package tui

import (
	"fmt"
	"strings"
	"time"

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
	if kind == transcriptCellAssistant {
		return renderTranscriptCellBody(kind, body, width)
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
	id          string
	action      string
	detail      string
	startedAt   time.Time
	completedAt time.Time
	completed   bool
}

type toolTranscriptDetail struct {
	id          string
	text        string
	startedAt   time.Time
	completedAt time.Time
	completed   bool
}

type toolTranscriptGroup struct {
	action       string
	details      []toolTranscriptDetail
	seenIDs      map[string]bool
	completedIDs map[string]bool
	completed    bool
}

func toolOperationTranscriptCell(id string, name string, detail string, completed ...bool) string {
	isCompleted := len(completed) > 0 && completed[0]

	return toolOperationTranscriptCellWithTiming(id, name, detail, time.Time{}, time.Time{}, isCompleted)
}

func toolOperationTranscriptCellWithTiming(
	id string,
	name string,
	detail string,
	startedAt time.Time,
	completedAt time.Time,
	completed bool,
) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	detail = normalizeToolTranscriptDetail(detail)
	if detail == "" {
		detail = name
	}
	lines := []string{}
	if id = strings.TrimSpace(id); id != "" {
		lines = append(lines, "id: "+id)
	}
	lines = append(lines, "detail: "+detail)
	if !startedAt.IsZero() {
		lines = append(lines, "started_at: "+startedAt.UTC().Format(time.RFC3339Nano))
	}
	if !completedAt.IsZero() {
		lines = append(lines, "completed_at: "+completedAt.UTC().Format(time.RFC3339Nano))
	}
	if completed {
		lines = append(lines, "status: completed")
	}

	return fmt.Sprintf("Tool %s:\n%s", getToolActionName(name), strings.Join(lines, "\n"))
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
		case "started_at":
			result.startedAt = parseToolTranscriptTime(value)
		case "completed_at":
			result.completedAt = parseToolTranscriptTime(value)
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
			group.mergeToolTranscriptCell(id, cell)
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
		group.details = append(group.details, toolTranscriptDetail{
			id:          strings.TrimSpace(cell.id),
			text:        detail,
			startedAt:   cell.startedAt,
			completedAt: cell.completedAt,
			completed:   cell.completed,
		})
	}
}

func (group *toolTranscriptGroup) mergeToolTranscriptCell(id string, cell toolTranscriptCell) {
	for index := range group.details {
		if group.details[index].id != id {
			continue
		}
		if group.details[index].startedAt.IsZero() {
			group.details[index].startedAt = cell.startedAt
		}
		if !cell.completedAt.IsZero() {
			group.details[index].completedAt = cell.completedAt
		}
		if cell.completed {
			group.details[index].completed = true
		}
		if group.details[index].text == "" {
			group.details[index].text = cell.detail
		}
		return
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

	headerTitle := getToolTranscriptTitle(action, completed)
	headerDuration := ""
	if len(group.details) == 1 {
		headerDuration = renderToolTranscriptDuration(group.details[0])
	}
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(getToolTranscriptDotColor(completed))).
		Bold(true).
		Render(getToolTranscriptDot(completed, frame)) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Render(" "+headerTitle) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("246")).
			Render(headerDuration)

	details := make([]toolTranscriptDetail, 0, len(group.details))
	for _, detail := range group.details {
		if strings.TrimSpace(detail.text) != "" {
			details = append(details, detail)
		}
	}
	if len(details) == 0 {
		return header
	}

	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	lines := []string{header}
	for index, detail := range details {
		branch := "├"
		if index == len(details)-1 {
			branch = "└"
		}
		detailText := getToolBranchDisplayDetail(group.action, detail.text, detail.completed)
		lines = append(lines, "  "+branchStyle.Render(branch)+" "+renderToolBranchDetail(detailText, renderToolTranscriptDuration(detail), detailStyle))
	}

	return strings.Join(lines, "\n")
}

func renderToolBranchDetail(detail string, duration string, style lipgloss.Style) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return style.Render(duration)
	}

	parts := strings.Fields(detail)
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		switch {
		case isToolDiffAdditionToken(part):
			rendered = append(rendered, lipgloss.NewStyle().Foreground(lipgloss.Color("83")).Render(part))
		case isToolDiffRemovalToken(part):
			rendered = append(rendered, lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(part))
		default:
			rendered = append(rendered, style.Render(part))
		}
	}

	return strings.Join(rendered, style.Render(" ")) + style.Render(duration)
}

func isToolDiffAdditionToken(value string) bool {
	return isToolSignedNumberToken(value, '+')
}

func isToolDiffRemovalToken(value string) bool {
	return isToolSignedNumberToken(value, '-')
}

func isToolSignedNumberToken(value string, sign byte) bool {
	if len(value) < 2 || value[0] != sign {
		return false
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
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

	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	lines := []string{header}
	for index, detail := range group.details {
		branch := "├"
		if index == len(group.details)-1 {
			branch = "└"
		}
		detailText := getToolBranchDisplayDetail(group.action, detail.text, detail.completed)
		lines = append(lines, "  "+branchStyle.Render(branch)+" "+detailStyle.Render("$ "+detailText+renderToolTranscriptDuration(detail)))
	}

	return strings.Join(lines, "\n")
}

func renderToolTranscriptDuration(detail toolTranscriptDetail) string {
	duration := getToolTranscriptDuration(detail)
	if duration <= 0 {
		return ""
	}

	return " (" + formatToolTranscriptDuration(duration) + ")"
}

func getToolTranscriptDuration(detail toolTranscriptDetail) time.Duration {
	if detail.startedAt.IsZero() {
		return 0
	}
	end := detail.completedAt
	if end.IsZero() {
		end = currentTime()
	}
	if end.Before(detail.startedAt) {
		return 0
	}

	return end.Sub(detail.startedAt).Round(time.Second)
}

func formatToolTranscriptDuration(duration time.Duration) string {
	seconds := int(duration.Seconds())
	if seconds < 1 {
		seconds = 1
	}

	return fmt.Sprintf("%ds", seconds)
}

func parseToolTranscriptTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}

	return parsed
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
	case "write", "write_file", "edit_file", "create_file":
		return "Write"
	case "patch", "apply_patch":
		return "Patch"
	case "web_search", "search_web", "search", "web":
		return "Web Search"
	case "memory_search", "search_memory", "memory":
		return "Memory Search"
	case "memory_extract", "extract_memory":
		return "Memory Extract"
	case "memory_add", "add_memory":
		return "Memory Add"
	case "memory_update", "update_memory":
		return "Memory Update"
	case "memory_delete", "delete_memory":
		return "Memory Delete"
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
	switch strings.TrimSpace(action) {
	case "Memory Search":
		if completed {
			return "Searched Memory"
		}

		return "Searching Memory"
	case "Memory Extract":
		if completed {
			return "Extracted Memory"
		}

		return "Extracting Memory"
	case "Memory Add":
		if completed {
			return "Added Memory"
		}

		return "Adding Memory"
	case "Memory Update":
		if completed {
			return "Updated Memory"
		}

		return "Updating Memory"
	case "Memory Delete":
		if completed {
			return "Deleted Memory"
		}

		return "Deleting Memory"
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
	case "Patch":
		return "Patch"
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
