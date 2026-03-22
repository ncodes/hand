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
)

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
	if strings.TrimSpace(req.Input) == "" {
		return nil, errors.New("input is required")
	}

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
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, 2)
	instructions := strings.TrimSpace(req.Instructions)
	if instructions != "" {
		messages = append(messages, openai.DeveloperMessage(instructions))
	}

	messages = append(messages, openai.UserMessage(strings.TrimSpace(req.Input)))

	return messages
}

func logRequestDebugDump(params openai.ChatCompletionNewParams) {
	raw, err := json.Marshal(params)
	if err != nil {
		log.Debug().Err(err).Msg("failed to marshal model request debug dump")
		return
	}

	log.Debug().
		Str("provider", "openai-compatible").
		RawJSON("request", raw).
		Msg("model request debug dump")
}
