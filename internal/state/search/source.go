package search

import (
	"fmt"
	"strconv"
	"strings"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/state/search/vectorstore"
)

func StableSessionMessageID(sessionID string, messageID uint) string {
	return fmt.Sprintf("%s:%s:%d", SourceKindSessionMessage, strings.TrimSpace(sessionID), messageID)
}

func StableMemoryItemID(memoryID string) string {
	return fmt.Sprintf("%s:%s", SourceKindMemoryItem, strings.TrimSpace(memoryID))
}

func SourceIDForMessage(sessionID string, messageID uint) string {
	return StableSessionMessageID(strings.TrimSpace(sessionID), messageID)
}

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
