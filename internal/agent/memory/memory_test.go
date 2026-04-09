package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
)

func TestNewService_LogsWhenSummaryProviderAndAPIModeDifferFromMain(t *testing.T) {
	cfg := &config.Config{
		Name:                "t",
		Model:               "openai/gpt-4o-mini",
		ModelProvider:       "openrouter",
		ModelAPIMode:        config.DefaultModelAPIMode,
		SummaryProvider:     "openai",
		SummaryModelAPIMode: "responses",
		ContextLength:       100,
	}
	cfg.Normalize()

	svc := NewService(cfg, nil, nil, nil)
	require.NotNil(t, svc)
}

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

func TestMemory_RenderSummaryInstructions(t *testing.T) {
	mem := &Memory{
		Summary: &SummaryState{
			SessionSummary: "Older work",
			CurrentTask:    "Fix tests",
			Discoveries:    []string{"one"},
			OpenQuestions:  []string{"two"},
			NextActions:    []string{"three"},
		},
	}

	message, ok := mem.RenderSummaryInstructions()
	require.True(t, ok)
	require.Contains(t, message, "# Session Summary\n\nOlder work")
	require.Contains(t, message, "# Current Task\n\nFix tests")
	require.Contains(t, message, "# Discoveries\n\n- one")
	require.Contains(t, message, "# Open Questions\n\n- two")
	require.Contains(t, message, "# Next Actions\n\n- three")
}

func TestMemory_RenderSummaryInstructions_ReturnsFalseWhenUnavailable(t *testing.T) {
	message, ok := (*Memory)(nil).RenderSummaryInstructions()
	require.False(t, ok)
	require.Empty(t, message)

	message, ok = (&Memory{}).RenderSummaryInstructions()
	require.False(t, ok)
	require.Empty(t, message)

	message, ok = (&Memory{Summary: &SummaryState{SessionSummary: "   "}}).RenderSummaryInstructions()
	require.False(t, ok)
	require.Empty(t, message)
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

	require.Empty(t, recall.PrefixMessages)
	require.Equal(t, history, recall.SessionHistory)
}

func TestMemory_Recall_DefaultsForNilAndPreservesHistoryWithSummary(t *testing.T) {
	history := []handmsg.Message{{Role: handmsg.RoleUser, Content: "history"}}

	recall := (*Memory)(nil).Recall(history)
	require.Empty(t, recall.PrefixMessages)
	require.Equal(t, history, recall.SessionHistory)

	recall = (&Memory{Summary: &SummaryState{SourceEndOffset: 99, SessionSummary: "Older work"}}).Recall(history)
	require.Empty(t, recall.PrefixMessages)
	require.Equal(t, history, recall.SessionHistory)
}
