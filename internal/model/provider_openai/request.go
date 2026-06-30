package provider_openai

import (
	"errors"
	"slices"

	models "github.com/wandxy/morph/internal/model"
	morphmsg "github.com/wandxy/morph/pkg/agent/message"
	"github.com/wandxy/morph/pkg/stringx"
)

type normalizedGenerateRequest struct {
	Model            string
	API              string
	Instructions     string
	Messages         []morphmsg.Message
	Tools            []ToolDefinition
	StructuredOutput *StructuredOutput
	MaxOutputTokens  int64
	Temperature      float64
	DebugRequests    bool
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

	api, err := normalizeRequestAPI(req.API)
	if err != nil {
		return normalizedGenerateRequest{}, err
	}

	return normalizedGenerateRequest{
		Model:            model,
		API:              api,
		Instructions:     stringx.String(req.Instructions).Trim(),
		Messages:         messages,
		Tools:            tools,
		StructuredOutput: normalizeStructuredOutput(req.StructuredOutput),
		MaxOutputTokens:  req.MaxOutputTokens,
		Temperature:      req.Temperature,
		DebugRequests:    req.DebugRequests,
	}, nil
}

func normalizeRequestAPI(api string) (string, error) {
	api = stringx.String(api).Normalized()
	switch api {
	case models.APIOpenAICompletions, models.APIOpenAIResponses:
		return api, nil
	case "":
		return "", errors.New("model API is required")
	default:
		return "", errors.New("model API must be one of: openai-completions, openai-responses")
	}
}

func normalizeStructuredOutput(value *StructuredOutput) *StructuredOutput {
	if value == nil {
		return nil
	}

	name := stringx.String(value.Name).Trim()
	if name == "" || len(value.Schema) == 0 {
		return nil
	}

	return &StructuredOutput{
		Name:        name,
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
		case morphmsg.RoleDeveloper:
			return nil, errors.New("developer messages must be provided via instructions")
		case morphmsg.RoleUser, morphmsg.RoleAssistant, morphmsg.RoleTool:
		default:
			return nil, errors.New("message role must be one of user, assistant, or tool; developer messages must be provided via instructions")
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

func normalizeStrictJSONSchema(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return nil
	}

	return normalizeStrictJSONSchemaValue(schema).(map[string]any)
}

func normalizeStrictJSONSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, item := range typed {
			cloned[key] = normalizeStrictJSONSchemaValue(item)
		}

		schemaType, _ := cloned["type"].(string)
		properties, _ := cloned["properties"].(map[string]any)
		if schemaType == "object" && len(properties) > 0 {
			for key, property := range properties {
				if propertySchema, ok := property.(map[string]any); ok && isUnsupportedStrictJSONObjectProperty(propertySchema) {
					delete(properties, key)
				}
			}

			required := make([]string, 0, len(properties))
			for key := range properties {
				required = append(required, key)
			}
			slices.Sort(required)
			cloned["required"] = required
		}

		return cloned
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, normalizeStrictJSONSchemaValue(item))
		}
		return cloned
	default:
		return value
	}
}

func isUnsupportedStrictJSONObjectProperty(schema map[string]any) bool {
	if len(schema) == 0 {
		return false
	}

	schemaType, _ := schema["type"].(string)
	if schemaType != "object" {
		return false
	}

	properties, _ := schema["properties"].(map[string]any)
	if len(properties) > 0 {
		return false
	}

	additionalProperties, ok := schema["additionalProperties"]
	if !ok {
		return false
	}

	switch typed := additionalProperties.(type) {
	case bool:
		return typed
	case map[string]any:
		return true
	default:
		return true
	}
}
