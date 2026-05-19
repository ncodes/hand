package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/agent"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/rpc/client"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/trace"
)

func TestTimelineMessageToTranscriptCell_MapsVisibleRoles(t *testing.T) {
	cases := []struct {
		name    string
		message handmsg.Message
		want    string
	}{
		{
			name:    "user",
			message: handmsg.Message{Role: handmsg.RoleUser, Content: "hello"},
			want:    "You: hello",
		},
		{
			name:    "assistant",
			message: handmsg.Message{Role: handmsg.RoleAssistant, Content: "hi"},
			want:    "Hand: hi",
		},
		{
			name:    "tool",
			message: handmsg.Message{Role: handmsg.RoleTool, Name: "read_file", Content: "done"},
			want:    toolOperationTranscriptCell("", "read_file", "", true),
		},
		{
			name:    "tool fallback",
			message: handmsg.Message{Role: handmsg.RoleTool, Content: "done"},
			want:    toolOperationTranscriptCell("", "tool", "", true),
		},
		{
			name:    "unknown",
			message: handmsg.Message{Role: "system", Content: "note"},
			want:    "system: note",
		},
		{
			name:    "empty content",
			message: handmsg.Message{Role: handmsg.RoleUser, Content: " "},
			want:    "",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, timelineMessageToTranscriptCell(tt.message, nil))
		})
	}
}

func TestTUIMessageToTranscriptCell_MapsLiveDisplayMessages(t *testing.T) {
	cases := []struct {
		name string
		msg  any
		want string
	}{
		{name: "user", msg: userMessageAcceptedMsg{Text: "hello"}, want: "You: hello"},
		{name: "assistant delta", msg: assistantTextDeltaMsg{Text: "hi"}, want: "Hand: hi"},
		{name: "assistant complete", msg: assistantResponseCompletedMsg{Text: "done"}, want: "Hand: done"},
		{name: "tool started", msg: toolInvocationStartedMsg{Name: "read_file"}, want: toolOperationTranscriptCell("", "read_file", "")},
		{name: "tool completed", msg: toolInvocationCompletedMsg{Name: "read_file"}, want: toolOperationTranscriptCell("", "read_file", "", true)},
		{name: "safety", msg: safetyEventMsg{Action: "blocked", FindingIDs: []string{"prompt_exfiltration"}}, want: "Safety: blocked: prompt_exfiltration"},
		{name: "error", msg: sessionErrorMsg{Message: "failed"}, want: "Error: failed"},
		{name: "empty", msg: userMessageAcceptedMsg{Text: " "}, want: ""},
		{name: "unknown", msg: struct{}{}, want: ""},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tuiMessageToTranscriptCell(tt.msg))
		})
	}
}

func TestRenderTranscriptCell_StylesCanonicalCells(t *testing.T) {
	rendered := renderTranscriptCells([]string{
		timelineMessageToTranscriptCell(handmsg.Message{Role: handmsg.RoleUser, Content: "hello"}, nil),
		tuiMessageToTranscriptCell(assistantResponseCompletedMsg{Text: "hi"}),
		tuiMessageToTranscriptCell(toolInvocationStartedMsg{Name: "read_file"}),
		tuiMessageToTranscriptCell(safetyEventMsg{Action: "blocked"}),
		tuiMessageToTranscriptCell(sessionErrorMsg{Message: "failed"}),
	})

	plain := stripANSI(rendered)
	require.Contains(t, plain, "❯ hello")
	require.NotContains(t, plain, "┌")
	require.NotContains(t, plain, "│ ❯ hello")
	require.NotContains(t, plain, "You: hello")
	require.Contains(t, plain, "Hand: hi")
	require.Contains(t, plain, "● Read")
	require.Contains(t, plain, "└ read_file")
	require.Contains(t, plain, "Safety: blocked")
	require.Contains(t, plain, "Error: failed")
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderTranscriptCells_GroupsAdjacentToolOperationsByAction(t *testing.T) {
	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCell("call_1", "write_file", ""),
		toolOperationTranscriptCell("call_2", "write_file", ""),
		toolOperationTranscriptCell("call_3", "read_file", ""),
	})
	plain := stripANSI(rendered)

	require.Equal(t, 1, strings.Count(plain, "● Write"))
	require.Equal(t, 1, strings.Count(plain, "● Read"))
	require.Contains(t, plain, "├ write_file")
	require.Contains(t, plain, "└ write_file")
	require.Contains(t, plain, "└ read_file")
}

func TestRenderTranscriptCells_DeduplicatesStartedAndCompletedToolEvents(t *testing.T) {
	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCell("call_1", "web_search", ""),
		toolOperationTranscriptCell("call_1", "web_search", "", true),
	})
	plain := stripANSI(rendered)

	require.Equal(t, 1, strings.Count(plain, "● Searched"))
	require.Equal(t, 1, strings.Count(plain, "web_search"))
}

func TestRenderTranscriptCells_AnimatesRunningToolDot(t *testing.T) {
	cells := []string{toolOperationTranscriptCell("call_1", "web_search", "")}
	first := stripANSI(renderTranscriptCellsWithFrame(cells, 80, 0))
	next := stripANSI(renderTranscriptCellsWithFrame(cells, 80, 1))
	completed := stripANSI(renderTranscriptCellsWithFrame([]string{
		toolOperationTranscriptCell("call_1", "web_search", "", true),
	}, 80, 1))

	require.Contains(t, first, "● Web Search")
	require.Contains(t, next, "◖ Web Search")
	require.Contains(t, completed, "● Searched")
}

func TestRenderTranscriptCells_RendersRunCommandsWithShellLayout(t *testing.T) {
	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCell("call_1", "run_command", `sleep 10 && echo "Done" (8s)`),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "● Running 1 shell command…")
	require.Contains(t, plain, `└ $ sleep 10 && echo "Done" (8s)`)
	require.NotContains(t, plain, "ctrl+b")
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderTranscriptCells_RendersCompletedRunCommandsWithPastTense(t *testing.T) {
	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCell("call_1", "run_command", `sleep 10 (30s)`, true),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "● Ran 1 shell command")
	require.Contains(t, plain, "└ $ sleep 10 (30s)")
	require.NotContains(t, plain, "Running")
	require.NotContains(t, plain, "…")
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderTranscriptCell_RendersUserMessageBox(t *testing.T) {
	rendered := renderTranscriptCellWithWidth("You: Some message.", 40)
	plain := stripANSI(rendered)

	require.Contains(t, plain, "❯ Some message.")
	require.NotContains(t, plain, "┌")
	require.NotContains(t, plain, "│")
	require.NotContains(t, plain, "└")
	require.NotContains(t, plain, "You:")
	require.Contains(t, rendered, "\x1b[")
	require.Contains(t, rendered, "48;2;21;21;21")
	require.Contains(t, rendered, "48;2;21;21;21mSome message")
}

func TestRenderTranscriptCell_RendersMultilineUserMessageWithSinglePrompt(t *testing.T) {
	rendered := renderTranscriptCellWithWidth("You: hello\nfriend", 40)
	plain := stripANSI(rendered)
	lines := strings.Split(plain, "\n")

	require.Len(t, lines, 2)
	require.Equal(t, "❯ hello", strings.TrimRight(lines[0], " "))
	require.Equal(t, "  friend", strings.TrimRight(lines[1], " "))
	require.Equal(t, 1, strings.Count(plain, "❯"))
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderTranscriptCell_RendersAssistantMarkdown(t *testing.T) {
	rendered := renderTranscriptCellWithWidth(
		"Hand: # Title\n\n## Key Complications\n\n### What Could Happen Next\n\n- first\n- second\n\n```go\nfmt.Println(\"hi\")\n```",
		60,
	)
	plain := stripANSI(rendered)

	require.Contains(t, plain, "Hand:")
	require.Contains(t, plain, "Title")
	require.Contains(t, plain, "Key Complications")
	require.Contains(t, plain, "What Could Happen Next")
	require.Contains(t, plain, "first")
	require.Contains(t, plain, "second")
	require.Contains(t, plain, `fmt.Println("hi")`)
	require.NotContains(t, plain, "# Title")
	require.NotContains(t, plain, "## Key Complications")
	require.NotContains(t, plain, "### What Could Happen Next")
	require.NotContains(t, plain, "```")
	require.Contains(t, rendered, "\x1b[")
	require.NotContains(t, rendered, "\x1b[38;5;39m")
	require.NotContains(t, rendered, "\x1b[48;5;63m")
}

func TestRenderTranscriptCell_RendersCompactMarkdownTables(t *testing.T) {
	rendered := renderTranscriptCellWithWidth(strings.Join([]string{
		"Hand: | **Issue** | Details |",
		"| --- | --- |",
		"| [One](https://example.com) | `Short` |",
		"| Two | Also **short** |",
	}, "\n"), 120)
	plain := stripANSI(rendered)
	lines := strings.Split(plain, "\n")

	require.Contains(t, plain, "  ┌───────┬────────────┐")
	require.Contains(t, plain, "  │ Issue │ Details    │")
	require.Contains(t, plain, "├───────┼────────────┤")
	require.Contains(t, plain, "│ Two   │ Also short │")
	require.Contains(t, plain, "└───────┴────────────┘")
	require.Equal(t, 2, strings.Count(plain, "├───────┼────────────┤"))
	require.NotContains(t, plain, strings.Repeat(" ", 20))
	require.Contains(t, rendered, "\x1b[")
	for _, line := range lines {
		if strings.Contains(line, "│ Issue") {
			require.Less(t, len(line), 40)
		}
	}
}

func TestRenderTranscriptCell_KeepsTableCloseToPrecedingHeading(t *testing.T) {
	rendered := renderTranscriptCellWithWidth(strings.Join([]string{
		"Hand: ## Key Complications",
		"",
		"| Issue | Details |",
		"| --- | --- |",
		"| One | Short |",
	}, "\n"), 120)
	lines := strings.Split(stripANSI(rendered), "\n")
	headingIndex := indexLineContaining(lines, "Key Complications")
	tableIndex := indexLineContaining(lines, "┌───────┬─────────┐")

	require.NotEqual(t, -1, headingIndex)
	require.NotEqual(t, -1, tableIndex)
	require.LessOrEqual(t, tableIndex-headingIndex, 2)
}

func TestRenderTranscriptCell_DoesNotRenderUserMarkdown(t *testing.T) {
	rendered := renderTranscriptCellWithWidth("You: # literal\n\n- keep", 60)
	plain := stripANSI(rendered)

	require.Contains(t, plain, "❯ # literal")
	require.Contains(t, plain, "  - keep")
	require.Equal(t, 1, strings.Count(plain, "❯"))
}

func TestRenderMarkdownForTranscript_LeavesPlainTextAlone(t *testing.T) {
	require.Equal(t, "hello there", renderMarkdownForTranscript("hello there", 60))
}

func TestHasTranscriptMarkdown_DetectsCommonSyntax(t *testing.T) {
	require.True(t, hasTranscriptMarkdown("1. first"))
	require.True(t, hasTranscriptMarkdown("**strong**"))
	require.True(t, hasTranscriptMarkdown("[link](https://example.com)"))
	require.False(t, hasTranscriptMarkdown("plain sentence"))
}

func indexLineContaining(lines []string, value string) int {
	for index, line := range lines {
		if strings.Contains(line, value) {
			return index
		}
	}

	return -1
}

func TestSessionTimelineToTranscriptCells_SkipsMessageBackedTraceDuplicates(t *testing.T) {
	now := time.Date(2026, 5, 18, 15, 0, 0, 0, time.UTC)
	cells := sessionTimelineToTranscriptCells(client.SessionTimeline{
		Messages: []agent.SessionTimelineMessage{
			{Message: handmsg.Message{Role: handmsg.RoleUser, Content: "hello there", CreatedAt: now}},
			{Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: "hello back", CreatedAt: now.Add(time.Second)}},
		},
		TraceEvents: []agent.SessionTimelineTraceEvent{
			{Event: storage.TraceEvent{
				Type:      trace.EvtFinalAssistantResponse,
				Timestamp: now.Add(time.Second),
				Payload:   map[string]any{"message": "hello back"},
			}},
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationStarted,
				Timestamp: now.Add(2 * time.Second),
				Payload:   map[string]any{"name": "read_file"},
			}},
		},
	})

	require.Equal(t, []string{
		"You: hello there",
		"Hand: hello back",
		toolOperationTranscriptCell("", "read_file", ""),
	}, cells)
}

func TestSessionTimelineToTranscriptCells_InterleavesMessagesAndTraceEventsByTime(t *testing.T) {
	now := time.Date(2026, 5, 18, 15, 0, 0, 0, time.UTC)

	cells := sessionTimelineToTranscriptCells(client.SessionTimeline{
		Messages: []agent.SessionTimelineMessage{
			{Message: handmsg.Message{Role: handmsg.RoleUser, Content: "older prompt", CreatedAt: now}},
			{Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: "older answer", CreatedAt: now.Add(time.Second)}},
			{Message: handmsg.Message{Role: handmsg.RoleUser, Content: "Hi", CreatedAt: now.Add(10 * time.Second)}},
			{Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: "Hi there", CreatedAt: now.Add(11 * time.Second)}},
		},
		TraceEvents: []agent.SessionTimelineTraceEvent{
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationStarted,
				Timestamp: now.Add(2 * time.Second),
				Payload:   map[string]any{"name": "web_search"},
			}},
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationCompleted,
				Timestamp: now.Add(3 * time.Second),
				Payload:   map[string]any{"name": "web_search"},
			}},
		},
	})

	require.Equal(t, []string{
		"You: older prompt",
		"Hand: older answer",
		toolOperationTranscriptCell("", "web_search", ""),
		toolOperationTranscriptCell("", "web_search", "", true),
		"You: Hi",
		"Hand: Hi there",
	}, cells)
}

func TestSessionTimelineToTranscriptCells_UsesPersistedToolCallInputForToolDetails(t *testing.T) {
	now := time.Date(2026, 5, 18, 15, 0, 0, 0, time.UTC)

	cells := sessionTimelineToTranscriptCells(client.SessionTimeline{
		Messages: []agent.SessionTimelineMessage{
			{Message: handmsg.Message{
				Role:      handmsg.RoleAssistant,
				ToolCalls: []handmsg.ToolCall{{ID: "call_1", Name: "run_command", Input: `{"command":"sleep 10","timeout_seconds":30}`}},
				CreatedAt: now,
			}},
			{Message: handmsg.Message{
				Role:       handmsg.RoleTool,
				Name:       "run_command",
				ToolCallID: "call_1",
				Content:    `{"output":"done"}`,
				CreatedAt:  now.Add(time.Second),
			}},
		},
	})

	require.Equal(t, []string{
		toolOperationTranscriptCell("call_1", "run_command", "sleep 10 (30s)", true),
	}, cells)
	plain := stripANSI(renderTranscriptCells(cells))
	require.Contains(t, plain, "● Ran 1 shell command")
	require.Contains(t, plain, "└ $ sleep 10 (30s)")
	require.NotContains(t, plain, "run_command")
}
