package search

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wandxy/morph/internal/state/search/vectorstore"
	morphmsg "github.com/wandxy/morph/pkg/agent/message"
)

// StableSessionMessageID returns the stable vector source ID for a session message.
func StableSessionMessageID(sessionID string, messageID uint) string {
	return fmt.Sprintf("%s:%s:%d", SourceKindSessionMessage, strings.TrimSpace(sessionID), messageID)
}

// StableMemoryItemID returns the stable vector source ID for a memory item.
func StableMemoryItemID(memoryID string) string {
	return fmt.Sprintf("%s:%s", SourceKindMemoryItem, strings.TrimSpace(memoryID))
}

// MemoryIDFromSourceID extracts a memory ID from a vector source ID.
func MemoryIDFromSourceID(sourceID string) (string, bool) {
	value, ok := strings.CutPrefix(sourceID, string(SourceKindMemoryItem)+":")
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}

	return value, true
}

// SourceIDForMessage returns the vector source ID for a session message.
func SourceIDForMessage(sessionID string, messageID uint) string {
	return StableSessionMessageID(strings.TrimSpace(sessionID), messageID)
}

// SourceIDsFromMessages returns vector source IDs for session messages.
func SourceIDsFromMessages(sessionID string, messages []morphmsg.Message) []string {
	if len(messages) == 0 {
		return nil
	}

	sourceIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		sourceIDs = append(sourceIDs, SourceIDForMessage(sessionID, message.ID))
	}

	return sourceIDs
}

// MessageRefFromSourceID extracts a session/message reference from a vector source ID.
func MessageRefFromSourceID(sourceID string) (string, uint, bool) {
	value, ok := strings.CutPrefix(sourceID, string(SourceKindSessionMessage)+":")
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

func validateRequiredSourceKind(sourceKind SourceKind, field string) error {
	return vectorstore.ValidateRequiredSourceKind(sourceKind, field)
}

func validateOptionalSourceKind(sourceKind SourceKind, field string) error {
	return vectorstore.ValidateOptionalSourceKind(sourceKind, field)
}
