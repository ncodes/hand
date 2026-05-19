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

func getToolBranchDisplayDetail(action string, detail string, completed bool) string {
	spec := getToolDisplaySpecForAction(action)
	if spec.branchDetail == nil {
		return strings.TrimSpace(detail)
	}

	return spec.branchDetail(detail, completed)
}

func getToolDisplaySpec(name string) toolDisplaySpec {
	action := getToolActionName(name)
	spec := getToolDisplaySpecForAction(action)
	if spec.inputDetail != nil || spec.branchDetail != nil {
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

func getToolDisplaySpecForAction(action string) toolDisplaySpec {
	switch strings.TrimSpace(action) {
	case "Run":
		return toolDisplaySpec{
			inputDetail:  getRunToolDisplayDetail,
			branchDetail: normalizeRunToolDetailText,
		}
	case "Web Search", "Memory Search":
		return toolDisplaySpec{
			inputDetail: getSearchToolDisplayDetail,
		}
	case "Memory Extract":
		return toolDisplaySpec{branchDetail: getStaticToolBranchDetail("Extract memories")}
	case "Memory Add":
		return toolDisplaySpec{branchDetail: getStaticToolBranchDetail("Add memory")}
	case "Memory Update":
		return toolDisplaySpec{branchDetail: getStaticToolBranchDetail("Update memory")}
	case "Memory Delete":
		return toolDisplaySpec{branchDetail: getStaticToolBranchDetail("Delete memory")}
	default:
		return toolDisplaySpec{}
	}
}

func getStaticToolBranchDetail(label string) func(string, bool) string {
	return func(string, bool) string {
		return label
	}
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
