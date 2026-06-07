package telegram

import "strings"

const (
	MessageTextLimit = 4096
	DraftCursor      = "..."
)

func ChunkText(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if limit <= 0 {
		limit = MessageTextLimit
	}

	runes := []rune(text)
	chunks := make([]string, 0, len(runes)/limit+1)
	for len(runes) > 0 {
		n := min(len(runes), limit)
		chunks = append(chunks, string(runes[:n]))
		runes = runes[n:]
	}

	return chunks
}

func SupportsNativeDraft(target Target) bool {
	return target.ChatType == "private" && strings.TrimSpace(target.ThreadID) == ""
}

func WithCursor(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return DraftCursor
	}

	return text + "\n" + DraftCursor
}
