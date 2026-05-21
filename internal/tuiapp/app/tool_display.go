package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/wandxy/hand/internal/guardrails"
)

var runToolLegacyTimeoutPattern = regexp.MustCompile(`\s+\(([0-9]+(?:\.[0-9]+)?s)\)$`)
var runToolTimeoutHintPattern = regexp.MustCompile(`\s+\[(?:terminates in|timeout) [^\]]+\]`)

type toolDisplaySpec struct {
	inputDetail  func(map[string]any) string
	outputDetail func(map[string]any) string
	inputState   func(map[string]any) *planToolDisplayState
	outputState  func(map[string]any) *planToolDisplayState
	branchDetail func(string, bool) string
}

type planToolDisplayOperation string

const (
	planToolDisplayOperationRead           planToolDisplayOperation = "read"
	planToolDisplayOperationUpdate         planToolDisplayOperation = "update"
	planToolDisplayOperationClearCompleted planToolDisplayOperation = "clear_completed"
)

type planToolDisplayState struct {
	Operation      planToolDisplayOperation
	ChangedCount   int
	TotalCount     int
	CompletedCount int
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

func getToolInputDisplayState(name string, input string) *planToolDisplayState {
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

func getToolOutputDisplayState(name string, output string) *planToolDisplayState {
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

func getPlanToolInputDisplayState(fields map[string]any) *planToolDisplayState {
	steps, hasSteps := fields["steps"]
	if !hasSteps || steps == nil {
		return &planToolDisplayState{Operation: planToolDisplayOperationRead}
	}

	stepCount := len(getMapAnySlice(fields, "steps"))
	if clearCompleted, _ := fields["clear_completed"].(bool); clearCompleted {
		return &planToolDisplayState{
			Operation:    planToolDisplayOperationClearCompleted,
			ChangedCount: stepCount,
		}
	}

	return &planToolDisplayState{
		Operation:    planToolDisplayOperationUpdate,
		ChangedCount: stepCount,
	}
}

func getPlanToolOutputDisplayState(fields map[string]any) *planToolDisplayState {
	summary, _ := fields["summary"].(map[string]any)
	return &planToolDisplayState{
		TotalCount:     getMapNumber(summary, "total"),
		CompletedCount: getMapNumber(summary, "completed"),
	}
}

func getPlanToolBranchDetail(state *planToolDisplayState, completed bool) string {
	if state == nil {
		return "Updated plan"
	}

	switch state.Operation {
	case planToolDisplayOperationRead:
		if completed && state.TotalCount > 0 {
			return fmt.Sprintf("Found %s", formatTaskCount(state.TotalCount))
		}

		return "Read current plan"
	case planToolDisplayOperationClearCompleted:
		if state.ChangedCount > 0 {
			return fmt.Sprintf("Cleared %s", formatTaskCount(state.ChangedCount))
		}

		return "Cleared completed tasks"
	default:
		if completed && state.TotalCount > 0 && state.CompletedCount == state.TotalCount {
			return fmt.Sprintf("Completed all %s", formatTaskCount(state.TotalCount))
		}
		if completed && state.TotalCount > 0 && state.ChangedCount > 0 && state.ChangedCount < state.TotalCount {
			return fmt.Sprintf("Updated %s of %d", formatTaskCount(state.ChangedCount), state.TotalCount)
		}
		if completed && state.TotalCount > 0 && state.ChangedCount == state.TotalCount {
			return fmt.Sprintf("Updated all %s", formatTaskCount(state.TotalCount))
		}
		if state.ChangedCount > 0 {
			return fmt.Sprintf("Updated %s", formatTaskCount(state.ChangedCount))
		}

		return "Updated plan"
	}
}

func clonePlanToolDisplayState(state *planToolDisplayState) *planToolDisplayState {
	if state == nil {
		return nil
	}

	cloned := *state
	return &cloned
}

func mergePlanToolDisplayState(current *planToolDisplayState, next *planToolDisplayState) *planToolDisplayState {
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
		merged.Operation != planToolDisplayOperationRead &&
		merged.Operation != planToolDisplayOperationClearCompleted {
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

	return &merged
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
