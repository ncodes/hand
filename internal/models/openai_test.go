package models

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	handctx "github.com/wandxy/hand/internal/context"
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

func TestNewOpenAICompletionCaller_UsesSDKClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"resp_123","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"hello back"},"finish_reason":"stop"}]}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	caller := newOpenAICompletionCaller(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("test-key"),
	)

	resp, err := caller(context.Background(), openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("test-model"),
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")},
	})

	require.NoError(t, err)
	require.Equal(t, "resp_123", resp.ID)
}

func TestOpenAIClient_ChatRequiresClient(t *testing.T) {
	var nilClient *OpenAIClient
	_, err := nilClient.Chat(context.Background(), GenerateRequest{})
	require.EqualError(t, err, "model client is required")

	client := &OpenAIClient{}
	_, err = client.Chat(context.Background(), GenerateRequest{})
	require.EqualError(t, err, "model client is required")
}

func TestOpenAIClient_ChatRejectsInvalidMessageRole(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			t.Fatal("completion call should not happen")
			return nil, nil
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{
		Model: "test-model",
		Messages: []handctx.Message{
			{Role: handctx.Role("invalid"), Content: "hello"},
		},
	})
	require.EqualError(t, err, "message role must be one of developer, user, assistant, or tool")
}

func TestOpenAIClient_ChatRejectsEmptyMessageContent(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			t.Fatal("completion call should not happen")
			return nil, nil
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{
		Model: "test-model",
		Messages: []handctx.Message{
			{Role: handctx.RoleUser, Content: "   "},
		},
	})
	require.EqualError(t, err, "message content is required")
}

func TestOpenAIClient_ChatRequiresModel(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			t.Fatal("completion call should not happen")
			return nil, nil
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{
		Messages: []handctx.Message{
			{Role: handctx.RoleUser, Content: "hello"},
		},
	})
	require.EqualError(t, err, "model is required")
}

func TestOpenAIClient_ChatRequiresMessages(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			t.Fatal("completion call should not happen")
			return nil, nil
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{Model: "test-model"})
	require.EqualError(t, err, "messages are required")
}

func TestOpenAIClient_ChatReturnsAPIError(t *testing.T) {
	expectedErr := errors.New("upstream failed")
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, expectedErr
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{
		Model:    "test-model",
		Messages: []handctx.Message{{Role: handctx.RoleUser, Content: "hello"}},
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
		Model:    "test-model",
		Messages: []handctx.Message{{Role: handctx.RoleUser, Content: "hello"}},
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
		Model:        "test-model",
		Instructions: "  be concise  ",
		Messages: []handctx.Message{
			{Role: handctx.RoleUser, Content: "  hello  "},
			{Role: handctx.RoleAssistant, Content: " previous reply "},
		},
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
	require.Contains(t, rawText, `"role":"assistant"`)
	require.Contains(t, rawText, `"content":"previous reply"`)
}

func TestOpenAIClient_ChatReturnsToolCalls(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(_ context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			raw, err := json.Marshal(params)
			require.NoError(t, err)
			require.Contains(t, string(raw), `"name":"time"`)

			return &openai.ChatCompletion{
				ID:    "resp_123",
				Model: "returned-model",
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							ToolCalls: []openai.ChatCompletionMessageToolCallUnion{
								{
									ID:   "call-1",
									Type: "function",
									Function: openai.ChatCompletionMessageFunctionToolCallFunction{
										Name:      "time",
										Arguments: "{}",
									},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	resp, err := client.Chat(context.Background(), GenerateRequest{
		Model: "test-model",
		Messages: []handctx.Message{
			{Role: handctx.RoleUser, Content: "what time is it?"},
		},
		Tools: []ToolDefinition{{
			Name:        "time",
			Description: "Returns the current time.",
			InputSchema: map[string]any{"type": "object"},
		}},
	})

	require.NoError(t, err)
	require.True(t, resp.RequiresToolCalls)
	require.Equal(t, []ToolCall{{ID: "call-1", Name: "time", Input: "{}"}}, resp.ToolCalls)
}

func TestOpenAIClient_ChatRejectsToolCallWithoutIDInResponse(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(_ context.Context, _ openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				ID:    "resp_123",
				Model: "returned-model",
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							ToolCalls: []openai.ChatCompletionMessageToolCallUnion{
								{
									Function: openai.ChatCompletionMessageFunctionToolCallFunction{
										Name:      "time",
										Arguments: "{}",
									},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{
		Model: "test-model",
		Messages: []handctx.Message{
			{Role: handctx.RoleUser, Content: "what time is it?"},
		},
	})

	require.EqualError(t, err, "tool call id is required")
}

func TestOpenAIClient_ChatRejectsToolCallWithoutNameInResponse(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(_ context.Context, _ openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				ID:    "resp_123",
				Model: "returned-model",
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							ToolCalls: []openai.ChatCompletionMessageToolCallUnion{
								{
									ID: "call-1",
									Function: openai.ChatCompletionMessageFunctionToolCallFunction{
										Arguments: "{}",
									},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), GenerateRequest{
		Model: "test-model",
		Messages: []handctx.Message{
			{Role: handctx.RoleUser, Content: "what time is it?"},
		},
	})

	require.EqualError(t, err, "tool call name is required")
}

func TestOpenAIClient_ChatLogsRequestDebugDumpWhenEnabled(t *testing.T) {
	originalLogger := log.Logger
	originalLevel := zerolog.GlobalLevel()
	t.Cleanup(func() {
		log.Logger = originalLogger
		zerolog.SetGlobalLevel(originalLevel)
	})

	buf := &bytes.Buffer{}
	log.Logger = zerolog.New(buf)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	client := &OpenAIClient{
		createChatCompletion: func(_ context.Context, _ openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
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

	_, err := client.Chat(context.Background(), GenerateRequest{
		Model:        "test-model",
		Instructions: "be concise",
		Messages: []handctx.Message{
			{Role: handctx.RoleUser, Content: "hello"},
		},
		DebugRequests: true,
	})

	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, "model request debug dump")
	require.Contains(t, output, `"provider":"openai-compatible"`)
	require.Contains(t, output, `"model":"test-model"`)
	require.Contains(t, output, `"content":"hello"`)
}

func TestLogRequestDebugDump_LogsMarshalError(t *testing.T) {
	originalLogger := log.Logger
	originalLevel := zerolog.GlobalLevel()
	originalMarshal := jsonMarshal
	t.Cleanup(func() {
		log.Logger = originalLogger
		zerolog.SetGlobalLevel(originalLevel)
		jsonMarshal = originalMarshal
	})

	buf := &bytes.Buffer{}
	log.Logger = zerolog.New(buf)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	jsonMarshal = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}

	logRequestDebugDump(openai.ChatCompletionNewParams{})

	require.Contains(t, buf.String(), "Failed to marshal model request debug dump")
}

func TestBuildMessages_WithoutInstructions(t *testing.T) {
	messages, err := buildMessages(GenerateRequest{
		Messages: []handctx.Message{{Role: handctx.RoleUser, Content: "  hello  "}},
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)

	raw, err := json.Marshal(messages)
	require.NoError(t, err)
	rawText := string(raw)
	require.NotContains(t, rawText, `"role":"developer"`)
	require.Contains(t, rawText, `"role":"user"`)
	require.Contains(t, rawText, `"content":"hello"`)
}

func TestBuildMessages_WithInstructions(t *testing.T) {
	messages, err := buildMessages(GenerateRequest{
		Instructions: "  be concise  ",
		Messages: []handctx.Message{
			{Role: handctx.RoleUser, Content: "hello"},
			{Role: handctx.RoleAssistant, Content: "done"},
			{Role: handctx.RoleTool, Content: "tool output", ToolCallID: "tool"},
		},
	})
	require.NoError(t, err)
	require.Len(t, messages, 4)

	raw, err := json.Marshal(messages)
	require.NoError(t, err)
	rawText := string(raw)
	require.True(t, strings.Index(rawText, `"role":"developer"`) < strings.Index(rawText, `"role":"user"`))
	require.Contains(t, rawText, `"content":"be concise"`)
	require.Contains(t, rawText, `"content":"hello"`)
	require.Contains(t, rawText, `"role":"assistant"`)
	require.Contains(t, rawText, `"content":"done"`)
	require.Contains(t, rawText, `"role":"tool"`)
	require.Contains(t, rawText, `"content":"tool output"`)
	require.Contains(t, rawText, `"tool_call_id":"tool"`)
}

func TestBuildMessages_WithDeveloperMessageInConversation(t *testing.T) {
	messages, err := buildMessages(GenerateRequest{
		Messages: []handctx.Message{
			{Role: handctx.RoleDeveloper, Content: "extra instruction"},
		},
	})
	require.NoError(t, err)

	raw, err := json.Marshal(messages)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"role":"developer"`)
	require.Contains(t, string(raw), `"content":"extra instruction"`)
}

func TestBuildMessages_WithAssistantToolCalls(t *testing.T) {
	messages, err := buildMessages(GenerateRequest{
		Messages: []handctx.Message{{
			Role:      handctx.RoleAssistant,
			ToolCalls: []handctx.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
		}},
	})
	require.NoError(t, err)

	raw, err := json.Marshal(messages)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"tool_calls"`)
	require.Contains(t, string(raw), `"id":"call-1"`)
	require.Contains(t, string(raw), `"name":"time"`)
	require.NotContains(t, string(raw), `"content":""`)
}

func TestBuildMessages_WithAssistantToolCallsAndContent(t *testing.T) {
	messages, err := buildMessages(GenerateRequest{
		Messages: []handctx.Message{{
			Role:      handctx.RoleAssistant,
			Content:   "calling tool",
			ToolCalls: []handctx.ToolCall{{ID: "call-1", Name: "time", Input: "  {}  "}},
		}},
	})
	require.NoError(t, err)

	raw, err := json.Marshal(messages)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"content":"calling tool"`)
	require.Contains(t, string(raw), `"arguments":"{}"`)
}

func TestBuildMessages_RejectsToolMessageWithoutToolCallID(t *testing.T) {
	_, err := buildMessages(GenerateRequest{
		Messages: []handctx.Message{{
			Role:    handctx.RoleTool,
			Content: "tool output",
		}},
	})

	require.EqualError(t, err, "tool call id is required")
}

func TestBuildMessages_RejectsToolCallWithoutID(t *testing.T) {
	_, err := buildMessages(GenerateRequest{
		Messages: []handctx.Message{{
			Role:      handctx.RoleAssistant,
			ToolCalls: []handctx.ToolCall{{Name: "time", Input: "{}"}},
		}},
	})

	require.EqualError(t, err, "tool call id is required")
}

func TestBuildMessages_RejectsToolCallWithoutName(t *testing.T) {
	_, err := buildMessages(GenerateRequest{
		Messages: []handctx.Message{{
			Role:      handctx.RoleAssistant,
			ToolCalls: []handctx.ToolCall{{ID: "call-1", Input: "{}"}},
		}},
	})

	require.EqualError(t, err, "tool call name is required")
}

func TestBuildMessages_TrimsContentAndRole(t *testing.T) {
	messages, err := buildMessages(GenerateRequest{
		Messages: []handctx.Message{
			{Role: handctx.Role(" User "), Content: "  hello  "},
			{Role: handctx.Role(" assistant "), Content: "  hi  "},
		},
	})
	require.NoError(t, err)

	raw, err := json.Marshal(messages)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"role":"user"`)
	require.Contains(t, string(raw), `"content":"hello"`)
	require.Contains(t, string(raw), `"role":"assistant"`)
	require.Contains(t, string(raw), `"content":"hi"`)
}
