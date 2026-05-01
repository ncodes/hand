package search

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
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

func TestMessageIndexRowForVectorRecord(t *testing.T) {
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

	sourceID := string(SourceKindSessionMessage) + ":ses_a:1"
	row, ok := MessageIndexRowForVectorRecord(rows, sourceID+":row:2")
	require.True(t, ok)
	require.Equal(t, "second", row.Body)

	_, ok = MessageIndexRowForVectorRecord(rows, sourceID)
	require.False(t, ok)
	_, ok = MessageIndexRowForVectorRecord(rows, sourceID+":row:3")
	require.False(t, ok)
	_, ok = MessageIndexRowForVectorRecord(nil, sourceID+":row:1")
	require.False(t, ok)
}
