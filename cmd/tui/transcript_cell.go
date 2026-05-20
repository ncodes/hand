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
	transcriptCellReasoning transcriptCellKind = "reasoning"
	transcriptCellThought   transcriptCellKind = "thought"
	transcriptCellTool      transcriptCellKind = "tool"
	transcriptCellSafety    transcriptCellKind = "safety"
	transcriptCellError     transcriptCellKind = "error"
	transcriptCellSystem    transcriptCellKind = "system"
)

const userTranscriptPrompt = inputPrompt
const userTranscriptBackground = "#151515"

type transcriptRenderContext struct {
	Width int
	Frame int
	Now   time.Time
}

type transcriptCell interface {
	Kind() transcriptCellKind
	Render(transcriptRenderContext) string
	PlainText() string
	IsEmpty() bool
}

type userTranscriptCell struct {
	text string
}

type assistantTranscriptCell struct {
	text string
}

type reasoningTranscriptCell struct {
	text      string
	startedAt time.Time
}

type thoughtTranscriptCell struct {
	duration time.Duration
}

type safetyTranscriptCell struct {
	action     string
	findingIDs []string
}

type errorTranscriptCell struct {
	message string
}

type systemTranscriptCell struct {
	text string
}

func (cell userTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellUser
}

func (cell userTranscriptCell) Render(ctx transcriptRenderContext) string {
	return renderUserTranscriptCell(cell.text, ctx.Width)
}

func (cell userTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}

	return "You: " + strings.TrimSpace(cell.text)
}

func (cell userTranscriptCell) IsEmpty() bool {
	return strings.TrimSpace(cell.text) == ""
}

func (cell assistantTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellAssistant
}

func (cell assistantTranscriptCell) Render(ctx transcriptRenderContext) string {
	return renderMarkdownForTranscript(cell.text, ctx.Width)
}

func (cell assistantTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}

	return "Hand: " + cell.text
}

func (cell assistantTranscriptCell) IsEmpty() bool {
	return strings.TrimSpace(cell.text) == ""
}

func (cell reasoningTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellReasoning
}

func (cell reasoningTranscriptCell) Render(ctx transcriptRenderContext) string {
	return renderReasoningTranscriptCell(cell.text, ctx.Width)
}

func (cell reasoningTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}

	return "Reasoning: " + cell.text
}

func (cell reasoningTranscriptCell) IsEmpty() bool {
	return strings.TrimSpace(cell.text) == ""
}

func (cell thoughtTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellThought
}

func (cell thoughtTranscriptCell) Render(transcriptRenderContext) string {
	return renderThoughtTranscriptCell(formatToolTranscriptDuration(cell.duration))
}

func (cell thoughtTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}

	return "Thought: " + formatToolTranscriptDuration(cell.duration)
}

func (cell thoughtTranscriptCell) IsEmpty() bool {
	return cell.duration <= 0
}

func (cell safetyTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellSafety
}

func (cell safetyTranscriptCell) Render(transcriptRenderContext) string {
	return transcriptCellLabelStyle(transcriptCellSafety).Render("Safety:") + " " + cell.safetyText()
}

func (cell safetyTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}

	return "Safety: " + cell.safetyText()
}

func (cell safetyTranscriptCell) IsEmpty() bool {
	return strings.TrimSpace(cell.safetyText()) == ""
}

func (cell safetyTranscriptCell) safetyText() string {
	parts := []string{}
	if action := strings.TrimSpace(cell.action); action != "" {
		parts = append(parts, action)
	}
	if len(cell.findingIDs) > 0 {
		parts = append(parts, strings.Join(cell.findingIDs, ", "))
	}

	return strings.Join(parts, ": ")
}

func (cell errorTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellError
}

func (cell errorTranscriptCell) Render(transcriptRenderContext) string {
	return transcriptCellLabelStyle(transcriptCellError).Render("Error:") + " " + strings.TrimSpace(cell.message)
}

func (cell errorTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}

	return "Error: " + strings.TrimSpace(cell.message)
}

func (cell errorTranscriptCell) IsEmpty() bool {
	return strings.TrimSpace(cell.message) == ""
}

func (cell systemTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellSystem
}

func (cell systemTranscriptCell) Render(ctx transcriptRenderContext) string {
	return renderMarkdownForTranscript(cell.text, ctx.Width)
}

func (cell systemTranscriptCell) PlainText() string {
	return strings.TrimSpace(cell.text)
}

func (cell systemTranscriptCell) IsEmpty() bool {
	return strings.TrimSpace(cell.text) == ""
}

func renderTranscriptCells(cells []transcriptCell) string {
	return renderTranscriptCellsWithWidth(cells, defaultWidth)
}

func renderTranscriptCellsWithWidth(cells []transcriptCell, width int) string {
	return renderTranscriptCellsWithFrame(cells, width, 0)
}

func renderTranscriptCellsWithFrame(cells []transcriptCell, width int, frame int) string {
	rendered := make([]string, 0, len(cells))
	var toolGroup *toolTranscriptGroup
	ctx := transcriptRenderContext{Width: width, Frame: frame, Now: currentTime()}
	for _, cell := range cells {
		if cell == nil || cell.IsEmpty() {
			continue
		}

		if toolCell, ok := cell.(toolTranscriptCell); ok {
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
		if renderedCell := cell.Render(ctx); renderedCell != "" {
			rendered = append(rendered, renderedCell)
		}
	}
	flushToolTranscriptGroup(&rendered, &toolGroup, frame)

	return strings.Join(rendered, "\n\n")
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

func renderReasoningTranscriptCell(body string, width int) string {
	contentWidth := max(width, 1)
	wrapWidth := max(contentWidth-4, 1)
	dotStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

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

	rendered := []string{dotStyle.Render("◌") + " " + titleStyle.Render("Thinking")}
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
		Foreground(lipgloss.Color("244")).
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

func (cell toolTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellTool
}

func (cell toolTranscriptCell) Render(ctx transcriptRenderContext) string {
	group := toolTranscriptGroup{action: cell.action}
	group.add(cell)
	return renderToolTranscriptGroup(group, ctx.Frame)
}

func (cell toolTranscriptCell) PlainText() string {
	return toolTranscriptPlainText(cell)
}

func (cell toolTranscriptCell) IsEmpty() bool {
	return strings.TrimSpace(cell.action) == "" && strings.TrimSpace(cell.detail) == ""
}

func newToolTranscriptCell(
	id string,
	name string,
	detail string,
	startedAt time.Time,
	completedAt time.Time,
	completed bool,
) transcriptCell {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	detail = normalizeToolTranscriptDetail(detail)
	if detail == "" {
		detail = name
	}

	return toolTranscriptCell{
		id:          strings.TrimSpace(id),
		action:      getToolActionName(name),
		detail:      detail,
		startedAt:   startedAt,
		completedAt: completedAt,
		completed:   completed,
	}
}

func toolTranscriptPlainText(cell toolTranscriptCell) string {
	action := strings.TrimSpace(cell.action)
	if action == "" {
		return ""
	}
	lines := []string{}
	if id := strings.TrimSpace(cell.id); id != "" {
		lines = append(lines, "id: "+id)
	}
	lines = append(lines, "detail: "+strings.TrimSpace(cell.detail))
	if !cell.startedAt.IsZero() {
		lines = append(lines, "started_at: "+cell.startedAt.UTC().Format(time.RFC3339Nano))
	}
	if !cell.completedAt.IsZero() {
		lines = append(lines, "completed_at: "+cell.completedAt.UTC().Format(time.RFC3339Nano))
	}
	if cell.completed {
		lines = append(lines, "status: completed")
	}

	return fmt.Sprintf("Tool %s:\n%s", action, strings.Join(lines, "\n"))
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
	case "search_files":
		return "Search Files"
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
	case "Search Files":
		if completed {
			return "Searched Files"
		}

		return "Searching Files"
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
