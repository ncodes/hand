package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
)

func TestMemory_SummaryToStorage_ReturnsZeroValueWithoutSummary(t *testing.T) {
	require.Equal(t, storage.SessionSummary{}, (*Memory)(nil).SummaryToStorage())
	require.Equal(t, storage.SessionSummary{}, (&Memory{}).SummaryToStorage())
}

func TestMemory_SummaryToStorage_ClonesSummary(t *testing.T) {
	mem := &Memory{
		Summary: &SummaryState{
			SessionID:          "ses_test",
			SourceEndOffset:    2,
			SourceMessageCount: 5,
			UpdatedAt:          time.Now().UTC(),
			SessionSummary:     "Older work",
			CurrentTask:        "Fix tests",
			Discoveries:        []string{"one"},
			OpenQuestions:      []string{"two"},
			NextActions:        []string{"three"},
		},
	}

	stored := mem.SummaryToStorage()
	require.Equal(t, "ses_test", stored.SessionID)
	require.Equal(t, "Older work", stored.SessionSummary)

	mem.Summary.Discoveries[0] = "changed"
	require.Equal(t, "one", stored.Discoveries[0])
}

func TestMemory_RenderSummaryMessage(t *testing.T) {
	mem := &Memory{
		Summary: &SummaryState{
			SessionSummary: "Older work",
			CurrentTask:    "Fix tests",
			Discoveries:    []string{"one"},
			OpenQuestions:  []string{"two"},
			NextActions:    []string{"three"},
		},
	}

	message, ok := mem.RenderSummaryMessage()
	require.True(t, ok)
	require.Equal(t, handmsg.RoleDeveloper, message.Role)
	require.Contains(t, message.Content, "Session Summary:\nOlder work")
	require.Contains(t, message.Content, "Current Task:\nFix tests")
	require.Contains(t, message.Content, "Discoveries:\n- one")
	require.Contains(t, message.Content, "Open Questions:\n- two")
	require.Contains(t, message.Content, "Next Actions:\n- three")
}

func TestMemory_RenderSummaryMessage_ReturnsFalseWhenUnavailable(t *testing.T) {
	message, ok := (*Memory)(nil).RenderSummaryMessage()
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	message, ok = (&Memory{}).RenderSummaryMessage()
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	message, ok = (&Memory{Summary: &SummaryState{SessionSummary: "   "}}).RenderSummaryMessage()
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)
}

func TestMemory_Recall(t *testing.T) {
	history := []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "old"},
		{Role: handmsg.RoleAssistant, Content: "recent"},
	}

	recall := (&Memory{Summary: &SummaryState{
		SourceEndOffset: 1,
		SessionSummary:  "Older work",
	}}).Recall(history)

	require.Len(t, recall.PrefixMessages, 1)
	require.Equal(t, handmsg.RoleDeveloper, recall.PrefixMessages[0].Role)
	require.Equal(t, []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "recent"}}, recall.SessionHistory)
}

func TestMemory_Recall_DefaultsForNilAndOutOfRangeSummary(t *testing.T) {
	history := []handmsg.Message{{Role: handmsg.RoleUser, Content: "history"}}

	recall := (*Memory)(nil).Recall(history)
	require.Empty(t, recall.PrefixMessages)
	require.Equal(t, history, recall.SessionHistory)

	recall = (&Memory{Summary: &SummaryState{SourceEndOffset: 99, SessionSummary: "Older work"}}).Recall(history)
	require.Len(t, recall.PrefixMessages, 1)
	require.Empty(t, recall.SessionHistory)
}
