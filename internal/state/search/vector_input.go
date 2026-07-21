package search

import (
	"errors"
	"fmt"
	"sort"
	"strings"
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

type VectorInputDiagnostics struct {
	SourceIDs            []string
	ToolNames            []string
	ChunkCount           int
	TruncatedSourceCount int
}

func GetVectorInputDiagnostics(
	rows []MessageIndexRow,
	inputs []VectorInput,
	options VectorChunkOptions,
) VectorInputDiagnostics {
	options = NormalizeVectorChunkOptions(options)
	sourceSet := make(map[string]struct{})
	toolSet := make(map[string]struct{})
	truncatedSet := make(map[string]struct{})
	for _, input := range inputs {
		sourceSet[input.SourceID] = struct{}{}
		if input.ToolName != "" {
			toolSet[input.ToolName] = struct{}{}
		}
	}
	for _, row := range rows {
		if len(strings.TrimSpace(row.SemanticBody)) > options.MaxDocumentBytes {
			truncatedSet[SourceIDForMessage(row.SessionID, row.MessageID)] = struct{}{}
		}
	}

	return VectorInputDiagnostics{
		SourceIDs:            getSortedKeys(sourceSet),
		ToolNames:            getSortedKeys(toolSet),
		ChunkCount:           len(inputs),
		TruncatedSourceCount: len(truncatedSet),
	}
}

func getSortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func CheckVectorInputSizes(inputs []VectorInput, maxBytes int) error {
	if maxBytes <= 0 {
		return errors.New("vector max input bytes must be greater than zero")
	}
	for _, input := range inputs {
		if len(input.Text) > maxBytes {
			return fmt.Errorf("vector input %q exceeds the configured byte limit", input.ID)
		}
	}
	return nil
}

func GetMaxVectorInputBytes(inputs []VectorInput) int {
	maxBytes := 0
	for _, input := range inputs {
		maxBytes = max(maxBytes, len(input.Text))
	}
	return maxBytes
}

// VectorInputsFromIndexRows converts searchable rows into stable embedding inputs.
func VectorInputsFromIndexRows(rows []MessageIndexRow, options VectorChunkOptions) []VectorInput {
	if len(rows) == 0 {
		return nil
	}

	options = NormalizeVectorChunkOptions(options)
	countsByMessageID := make(map[uint]int, len(rows))
	inputs := make([]VectorInput, 0, len(rows))
	for _, row := range rows {
		countsByMessageID[row.MessageID]++
		if row.SemanticBody == "" {
			continue
		}
		sourceID := SourceIDForMessage(row.SessionID, row.MessageID)
		chunks, _ := ChunkVectorText(row.SemanticBody, options)
		for chunkIndex, chunk := range chunks {
			inputs = append(inputs, VectorInput{
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
				ID: fmt.Sprintf(
					"%s:row:%d:chunk:%d",
					sourceID,
					countsByMessageID[row.MessageID],
					chunkIndex+1,
				),
				SourceID:  sourceID,
				SessionID: row.SessionID,
				Role:      row.Role,
				ToolName:  row.ToolName,
				Text:      chunk,
			})
		}
	}

	return inputs
}
