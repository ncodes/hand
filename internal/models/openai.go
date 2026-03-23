package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog/log"

	handctx "github.com/wandxy/hand/internal/context"
)

var jsonMarshal = json.Marshal

type OpenAIClient struct {
	createChatCompletion func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
}

var newOpenAICompletionCaller = func(opts ...option.RequestOption) func(
	context.Context,
	openai.ChatCompletionNewParams,
) (*openai.ChatCompletion, error) {
	client := openai.NewClient(opts...)
	return func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		return client.Chat.Completions.New(ctx, params)
	}
}

// NewOpenAIClient creates a new OpenAI client.
func NewOpenAIClient(apiKey string, opts ...option.RequestOption) (*OpenAIClient, error) {
	clientOptions := make([]option.RequestOption, 0, len(opts)+1)
	if trimmed := strings.TrimSpace(apiKey); trimmed != "" {
		clientOptions = append(clientOptions, option.WithAPIKey(trimmed))
	}
	clientOptions = append(clientOptions, opts...)

	return &OpenAIClient{
		createChatCompletion: newOpenAICompletionCaller(clientOptions...),
	}, nil
}

// Chat sends a chat message to the OpenAI client and returns the response.
func (c *OpenAIClient) Chat(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if c == nil || c.createChatCompletion == nil {
		return nil, errors.New("model client is required")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("model is required")
	}
	if len(req.Messages) == 0 {
		return nil, errors.New("messages are required")
	}

	messages, err := buildMessages(req)
	if err != nil {
		return nil, err
	}

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(strings.TrimSpace(req.Model)),
		Messages: messages,
	}
	if len(req.Tools) > 0 {
		params.Tools = buildTools(req.Tools)
	}
	if req.MaxOutputTokens > 0 {
		params.MaxCompletionTokens = openai.Int(req.MaxOutputTokens)
	}
	if req.Temperature > 0 {
		params.Temperature = openai.Float(req.Temperature)
	}
	if req.DebugRequests {
		logRequestDebugDump(params)
	}

	resp, err := c.createChatCompletion(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("chat completion response %q contained no choices", resp.ID)
	}

	toolCalls, err := extractToolCalls(resp.Choices[0].Message.ToolCalls)
	if err != nil {
		return nil, err
	}

	return &GenerateResponse{
		ID:                resp.ID,
		Model:             resp.Model,
		OutputText:        strings.TrimSpace(resp.Choices[0].Message.Content),
		ToolCalls:         toolCalls,
		RequiresToolCalls: len(toolCalls) > 0,
	}, nil
}

func buildMessages(req GenerateRequest) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	instructions := strings.TrimSpace(req.Instructions)
	if instructions != "" {
		messages = append(messages, openai.DeveloperMessage(instructions))
	}

	for _, message := range req.Messages {
		role := handctx.Role(strings.TrimSpace(strings.ToLower(string(message.Role))))
		content := strings.TrimSpace(message.Content)
		toolCallID := strings.TrimSpace(message.ToolCallID)

		toolCalls, err := normalizeToolCalls(message.ToolCalls)
		if err != nil {
			return nil, err
		}
		if content == "" && !(role == handctx.RoleAssistant && len(toolCalls) > 0) {
			return nil, errors.New("message content is required")
		}
		if role == handctx.RoleTool && toolCallID == "" {
			return nil, errors.New("tool call id is required")
		}

		switch role {
		case handctx.RoleDeveloper:
			messages = append(messages, openai.DeveloperMessage(content))
		case handctx.RoleUser:
			messages = append(messages, openai.UserMessage(content))
		case handctx.RoleAssistant:
			if len(toolCalls) == 0 {
				messages = append(messages, openai.AssistantMessage(content))
				continue
			}

			openAIToolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(toolCalls))
			for _, toolCall := range toolCalls {
				openAIToolCalls = append(openAIToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: toolCall.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      toolCall.Name,
							Arguments: toolCall.Input,
						},
					},
				})
			}

			assistant := openai.ChatCompletionAssistantMessageParam{
				ToolCalls: openAIToolCalls,
			}
			if content != "" {
				assistant.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(content),
				}
			}
			messages = append(messages, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &assistant,
			})
		case handctx.RoleTool:
			messages = append(messages, openai.ToolMessage(content, toolCallID))
		default:
			return nil, errors.New("message role must be one of developer, user, assistant, or tool")
		}
	}

	return messages, nil
}

func normalizeToolCalls(toolCalls []handctx.ToolCall) ([]handctx.ToolCall, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	normalized := make([]handctx.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		if strings.TrimSpace(toolCall.ID) == "" {
			return nil, errors.New("tool call id is required")
		}
		if strings.TrimSpace(toolCall.Name) == "" {
			return nil, errors.New("tool call name is required")
		}
		normalized = append(normalized, handctx.ToolCall{
			ID:    strings.TrimSpace(toolCall.ID),
			Name:  strings.TrimSpace(toolCall.Name),
			Input: strings.TrimSpace(toolCall.Input),
		})
	}

	return normalized, nil
}

func buildTools(definitions []ToolDefinition) []openai.ChatCompletionToolUnionParam {
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        strings.TrimSpace(definition.Name),
			Description: openai.String(strings.TrimSpace(definition.Description)),
			Parameters:  shared.FunctionParameters(definition.InputSchema),
			Strict:      openai.Bool(true),
		}))
	}
	return tools
}

func extractToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) ([]ToolCall, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	normalized := make([]ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		id := strings.TrimSpace(toolCall.ID)
		name := strings.TrimSpace(toolCall.Function.Name)
		if id == "" {
			return nil, errors.New("tool call id is required")
		}
		if name == "" {
			return nil, errors.New("tool call name is required")
		}

		normalized = append(normalized, ToolCall{
			ID:    id,
			Name:  name,
			Input: strings.TrimSpace(toolCall.Function.Arguments),
		})
	}

	return normalized, nil
}

func logRequestDebugDump(params openai.ChatCompletionNewParams) {
	raw, err := jsonMarshal(params)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to marshal model request debug dump")
		return
	}

	log.Debug().
		Str("provider", "openai-compatible").
		RawJSON("request", raw).
		Msg("model request debug dump")
}
