package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/rs/zerolog/log"

	handctx "github.com/wandxy/hand/internal/context"
)

var jsonMarshal = json.Marshal

type OpenAIClient struct {
	createChatCompletion func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
}

var newOpenAICompletionCaller = func(opts ...option.RequestOption) func(
	context.Context,
	openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	client := openai.NewClient(opts...)
	return func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		return client.Chat.Completions.New(ctx, params)
	}
}

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
	messages, err := normalizeMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	req.Messages = messages

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(strings.TrimSpace(req.Model)),
		Messages: buildMessages(req),
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

	return &GenerateResponse{
		ID:         resp.ID,
		Model:      resp.Model,
		OutputText: resp.Choices[0].Message.Content,
	}, nil
}

func buildMessages(req GenerateRequest) []openai.ChatCompletionMessageParamUnion {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	instructions := strings.TrimSpace(req.Instructions)
	if instructions != "" {
		messages = append(messages, openai.DeveloperMessage(instructions))
	}

	for _, message := range req.Messages {
		switch message.Role {
		case handctx.RoleDeveloper:
			messages = append(messages, openai.DeveloperMessage(strings.TrimSpace(message.Content)))
		case handctx.RoleUser:
			messages = append(messages, openai.UserMessage(strings.TrimSpace(message.Content)))
		case handctx.RoleAssistant:
			messages = append(messages, openai.AssistantMessage(strings.TrimSpace(message.Content)))
		case handctx.RoleTool:
			messages = append(messages, openai.ToolMessage(strings.TrimSpace(message.Content), "tool"))
		}
	}

	return messages
}

func normalizeMessages(messages []handctx.Message) ([]handctx.Message, error) {
	normalized := make([]handctx.Message, 0, len(messages))
	for _, message := range messages {
		role := handctx.Role(strings.TrimSpace(strings.ToLower(string(message.Role))))
		content := strings.TrimSpace(message.Content)

		if content == "" {
			return nil, errors.New("message content is required")
		}

		switch role {
		case handctx.RoleDeveloper, handctx.RoleUser, handctx.RoleAssistant, handctx.RoleTool:
			normalized = append(normalized, handctx.Message{
				Role:      role,
				Content:   content,
				CreatedAt: message.CreatedAt,
			})
		default:
			return nil, errors.New("message role must be one of developer, user, assistant, or tool")
		}
	}

	return normalized, nil
}

func logRequestDebugDump(params openai.ChatCompletionNewParams) {
	raw, err := jsonMarshal(params)
	if err != nil {
		log.Debug().Err(err).Msg("failed to marshal model request debug dump")
		return
	}

	log.Debug().
		Str("provider", "openai-compatible").
		RawJSON("request", raw).
		Msg("model request debug dump")
}
