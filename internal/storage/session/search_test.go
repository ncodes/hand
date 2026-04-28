package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage/retrieval"
)

func TestMessageIndexRowsFromMessage(t *testing.T) {
	now := time.Now().UTC()

	require.Nil(t, MessageIndexRowsFromMessage("ses_a", handmsg.Message{Role: handmsg.RoleUser}))
	require.Nil(t, MessageIndexRowsFromMessage("ses_a", handmsg.Message{Role: handmsg.RoleTool, Name: "process"}))
	require.Nil(t, MessageIndexRowsFromMessage("ses_a", handmsg.Message{
		Role:      handmsg.RoleAssistant,
		ToolCalls: []handmsg.ToolCall{{}},
	}))

	rows := MessageIndexRowsFromMessage("ses_a", handmsg.Message{
		ID:        3,
		Role:      handmsg.RoleUser,
		Content:   "user body",
		CreatedAt: now,
	})
	require.Len(t, rows, 1)
	require.Equal(t, "user body", rows[0].Body)

	rows = MessageIndexRowsFromMessage(" ses_a ", handmsg.Message{
		ID:        1,
		Role:      handmsg.RoleAssistant,
		Content:   "assistant body",
		CreatedAt: now,
		ToolCalls: []handmsg.ToolCall{{
			ID:    "call-1",
			Name:  "Search Files",
			Input: `{"pattern":"needle"}`,
		}},
	})
	require.Len(t, rows, 2)
	require.Equal(t, "ses_a", rows[0].SessionID)
	require.Equal(t, "assistant body", rows[0].Body)
	require.Equal(t, "search files", rows[1].ToolName)

	rows = MessageIndexRowsFromMessage("ses_a", handmsg.Message{
		ID:      2,
		Role:    handmsg.RoleTool,
		Name:    "Plan Tool",
		Content: "tool body",
	})
	require.Len(t, rows, 1)
	require.Equal(t, "plan tool", rows[0].ToolName)
}

func TestVectorInputAndSourceHelpers(t *testing.T) {
	now := time.Now().UTC()
	rows := []MessageIndexRow{{
		CreatedAt: now,
		UpdatedAt: now,
		MessageID: 1,
		SessionID: "ses_a",
		Role:      string(handmsg.RoleUser),
		Body:      "first",
	}, {
		CreatedAt: now,
		UpdatedAt: now,
		MessageID: 1,
		SessionID: "ses_a",
		Role:      string(handmsg.RoleUser),
		ToolName:  "process",
		Body:      "second",
	}}

	inputs := VectorInputsFromIndexRows(rows)
	require.Len(t, inputs, 2)
	require.Equal(t, SourceIDForMessage("ses_a", 1)+":row:1", inputs[0].ID)
	require.Equal(t, SourceIDForMessage("ses_a", 1)+":row:2", inputs[1].ID)
	require.Equal(t, "process", inputs[1].ToolName)
	require.Nil(t, VectorInputsFromIndexRows(nil))

	row, ok := MessageIndexRowForVectorRecord(rows, inputs[1].ID)
	require.True(t, ok)
	require.Equal(t, "second", row.Body)
	_, ok = MessageIndexRowForVectorRecord(rows, SourceIDForMessage("ses_a", 1))
	require.False(t, ok)
	_, ok = MessageIndexRowForVectorRecord(rows, SourceIDForMessage("ses_a", 1)+":row:3")
	require.False(t, ok)
	_, ok = MessageIndexRowForVectorRecord(nil, inputs[0].ID)
	require.False(t, ok)

	sessionID, messageID, ok := MessageRefFromSourceID(SourceIDForMessage("ses_a", 2))
	require.True(t, ok)
	require.Equal(t, "ses_a", sessionID)
	require.Equal(t, uint(2), messageID)
	_, _, ok = MessageRefFromSourceID("bad")
	require.False(t, ok)
	_, _, ok = MessageRefFromSourceID(string(retrieval.SourceKindSessionMessage) + ":ses_a:")
	require.False(t, ok)
	_, _, ok = MessageRefFromSourceID(SourceIDForMessage("ses_a", 0))
	require.False(t, ok)

	require.Equal(t, []string{SourceIDForMessage("ses_a", 1)}, SourceIDsFromMessages("ses_a", []handmsg.Message{{ID: 1}}))
	require.Nil(t, SourceIDsFromMessages("ses_a", nil))
}

func TestSearchSharedRankingAndFilters(t *testing.T) {
	require.Equal(t, []string{"one", "two"}, UniqueStrings([]string{" one ", "", "two", "one"}))
	require.Nil(t, UniqueStrings(nil))
	require.Equal(t, "search files", NormalizeMatchValue(" Search   Files "))

	require.False(t, MessageIndexRowMatchesSearchOptions(MessageIndexRow{ToolName: "process"}, SearchMessageOptions{ToolName: "search_files"}))
	require.True(t, MessageIndexRowMatchesSearchOptions(MessageIndexRow{ToolName: "process"}, SearchMessageOptions{ToolName: " process "}))

	require.Equal(t, DefaultHybridRetrievalCandidateLimit, HybridRetrievalCandidateLimit(SearchMessageOptions{}))
	require.Equal(t, 120, HybridRetrievalCandidateLimit(SearchMessageOptions{
		MaxSessions:           12,
		MaxMessagesPerSession: 10,
	}))
	require.Equal(t, MaxHybridRetrievalCandidateLimit, HybridRetrievalCandidateLimit(SearchMessageOptions{
		MaxSessions:           MaxHybridRetrievalCandidateLimit,
		MaxMessagesPerSession: MaxHybridRetrievalCandidateLimit,
	}))

	require.Equal(t, float64(0), FusedCandidateScore(false, 0, false, 0))
	require.Greater(t, FusedCandidateScore(true, 1, true, 2), float64(0))
	require.Equal(t, 9.0, CandidateRankingScore(true, 9, 1))
	require.Equal(t, 1.0, CandidateRankingScore(false, 9, 1))

	now := time.Now().UTC()
	older := now.Add(-time.Minute)
	require.Equal(t, -1, CompareCandidateOrder(2, 1, now, now, "a", "a", 1, 1))
	require.Equal(t, 1, CompareCandidateOrder(1, 2, now, now, "a", "a", 1, 1))
	require.Equal(t, -1, CompareCandidateOrder(1, 1, now, older, "a", "a", 1, 1))
	require.Equal(t, 1, CompareCandidateOrder(1, 1, older, now, "a", "a", 1, 1))
	require.Equal(t, -1, CompareCandidateOrder(1, 1, now, now, "a", "b", 1, 1))
	require.Equal(t, 1, CompareCandidateOrder(1, 1, now, now, "b", "a", 1, 1))
	require.Equal(t, -1, CompareCandidateOrder(1, 1, now, now, "a", "a", 2, 1))
	require.Equal(t, 1, CompareCandidateOrder(1, 1, now, now, "a", "a", 1, 2))
	require.Equal(t, 0, CompareCandidateOrder(1, 1, now, now, "a", "a", 1, 1))
}
