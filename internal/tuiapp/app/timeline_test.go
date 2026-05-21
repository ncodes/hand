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

func transcriptCellPlainText(cell transcriptCell) string {
	if cell == nil || cell.IsEmpty() {
		return ""
	}

	return cell.PlainText()
}

func transcriptCellPlainTexts(cells []transcriptCell) []string {
	values := make([]string, 0, len(cells))
	for _, cell := range cells {
		if value := transcriptCellPlainText(cell); value != "" {
			values = append(values, value)
		}
	}

	return values
}

func renderTranscriptTestCellWithWidth(cell transcriptCell, width int) string {
	if cell == nil || cell.IsEmpty() {
		return ""
	}
	if toolCell, ok := cell.(toolTranscriptCell); ok {
		group := toolTranscriptGroup{action: toolCell.action}
		group.add(toolCell)
		return renderToolTranscriptGroup(group, 0)
	}

	return defaultTranscriptRenderer.RenderCell(cell, transcriptRenderContext{Width: width, Now: currentTime()})
}

func renderTranscriptTestCell(cell transcriptCell) string {
	return renderTranscriptTestCellWithWidth(cell, defaultWidth)
}

func toolTranscriptTestCell(id string, name string, detail string, completed ...bool) transcriptCell {
	isCompleted := len(completed) > 0 && completed[0]

	return newToolTranscriptCell(id, name, detail, time.Time{}, time.Time{}, isCompleted)
}

func toolTranscriptTestCellWithTiming(
	id string,
	name string,
	detail string,
	startedAt time.Time,
	completedAt time.Time,
	completed bool,
) transcriptCell {
	return newToolTranscriptCell(id, name, detail, startedAt, completedAt, completed)
}

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
			want:    transcriptCellPlainText(toolTranscriptTestCell("", "read_file", "", true)),
		},
		{
			name:    "tool fallback",
			message: handmsg.Message{Role: handmsg.RoleTool, Content: "done"},
			want:    transcriptCellPlainText(toolTranscriptTestCell("", "tool", "", true)),
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
			require.Equal(t, tt.want, transcriptCellPlainText(timelineMessageToTranscriptCell(tt.message, nil)))
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
		{name: "tool started", msg: toolInvocationStartedMsg{Name: "read_file"}, want: transcriptCellPlainText(toolTranscriptTestCell("", "read_file", ""))},
		{name: "tool completed", msg: toolInvocationCompletedMsg{Name: "read_file"}, want: transcriptCellPlainText(toolTranscriptTestCell("", "read_file", "", true))},
		{name: "safety", msg: safetyEventMsg{Action: "blocked", FindingIDs: []string{"prompt_exfiltration"}}, want: "Safety: blocked: prompt_exfiltration"},
		{name: "error", msg: sessionErrorMsg{Message: "failed"}, want: "Error: failed"},
		{name: "empty", msg: userMessageAcceptedMsg{Text: " "}, want: ""},
		{name: "unknown", msg: struct{}{}, want: ""},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, transcriptCellPlainText(tuiMessageToTranscriptCell(tt.msg)))
		})
	}
}

func TestTranscriptCells_ExposeTypedCellContract(t *testing.T) {
	startedAt := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(2 * time.Second)
	cases := []struct {
		name       string
		cell       transcriptCell
		kind       transcriptCellKind
		plainText  string
		renderText string
	}{
		{
			name:       "user",
			cell:       userTranscriptCell{text: "hello"},
			kind:       transcriptCellUser,
			plainText:  "You: hello",
			renderText: "❯ hello",
		},
		{
			name:       "assistant",
			cell:       assistantTranscriptCell{text: "hi"},
			kind:       transcriptCellAssistant,
			plainText:  "Hand: hi",
			renderText: "hi",
		},
		{
			name:       "reasoning",
			cell:       reasoningTranscriptCell{text: "thinking", startedAt: startedAt},
			kind:       transcriptCellReasoning,
			plainText:  "Reasoning: thinking",
			renderText: "Thinking",
		},
		{
			name:       "thought",
			cell:       thoughtTranscriptCell{duration: 3 * time.Second},
			kind:       transcriptCellThought,
			plainText:  "Thought: 3s",
			renderText: "Thought for 3s",
		},
		{
			name:       "safety",
			cell:       safetyTranscriptCell{action: "blocked", findingIDs: []string{"prompt_exfiltration"}},
			kind:       transcriptCellSafety,
			plainText:  "Safety: blocked: prompt_exfiltration",
			renderText: "Safety: blocked: prompt_exfiltration",
		},
		{
			name:       "error",
			cell:       errorTranscriptCell{message: "failed"},
			kind:       transcriptCellError,
			plainText:  "Error: failed",
			renderText: "Error: failed",
		},
		{
			name:       "system",
			cell:       systemTranscriptCell{text: "note"},
			kind:       transcriptCellSystem,
			plainText:  "note",
			renderText: "note",
		},
		{
			name:       "tool",
			cell:       newToolTranscriptCell("call_1", "list_files", "list_files(path=.)", startedAt, completedAt, true),
			kind:       transcriptCellTool,
			plainText:  "Tool List Files:\nid: call_1\ndetail: list_files(path=.)\nstarted_at: 2026-05-20T09:00:00Z\ncompleted_at: 2026-05-20T09:00:02Z\nstatus: completed",
			renderText: "List Files",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.cell)
			require.Equal(t, tt.kind, tt.cell.Kind())
			require.False(t, tt.cell.IsEmpty())
			require.Equal(t, tt.plainText, tt.cell.PlainText())
			rendered := defaultTranscriptRenderer.RenderCell(
				tt.cell,
				transcriptRenderContext{Width: 40, Now: completedAt},
			)
			require.Contains(t, stripANSI(rendered), tt.renderText)
		})
	}
}

func TestTranscriptCells_ReportEmptyState(t *testing.T) {
	cases := []transcriptCell{
		userTranscriptCell{text: " "},
		assistantTranscriptCell{text: " "},
		reasoningTranscriptCell{text: " "},
		thoughtTranscriptCell{},
		safetyTranscriptCell{},
		errorTranscriptCell{},
		systemTranscriptCell{text: " "},
	}

	for _, cell := range cases {
		require.True(t, cell.IsEmpty())
		require.Empty(t, cell.PlainText())
	}
}

func TestRenderTranscriptCell_StylesCanonicalCells(t *testing.T) {
	rendered := renderTranscriptCells([]transcriptCell{
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
	rendered := renderTranscriptTestCellWithWidth(reasoningTranscriptCell{text: "first token\nsecond token"}, 40)

	plain := stripANSI(rendered)
	require.Contains(t, plain, "Thinking")
	require.Contains(t, plain, "└ first token")
	require.Contains(t, plain, "  second token")
	require.NotContains(t, plain, "Reasoning:")
}

func TestRenderTranscriptCell_RendersCollapsedThought(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(thoughtTranscriptCell{duration: 3 * time.Second}, 40)

	plain := stripANSI(rendered)
	require.Equal(t, "Thought for 3s", plain)
	require.NotContains(t, plain, "Thought:")
}

func TestRenderTranscriptCells_GroupsAdjacentToolOperationsByAction(t *testing.T) {
	rendered := renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCell("call_1", "write_file", ""),
		toolTranscriptTestCell("call_2", "write_file", ""),
		toolTranscriptTestCell("call_3", "read_file", ""),
	})
	plain := stripANSI(rendered)

	require.Equal(t, 1, strings.Count(plain, "● Write"))
	require.Equal(t, 1, strings.Count(plain, "● Read"))
	require.Contains(t, plain, "├ write_file")
	require.Contains(t, plain, "└ write_file")
	require.Contains(t, plain, "└ read_file")
}

func TestRenderTranscriptCells_DeduplicatesStartedAndCompletedToolEvents(t *testing.T) {
	rendered := renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCell("call_1", "web_search", `Search "what is todays news..."`),
		toolTranscriptTestCell("call_1", "web_search", `Search "what is todays news..."`, true),
	})
	plain := stripANSI(rendered)

	require.Equal(t, 1, strings.Count(plain, "● Searched"))
	require.Equal(t, 1, strings.Count(plain, `Search "what is todays news..."`))
	require.NotContains(t, plain, "web_search")
}

func TestRenderTranscriptCells_RendersMemorySearchLikeSearch(t *testing.T) {
	rendered := renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCell("call_1", "memory_search", `Search "commit preferences"`),
		toolTranscriptTestCell("call_1", "memory_search", `Search "commit preferences"`, true),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "● Searched Memory")
	require.Contains(t, plain, `Search "commit preferences"`)
	require.NotContains(t, plain, "memory_search")
}

func TestRenderTranscriptCells_RendersRunningMemorySearchTitle(t *testing.T) {
	rendered := renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCell("call_1", "memory_search", `Search "commit preferences"`),
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
			running := stripANSI(renderTranscriptCells([]transcriptCell{
				toolTranscriptTestCell("call_1", tt.tool, ""),
			}))
			completed := stripANSI(renderTranscriptCells([]transcriptCell{
				toolTranscriptTestCell("call_1", tt.tool, "", true),
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

func TestRenderTranscriptCells_RendersSessionMessagesWithFriendlyText(t *testing.T) {
	running := stripANSI(renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCell("call_1", "session_messages", ""),
	}))
	completed := stripANSI(renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCell("call_1", "session_messages", "", true),
	}))
	withDetail := stripANSI(renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCell("call_1", "session_messages", "session_messages(offset_start=3 offset_end=9)", true),
	}))

	require.Contains(t, running, "● Fetching Session Messages")
	require.NotContains(t, running, "└")
	require.NotContains(t, running, "session_messages")
	require.Contains(t, completed, "● Fetched Session Messages")
	require.NotContains(t, completed, "└")
	require.NotContains(t, completed, "session_messages")
	require.Contains(t, withDetail, "● Fetched Session Messages")
	require.NotContains(t, withDetail, "└")
	require.NotContains(t, withDetail, "session_messages")
}

func TestRenderTranscriptCells_RendersSessionSearchWithFriendlyText(t *testing.T) {
	startedAt := time.Date(2026, 5, 20, 23, 0, 0, 0, time.UTC)
	running := stripANSI(defaultTranscriptRenderer.RenderCells([]transcriptCell{
		toolTranscriptTestCellWithTiming("call_1", "session_search", `Search "favorite color"`, startedAt, time.Time{}, false),
	}, transcriptRenderContext{Width: 80, Now: startedAt.Add(45 * time.Second)}))
	completed := stripANSI(defaultTranscriptRenderer.RenderCells([]transcriptCell{
		toolTranscriptTestCellWithTiming("call_1", "session_search", `Search "favorite color"`, startedAt, startedAt.Add(45*time.Second), true),
	}, transcriptRenderContext{Width: 80, Now: startedAt.Add(45 * time.Second)}))

	require.Contains(t, running, "● Searching Session (45s)")
	require.NotContains(t, running, "└")
	require.NotContains(t, running, "session_search")
	require.Contains(t, completed, "● Searched Session (45s)")
	require.NotContains(t, completed, "└")
	require.NotContains(t, completed, "session_search")
}

func TestRenderTranscriptCells_AnimatesRunningToolDot(t *testing.T) {
	cells := []transcriptCell{toolTranscriptTestCell("call_1", "web_search", "")}
	first := stripANSI(renderTranscriptCellsWithFrame(cells, 80, 0))
	next := stripANSI(renderTranscriptCellsWithFrame(cells, 80, 1))
	completed := stripANSI(renderTranscriptCellsWithFrame([]transcriptCell{
		toolTranscriptTestCell("call_1", "web_search", "", true),
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

	running := stripANSI(renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCellWithTiming("call_1", "list_files", "", startedAt, time.Time{}, false),
	}))
	completed := stripANSI(renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCellWithTiming("call_1", "list_files", "", startedAt, startedAt.Add(12*time.Second), true),
	}))

	require.Contains(t, running, "● List Files (10s)")
	require.Contains(t, running, "└ list_files (10s)")
	require.Contains(t, completed, "● List Files (12s)")
	require.Contains(t, completed, "└ list_files (12s)")
}

func TestRenderTranscriptCells_RendersFileToolDetails(t *testing.T) {
	startedAt := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	rendered := renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCellWithTiming(
			"call_1",
			"write_file",
			"write_file file.txt",
			startedAt,
			startedAt.Add(30*time.Second),
			true,
		),
		toolTranscriptTestCellWithTiming(
			"call_2",
			"patch",
			"patch file.txt +1 -1",
			startedAt,
			startedAt.Add(30*time.Second),
			true,
		),
		toolTranscriptTestCellWithTiming(
			"call_3",
			"patch",
			"patch",
			startedAt,
			startedAt.Add(30*time.Second),
			true,
		),
		toolTranscriptTestCellWithTiming(
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
	rendered := renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCellWithTiming(
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
	rendered := renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCell("call_1", "run_command", `sleep 10 && echo "Done" [timeout 8s]`),
	})
	plain := stripANSI(rendered)

	require.Contains(t, plain, "● Running 1 shell command…")
	require.Contains(t, plain, `└ $ sleep 10 && echo "Done" [timeout 8s]`)
	require.NotContains(t, plain, "ctrl+b")
	require.Contains(t, rendered, "\x1b[")
}

func TestRenderTranscriptCells_RendersCompletedRunCommandsWithPastTense(t *testing.T) {
	rendered := renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCell("call_1", "run_command", `sleep 10 [timeout 30s]`, true),
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

	rendered := renderTranscriptCells([]transcriptCell{
		toolTranscriptTestCellWithTiming("call_1", "run_command", "sleep 10 (30s)", startedAt, time.Time{}, false),
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
			rendered := renderTranscriptCells([]transcriptCell{
				toolTranscriptTestCellWithTiming(
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
	rendered := renderTranscriptTestCellWithWidth(userTranscriptCell{text: "Some message."}, 40)
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
	rendered := renderTranscriptTestCellWithWidth(userTranscriptCell{text: "hello\nfriend"}, 40)
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
	rendered := renderTranscriptTestCellWithWidth(
		assistantTranscriptCell{text: "# Title\n\n## Key Complications\n\n### What Could Happen Next\n\n- first\n- second\n\n```go\nfmt.Println(\"hi\")\n```"},
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

func TestRenderTranscriptCells_AlignsAssistantMarkdownWithThoughtCell(t *testing.T) {
	rendered := renderTranscriptCellsWithWidth([]transcriptCell{
		thoughtTranscriptCell{duration: time.Second},
		assistantTranscriptCell{text: "**54 sensors are working.**\n\nRechecked: 9 containers."},
	}, 80)
	lines := strings.Split(stripANSI(rendered), "\n")

	thoughtLine := indexLineContaining(lines, "Thought for 1s")
	answerLine := indexLineContaining(lines, "54 sensors are working.")
	require.NotEqual(t, -1, thoughtLine)
	require.NotEqual(t, -1, answerLine)
	require.Equal(t, countLeadingSpaces(lines[thoughtLine]), countLeadingSpaces(lines[answerLine]))
}

func TestRenderTranscriptCell_RendersCompactMarkdownTables(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(assistantTranscriptCell{text: strings.Join([]string{
		"| **Issue** | Details |",
		"| --- | --- |",
		"| [One](https://example.com) | `Short` |",
		"| Two | Also **short** |",
	}, "\n")}, 120)
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
	rendered := renderTranscriptTestCellWithWidth(assistantTranscriptCell{text: strings.Join([]string{
		"## Key Complications",
		"",
		"| Issue | Details |",
		"| --- | --- |",
		"| One | Short |",
	}, "\n")}, 120)
	lines := strings.Split(stripANSI(rendered), "\n")
	headingIndex := indexLineContaining(lines, "Key Complications")
	tableIndex := indexLineContaining(lines, "┌───────┬─────────┐")

	require.NotEqual(t, -1, headingIndex)
	require.NotEqual(t, -1, tableIndex)
	require.LessOrEqual(t, tableIndex-headingIndex, 2)
}

func TestRenderTranscriptCell_DoesNotRenderUserMarkdown(t *testing.T) {
	rendered := renderTranscriptTestCellWithWidth(userTranscriptCell{text: "# literal\n\n- keep"}, 60)
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
		transcriptCellPlainText(toolTranscriptTestCellWithTiming("", "read_file", "", now.Add(2*time.Second), time.Time{}, false)),
	}, transcriptCellPlainTexts(cells))
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
		transcriptCellPlainText(toolTranscriptTestCellWithTiming("", "web_search", "", now.Add(2*time.Second), time.Time{}, false)),
		transcriptCellPlainText(toolTranscriptTestCellWithTiming("", "web_search", "", time.Time{}, now.Add(3*time.Second), true)),
		"You: Hi",
		"Hand: Hi there",
	}, transcriptCellPlainTexts(cells))
}

func TestSessionTimelineToTranscriptCells_RendersPersistedThoughtSummary(t *testing.T) {
	now := time.Date(2026, 5, 18, 15, 0, 0, 0, time.UTC)

	cells := sessionTimelineToTranscriptCells(client.SessionTimeline{
		Messages: []agent.SessionTimelineMessage{
			{Message: handmsg.Message{Role: handmsg.RoleUser, Content: "think", CreatedAt: now}},
			{Message: handmsg.Message{Role: handmsg.RoleAssistant, Content: "done", CreatedAt: now.Add(2 * time.Second)}},
		},
		TraceEvents: []agent.SessionTimelineTraceEvent{{
			Event: storage.TraceEvent{
				Type:      trace.EvtModelReasoningCompleted,
				Timestamp: now.Add(time.Second),
				Payload:   map[string]any{"duration_ms": float64(2000)},
			},
		}},
	})

	require.Equal(t, []string{
		"You: think",
		"Thought: 2s",
		"Hand: done",
	}, transcriptCellPlainTexts(cells))
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
		transcriptCellPlainText(toolTranscriptTestCellWithTiming(
			"call_1",
			"run_command",
			"sleep 10 [timeout 30s]",
			now,
			now.Add(time.Second),
			true,
		)),
	}, transcriptCellPlainTexts(cells))
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

func TestSessionTimelineToTranscriptCells_RendersHydratedSessionMessagesLikeLiveTrace(t *testing.T) {
	now := time.Date(2026, 5, 18, 15, 0, 0, 0, time.UTC)
	detail := "session_messages(anchor_message_id=42 before=2 after=3 max_chars=1200)"
	hydratedCells := sessionTimelineToTranscriptCells(client.SessionTimeline{
		Messages: []agent.SessionTimelineMessage{
			{Message: handmsg.Message{
				Role: handmsg.RoleAssistant,
				ToolCalls: []handmsg.ToolCall{{
					ID:    "call_1",
					Name:  "session_messages",
					Input: `{"anchor_message_id":42,"before":2,"after":3,"max_chars":1200}`,
				}},
				CreatedAt: now,
			}},
			{Message: handmsg.Message{
				Role:       handmsg.RoleTool,
				Name:       "session_messages",
				ToolCallID: "call_1",
				Content:    `{"messages":[]}`,
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
					"name":   "session_messages",
					"detail": detail,
				},
			}},
			{Event: storage.TraceEvent{
				Type:      trace.EvtToolInvocationCompleted,
				Timestamp: now.Add(time.Second),
				Payload: map[string]any{
					"tool_call_id": "call_1",
					"name":         "session_messages",
				},
			}},
		},
	})

	hydratedPlain := stripANSI(renderTranscriptCells(hydratedCells))
	livePlain := stripANSI(renderTranscriptCells(liveCells))
	require.Contains(t, hydratedPlain, "● Fetched Session Messages (1s)")
	require.NotContains(t, hydratedPlain, "└")
	require.NotContains(t, hydratedPlain, detail)
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
