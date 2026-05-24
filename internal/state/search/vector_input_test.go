package search

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

func TestVectorInputsFromIndexRows(t *testing.T) {
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
}
