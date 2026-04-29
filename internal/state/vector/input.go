package vector

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/state/indexing"
	"github.com/wandxy/hand/internal/state/retrieval"
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

// VectorStoreOptions configures vector indexing, retrieval, and reranking for a store.
type VectorStoreOptions struct {
	Embedder            retrieval.Embedder
	Reranker            retrieval.Reranker
	VectorStore         retrieval.VectorStore
	EnableRerank        *bool
	EmbeddingModel      string
	RebuildBatchSize    int
	RerankMaxCandidates int
	Diagnostics         bool
	Required            bool
}

// VectorConfig is the normalized vector configuration kept by concrete stores.
type VectorConfig struct {
	Provider    retrieval.Embedder
	Reranker    retrieval.Reranker
	Store       retrieval.VectorStore
	Model       string
	RerankMax   int
	Diagnostics bool
	Rerank      bool
	Required    bool
}

// VectorInputsFromIndexRows converts searchable rows into stable embedding inputs.
func VectorInputsFromIndexRows(rows []indexing.MessageIndexRow) []VectorInput {
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

// SourceIDsFromMessages returns stable vector source IDs for the supplied messages.
func SourceIDsFromMessages(sessionID string, messages []handmsg.Message) []string {
	if len(messages) == 0 {
		return nil
	}

	sourceIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		sourceIDs = append(sourceIDs, SourceIDForMessage(sessionID, message.ID))
	}

	return sourceIDs
}

// SourceIDForMessage returns the stable vector source ID for a session message.
func SourceIDForMessage(sessionID string, messageID uint) string {
	return retrieval.StableSessionMessageID(strings.TrimSpace(sessionID), messageID)
}

// MessageRefFromSourceID parses a session ID and message ID from a vector source ID.
func MessageRefFromSourceID(sourceID string) (string, uint, bool) {
	value, ok := strings.CutPrefix(sourceID, string(retrieval.SourceKindSessionMessage)+":")
	if !ok {
		return "", 0, false
	}
	idx := strings.LastIndex(value, ":")
	if idx <= 0 || idx == len(value)-1 {
		return "", 0, false
	}
	messageID, err := strconv.ParseUint(value[idx+1:], 10, 64)
	if err != nil || messageID == 0 {
		return "", 0, false
	}

	return value[:idx], uint(messageID), true
}
