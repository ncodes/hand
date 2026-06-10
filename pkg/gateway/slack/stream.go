package slack

import "strings"

const MarkdownTextLimit = 12000

type PostMessageRequest struct {
	Channel  string `json:"channel"`
	Text     string `json:"text"`
	ThreadTS string `json:"thread_ts,omitempty"`
}

type StartStreamRequest struct {
	Channel         string  `json:"channel"`
	ThreadTS        string  `json:"thread_ts"`
	Chunks          []Chunk `json:"chunks,omitempty"`
	RecipientUserID string  `json:"recipient_user_id,omitempty"`
	RecipientTeamID string  `json:"recipient_team_id,omitempty"`
}

type AppendStreamRequest struct {
	Channel string  `json:"channel"`
	TS      string  `json:"ts"`
	Chunks  []Chunk `json:"chunks,omitempty"`
}

type StopStreamRequest struct {
	Channel string  `json:"channel"`
	TS      string  `json:"ts"`
	Chunks  []Chunk `json:"chunks,omitempty"`
}

type Stream struct {
	ChannelID string
	TS        string
}

type Chunk struct {
	Type   string  `json:"type"`
	Text   string  `json:"text,omitempty"`
	Blocks []Block `json:"blocks,omitempty"`
}

type Block struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	Elements []RichTextElement `json:"elements,omitempty"`
}

type RichTextElement struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	Elements []RichTextElement `json:"elements,omitempty"`
}

func MarkdownTextChunk(text string) Chunk {
	return Chunk{Type: "markdown_text", Text: text}
}

func FencedCodeChunk(text string) Chunk {
	return MarkdownTextChunk("```\n" + ensureTrailingNewline(text) + "```")
}

func FencedCodeChunks(text string) []Chunk {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	chunks := []Chunk{MarkdownTextChunk("```\n")}
	for _, line := range strings.SplitAfter(ensureTrailingNewline(text), "\n") {
		if line != "" {
			chunks = append(chunks, MarkdownTextChunk(line))
		}
	}
	chunks = append(chunks, MarkdownTextChunk("```"))

	return chunks
}

func ensureTrailingNewline(text string) string {
	if strings.HasSuffix(text, "\n") {
		return text
	}

	return text + "\n"
}

func MarkdownBlockChunk(text string) Chunk {
	return Chunk{
		Type: "blocks",
		Blocks: []Block{
			{Type: "markdown", Text: text},
		},
	}
}

func PreformattedBlockChunk(text string) Chunk {
	return Chunk{
		Type: "blocks",
		Blocks: []Block{
			{
				Type: "rich_text",
				Elements: []RichTextElement{
					{
						Type: "rich_text_preformatted",
						Elements: []RichTextElement{
							{Type: "text", Text: text},
						},
					},
				},
			},
		},
	}
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
