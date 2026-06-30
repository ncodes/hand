package provider_anthropic

import (
	"errors"

	morphmsg "github.com/wandxy/morph/pkg/agent/message"
	"github.com/wandxy/morph/pkg/stringx"
)

const defaultMaxOutputTokens int64 = 4096

type normalizedGenerateRequest struct {
	Model            string
	Instructions     string
	Messages         []morphmsg.Message
	Tools            []ToolDefinition
	StructuredOutput *StructuredOutput
	MaxOutputTokens  int64
	Temperature      float64
	DebugRequests    bool
	SubscriptionAuth bool
}

func normalizeGenerateRequest(req Request) (normalizedGenerateRequest, error) {
	model := stringx.String(req.Model).Trim()
	if model == "" {
		return normalizedGenerateRequest{}, errors.New("model is required")
	}

	if len(req.Messages) == 0 {
		return normalizedGenerateRequest{}, errors.New("messages are required")
	}

	messages, err := normalizeMessages(req.Messages)
	if err != nil {
		return normalizedGenerateRequest{}, err
	}
	tools, err := normalizeToolDefinitions(req.Tools)
	if err != nil {
		return normalizedGenerateRequest{}, err
	}

	maxOutputTokens := req.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = defaultMaxOutputTokens
	}

	return normalizedGenerateRequest{
		Model:            model,
		Instructions:     stringx.String(req.Instructions).Trim(),
		Messages:         messages,
		Tools:            tools,
		StructuredOutput: normalizeStructuredOutput(req.StructuredOutput),
		MaxOutputTokens:  maxOutputTokens,
		Temperature:      req.Temperature,
		DebugRequests:    req.DebugRequests,
	}, nil
}

func normalizeStructuredOutput(value *StructuredOutput) *StructuredOutput {
	if value == nil || len(value.Schema) == 0 {
		return nil
	}

	return &StructuredOutput{
		Name:        stringx.String(value.Name).Trim(),
		Description: stringx.String(value.Description).Trim(),
		Schema:      value.Schema,
		Strict:      value.Strict,
	}
}

func normalizeMessages(messages []morphmsg.Message) ([]morphmsg.Message, error) {
	normalized := make([]morphmsg.Message, 0, len(messages))
	for _, message := range messages {
		role := morphmsg.Role(stringx.String(string(message.Role)).Normalized())
		content := stringx.String(message.Content).Trim()
		toolCallID := stringx.String(message.ToolCallID).Trim()
		toolCalls, err := normalizeToolCalls(message.ToolCalls)
		if err != nil {
			return nil, err
		}

		switch role {
		case morphmsg.RoleDeveloper, morphmsg.RoleUser, morphmsg.RoleAssistant, morphmsg.RoleTool:
		default:
			return nil, errors.New("message role must be one of developer, user, assistant, or tool")
		}

		if content == "" && !(role == morphmsg.RoleAssistant && len(toolCalls) > 0) {
			return nil, errors.New("message content is required")
		}
		if role == morphmsg.RoleTool && toolCallID == "" {
			return nil, errors.New("tool call id is required")
		}

		normalized = append(normalized, morphmsg.Message{
			Role:       role,
			Content:    content,
			Name:       stringx.String(message.Name).Trim(),
			ToolCallID: toolCallID,
			ToolCalls:  toolCalls,
			CreatedAt:  message.CreatedAt,
		})
	}

	return normalized, nil
}

func normalizeToolDefinitions(definitions []ToolDefinition) ([]ToolDefinition, error) {
	if len(definitions) == 0 {
		return nil, nil
	}

	normalized := make([]ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		name := stringx.String(definition.Name).Trim()
		if name == "" {
			return nil, errors.New("tool name is required")
		}
		normalized = append(normalized, ToolDefinition{
			Name:         name,
			Description:  stringx.String(definition.Description).Trim(),
			InputSchema:  definition.InputSchema,
			ParallelSafe: definition.ParallelSafe,
		})
	}

	return normalized, nil
}

func normalizeToolCalls(toolCalls []morphmsg.ToolCall) ([]morphmsg.ToolCall, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	normalized := make([]morphmsg.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		id := stringx.String(toolCall.ID).Trim()
		name := stringx.String(toolCall.Name).Trim()
		if id == "" {
			return nil, errors.New("tool call id is required")
		}
		if name == "" {
			return nil, errors.New("tool call name is required")
		}
		normalized = append(normalized, morphmsg.ToolCall{
			ID:    id,
			Name:  name,
			Input: stringx.String(toolCall.Input).Trim(),
		})
	}

	return normalized, nil
}
