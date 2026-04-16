package messages

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageSearchText_NormalizesAssistantToolCalls(t *testing.T) {
	value := MessageSearchText(Message{
		Role: RoleAssistant,
		ToolCalls: []ToolCall{{
			ID:    "call-1",
			Name:  "process",
			Input: `{"action":"start","command":"python3"}`,
		}},
	})

	require.Contains(t, value, "tool process")
	require.Contains(t, value, "tool_call_id call-1")
	require.Contains(t, value, "input.action start")
	require.Contains(t, value, "start")
	require.Contains(t, value, "input.command python3")
	require.Contains(t, value, "python3")
}

func TestMessageSearchText_NormalizesToolContentJSON(t *testing.T) {
	value := MessageSearchText(Message{
		Role:    RoleTool,
		Content: `{"process":{"id":"proc_1","status":"running"}}`,
	})

	require.Contains(t, value, "process.id proc_1")
	require.Contains(t, value, "proc_1")
	require.Contains(t, value, "process.status running")
	require.Contains(t, value, "running")
}

func TestMessageSearchText_LeavesMalformedToolContentEmpty(t *testing.T) {
	require.Empty(t, MessageSearchText(Message{
		Role:    RoleTool,
		Content: "{bad json",
	}))
}

func TestMessageSearchText_LeavesPlainTextMessagesEmpty(t *testing.T) {
	require.Empty(t, MessageSearchText(Message{Role: RoleUser, Content: "hello"}))
}

func TestMessageSearchText_LeavesAssistantWithoutToolCallsEmpty(t *testing.T) {
	require.Empty(t, MessageSearchText(Message{Role: RoleAssistant, Content: "hello"}))
}

func TestToolCallsSearchText_HandlesEmptyInput(t *testing.T) {
	require.Empty(t, ToolCallsSearchText(nil))
	require.Empty(t, ToolCallsSearchText([]ToolCall{{}}))
}

func TestToolCallSearchText_IsDeterministic(t *testing.T) {
	toolCall := ToolCall{
		ID:    "call-1",
		Name:  "search_files",
		Input: `{"pattern":"hello","path":"internal"}`,
	}

	first := ToolCallSearchText(toolCall)
	second := ToolCallSearchText(toolCall)

	require.Equal(t, first, second)
}

func TestToolCallSearchText_KeepsMetadataWhenInputIsMalformed(t *testing.T) {
	value := ToolCallSearchText(ToolCall{
		ID:    "call-1",
		Name:  "process",
		Input: "{bad json",
	})

	require.Contains(t, value, "tool process")
	require.Contains(t, value, "tool_call_id call-1")
	require.NotContains(t, value, "{bad json")
}

func TestNormalizeSearchTextScalar_HandlesEmptyAndWhitespace(t *testing.T) {
	require.Empty(t, normalizeSearchTextScalar(""))
	require.Empty(t, normalizeSearchTextScalar("   "))
	require.Equal(t, "hello world", normalizeSearchTextScalar("  Hello   World  "))
}

func TestDedupeSearchTextLines_HandlesEmptyAndDuplicates(t *testing.T) {
	require.Empty(t, dedupeSearchTextLines(nil))
	require.Equal(t, "one\ntwo", dedupeSearchTextLines([]string{" one ", "", "two", "one"}))
}

func TestSearchableMessageText_CoversRoleAndFallbackBranches(t *testing.T) {
	t.Run("Assistant plain message", func(t *testing.T) {
		assistantPlain := Message{Role: RoleAssistant, Content: "plain"}
		text, toolName := SearchableMessageText(assistantPlain, "")
		require.Equal(t, "plain", text)
		require.Empty(t, toolName)

		text, toolName = SearchableMessageText(assistantPlain, "process")
		require.Empty(t, text)
		require.Empty(t, toolName)
	})

	t.Run("Assistant structured message with SearchText", func(t *testing.T) {
		assistantStructured := Message{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call-1", Name: "process", Input: `{"action":"start"}`},
			},
			SearchText: MessageSearchText(Message{
				Role: RoleAssistant,
				ToolCalls: []ToolCall{
					{ID: "call-1", Name: "process", Input: `{"action":"start"}`},
				},
			}),
		}

		text, toolName := SearchableMessageText(assistantStructured, "")
		require.NotEmpty(t, text)
		require.Equal(t, "process", toolName)

		text, toolName = SearchableMessageText(assistantStructured, "process")
		require.Contains(t, text, "tool_name process")
		require.Equal(t, "process", toolName)

		text, toolName = SearchableMessageText(assistantStructured, "search_files")
		require.Empty(t, text)
		require.Empty(t, toolName)
	})

	t.Run("Assistant with tool calls but no SearchText", func(t *testing.T) {
		assistantNoSearchText := Message{
			Role:      RoleAssistant,
			ToolCalls: []ToolCall{{ID: "call-1", Name: "process", Input: `{"action":"start"}`}},
		}
		text, toolName := SearchableMessageText(assistantNoSearchText, "")
		require.Empty(t, text)
		require.Empty(t, toolName)
	})

	t.Run("Tool role message, filtering by tool name", func(t *testing.T) {
		toolMessage := Message{
			Role:    RoleTool,
			Name:    "process",
			Content: `{"status":"running"}`,
		}
		text, toolName := SearchableMessageText(toolMessage, "other")
		require.Empty(t, text)
		require.Empty(t, toolName)

		text, toolName = SearchableMessageText(toolMessage, "")
		require.Equal(t, `{"status":"running"}`, text)
		require.Equal(t, "process", toolName)

		toolMessage.SearchText = "tool structured"
		text, toolName = SearchableMessageText(toolMessage, "")
		require.Equal(t, "tool structured", text)
		require.Equal(t, "process", toolName)
	})

	t.Run("User role message", func(t *testing.T) {
		userMessage := Message{Role: RoleUser, Content: "needle"}
		text, toolName := SearchableMessageText(userMessage, "")
		require.Equal(t, "needle", text)
		require.Empty(t, toolName)

		text, toolName = SearchableMessageText(userMessage, "process")
		require.Empty(t, text)
		require.Empty(t, toolName)
	})
}

func TestMatchAssistantToolName(t *testing.T) {
	require.Empty(t, matchAssistantToolName([]ToolCall{{Name: "process"}}, ""))
	require.Empty(t, matchAssistantToolName([]ToolCall{{Name: "process"}}, "search_files"))
	require.Equal(t, "Process", matchAssistantToolName([]ToolCall{{Name: " Process "}}, "process"))
}
