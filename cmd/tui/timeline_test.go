package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
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
