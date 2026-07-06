package provider_anthropic

import (
	"errors"

	morphmsg "github.com/wandxy/morph/pkg/agent/message"
	"github.com/wandxy/morph/pkg/str"
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
	stringValue1 := str.String(req.Model)
	model := stringValue1.Trim()
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
	stringValue2 := str.String(req.Instructions)
	return normalizedGenerateRequest{
		Model:            model,
		Instructions:     stringValue2.Trim(),
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
	stringValue3 := str.String(value.Name)
	stringValue4 := str.String(value.Description)
	return &StructuredOutput{
		Name:        stringValue3.Trim(),
		Description: stringValue4.Trim(),
		Schema:      value.Schema,
		Strict:      value.Strict,
	}
}

func normalizeMessages(messages []morphmsg.Message) ([]morphmsg.Message, error) {
	normalized := make([]morphmsg.Message, 0, len(messages))
	for _, message := range messages {
		stringValue5 := str.String(string(message.Role))
		role := morphmsg.Role(stringValue5.Normalized())
		stringValue6 := str.String(message.Content)
		content := stringValue6.Trim()
		stringValue7 := str.String(message.ToolCallID)
		toolCallID := stringValue7.Trim()
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
		stringValue8 := str.String(message.Name)
		normalized = append(normalized, morphmsg.Message{
			Role:       role,
			Content:    content,
			Name:       stringValue8.Trim(),
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
		stringValue9 := str.String(definition.Name)
		name := stringValue9.Trim()
		if name == "" {
			return nil, errors.New("tool name is required")
		}
		stringValue10 := str.String(definition.Description)
		normalized = append(normalized, ToolDefinition{
			Name:         name,
			Description:  stringValue10.Trim(),
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
		stringValue11 := str.String(toolCall.ID)
		id := stringValue11.Trim()
		stringValue12 := str.String(toolCall.Name)
		name := stringValue12.Trim()
		if id == "" {
			return nil, errors.New("tool call id is required")
		}
		if name == "" {
			return nil, errors.New("tool call name is required")
		}
		stringValue13 := str.String(toolCall.Input)
		normalized = append(normalized, morphmsg.ToolCall{
			ID:    id,
			Name:  name,
			Input: stringValue13.Trim(),
		})
	}

	return normalized, nil
}
