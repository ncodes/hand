package provider_anthropic

import (
	"errors"
	"strings"

	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

const defaultMaxOutputTokens int64 = 4096

type normalizedGenerateRequest struct {
	Model            string
	Instructions     string
	Messages         []handmsg.Message
	Tools            []ToolDefinition
	StructuredOutput *StructuredOutput
	MaxOutputTokens  int64
	Temperature      float64
	DebugRequests    bool
}

func normalizeGenerateRequest(req Request) (normalizedGenerateRequest, error) {
	model := strings.TrimSpace(req.Model)
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
		Instructions:     strings.TrimSpace(req.Instructions),
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
		Name:        strings.TrimSpace(value.Name),
		Description: strings.TrimSpace(value.Description),
		Schema:      value.Schema,
		Strict:      value.Strict,
	}
}

func normalizeMessages(messages []handmsg.Message) ([]handmsg.Message, error) {
	normalized := make([]handmsg.Message, 0, len(messages))
	for _, message := range messages {
		role := handmsg.Role(strings.TrimSpace(strings.ToLower(string(message.Role))))
		content := strings.TrimSpace(message.Content)
		toolCallID := strings.TrimSpace(message.ToolCallID)
		toolCalls, err := normalizeToolCalls(message.ToolCalls)
		if err != nil {
			return nil, err
		}

		switch role {
		case handmsg.RoleDeveloper, handmsg.RoleUser, handmsg.RoleAssistant, handmsg.RoleTool:
		default:
			return nil, errors.New("message role must be one of developer, user, assistant, or tool")
		}

		if content == "" && !(role == handmsg.RoleAssistant && len(toolCalls) > 0) {
			return nil, errors.New("message content is required")
		}
		if role == handmsg.RoleTool && toolCallID == "" {
			return nil, errors.New("tool call id is required")
		}

		normalized = append(normalized, handmsg.Message{
			Role:       role,
			Content:    content,
			Name:       strings.TrimSpace(message.Name),
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
		name := strings.TrimSpace(definition.Name)
		if name == "" {
			return nil, errors.New("tool name is required")
		}
		normalized = append(normalized, ToolDefinition{
			Name:        name,
			Description: strings.TrimSpace(definition.Description),
			InputSchema: definition.InputSchema,
		})
	}

	return normalized, nil
}

func normalizeToolCalls(toolCalls []handmsg.ToolCall) ([]handmsg.ToolCall, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	normalized := make([]handmsg.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		id := strings.TrimSpace(toolCall.ID)
		name := strings.TrimSpace(toolCall.Name)
		if id == "" {
			return nil, errors.New("tool call id is required")
		}
		if name == "" {
			return nil, errors.New("tool call name is required")
		}
		normalized = append(normalized, handmsg.ToolCall{
			ID:    id,
			Name:  name,
			Input: strings.TrimSpace(toolCall.Input),
		})
	}

	return normalized, nil
}
