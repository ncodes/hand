package models

import (
	"context"

	handmsg "github.com/wandxy/hand/internal/messages"
)

type Client interface {
	Chat(ctx context.Context, req Request) (*Response, error)
}

const (
	APIModeChatCompletions = "chat-completions"
	APIModeResponses       = "responses"
)

type Request struct {
	Model           string
	APIMode         string
	Instructions    string
	Messages        []handmsg.Message
	Tools           []ToolDefinition
	MaxOutputTokens int64
	Temperature     float64
	DebugRequests   bool
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
