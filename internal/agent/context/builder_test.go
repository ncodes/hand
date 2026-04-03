package context

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/agent/memory"
	handmsg "github.com/wandxy/hand/internal/messages"
)

func TestBuilder_BuildReturnsSessionHistoryThenEmittedMessages(t *testing.T) {
	builder := New()
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	messages := builder.Build(Input{
		SessionHistory:  []handmsg.Message{{Role: handmsg.RoleUser, Content: "before", CreatedAt: now}},
		EmittedMessages: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "after", CreatedAt: now.Add(time.Second)}},
	})

	require.Equal(t, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "before", CreatedAt: now},
		{Role: handmsg.RoleAssistant, Content: "after", CreatedAt: now.Add(time.Second)},
	}, messages)
}

func TestBuilder_BuildClonesReturnedMessages(t *testing.T) {
	builder := New()
	input := Input{
		SessionHistory: []handmsg.Message{{
			Role:      handmsg.RoleAssistant,
			ToolCalls: []handmsg.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
		}},
		EmittedMessages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	}

	messages := builder.Build(input)
	messages[0].ToolCalls[0].Name = "changed"
	messages[1].Content = "mutated"

	require.Equal(t, "time", input.SessionHistory[0].ToolCalls[0].Name)
	require.Equal(t, "hello", input.EmittedMessages[0].Content)
}

func TestBuilder_BuildReturnsOnlyEmittedMessagesWhenHistoryEmpty(t *testing.T) {
	builder := New()
	messages := builder.Build(Input{
		EmittedMessages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.Equal(t, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}, messages)
}

func TestBuilder_BuildReturnsOnlySessionHistoryWhenEmittedMessagesEmpty(t *testing.T) {
	builder := New()
	messages := builder.Build(Input{
		SessionHistory: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "hello"}},
	})
	require.Equal(t, []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "hello"}}, messages)
}

func TestBuilder_BuildIgnoresNilOrEmptySummary(t *testing.T) {
	builder := New()

	withNil := builder.Build(Input{
		SessionHistory:  []handmsg.Message{{Role: handmsg.RoleUser, Content: "history"}},
		EmittedMessages: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "emitted"}},
	})
	withEmpty := builder.Build(Input{
		SessionHistory:  []handmsg.Message{{Role: handmsg.RoleUser, Content: "history"}},
		EmittedMessages: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "emitted"}},
		Memory:          &memory.Memory{Summary: &memory.SummaryState{}},
	})

	require.Equal(t, withNil, withEmpty)
}

func TestBuilder_BuildUsesPopulatedSummary(t *testing.T) {
	builder := New()

	withState := builder.Build(Input{
		SessionHistory: []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "history-1"},
			{Role: handmsg.RoleAssistant, Content: "history-2"},
			{Role: handmsg.RoleUser, Content: "history-3"},
		},
		EmittedMessages: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "emitted"}},
		Memory: &memory.Memory{
			Summary: &memory.SummaryState{
				SessionID:          "ses_summary",
				SourceEndOffset:    2,
				SourceMessageCount: 3,
				SessionSummary:     "Older context",
				CurrentTask:        "Fix tests",
				Discoveries:        []string{"one"},
				OpenQuestions:      []string{"two"},
				NextActions:        []string{"three"},
			},
		},
	})

	require.Len(t, withState, 3)
	require.Equal(t, handmsg.RoleDeveloper, withState[0].Role)
	require.Contains(t, withState[0].Content, "Session Summary:\nOlder context")
	require.Contains(t, withState[0].Content, "Current Task:\nFix tests")
	require.Contains(t, withState[0].Content, "Discoveries:\n- one")
	require.Contains(t, withState[0].Content, "Open Questions:\n- two")
	require.Contains(t, withState[0].Content, "Next Actions:\n- three")
	require.Equal(t, "history-3", withState[1].Content)
	require.Equal(t, "emitted", withState[2].Content)
}

func TestBuilder_BuildClampsSummaryOffsetToSessionHistoryLength(t *testing.T) {
	builder := New()

	messages := builder.Build(Input{
		SessionHistory: []handmsg.Message{{Role: handmsg.RoleUser, Content: "history"}},
		Memory: &memory.Memory{Summary: &memory.SummaryState{
			SessionID:          "ses_summary",
			SourceEndOffset:    5,
			SourceMessageCount: 1,
			SessionSummary:     "Older context",
		}},
	})

	require.Len(t, messages, 1)
	require.Equal(t, handmsg.RoleDeveloper, messages[0].Role)
}
