package search

import (
	"fmt"
	"time"
)

// VectorInput is the normalized embedding input for one searchable message row.
type VectorInput struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	ID        string
	SourceID  string
	SessionID string
	Role      string
	ToolName  string
	Text      string
}

// VectorInputsFromIndexRows converts searchable rows into stable embedding inputs.
func VectorInputsFromIndexRows(rows []MessageIndexRow) []VectorInput {
	if len(rows) == 0 {
		return nil
	}

	countsByMessageID := make(map[uint]int, len(rows))
	inputs := make([]VectorInput, 0, len(rows))
	for _, row := range rows {
		sourceID := SourceIDForMessage(row.SessionID, row.MessageID)
		countsByMessageID[row.MessageID]++
		inputs = append(inputs, VectorInput{
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
			ID:        fmt.Sprintf("%s:row:%d", sourceID, countsByMessageID[row.MessageID]),
			SourceID:  sourceID,
			SessionID: row.SessionID,
			Role:      row.Role,
			ToolName:  row.ToolName,
			Text:      row.Body,
		})
	}

	return inputs
}
