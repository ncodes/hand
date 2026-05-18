package tui

import (
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
			want:    "Tool read_file: done",
		},
		{
			name:    "tool fallback",
			message: handmsg.Message{Role: handmsg.RoleTool, Content: "done"},
			want:    "Tool tool: done",
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
			require.Equal(t, tt.want, timelineMessageToTranscriptCell(tt.message))
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
		{name: "tool started", msg: toolInvocationStartedMsg{Name: "read_file"}, want: "Tool started: read_file"},
		{name: "tool completed", msg: toolInvocationCompletedMsg{Name: "read_file"}, want: "Tool completed: read_file"},
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
		timelineMessageToTranscriptCell(handmsg.Message{Role: handmsg.RoleUser, Content: "hello"}),
		tuiMessageToTranscriptCell(assistantResponseCompletedMsg{Text: "hi"}),
		tuiMessageToTranscriptCell(toolInvocationStartedMsg{Name: "read_file"}),
		tuiMessageToTranscriptCell(safetyEventMsg{Action: "blocked"}),
		tuiMessageToTranscriptCell(sessionErrorMsg{Message: "failed"}),
	})

	plain := stripANSI(rendered)
	require.Contains(t, plain, "You: hello")
	require.Contains(t, plain, "Hand: hi")
	require.Contains(t, plain, "Tool started: read_file")
	require.Contains(t, plain, "Safety: blocked")
	require.Contains(t, plain, "Error: failed")
	require.Contains(t, rendered, "\x1b[")
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
		"Tool started: read_file",
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
		"Tool started: web_search",
		"Tool completed: web_search",
		"You: Hi",
		"Hand: Hi there",
	}, cells)
}
