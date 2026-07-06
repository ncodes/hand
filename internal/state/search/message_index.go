package search

import (
	"strconv"
	"strings"
	"time"

	state "github.com/wandxy/morph/internal/state/core"
	morphmsg "github.com/wandxy/morph/pkg/agent/message"
	"github.com/wandxy/morph/pkg/str"
)

// MessageIndexRow is one searchable text row derived from a session message.
type MessageIndexRow struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	MessageID uint
	SessionID string
	Role      string
	ToolName  string
	Body      string
}

// MessageIndexRowsFromMessage converts a message into the text rows used by lexical and vector search.
func MessageIndexRowsFromMessage(sessionID string, message morphmsg.Message) []MessageIndexRow {
	stringValue1 := str.String(sessionID)
	baseRow := MessageIndexRow{
		CreatedAt: message.CreatedAt,
		UpdatedAt: message.CreatedAt,
		MessageID: message.ID,
		SessionID: stringValue1.Trim(),
		Role:      state.NormalizeMatchValue(string(message.Role)),
	}
	stringValue2 := str.String(message.Content)
	body := stringValue2.Trim()
	switch message.Role {
	case morphmsg.RoleAssistant:
		rows := make([]MessageIndexRow, 0, len(message.ToolCalls)+1)
		if body != "" {
			row := baseRow
			row.Body = body
			rows = append(rows, row)
		}
		for _, toolCall := range message.ToolCalls {
			stringValue3 := str.String(morphmsg.ToolCallSearchText(toolCall))
			toolBody := stringValue3.Trim()
			if toolBody == "" {
				continue
			}
			row := baseRow
			row.ToolName = state.NormalizeMatchValue(toolCall.Name)
			row.Body = toolBody
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			return nil
		}
		return rows
	case morphmsg.RoleTool:
		if body == "" {
			return nil
		}
		row := baseRow
		row.ToolName = state.NormalizeMatchValue(message.Name)
		row.Body = body
		return []MessageIndexRow{row}
	default:
		if body == "" {
			return nil
		}
		row := baseRow
		row.Body = body
		return []MessageIndexRow{row}
	}
}

// MessageIndexRowForVectorRecord returns the searchable row represented by a vector record ID.
func MessageIndexRowForVectorRecord(rows []MessageIndexRow, vectorID string) (MessageIndexRow, bool) {
	if len(rows) == 0 {
		return MessageIndexRow{}, false
	}

	idx := strings.LastIndex(vectorID, ":row:")
	if idx < 0 {
		return MessageIndexRow{}, false
	}
	rowNumber, err := strconv.Atoi(vectorID[idx+5:])
	if err != nil || rowNumber <= 0 || rowNumber > len(rows) {
		return MessageIndexRow{}, false
	}

	return rows[rowNumber-1], true
}

// MessageIndexRowMatchesSearchOptions reports whether a row satisfies non-text search filters.
func MessageIndexRowMatchesSearchOptions(row MessageIndexRow, opts state.SearchMessageOptions) bool {
	if toolName := state.NormalizeMatchValue(opts.ToolName); toolName != "" && row.ToolName != toolName {
		return false
	}

	return true
}
