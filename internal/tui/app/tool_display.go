package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/wandxy/morph/internal/guardrails"
	"github.com/wandxy/morph/internal/trace"
)

var runToolLegacyTimeoutPattern = regexp.MustCompile(`\s+\(([0-9]+(?:\.[0-9]+)?s)\)$`)
var runToolTimeoutHintPattern = regexp.MustCompile(`\s+\[(?:terminates in|timeout) [^\]]+\]`)

type toolDisplaySpec struct {
	inputDetail  func(map[string]any) string
	outputDetail func(map[string]any) string
	inputState   func(map[string]any) *trace.PlanToolState
	outputState  func(map[string]any) *trace.PlanToolState
	branchDetail func(string, bool) string
}

func getToolInputDisplayDetail(name string, input string) string {
	var fields map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &fields); err != nil {
		return ""
	}

	spec := getToolDisplaySpec(name)
	if spec.inputDetail == nil {
		return ""
	}

	return spec.inputDetail(fields)
}

func getToolInputDisplayState(name string, input string) *trace.PlanToolState {
	var fields map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &fields); err != nil {
		return nil
	}

	spec := getToolDisplaySpec(name)
	if spec.inputState == nil {
		return nil
	}

	return spec.inputState(fields)
}

func getToolOutputDisplayDetail(name string, output string) string {
	var fields map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &fields); err != nil {
		return ""
	}

	spec := getToolDisplaySpec(name)
	if spec.outputDetail == nil {
		return ""
	}

	return spec.outputDetail(fields)
}

func getToolOutputDisplayState(name string, output string) *trace.PlanToolState {
	var fields map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &fields); err != nil {
		return nil
	}

	spec := getToolDisplaySpec(name)
	if spec.outputState == nil {
		return nil
	}

	return spec.outputState(fields)
}

func getToolInputProcessDisplayState(name string, input string) *trace.ProcessToolState {
	if getToolActionName(name) != "Process" {
		return nil
	}

	return trace.ProcessToolInputState(input)
}

func getToolOutputProcessDisplayState(name string, output string) *trace.ProcessToolState {
	if getToolActionName(name) != "Process" {
		return nil
	}

	return trace.ProcessToolOutputState(output)
}

func getToolBranchDisplayDetail(action string, detail string, completed bool) string {
	spec := getToolDisplaySpecForAction(action)
	if spec.branchDetail == nil {
		return strings.TrimSpace(detail)
	}

	return spec.branchDetail(detail, completed)
}

func getToolTranscriptBranchDisplayDetail(action string, detail toolTranscriptDetail) string {
	if strings.TrimSpace(action) == "Plan" {
		return getPlanToolBranchDetail(detail.planState, detail.completed)
	}
	if strings.TrimSpace(action) == "Process" {
		if branch := getProcessToolBranchDetail(detail.processState, detail.completed); branch != "" {
			return branch
		}

		return strings.TrimSpace(detail.text)
	}

	return getToolBranchDisplayDetail(action, detail.text, detail.completed)
}

func getToolDisplaySpec(name string) toolDisplaySpec {
	switch normalizeToolDisplayName(name) {
	case "search_files":
		return toolDisplaySpec{
			inputDetail: getSearchFilesToolDisplayDetail,
		}
	case "read", "read_file", "view_file", "open_file", "cat":
		return toolDisplaySpec{
			inputDetail: func(fields map[string]any) string {
				return getPathToolDisplayDetail(name, fields)
			},
		}
	case "write", "write_file", "edit_file", "create_file":
		return toolDisplaySpec{
			inputDetail: func(fields map[string]any) string {
				return getPathToolDisplayDetail(name, fields)
			},
		}
	case "patch", "apply_patch":
		return toolDisplaySpec{
			inputDetail: func(fields map[string]any) string {
				return getPatchToolDisplayDetail(name, fields)
			},
		}
	}

	action := getToolActionName(name)
	spec := getToolDisplaySpecForAction(action)
	if spec.inputDetail != nil || spec.outputDetail != nil || spec.inputState != nil ||
		spec.outputState != nil || spec.branchDetail != nil {
		return spec
	}

	if isGenericToolParamDisplayEnabled(name) {
		return toolDisplaySpec{
			inputDetail: func(fields map[string]any) string {
				return getGenericToolDisplayDetail(name, fields)
			},
		}
	}

	return toolDisplaySpec{}
}

func normalizeToolDisplayName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	return strings.ReplaceAll(name, "-", "_")
}

func getToolDisplaySpecForAction(action string) toolDisplaySpec {
	switch strings.TrimSpace(action) {
	case "Run":
		return toolDisplaySpec{
			inputDetail:  getRunToolDisplayDetail,
			branchDetail: normalizeRunToolDetailText,
		}
	case "Web Search", "Memory Search", "Session Search":
		return toolDisplaySpec{
			inputDetail: getSearchToolDisplayDetail,
		}
	case "Plan":
		return toolDisplaySpec{
			inputState:  getPlanToolInputDisplayState,
			outputState: getPlanToolOutputDisplayState,
		}
	case "Memory Extract":
		return toolDisplaySpec{branchDetail: getStaticToolBranchDetail("Extract memories")}
	case "Memory Add":
		return toolDisplaySpec{branchDetail: getStaticToolBranchDetail("Add memory")}
	case "Memory Update":
		return toolDisplaySpec{branchDetail: getStaticToolBranchDetail("Update memory")}
	case "Memory Delete":
		return toolDisplaySpec{branchDetail: getStaticToolBranchDetail("Delete memory")}
	case "Session Messages":
		return toolDisplaySpec{
			inputDetail:  getSessionMessagesToolDisplayDetail,
			branchDetail: getSessionMessagesToolBranchDetail,
		}
	default:
		return toolDisplaySpec{}
	}
}

func getStaticToolBranchDetail(label string) func(string, bool) string {
	return func(string, bool) string {
		return label
	}
}

func getPlanToolInputDisplayState(fields map[string]any) *trace.PlanToolState {
	steps, hasSteps := fields["steps"]
	if !hasSteps || steps == nil {
		return &trace.PlanToolState{Operation: trace.PlanToolOperationRead}
	}

	stepCount := len(getMapAnySlice(fields, "steps"))
	if clearCompleted, _ := fields["clear_completed"].(bool); clearCompleted {
		return &trace.PlanToolState{
			Operation:    trace.PlanToolOperationClearCompleted,
			ChangedCount: stepCount,
		}
	}

	return &trace.PlanToolState{
		Operation:    trace.PlanToolOperationUpdate,
		ChangedCount: stepCount,
	}
}

func getPlanToolOutputDisplayState(fields map[string]any) *trace.PlanToolState {
	fields = getPlanToolOutputFields(fields)
	summary, _ := fields["summary"].(map[string]any)
	return &trace.PlanToolState{
		TotalCount:     getMapNumber(summary, "total"),
		CompletedCount: getMapNumber(summary, "completed"),
		Changes:        getPlanToolChanges(fields["changes"]),
	}
}

func getPlanToolOutputFields(fields map[string]any) map[string]any {
	if len(fields) == 0 || fields["summary"] != nil || fields["changes"] != nil {
		return fields
	}

	output, ok := fields["output"].(string)
	if !ok || strings.TrimSpace(output) == "" {
		return fields
	}

	unwrapped := map[string]any{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &unwrapped); err != nil {
		return fields
	}
	if len(unwrapped) == 0 {
		return fields
	}

	return unwrapped
}

func getPlanToolBranchDetail(state *trace.PlanToolState, completed bool) string {
	if state == nil {
		return ""
	}

	switch state.Operation {
	case trace.PlanToolOperationRead:
		if completed && state.TotalCount > 0 {
			return fmt.Sprintf("Found %s", formatTaskCount(state.TotalCount))
		}
		if completed {
			return ""
		}

		return "Read current plan"
	case trace.PlanToolOperationClearCompleted:
		if state.ChangedCount > 0 {
			return fmt.Sprintf("Cleared %s", formatTaskCount(state.ChangedCount))
		}
		if completed {
			return ""
		}

		return "Cleared completed tasks"
	default:
		if detail := getPlanToolChangeBranchDetail(state.Changes); detail != "" {
			return detail
		}
		if completed && state.TotalCount > 0 && state.CompletedCount == state.TotalCount {
			return fmt.Sprintf("Completed all %s", formatTaskCount(state.TotalCount))
		}
		if !completed && state.ChangedCount > 0 {
			return fmt.Sprintf("Updated %s", formatTaskCount(state.ChangedCount))
		}

		return ""
	}
}

func clonePlanToolDisplayState(state *trace.PlanToolState) *trace.PlanToolState {
	if state == nil {
		return nil
	}

	cloned := *state
	cloned.Changes = append([]trace.PlanToolChange(nil), state.Changes...)
	return &cloned
}

func mergePlanToolDisplayState(current *trace.PlanToolState, next *trace.PlanToolState) *trace.PlanToolState {
	if current == nil && next == nil {
		return nil
	}
	if current == nil {
		return clonePlanToolDisplayState(next)
	}
	if next == nil {
		return clonePlanToolDisplayState(current)
	}

	merged := *current
	if merged.Operation == "" {
		merged.Operation = next.Operation
	}
	if next.Operation != "" &&
		merged.Operation != trace.PlanToolOperationRead &&
		merged.Operation != trace.PlanToolOperationClearCompleted {
		merged.Operation = next.Operation
	}
	if next.ChangedCount > 0 {
		merged.ChangedCount = next.ChangedCount
	}
	if next.TotalCount > 0 {
		merged.TotalCount = next.TotalCount
	}
	if next.CompletedCount > 0 {
		merged.CompletedCount = next.CompletedCount
	}
	if len(next.Changes) > 0 {
		merged.Changes = append([]trace.PlanToolChange(nil), next.Changes...)
	}

	return &merged
}

func getProcessToolBranchDetail(state *trace.ProcessToolState, completed bool) string {
	if state == nil {
		return ""
	}
	if detail := getProcessToolErrorDetail(state); detail != "" {
		return detail
	}

	switch state.Operation {
	case trace.ProcessToolOperationStart:
		if completed {
			return getProcessToolStatusDetail(state)
		}

		return firstNonEmptyToolDisplay(state.Command, "Start process")
	case trace.ProcessToolOperationStatus:
		if completed {
			return getProcessToolStatusDetail(state)
		}

		return firstNonEmptyToolDisplay(state.ProcessID, "Check process status")
	case trace.ProcessToolOperationRead:
		if completed {
			return getProcessToolOutputDetail(state)
		}

		return firstNonEmptyToolDisplay(state.ProcessID, "Read process output")
	case trace.ProcessToolOperationStop:
		if completed {
			return getProcessToolStatusDetail(state)
		}

		return firstNonEmptyToolDisplay(state.ProcessID, "Stop process")
	case trace.ProcessToolOperationList:
		if completed {
			return fmt.Sprintf("Found %s", formatProcessCount(state.Count))
		}

		return ""
	default:
		return getProcessToolStatusDetail(state)
	}
}

func getProcessToolErrorDetail(state *trace.ProcessToolState) string {
	if state == nil {
		return ""
	}

	message := strings.TrimSpace(state.Error)
	if message == "" {
		return ""
	}

	prefix := "Failed"
	if state.Operation != "" {
		prefix = strings.TrimSpace(string(state.Operation)) + " failed"
	}
	if code := strings.TrimSpace(state.ErrorCode); code != "" {
		return prefix + ": " + message + " (" + code + ")"
	}

	return prefix + ": " + message
}

func getProcessToolStatusDetail(state *trace.ProcessToolState) string {
	if state == nil {
		return ""
	}

	parts := []string{}
	if state.ProcessID != "" {
		parts = append(parts, state.ProcessID)
	}
	if state.Status != "" {
		parts = append(parts, state.Status)
	}
	if state.ExitCode != nil {
		parts = append(parts, fmt.Sprintf("exit %d", *state.ExitCode))
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}

	return strings.TrimSpace(state.Command)
}

func getProcessToolOutputDetail(state *trace.ProcessToolState) string {
	parts := []string{}
	if state.ProcessID != "" {
		parts = append(parts, state.ProcessID)
	}
	parts = append(
		parts,
		formatProcessBytes(state.StdoutBytes)+" stdout",
		formatProcessBytes(state.StderrBytes)+" stderr",
	)

	return strings.Join(parts, " ")
}

func cloneProcessToolDisplayState(state *trace.ProcessToolState) *trace.ProcessToolState {
	if state == nil {
		return nil
	}

	cloned := *state
	if state.ExitCode != nil {
		cloned.ExitCode = new(*state.ExitCode)
	}

	return &cloned
}

func mergeProcessToolDisplayState(current *trace.ProcessToolState, next *trace.ProcessToolState) *trace.ProcessToolState {
	if current == nil && next == nil {
		return nil
	}
	if current == nil {
		return cloneProcessToolDisplayState(next)
	}
	if next == nil {
		return cloneProcessToolDisplayState(current)
	}

	merged := *current
	if next.Operation != "" {
		merged.Operation = next.Operation
	}
	if next.ProcessID != "" {
		merged.ProcessID = next.ProcessID
	}
	if next.Command != "" {
		merged.Command = next.Command
	}
	if next.Status != "" {
		merged.Status = next.Status
	}
	if next.ExitCode != nil {
		merged.ExitCode = new(*next.ExitCode)
	} else if current.ExitCode != nil {
		merged.ExitCode = new(*current.ExitCode)
	}
	if next.StdoutBytes != 0 {
		merged.StdoutBytes = next.StdoutBytes
	}
	if next.StderrBytes != 0 {
		merged.StderrBytes = next.StderrBytes
	}
	if next.Count != 0 {
		merged.Count = next.Count
	}
	if next.ErrorCode != "" {
		merged.ErrorCode = next.ErrorCode
	}
	if next.Error != "" {
		merged.Error = next.Error
	}

	return &merged
}

func hasProcessToolError(state *trace.ProcessToolState) bool {
	return state != nil && strings.TrimSpace(state.Error) != ""
}

func formatProcessBytes(value int) string {
	if value < 0 {
		value = 0
	}

	return fmt.Sprintf("%dB", value)
}

func formatProcessCount(value int) string {
	if value == 1 {
		return "1 process"
	}

	return fmt.Sprintf("%d processes", value)
}

func firstNonEmptyToolDisplay(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}

	return ""
}

func getPlanToolChanges(value any) []trace.PlanToolChange {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}

	changes := make([]trace.PlanToolChange, 0, len(items))
	for _, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			continue
		}
		change := trace.PlanToolChange{
			Index:  getMapNumber(fields, "index"),
			ID:     getMapString(fields, "id"),
			Action: getMapString(fields, "action"),
			Fields: getMapStringSlice(fields, "fields"),
		}
		if change.Index == 0 && change.ID == "" && change.Action == "" {
			continue
		}
		changes = append(changes, change)
	}
	if len(changes) == 0 {
		return nil
	}

	return changes
}

func getPlanToolChangeBranchDetail(changes []trace.PlanToolChange) string {
	if len(changes) == 0 {
		return ""
	}

	if len(changes) > 2 {
		return getPlanToolChangeSummary(changes)
	}

	parts := make([]string, 0, len(changes))
	for _, change := range changes {
		part := getPlanToolChangeText(change)
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "; ")
}

func getPlanToolChangeSummary(changes []trace.PlanToolChange) string {
	type summaryKey struct {
		action string
		fields string
	}

	counts := map[summaryKey]int{}
	order := make([]string, 0, len(changes))
	for _, change := range changes {
		action := strings.TrimSpace(strings.ToLower(change.Action))
		if action == "" {
			continue
		}
		fields := ""
		if action == "updated" {
			fields = getPlanToolUpdatedFieldsLabel(change.Fields)
		}
		key := summaryKey{action: action, fields: fields}
		if _, ok := counts[key]; !ok {
			order = append(order, action+"\x00"+fields)
		}
		counts[key]++
	}
	if len(order) == 0 {
		return ""
	}

	parts := make([]string, 0, len(order))
	for _, raw := range order {
		action, fields, _ := strings.Cut(raw, "\x00")
		key := summaryKey{action: action, fields: fields}
		label := getPlanToolChangeSummaryLabel(action, fields, counts[key])
		if label != "" {
			parts = append(parts, label)
		}
	}
	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "; ")
}

func getPlanToolChangeSummaryLabel(action string, fields string, count int) string {
	if count <= 0 {
		return ""
	}

	switch action {
	case "added":
		return "Added " + formatTaskCount(count)
	case "completed":
		return "Completed " + formatTaskCount(count)
	case "cancelled":
		return "Cancelled " + formatTaskCount(count)
	case "removed":
		return "Removed " + formatTaskCount(count)
	case "updated":
		if fields != "" {
			return "Updated " + fields + " for " + formatTaskCount(count)
		}

		return ""
	default:
		return capitalizePlanToolChangeAction(action) + " " + formatTaskCount(count)
	}
}

func capitalizePlanToolChangeAction(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return ""
	}

	return strings.ToUpper(action[:1]) + action[1:]
}

func getPlanToolChangeText(change trace.PlanToolChange) string {
	action := strings.TrimSpace(strings.ToLower(change.Action))
	if action == "" {
		return ""
	}

	subject := "Task"
	if change.Index > 0 {
		subject = fmt.Sprintf("Task %d", change.Index)
	}

	switch action {
	case "added":
		return subject + " added"
	case "completed":
		return subject + " completed"
	case "cancelled":
		return subject + " cancelled"
	case "removed":
		return subject + " removed"
	case "updated":
		if label := getPlanToolUpdatedFieldsLabel(change.Fields); label != "" {
			return subject + " " + label + " updated"
		}

		return ""
	default:
		return subject + " " + action
	}
}

func getPlanToolUpdatedFieldsLabel(fields []string) string {
	fields = normalizePlanToolChangedFields(fields)
	if len(fields) == 0 {
		return ""
	}

	if len(fields) == 1 {
		switch fields[0] {
		case "content":
			return "content"
		case "status":
			return "status"
		default:
			return fields[0]
		}
	}

	return strings.Join(fields, "+")
}

func normalizePlanToolChangedFields(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(strings.ToLower(field))
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	if len(result) == 0 {
		return nil
	}

	return result
}

func formatTaskCount(count int) string {
	if count == 1 {
		return "1 task"
	}

	return fmt.Sprintf("%d tasks", count)
}

func isGenericToolParamDisplayEnabled(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "list_files":
		return true
	default:
		return false
	}
}

func getGenericToolDisplayDetail(name string, fields map[string]any) string {
	name = strings.TrimSpace(name)
	if name == "" || len(fields) == 0 {
		return ""
	}

	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		if strings.TrimSpace(key) == "" || isEmptyToolInputValue(value) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, strings.TrimSpace(key)+"="+formatToolInputValueForKey(key, fields[key]))
	}

	return name + "(" + strings.Join(parts, " ") + ")"
}

func getRunToolDisplayDetail(fields map[string]any) string {
	command := getMapString(fields, "command")
	if command == "" {
		return ""
	}

	args := getMapStringSlice(fields, "args")
	if len(args) == 0 {
		return appendToolTimeout(command, fields["timeout_seconds"])
	}

	parts := append([]string{command}, args...)
	for index, part := range parts {
		parts[index] = shellQuoteCommandPart(part)
	}

	return appendToolTimeout(strings.Join(parts, " "), fields["timeout_seconds"])
}

func getSearchToolDisplayDetail(fields map[string]any) string {
	query := getMapString(fields, "query", "q", "search_query")
	if query == "" {
		return ""
	}

	sanitized, _ := guardrails.NewRedactor().Sanitize(query).(string)
	sanitized = truncateToolDetail(sanitized, 80)
	if sanitized == "" {
		return ""
	}

	return `Search "` + strings.ReplaceAll(sanitized, `"`, `'`) + `"`
}

func getSearchFilesToolDisplayDetail(fields map[string]any) string {
	pattern := getMapString(fields, "pattern", "query", "q")
	if pattern == "" {
		return ""
	}

	sanitized, _ := guardrails.NewRedactor().Sanitize(pattern).(string)
	sanitized = truncateToolDetail(sanitized, 80)
	if sanitized == "" {
		return ""
	}

	detail := `Search "` + strings.ReplaceAll(sanitized, `"`, `'`) + `"`
	if path := getToolDisplayPath(fields); path != "" {
		detail += " in " + path
	}
	if maxResults := formatOptionalToolNumber(fields["max_results"]); maxResults != "" {
		detail += " max_results=" + maxResults
	}

	return detail
}

func getSessionMessagesToolDisplayDetail(fields map[string]any) string {
	keys := []string{
		"session_id",
		"message_ids",
		"anchor_message_id",
		"offset_start",
		"offset_end",
		"before",
		"after",
		"max_chars",
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if part := getSessionMessagesToolParam(key, fields[key]); part != "" {
			parts = append(parts, part)
		}
	}

	if len(parts) == 0 {
		return "session_messages()"
	}

	return "session_messages(" + strings.Join(parts, " ") + ")"
}

func getSessionMessagesToolBranchDetail(detail string, _ bool) string {
	detail = strings.TrimSpace(detail)
	if detail == "" || normalizeToolDisplayName(detail) == "session_messages" ||
		normalizeToolDisplayName(detail) == "session_message" {
		return "session_messages()"
	}

	return detail
}

func getSessionMessagesToolParam(key string, value any) string {
	key = strings.TrimSpace(key)
	if key == "" || isEmptyToolInputValue(value) {
		return ""
	}
	if key == "message_ids" {
		ids := formatToolInputValueForKey(key, value)
		if ids == "" {
			return ""
		}

		return key + "=" + ids
	}
	formatted := formatToolInputValueForKey(key, value)
	if formatted == "" || formatted == "0" {
		return ""
	}

	return key + "=" + formatted
}

func getPathToolDisplayDetail(name string, fields map[string]any) string {
	path := getToolDisplayPath(fields)
	if path == "" {
		return ""
	}

	return strings.TrimSpace(name) + " " + path
}

func getPatchToolDisplayDetail(name string, fields map[string]any) string {
	patch := getMapString(fields, "patch", "diff", "unified_diff")
	path, added, removed := getPatchToolDisplaySummary(patch)
	if path == "" {
		path = getToolDisplayPath(fields)
	}

	parts := []string{strings.TrimSpace(name)}
	if path != "" {
		parts = append(parts, path)
	}
	if added > 0 || removed > 0 {
		parts = append(parts, fmt.Sprintf("+%d -%d", added, removed))
	}

	return strings.Join(parts, " ")
}

func getToolDisplayPath(fields map[string]any) string {
	path := getMapString(fields, "path", "file", "filepath", "filename")
	if path == "" {
		return ""
	}

	sanitized, _ := guardrails.NewRedactor().Sanitize(path).(string)
	return shortenToolPath(sanitized, 42)
}

func getPatchToolDisplaySummary(patch string) (string, int, int) {
	var path string
	added := 0
	removed := 0

	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			candidate := normalizePatchToolPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")))
			if candidate != "" && candidate != "/dev/null" {
				path = candidate
			}
		case strings.HasPrefix(line, "--- "):
			if path == "" {
				candidate := normalizePatchToolPath(strings.TrimSpace(strings.TrimPrefix(line, "--- ")))
				if candidate != "" && candidate != "/dev/null" {
					path = candidate
				}
			}
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}

	if path != "" {
		sanitized, _ := guardrails.NewRedactor().Sanitize(path).(string)
		path = shortenToolPath(sanitized, 42)
	}

	return path, added, removed
}

func normalizePatchToolPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return strings.Trim(path, `"`)
}

func getMapString(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		value, _ := fields[key].(string)
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}

	return ""
}

func getMapStringSlice(fields map[string]any, key string) []string {
	raw, ok := fields[key].([]any)
	if !ok {
		return nil
	}

	values := make([]string, 0, len(raw))
	for _, value := range raw {
		text, ok := value.(string)
		if !ok {
			continue
		}
		if text = strings.TrimSpace(text); text != "" {
			values = append(values, text)
		}
	}

	return values
}

func getMapAnySlice(fields map[string]any, key string) []any {
	raw, ok := fields[key].([]any)
	if !ok {
		return nil
	}

	return raw
}

func getMapNumber(fields map[string]any, key string) int {
	value, ok := fields[key]
	if !ok {
		return 0
	}

	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func isEmptyToolInputValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func formatOptionalToolNumber(value any) string {
	switch typed := value.(type) {
	case float64:
		if typed <= 0 {
			return ""
		}
	case float32:
		if typed <= 0 {
			return ""
		}
	case int:
		if typed <= 0 {
			return ""
		}
	case int64:
		if typed <= 0 {
			return ""
		}
	case int32:
		if typed <= 0 {
			return ""
		}
	}

	formatted := formatToolInputNumber(value)
	if formatted == "0" {
		return ""
	}

	return formatted
}

func formatToolInputNumber(value any) string {
	switch typed := value.(type) {
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", typed), "0"), ".")
	case float32:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", typed), "0"), ".")
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case int32:
		return fmt.Sprintf("%d", typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func formatToolInputValueForKey(key string, value any) string {
	switch typed := value.(type) {
	case string:
		sanitized, _ := guardrails.NewRedactor().Sanitize(typed).(string)
		if strings.EqualFold(strings.TrimSpace(key), "path") {
			return shortenToolPath(sanitized, 42)
		}
		return truncateToolDetail(sanitized, 60)
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", typed), "0"), ".")
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return truncateToolDetail(fmt.Sprintf("%v", typed), 60)
		}
		return truncateToolDetail(string(data), 60)
	}
}

func shortenToolPath(path string, limit int) string {
	path = strings.Join(strings.Fields(strings.TrimSpace(path)), " ")
	if limit <= 0 {
		return path
	}

	runes := []rune(path)
	if len(runes) <= limit {
		return path
	}
	if limit <= 5 {
		return string(runes[:limit])
	}

	separator := "/"
	if strings.Contains(path, "\\") && !strings.Contains(path, "/") {
		separator = "\\"
	}
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	tail := ""
	if len(parts) > 0 {
		tail = parts[len(parts)-1]
	}
	if tail == "" {
		return truncateToolDetail(path, limit)
	}

	tailRunes := []rune(tail)
	if len(tailRunes)+5 >= limit {
		return "..." + separator + string(tailRunes[max(len(tailRunes)-(limit-4), 0):])
	}

	prefixLimit := limit - len(tailRunes) - 4
	prefix := string(runes[:max(prefixLimit, 1)])

	return strings.TrimRight(prefix, `/\`) + separator + "..." + separator + tail
}

func shellQuoteCommandPart(value string) string {
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\n\"'\\$&|;()<>*?![]{}") {
		return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
	}

	return value
}

func appendToolTimeout(command string, raw any) string {
	timeout, ok := raw.(float64)
	if !ok || timeout <= 0 {
		return command
	}

	return command + " [timeout " + formatToolTimeoutSeconds(timeout) + "s]"
}

func formatToolTimeoutSeconds(timeout float64) string {
	return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", timeout), "0"), ".")
}

func normalizeRunToolDetailText(detail string, completed bool) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return detail
	}
	if completed {
		detail = runToolTimeoutHintPattern.ReplaceAllString(detail, "")
		return strings.TrimSpace(runToolLegacyTimeoutPattern.ReplaceAllString(detail, ""))
	}
	if strings.Contains(detail, "[timeout ") || strings.Contains(detail, "[terminates in ") {
		return detail
	}

	return runToolLegacyTimeoutPattern.ReplaceAllString(detail, " [timeout $1]")
}

func truncateToolDetail(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 {
		return value
	}

	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}

	return string(runes[:limit-3]) + "..."
}
