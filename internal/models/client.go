package models

import (
	"context"

	handctx "github.com/wandxy/hand/internal/context"
)

type Client interface {
	Chat(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
}

type GenerateRequest struct {
	Model           string
	Instructions    string
	Messages        []handctx.Message
	Tools           []ToolDefinition
	MaxOutputTokens int64
	Temperature     float64
	DebugRequests   bool
}

type GenerateResponse struct {
	ID         string
	Model      string
	OutputText string
	ToolCalls  []ToolCall
	RequiresToolCalls bool
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
