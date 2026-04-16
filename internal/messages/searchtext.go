package messages

import (
	"strings"

	"github.com/wandxy/hand/pkg/jsonterms"
)

func MessageSearchText(message Message) string {
	switch message.Role {
	case RoleAssistant:
		if len(message.ToolCalls) == 0 {
			return ""
		}
		return ToolCallsSearchText(message.ToolCalls)
	case RoleTool:
		return jsonterms.Terms(message.Content)
	default:
		return ""
	}
}

func ToolCallsSearchText(toolCalls []ToolCall) string {
	if len(toolCalls) == 0 {
		return ""
	}

	parts := make([]string, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		text := ToolCallSearchText(toolCall)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}

	return strings.Join(parts, "\n")
}

func ToolCallSearchText(toolCall ToolCall) string {
	lines := make([]string, 0, 4)

	name := normalizeSearchTextScalar(toolCall.Name)
	if name != "" {
		lines = append(lines, "tool "+name, "tool", name, "tool_name "+name)
	}

	id := normalizeSearchTextScalar(toolCall.ID)
	if id != "" {
		lines = append(lines, "tool_call_id "+id)
	}

	input := jsonterms.Terms(toolCall.Input, "input")
	if input != "" {
		lines = append(lines, input)
	}

	return dedupeSearchTextLines(lines)
}

func SearchableMessageText(message Message, toolName string) (string, string) {
	switch message.Role {
	case RoleAssistant:
		if len(message.ToolCalls) == 0 {
			if toolName != "" {
				return "", ""
			}
			return strings.TrimSpace(message.Content), ""
		}

		matchedToolName := matchAssistantToolName(message.ToolCalls, toolName)
		if toolName != "" && matchedToolName == "" {
			return "", ""
		}

		if toolName != "" {
			for _, toolCall := range message.ToolCalls {
				if strings.EqualFold(strings.TrimSpace(toolCall.Name), toolName) {
					return ToolCallSearchText(toolCall), strings.TrimSpace(toolCall.Name)
				}
			}
		}

		searchText := strings.TrimSpace(message.SearchText)
		if searchText == "" {
			return "", ""
		}

		if matchedToolName == "" && len(message.ToolCalls) == 1 {
			matchedToolName = strings.TrimSpace(message.ToolCalls[0].Name)
		}

		return searchText, matchedToolName
	case RoleTool:
		messageToolName := strings.TrimSpace(message.Name)
		if toolName != "" && !strings.EqualFold(messageToolName, toolName) {
			return "", ""
		}

		searchText := strings.TrimSpace(message.SearchText)
		if searchText == "" {
			searchText = strings.TrimSpace(message.Content)
		}

		return searchText, messageToolName
	default:
		if toolName != "" {
			return "", ""
		}
		return strings.TrimSpace(message.Content), ""
	}
}

func matchAssistantToolName(toolCalls []ToolCall, toolName string) string {
	if toolName == "" {
		return ""
	}

	for _, toolCall := range toolCalls {
		if strings.EqualFold(strings.TrimSpace(toolCall.Name), toolName) {
			return strings.TrimSpace(toolCall.Name)
		}
	}

	return ""
}

func normalizeSearchTextScalar(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func dedupeSearchTextLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	seen := make(map[string]struct{}, len(lines))
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}

	return strings.Join(out, "\n")
}
