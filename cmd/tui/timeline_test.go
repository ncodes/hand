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
		{name: "reasoning delta", msg: assistantTextDeltaMsg{Channel: "reasoning", Text: "thinking"}, want: "Reasoning: thinking"},
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
	require.Contains(t, plain, "hi")
	require.NotContains(t, plain, "Hand: hi")
	require.Contains(t, plain, "● Read")
	require.Contains(t, plain, "└ read_file")
	require.Contains(t, plain, "Safety: blocked")
	require.Contains(t, plain, "Error: failed")
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderTranscriptCell_RendersReasoningDeltas(t *testing.T) {
	rendered := renderTranscriptCellWithWidth("Reasoning: first token\nsecond token", 40)

	plain := stripANSI(rendered)
	require.Contains(t, plain, "> first token")
	require.Contains(t, plain, "> second token")
	require.NotContains(t, plain, "Reasoning:")
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
		toolOperationTranscriptCell("call_1", "web_search", `Search "what is todays news..."`),
		toolOperationTranscriptCell("call_1", "web_search", `Search "what is todays news..."`, true),
	})
	plain := stripANSI(rendered)

	require.Equal(t, 1, strings.Count(plain, "● Searched"))
	require.Equal(t, 1, strings.Count(plain, `Search "what is todays news..."`))
	require.NotContains(t, plain, "web_search")
}

func TestRenderTranscriptCells_RendersMemorySearchLikeSearch(t *testing.T) {
	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCell("call_1", "memory_search", `Search "commit preferences"`),
		toolOperationTranscriptCell("call_1", "memory_search", `Search "commit preferences"`, true),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "● Searched Memory")
	require.Contains(t, plain, `Search "commit preferences"`)
	require.NotContains(t, plain, "memory_search")
}

func TestRenderTranscriptCells_RendersRunningMemorySearchTitle(t *testing.T) {
	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCell("call_1", "memory_search", `Search "commit preferences"`),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "● Searching Memory")
	require.Contains(t, plain, `Search "commit preferences"`)
	require.NotContains(t, plain, "Memory Search")
}

func TestRenderTranscriptCells_RendersMemoryToolsWithFriendlyText(t *testing.T) {
	cases := []struct {
		name      string
		tool      string
		running   string
		completed string
		branch    string
	}{
		{
			name:      "extract",
			tool:      "memory_extract",
			running:   "Extracting Memory",
			completed: "Extracted Memory",
			branch:    "Extract memories",
		},
		{
			name:      "add",
			tool:      "memory_add",
			running:   "Adding Memory",
			completed: "Added Memory",
			branch:    "Add memory",
		},
		{
			name:      "update",
			tool:      "memory_update",
			running:   "Updating Memory",
			completed: "Updated Memory",
			branch:    "Update memory",
		},
		{
			name:      "delete",
			tool:      "memory_delete",
			running:   "Deleting Memory",
			completed: "Deleted Memory",
			branch:    "Delete memory",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			running := stripANSI(renderTranscriptCells([]string{
				toolOperationTranscriptCell("call_1", tt.tool, ""),
			}))
			completed := stripANSI(renderTranscriptCells([]string{
				toolOperationTranscriptCell("call_1", tt.tool, "", true),
			}))

			require.Contains(t, running, "● "+tt.running)
			require.Contains(t, running, "└ "+tt.branch)
			require.Contains(t, completed, "● "+tt.completed)
			require.Contains(t, completed, "└ "+tt.branch)
			require.NotContains(t, running, tt.tool)
			require.NotContains(t, completed, tt.tool)
		})
	}
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

func TestRenderTranscriptCells_RendersToolElapsedTime(t *testing.T) {
	originalCurrentTime := currentTime
	t.Cleanup(func() { currentTime = originalCurrentTime })
	startedAt := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	currentTime = func() time.Time {
		return startedAt.Add(10 * time.Second)
	}

	running := stripANSI(renderTranscriptCells([]string{
		toolOperationTranscriptCellWithTiming("call_1", "list_files", "", startedAt, time.Time{}, false),
	}))
	completed := stripANSI(renderTranscriptCells([]string{
		toolOperationTranscriptCellWithTiming("call_1", "list_files", "", startedAt, startedAt.Add(12*time.Second), true),
	}))

	require.Contains(t, running, "● List Files (10s)")
	require.Contains(t, running, "└ list_files (10s)")
	require.Contains(t, completed, "● List Files (12s)")
	require.Contains(t, completed, "└ list_files (12s)")
}

func TestRenderTranscriptCells_RendersFileToolDetails(t *testing.T) {
	startedAt := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCellWithTiming(
			"call_1",
			"write_file",
			"write_file file.txt",
			startedAt,
			startedAt.Add(30*time.Second),
			true,
		),
		toolOperationTranscriptCellWithTiming(
			"call_2",
			"patch",
			"patch file.txt +1 -1",
			startedAt,
			startedAt.Add(30*time.Second),
			true,
		),
		toolOperationTranscriptCellWithTiming(
			"call_3",
			"patch",
			"patch",
			startedAt,
			startedAt.Add(30*time.Second),
			true,
		),
		toolOperationTranscriptCellWithTiming(
			"call_4",
			"read_file",
			"read_file file.txt",
			startedAt,
			startedAt.Add(30*time.Second),
			true,
		),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "● Wrote (30s)")
	require.Contains(t, plain, "└ write_file file.txt (30s)")
	require.Contains(t, plain, "● Patch")
	require.Contains(t, plain, "├ patch file.txt +1 -1 (30s)")
	require.Contains(t, plain, "└ patch (30s)")
	require.Contains(t, plain, "● Read (30s)")
	require.Contains(t, plain, "└ read_file file.txt (30s)")
	require.Contains(t, rendered, "\x1b[38;5;83m+1")
	require.Contains(t, rendered, "\x1b[38;5;203m-1")
}

func TestRenderTranscriptCells_RendersSearchFilesDetail(t *testing.T) {
	startedAt := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCellWithTiming(
			"call_1",
			"search_files",
			`Search "println" in .`,
			startedAt,
			startedAt.Add(3*time.Second),
			true,
		),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "● Searched Files (3s)")
	require.Contains(t, plain, `└ Search "println" in . (3s)`)
	require.NotContains(t, plain, "search_files")
}

func TestRenderTranscriptCells_RendersRunCommandsWithShellLayout(t *testing.T) {
	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCell("call_1", "run_command", `sleep 10 && echo "Done" [timeout 8s]`),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "● Running 1 shell command…")
	require.Contains(t, plain, `└ $ sleep 10 && echo "Done" [timeout 8s]`)
	require.NotContains(t, plain, "ctrl+b")
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderTranscriptCells_RendersCompletedRunCommandsWithPastTense(t *testing.T) {
	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCell("call_1", "run_command", `sleep 10 [timeout 30s]`, true),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "● Ran 1 shell command")
	require.Contains(t, plain, "└ $ sleep 10")
	require.NotContains(t, plain, "[timeout 30s]")
	require.NotContains(t, plain, "Running")
	require.NotContains(t, plain, "…")
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderTranscriptCells_NormalizesLegacyRunCommandTimeouts(t *testing.T) {
	startedAt := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	originalCurrentTime := currentTime
	t.Cleanup(func() { currentTime = originalCurrentTime })
	currentTime = func() time.Time {
		return startedAt.Add(6 * time.Second)
	}

	rendered := renderTranscriptCells([]string{
		toolOperationTranscriptCellWithTiming("call_1", "run_command", "sleep 10 (30s)", startedAt, time.Time{}, false),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "└ $ sleep 10 [timeout 30s] (6s)")
	require.NotContains(t, plain, "sleep 10 (30s) (6s)")
}

func TestRenderTranscriptCells_RemovesRunCommandTimeoutHintWhenCompleted(t *testing.T) {
	startedAt := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		detail     string
		notContain string
	}{
		{name: "current timeout hint", detail: "sleep 10 [timeout 30s]", notContain: "[timeout 30s]"},
		{name: "legacy termination hint", detail: "sleep 10 [terminates in 30s]", notContain: "[terminates in 30s]"},
		{name: "legacy parenthetical timeout", detail: "sleep 10 (30s)", notContain: "sleep 10 (30s) (6s)"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rendered := renderTranscriptCells([]string{
				toolOperationTranscriptCellWithTiming(
					"call_1",
					"run_command",
					tt.detail,
					startedAt,
					startedAt.Add(6*time.Second),
					true,
				),
			})
			plain := stripANSI(rendered)

			require.Contains(t, plain, "└ $ sleep 10 (6s)")
			require.NotContains(t, plain, tt.notContain)
		})
	}
}

func TestRenderTranscriptCell_RendersUserMessageBox(t *testing.T) {
	rendered := renderTranscriptCellWithWidth("You: Some message.", 40)
	plain := stripANSI(rendered)
	lines := strings.Split(plain, "\n")

	require.Contains(t, plain, "❯ Some message.")
	require.Len(t, lines, 3)
	require.Equal(t, strings.Repeat("▄", 40), lines[0])
	require.Equal(t, "❯ Some message.", strings.TrimRight(lines[1], " "))
	require.Equal(t, strings.Repeat("▀", 40), lines[2])
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

	require.Len(t, lines, 4)
	require.Equal(t, strings.Repeat("▄", 40), lines[0])
	require.Equal(t, "❯ hello", strings.TrimRight(lines[1], " "))
	require.Equal(t, "  friend", strings.TrimRight(lines[2], " "))
	require.Equal(t, strings.Repeat("▀", 40), lines[3])
	require.Equal(t, 1, strings.Count(plain, "❯"))
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderTranscriptCell_RendersAssistantMarkdown(t *testing.T) {
	rendered := renderTranscriptCellWithWidth(
		"Hand: # Title\n\n## Key Complications\n\n### What Could Happen Next\n\n- first\n- second\n\n```go\nfmt.Println(\"hi\")\n```",
		60,
	)
	plain := stripANSI(rendered)

	require.NotContains(t, plain, "Hand:")
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
		toolOperationTranscriptCellWithTiming("", "read_file", "", now.Add(2*time.Second), time.Time{}, false),
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
		toolOperationTranscriptCellWithTiming("", "web_search", "", now.Add(2*time.Second), time.Time{}, false),
		toolOperationTranscriptCellWithTiming("", "web_search", "", time.Time{}, now.Add(3*time.Second), true),
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
		toolOperationTranscriptCellWithTiming(
			"call_1",
			"run_command",
			"sleep 10 [timeout 30s]",
			now,
			now.Add(time.Second),
			true,
		),
	}, cells)
	plain := stripANSI(renderTranscriptCells(cells))
	require.Contains(t, plain, "● Ran 1 shell command")
	require.Contains(t, plain, "└ $ sleep 10 (1s)")
	require.NotContains(t, plain, "[timeout 30s]")
	require.NotContains(t, plain, "run_command")
}

func TestSessionTimelineToTranscriptCells_RendersHydratedRunCommandLikeLiveTrace(t *testing.T) {
	now := time.Date(2026, 5, 18, 15, 0, 0, 0, time.UTC)
	hydratedCells := sessionTimelineToTranscriptCells(client.SessionTimeline{
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
				CreatedAt:  now.Add(6 * time.Second),
			}},
		},
	})
	liveCells := sessionTimelineToTranscriptCells(client.SessionTimeline{
		TraceEvents: []agent.SessionTimelineTraceEvent{
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationStarted,
				Timestamp: now,
				Payload: map[string]any{
					"id":     "call_1",
					"name":   "run_command",
					"detail": "sleep 10 [timeout 30s]",
				},
			}},
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationCompleted,
				Timestamp: now.Add(6 * time.Second),
				Payload: map[string]any{
					"tool_call_id": "call_1",
					"name":         "run_command",
				},
			}},
		},
	})

	hydratedPlain := stripANSI(renderTranscriptCells(hydratedCells))
	livePlain := stripANSI(renderTranscriptCells(liveCells))
	require.Contains(t, hydratedPlain, "● Ran 1 shell command")
	require.Contains(t, livePlain, "● Ran 1 shell command")
	require.NotContains(t, hydratedPlain, "[timeout 30s]")
	require.NotContains(t, livePlain, "[timeout 30s]")
	require.Contains(t, hydratedPlain, "└ $ sleep 10 (6s)")
	require.Contains(t, livePlain, "└ $ sleep 10 (6s)")
	require.Equal(t, livePlain, hydratedPlain)
}

func TestSessionTimelineToTranscriptCells_RendersHydratedListFilesLikeLiveTrace(t *testing.T) {
	now := time.Date(2026, 5, 18, 15, 0, 0, 0, time.UTC)
	detail := "list_files(include_hidden=false max_entries=50 path=. recursive=false)"
	hydratedCells := sessionTimelineToTranscriptCells(client.SessionTimeline{
		Messages: []agent.SessionTimelineMessage{
			{Message: handmsg.Message{
				Role:      handmsg.RoleAssistant,
				ToolCalls: []handmsg.ToolCall{{ID: "call_1", Name: "list_files", Input: `{"path":".","recursive":false,"include_hidden":false,"max_entries":50}`}},
				CreatedAt: now,
			}},
			{Message: handmsg.Message{
				Role:       handmsg.RoleTool,
				Name:       "list_files",
				ToolCallID: "call_1",
				Content:    `{"output":"done"}`,
				CreatedAt:  now.Add(time.Second),
			}},
		},
	})
	liveCells := sessionTimelineToTranscriptCells(client.SessionTimeline{
		TraceEvents: []agent.SessionTimelineTraceEvent{
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationStarted,
				Timestamp: now,
				Payload: map[string]any{
					"id":     "call_1",
					"name":   "list_files",
					"detail": detail,
				},
			}},
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationCompleted,
				Timestamp: now.Add(time.Second),
				Payload: map[string]any{
					"tool_call_id": "call_1",
					"name":         "list_files",
				},
			}},
		},
	})

	hydratedPlain := stripANSI(renderTranscriptCells(hydratedCells))
	livePlain := stripANSI(renderTranscriptCells(liveCells))
	require.Contains(t, hydratedPlain, "● List Files (1s)")
	require.Contains(t, hydratedPlain, "└ "+detail+" (1s)")
	require.Equal(t, livePlain, hydratedPlain)
}

func TestSessionTimelineToTranscriptCells_RendersHydratedFileToolsLikeLiveTrace(t *testing.T) {
	now := time.Date(2026, 5, 18, 15, 0, 0, 0, time.UTC)
	hydratedCells := sessionTimelineToTranscriptCells(client.SessionTimeline{
		Messages: []agent.SessionTimelineMessage{
			{Message: handmsg.Message{
				Role: handmsg.RoleAssistant,
				ToolCalls: []handmsg.ToolCall{
					{
						ID:    "call_1",
						Name:  "write_file",
						Input: `{"path":"file.txt","content":"SECRET=example"}`,
					},
					{
						ID:    "call_2",
						Name:  "patch",
						Input: `{"patch":"--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n"}`,
					},
					{
						ID:    "call_3",
						Name:  "read_file",
						Input: `{"path":"file.txt"}`,
					},
				},
				CreatedAt: now,
			}},
			{Message: handmsg.Message{
				Role:       handmsg.RoleTool,
				Name:       "write_file",
				ToolCallID: "call_1",
				Content:    `{"output":"done"}`,
				CreatedAt:  now.Add(30 * time.Second),
			}},
			{Message: handmsg.Message{
				Role:       handmsg.RoleTool,
				Name:       "patch",
				ToolCallID: "call_2",
				Content:    `{"output":"done"}`,
				CreatedAt:  now.Add(30 * time.Second),
			}},
			{Message: handmsg.Message{
				Role:       handmsg.RoleTool,
				Name:       "read_file",
				ToolCallID: "call_3",
				Content:    `{"output":"done"}`,
				CreatedAt:  now.Add(30 * time.Second),
			}},
		},
	})
	liveCells := sessionTimelineToTranscriptCells(client.SessionTimeline{
		TraceEvents: []agent.SessionTimelineTraceEvent{
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationStarted,
				Timestamp: now,
				Payload: map[string]any{
					"id":     "call_1",
					"name":   "write_file",
					"detail": "write_file file.txt",
				},
			}},
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationCompleted,
				Timestamp: now.Add(30 * time.Second),
				Payload: map[string]any{
					"tool_call_id": "call_1",
					"name":         "write_file",
				},
			}},
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationStarted,
				Timestamp: now.Add(31 * time.Second),
				Payload: map[string]any{
					"id":     "call_2",
					"name":   "patch",
					"detail": "patch file.txt +1 -1",
				},
			}},
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationCompleted,
				Timestamp: now.Add(61 * time.Second),
				Payload: map[string]any{
					"tool_call_id": "call_2",
					"name":         "patch",
				},
			}},
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationStarted,
				Timestamp: now.Add(62 * time.Second),
				Payload: map[string]any{
					"id":     "call_3",
					"name":   "read_file",
					"detail": "read_file file.txt",
				},
			}},
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationCompleted,
				Timestamp: now.Add(92 * time.Second),
				Payload: map[string]any{
					"tool_call_id": "call_3",
					"name":         "read_file",
				},
			}},
		},
	})

	hydratedPlain := stripANSI(renderTranscriptCells(hydratedCells))
	livePlain := stripANSI(renderTranscriptCells(liveCells))
	require.Contains(t, hydratedPlain, "└ write_file file.txt (30s)")
	require.Contains(t, hydratedPlain, "└ patch file.txt +1 -1 (30s)")
	require.Contains(t, hydratedPlain, "└ read_file file.txt (30s)")
	require.NotContains(t, hydratedPlain, "SECRET=example")
	require.Equal(t, livePlain, hydratedPlain)
}
