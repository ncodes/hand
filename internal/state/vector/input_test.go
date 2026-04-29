package vector

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/state/indexing"
	"github.com/wandxy/hand/internal/state/retrieval"
)

func TestVectorInputAndSourceHelpers(t *testing.T) {
	now := time.Now().UTC()
	rows := []indexing.MessageIndexRow{{
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
