package models

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"
)

func TestNewOpenAIClient_IncludesAPIKeyOptionWhenProvided(t *testing.T) {
	originalFactory := newOpenAICompletionCaller
	t.Cleanup(func() {
		newOpenAICompletionCaller = originalFactory
	})

	called := 0
	optCount := 0
	newOpenAICompletionCaller = func(opts ...option.RequestOption) func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		called++
		optCount = len(opts)
		return func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, nil
		}
	}

	client, err := NewOpenAIClient(" test-key ", option.WithBaseURL("https://example.com/v1"))
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, 1, called)
	require.Equal(t, 2, optCount)
}

func TestNewOpenAIClient_OmitsAPIKeyOptionWhenEmpty(t *testing.T) {
	originalFactory := newOpenAICompletionCaller
	t.Cleanup(func() {
		newOpenAICompletionCaller = originalFactory
	})

	optCount := 0
	newOpenAICompletionCaller = func(opts ...option.RequestOption) func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		optCount = len(opts)
		return func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, nil
		}
	}

	client, err := NewOpenAIClient("   ", option.WithBaseURL("https://example.com/v1"))
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, 1, optCount)
}

func TestOpenAIClient_ChatRequiresClient(t *testing.T) {
	var nilClient *OpenAIClient
	_, err := nilClient.Chat(context.Background(), GenerateRequest{})
	require.EqualError(t, err, "model client is required")

	client := &OpenAIClient{}
	_, err = client.Chat(context.Background(), GenerateRequest{})
	require.EqualError(t, err, "model client is required")
}

func TestOpenAIClient_ChatRequiresModel(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			t.Fatal("completion call should not happen")
			return nil, nil
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{Input: "hello"})
	require.EqualError(t, err, "model is required")
}

func TestOpenAIClient_ChatRequiresInput(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			t.Fatal("completion call should not happen")
			return nil, nil
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{Model: "test-model"})
	require.EqualError(t, err, "input is required")
}

func TestOpenAIClient_ChatReturnsAPIError(t *testing.T) {
	expectedErr := errors.New("upstream failed")
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, expectedErr
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{
		Model: "test-model",
		Input: "hello",
	})
	require.ErrorIs(t, err, expectedErr)
}

func TestOpenAIClient_ChatReturnsErrorWhenNoChoices(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				ID:      "resp_123",
				Model:   "test-model",
				Choices: []openai.ChatCompletionChoice{},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{
		Model: "test-model",
		Input: "hello",
	})
	require.EqualError(t, err, `chat completion response "resp_123" contained no choices`)
}

func TestOpenAIClient_ChatReturnsResponseAndBuildsRequest(t *testing.T) {
	var captured openai.ChatCompletionNewParams
	client := &OpenAIClient{
		createChatCompletion: func(_ context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			captured = params
			return &openai.ChatCompletion{
				ID:    "resp_123",
				Model: "returned-model",
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{Content: "hello back"},
					},
				},
			}, nil
		},
	}

	resp, err := client.Chat(context.Background(), GenerateRequest{
		Model:           "test-model",
		Input:           "  hello  ",
		Instructions:    "  be concise  ",
		MaxOutputTokens: 123,
		Temperature:     0.7,
	})
	require.NoError(t, err)
	require.Equal(t, &GenerateResponse{
		ID:         "resp_123",
		Model:      "returned-model",
		OutputText: "hello back",
	}, resp)

	raw, err := json.Marshal(captured)
	require.NoError(t, err)
	rawText := string(raw)
	require.Contains(t, rawText, `"model":"test-model"`)
	require.Contains(t, rawText, `"max_completion_tokens":123`)
	require.Contains(t, rawText, `"temperature":0.7`)
	require.Contains(t, rawText, `"role":"developer"`)
	require.Contains(t, rawText, `"content":"be concise"`)
	require.Contains(t, rawText, `"role":"user"`)
	require.Contains(t, rawText, `"content":"hello"`)
}

func TestBuildMessages_WithoutInstructions(t *testing.T) {
	messages := buildMessages(GenerateRequest{Input: "  hello  "})
	require.Len(t, messages, 1)

	raw, err := json.Marshal(messages)
	require.NoError(t, err)
	rawText := string(raw)
	require.NotContains(t, rawText, `"role":"developer"`)
	require.Contains(t, rawText, `"role":"user"`)
	require.Contains(t, rawText, `"content":"hello"`)
}

func TestBuildMessages_WithInstructions(t *testing.T) {
	messages := buildMessages(GenerateRequest{
		Input:        "hello",
		Instructions: "  be concise  ",
	})
	require.Len(t, messages, 2)

	raw, err := json.Marshal(messages)
	require.NoError(t, err)
	rawText := string(raw)
	require.True(t, strings.Index(rawText, `"role":"developer"`) < strings.Index(rawText, `"role":"user"`))
	require.Contains(t, rawText, `"content":"be concise"`)
	require.Contains(t, rawText, `"content":"hello"`)
}
