package models

import (
	"context"

	handmsg "github.com/wandxy/hand/internal/messages"
)

type Client interface {
	Complete(ctx context.Context, req Request) (*Response, error)
	CompleteStream(ctx context.Context, req Request, onTextDelta func(StreamDelta)) (*Response, error)
}

type StreamChannel string

const (
	StreamChannelAssistant StreamChannel = "assistant"
	StreamChannelReasoning StreamChannel = "reasoning"
)

type StreamDelta struct {
	Channel StreamChannel
	Text    string
}

const (
	// APIModeCompletions selects the chat completions API path (OpenAI-compatible /v1/chat/completions).
	APIModeCompletions = "completions"
	APIModeResponses   = "responses"
)

type Request struct {
	Model            string
	APIMode          string
	Instructions     string
	Messages         []handmsg.Message
	Tools            []ToolDefinition
	StructuredOutput *StructuredOutput
	MaxOutputTokens  int64
	Temperature      float64
	DebugRequests    bool
}

type StructuredOutput struct {
	Name        string
	Description string
	Schema      map[string]any
	Strict      bool
}

type Response struct {
	ID                string
	Model             string
	OutputText        string
	ToolCalls         []ToolCall
	RequiresToolCalls bool
	PromptTokens      int
	CompletionTokens  int
	TotalTokens       int
}

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type ToolCall struct {
	ID    string
	Name  string
	Input string
}
