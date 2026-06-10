package slack

import "strings"

const MarkdownTextLimit = 12000

type PostMessageRequest struct {
	Channel  string `json:"channel"`
	Text     string `json:"text"`
	ThreadTS string `json:"thread_ts,omitempty"`
}

type StartStreamRequest struct {
	Channel         string `json:"channel"`
	ThreadTS        string `json:"thread_ts"`
	MarkdownText    string `json:"markdown_text,omitempty"`
	RecipientUserID string `json:"recipient_user_id,omitempty"`
	RecipientTeamID string `json:"recipient_team_id,omitempty"`
}

type AppendStreamRequest struct {
	Channel      string `json:"channel"`
	TS           string `json:"ts"`
	MarkdownText string `json:"markdown_text"`
}

type StopStreamRequest struct {
	Channel      string `json:"channel"`
	TS           string `json:"ts"`
	MarkdownText string `json:"markdown_text,omitempty"`
}

type Stream struct {
	ChannelID string
	TS        string
}

func ChunkMarkdown(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if limit <= 0 {
		limit = MarkdownTextLimit
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
