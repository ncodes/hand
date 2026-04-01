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

func TestBuilder_BuildIgnoresNilOrEmptySummaryAndPinnedState(t *testing.T) {
	builder := New()

	withNil := builder.Build(Input{
		SessionHistory:  []handmsg.Message{{Role: handmsg.RoleUser, Content: "history"}},
		EmittedMessages: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "emitted"}},
	})
	withEmpty := builder.Build(Input{
		SessionHistory:  []handmsg.Message{{Role: handmsg.RoleUser, Content: "history"}},
		EmittedMessages: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "emitted"}},
		Summary:         &memory.SummaryState{},
		Pinned:          &memory.PinnedState{},
	})

	require.Equal(t, withNil, withEmpty)
}

func TestBuilder_BuildIgnoresPopulatedSummaryAndPinnedState(t *testing.T) {
	builder := New()

	baseline := builder.Build(Input{
		SessionHistory:  []handmsg.Message{{Role: handmsg.RoleUser, Content: "history"}},
		EmittedMessages: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "emitted"}},
	})
	withState := builder.Build(Input{
		SessionHistory:  []handmsg.Message{{Role: handmsg.RoleUser, Content: "history"}},
		EmittedMessages: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "emitted"}},
		Summary:         &memory.SummaryState{Messages: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "summary"}}},
		Pinned:          &memory.PinnedState{Messages: []handmsg.Message{{Role: handmsg.RoleDeveloper, Content: "pinned"}}},
	})

	require.Equal(t, baseline, withState)
}
