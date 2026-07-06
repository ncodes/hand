package telegram

import "github.com/wandxy/morph/pkg/str"

const (
	MessageTextLimit = 4096
	DraftCursor      = "..."
)

func ChunkText(text string, limit int) []string {
	stringValue1 := str.String(text)
	text = stringValue1.Trim()
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
	stringValue2 := str.String(target.ThreadID)
	return target.ChatType == "private" && stringValue2.Trim() == ""
}

func WithCursor(text string) string {
	stringValue3 := str.String(text)
	text = stringValue3.Trim()
	if text == "" {
		return DraftCursor
	}

	return text + "\n" + DraftCursor
}
