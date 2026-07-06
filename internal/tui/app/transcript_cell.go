package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/wandxy/morph/internal/trace"
	"github.com/wandxy/morph/pkg/str"
)

type transcriptCellKind string

const (
	transcriptCellUser       transcriptCellKind = "user"
	transcriptCellAssistant  transcriptCellKind = "assistant"
	transcriptCellReasoning  transcriptCellKind = "reasoning"
	transcriptCellThought    transcriptCellKind = "thought"
	transcriptCellTool       transcriptCellKind = "tool"
	transcriptCellSafety     transcriptCellKind = "safety"
	transcriptCellError      transcriptCellKind = "error"
	transcriptCellSystem     transcriptCellKind = "system"
	transcriptCellCompaction transcriptCellKind = "compaction"
)

const userTranscriptPrompt = inputPrompt

type transcriptRenderContext struct {
	Width int
	Frame int
	Now   time.Time
}

type transcriptCell interface {
	Kind() transcriptCellKind
	PlainText() string
	IsEmpty() bool
}

type userTranscriptCell struct {
	text string
}

type assistantTranscriptCell struct {
	text     string
	duration time.Duration
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

type manualCompactionTranscriptCell struct {
	state manualCompactionState
}

func (cell userTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellUser
}

func (cell userTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}
	stringValue1 := str.String(cell.text)
	return "You: " + stringValue1.Trim()
}

func (cell userTranscriptCell) IsEmpty() bool {
	stringValue2 := str.String(cell.text)
	return stringValue2.Trim() == ""
}

func (cell assistantTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellAssistant
}

func (cell assistantTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}

	text := "Morph: " + cell.text
	if cell.duration > 0 {
		text += "\nWorked for " + formatToolTranscriptDuration(cell.duration)
	}

	return text
}

func (cell assistantTranscriptCell) IsEmpty() bool {
	stringValue3 := str.String(cell.text)
	return stringValue3.Trim() == ""
}

func (cell reasoningTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellReasoning
}

func (cell reasoningTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}

	return "Reasoning: " + cell.text
}

func (cell reasoningTranscriptCell) IsEmpty() bool {
	stringValue4 := str.String(cell.text)
	return stringValue4.Trim() == ""
}

func (cell thoughtTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellThought
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

func (cell safetyTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}

	return "Safety: " + cell.safetyText()
}

func (cell safetyTranscriptCell) IsEmpty() bool {
	stringValue5 := str.String(cell.safetyText())
	return stringValue5.Trim() == ""
}

func (cell safetyTranscriptCell) safetyText() string {
	parts := []string{}
	stringValue6 := str.String(cell.action)
	if action := stringValue6.Trim(); action != "" {
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

func (cell errorTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}
	stringValue7 := str.String(cell.message)
	return "Error: " + stringValue7.Trim()
}

func (cell errorTranscriptCell) IsEmpty() bool {
	stringValue8 := str.String(cell.message)
	return stringValue8.Trim() == ""
}

func (cell systemTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellSystem
}

func (cell systemTranscriptCell) PlainText() string {
	stringValue9 := str.String(cell.text)
	return stringValue9.Trim()
}

func (cell systemTranscriptCell) IsEmpty() bool {
	stringValue10 := str.String(cell.text)
	return stringValue10.Trim() == ""
}

func (cell manualCompactionTranscriptCell) Kind() transcriptCellKind {
	return transcriptCellCompaction
}

func (cell manualCompactionTranscriptCell) PlainText() string {
	if cell.IsEmpty() {
		return ""
	}

	return cell.state.displayText()
}

func (cell manualCompactionTranscriptCell) IsEmpty() bool {
	stringValue11 := str.String(cell.state.displayText())
	return stringValue11.Trim() == ""
}

func renderTranscriptCells(cells []transcriptCell) string {
	return renderTranscriptCellsWithWidth(cells, defaultWidth)
}

func renderTranscriptCellsWithWidth(cells []transcriptCell, width int) string {
	return renderTranscriptCellsWithFrame(cells, width, 0)
}

func renderTranscriptCellsWithFrame(cells []transcriptCell, width int, frame int) string {
	ctx := transcriptRenderContext{Width: width, Frame: frame, Now: currentTime()}
	return defaultTranscriptRenderer.RenderCells(cells, ctx)
}

type toolTranscriptCell struct {
	id           string
	action       string
	detail       string
	planState    *trace.PlanToolState
	processState *trace.ProcessToolState
	startedAt    time.Time
	completedAt  time.Time
	completed    bool
}

type toolTranscriptDetail struct {
	id           string
	text         string
	planState    *trace.PlanToolState
	processState *trace.ProcessToolState
	startedAt    time.Time
	completedAt  time.Time
	completed    bool
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

func (cell toolTranscriptCell) PlainText() string {
	return toolTranscriptPlainText(cell)
}

func (cell toolTranscriptCell) IsEmpty() bool {
	stringValue12 := str.String(cell.action)
	stringValue13 := str.String(cell.detail)
	return stringValue12.Trim() == "" && stringValue13.
		Trim() == "" &&
		cell.planState == nil &&
		cell.processState == nil
}

func newToolTranscriptCell(
	id string,
	name string,
	detail string,
	planState *trace.PlanToolState,
	processState *trace.ProcessToolState,
	startedAt time.Time,
	completedAt time.Time,
	completed bool,
) transcriptCell {
	stringValue14 := str.String(name)
	name = stringValue14.Trim()
	if name == "" {
		return nil
	}

	detail = normalizeToolTranscriptDetail(detail)
	if detail == "" && planState == nil && processState == nil {
		detail = name
	}
	stringValue15 := str.String(id)
	return toolTranscriptCell{
		id:           stringValue15.Trim(),
		action:       getToolActionName(name),
		detail:       detail,
		planState:    planState,
		processState: processState,
		startedAt:    startedAt,
		completedAt:  completedAt,
		completed:    completed,
	}
}

func toolTranscriptPlainText(cell toolTranscriptCell) string {
	stringValue16 := str.String(cell.action)
	action := stringValue16.Trim()
	if action == "" {
		return ""
	}
	lines := []string{}
	stringValue17 := str.String(cell.id)
	if id := stringValue17.Trim(); id != "" {
		lines = append(lines, "id: "+id)
	}
	stringValue18 := str.String(cell.detail)
	lines = append(lines, "detail: "+stringValue18.Trim())
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
	stringValue19 := str.String(cell.id)
	if id := stringValue19.Trim(); id != "" {
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
	stringValue20 := str.String(cell.detail)
	detail := stringValue20.Trim()
	if detail == "" {
		stringValue21 := str.String(cell.action)
		detail = stringValue21.Trim()
	}
	if detail != "" || cell.planState != nil || cell.processState != nil {
		stringValue22 := str.String(cell.id)
		group.details = append(group.details, toolTranscriptDetail{
			id:           stringValue22.Trim(),
			text:         detail,
			planState:    clonePlanToolDisplayState(cell.planState),
			processState: cloneProcessToolDisplayState(cell.processState),
			startedAt:    cell.startedAt,
			completedAt:  cell.completedAt,
			completed:    cell.completed,
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
		if merged := mergePlanToolDisplayState(group.details[index].planState, cell.planState); merged != nil {
			group.details[index].planState = merged
		}
		if merged := mergeProcessToolDisplayState(group.details[index].processState, cell.processState); merged != nil {
			group.details[index].processState = merged
		}
		if group.details[index].text == "" {
			group.details[index].text = cell.detail
		}
		return
	}
}

func flushToolTranscriptGroupWithContext(
	rendered *[]string,
	group **toolTranscriptGroup,
	ctx transcriptRenderContext,
) {
	if group == nil || *group == nil {
		return
	}
	if cell := renderToolTranscriptGroupWithContext(**group, ctx); cell != "" {
		*rendered = append(*rendered, cell)
	}
	*group = nil
}

func renderToolTranscriptGroup(group toolTranscriptGroup, frame int) string {
	return renderToolTranscriptGroupWithContext(
		group,
		transcriptRenderContext{Frame: frame, Now: currentTime()},
	)
}

func renderToolTranscriptGroupWithContext(group toolTranscriptGroup, ctx transcriptRenderContext) string {
	return defaultToolTranscriptRenderer.RenderGroup(group, ctx)
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
	stringValue23 := str.String(name)
	normalized := stringValue23.Normalized()
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
	case "web_extract", "extract_web":
		return "Web Extract"
	case "plan", "plan_tool", "update_plan":
		return "Plan"
	case "search_files":
		return "Search Files"
	case "session_search", "search_session":
		return "Session Search"
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
	case "session_message", "session_messages":
		return "Session Messages"
	case "process":
		return "Process"
	case "exec", "exec_command", "run", "run_command", "shell", "bash":
		return "Run"
	default:
		return humanizeToolActionName(name)
	}
}

func normalizeToolTranscriptDetail(detail string) string {
	stringValue24 := str.String(detail)
	return strings.Join(strings.Fields(stringValue24.Trim()), " ")
}

func humanizeToolActionName(name string) string {
	stringValue25 := str.String(name)
	parts := strings.FieldsFunc(stringValue25.Trim(), func(r rune) bool {
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
