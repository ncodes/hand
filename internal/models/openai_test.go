package models

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/pkg/logutils"
)

func TestNewOpenAIClient_IncludesAPIKeyOptionWhenProvided(t *testing.T) {
	originalCompletionFactory := newOpenAICompletionCaller
	originalResponseFactory := newOpenAIResponseCaller
	t.Cleanup(func() {
		newOpenAICompletionCaller = originalCompletionFactory
		newOpenAIResponseCaller = originalResponseFactory
	})

	completionCalls := 0
	responseCalls := 0
	completionOptCount := 0
	responseOptCount := 0
	newOpenAICompletionCaller = func(opts ...option.RequestOption) func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		completionCalls++
		completionOptCount = len(opts)
		return func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, nil
		}
	}
	newOpenAIResponseCaller = func(opts ...option.RequestOption) func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
		responseCalls++
		responseOptCount = len(opts)
		return func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return nil, nil
		}
	}

	client, err := NewOpenAIClient(" test-key ", option.WithBaseURL("https://example.com/v1"))
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, 1, completionCalls)
	require.Equal(t, 1, responseCalls)
	require.Equal(t, 2, completionOptCount)
	require.Equal(t, 2, responseOptCount)
}

func TestNewOpenAIClient_OmitsAPIKeyOptionWhenEmpty(t *testing.T) {
	originalCompletionFactory := newOpenAICompletionCaller
	originalResponseFactory := newOpenAIResponseCaller
	t.Cleanup(func() {
		newOpenAICompletionCaller = originalCompletionFactory
		newOpenAIResponseCaller = originalResponseFactory
	})

	completionOptCount := 0
	responseOptCount := 0
	newOpenAICompletionCaller = func(opts ...option.RequestOption) func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		completionOptCount = len(opts)
		return func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, nil
		}
	}
	newOpenAIResponseCaller = func(opts ...option.RequestOption) func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
		responseOptCount = len(opts)
		return func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return nil, nil
		}
	}

	client, err := NewOpenAIClient("   ", option.WithBaseURL("https://example.com/v1"))
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, 1, completionOptCount)
	require.Equal(t, 1, responseOptCount)
}

func TestNewOpenAICompletionCaller_UsesSDKClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"resp_123","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"hello back"},"finish_reason":"stop"}]}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	caller := newOpenAICompletionCaller(option.WithBaseURL(server.URL), option.WithAPIKey("test-key"))
	resp, err := caller(context.Background(), openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("test-model"),
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")},
	})

	require.NoError(t, err)
	require.Equal(t, "resp_123", resp.ID)
}

func TestNewOpenAIResponseCaller_UsesSDKClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/responses", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"resp_123","object":"response","created_at":0,"model":"gpt-5.1","output":[{"id":"msg_123","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello back","annotations":[]}]}],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1,"status":"completed","text":{"format":{"type":"text"}}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	caller := newOpenAIResponseCaller(option.WithBaseURL(server.URL), option.WithAPIKey("test-key"))
	resp, err := caller(context.Background(), responses.ResponseNewParams{
		Model: "gpt-5.1",
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String("hello")},
	})

	require.NoError(t, err)
	require.Equal(t, "resp_123", resp.ID)
	require.Equal(t, "hello back", resp.OutputText())
}

func TestOpenAIClient_ChatRequiresClient(t *testing.T) {
	var nilClient *OpenAIClient
	_, err := nilClient.Chat(context.Background(), Request{})
	require.EqualError(t, err, "model client is required")
}

func TestOpenAIClient_ChatRejectsInvalidAPIMode(t *testing.T) {
	client := &OpenAIClient{}
	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		APIMode:  "invalid",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "model api mode must be one of: chat-completions, responses")
}

func TestOpenAIClient_ChatRequiresModel(t *testing.T) {
	client := &OpenAIClient{}

	_, err := client.Chat(context.Background(), Request{
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "model is required")
}

func TestOpenAIClient_ChatRequiresMessages(t *testing.T) {
	client := &OpenAIClient{}

	_, err := client.Chat(context.Background(), Request{Model: "test-model"})
	require.EqualError(t, err, "messages are required")
}

func TestOpenAIClient_ChatRequiresSelectedModeHandler(t *testing.T) {
	client := &OpenAIClient{}
	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "model client is required")
}

func TestOpenAIClient_ChatRequiresChatCompletionsHandler(t *testing.T) {
	client := &OpenAIClient{createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
		return &responses.Response{}, nil
	}}

	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "model client is required")
}

func TestOpenAIClient_ChatRejectsInvalidMessageRole(t *testing.T) {
	client := &OpenAIClient{}
	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.Role("invalid"), Content: "hello"}},
	})
	require.EqualError(t, err, "message role must be one of user, assistant, or tool; developer messages must be provided via instructions")
}

func TestOpenAIClient_ChatRejectsEmptyMessageContent(t *testing.T) {
	client := &OpenAIClient{}
	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "   "}},
	})
	require.EqualError(t, err, "message content is required")
}

func TestOpenAIClient_ChatRejectsDeveloperMessageInConversation(t *testing.T) {
	client := &OpenAIClient{}
	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleDeveloper, Content: "system"}},
	})
	require.EqualError(t, err, "developer messages must be provided via instructions")
}

func TestOpenAIClient_ChatRejectsBlankToolDefinitionName(t *testing.T) {
	client := &OpenAIClient{}
	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
		Tools:    []ToolDefinition{{Name: "   "}},
	})
	require.EqualError(t, err, "tool name is required")
}

func TestOpenAIClient_ChatReturnsAPIErrorChatCompletions(t *testing.T) {
	expectedErr := errors.New("upstream failed")
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, expectedErr
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.ErrorIs(t, err, expectedErr)
}

func TestOpenAIClient_ChatRequiresChatCompletionsResponse(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "model response is required")
}

func TestOpenAIClient_ChatReturnsResponseAndBuildsChatCompletionsRequest(t *testing.T) {
	var captured openai.ChatCompletionNewParams
	client := &OpenAIClient{
		createChatCompletion: func(_ context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			captured = params
			return &openai.ChatCompletion{
				ID:    "resp_123",
				Model: "returned-model",
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: "hello back"},
				}},
			}, nil
		},
	}

	resp, err := client.Chat(context.Background(), Request{
		Model:        "test-model",
		Instructions: "  be concise  ",
		Messages: []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "  hello  "},
			{Role: handmsg.RoleAssistant, Content: " previous reply "},
		},
		MaxOutputTokens: 123,
		Temperature:     0.7,
	})
	require.NoError(t, err)
	require.Equal(t, &Response{ID: "resp_123", Model: "returned-model", OutputText: "hello back"}, resp)

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

func TestOpenAIClient_ChatBuildsChatCompletionsStructuredOutputRequest(t *testing.T) {
	var captured openai.ChatCompletionNewParams
	client := &OpenAIClient{
		createChatCompletion: func(_ context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			captured = params
			return &openai.ChatCompletion{
				ID:    "resp_123",
				Model: "returned-model",
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"session_summary":"ok","current_task":"","discoveries":[],"open_questions":[],"next_actions":[]}`},
				}},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
		StructuredOutput: &StructuredOutput{
			Name:        "session_summary",
			Description: "summary payload",
			Strict:      true,
			Schema:      map[string]any{"type": "object"},
		},
	})
	require.NoError(t, err)

	raw, err := json.Marshal(captured)
	require.NoError(t, err)
	rawText := string(raw)
	require.Contains(t, rawText, `"response_format":{`)
	require.Contains(t, rawText, `"type":"json_schema"`)
	require.Contains(t, rawText, `"name":"session_summary"`)
	require.Contains(t, rawText, `"description":"summary payload"`)
	require.Contains(t, rawText, `"strict":true`)
}

func TestExtractChatCompletionsResponse_IncludesUsage(t *testing.T) {
	resp, err := extractChatCompletionsResponse(&openai.ChatCompletion{
		ID:    "resp_123",
		Model: "gpt-test",
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{Content: "hello"},
		}},
		Usage: openai.CompletionUsage{
			PromptTokens:     12,
			CompletionTokens: 7,
			TotalTokens:      19,
		},
	})

	require.NoError(t, err)
	require.Equal(t, 12, resp.PromptTokens)
	require.Equal(t, 7, resp.CompletionTokens)
	require.Equal(t, 19, resp.TotalTokens)
}

func TestExtractResponsesResponse_IncludesUsage(t *testing.T) {
	resp, err := extractResponsesResponse(&responses.Response{
		ID:     "resp_123",
		Model:  "gpt-test",
		Status: responses.ResponseStatusCompleted,
		Output: []responses.ResponseOutputItemUnion{{
			Type:    "message",
			ID:      "msg_123",
			Role:    "assistant",
			Status:  "completed",
			Content: []responses.ResponseOutputMessageContentUnion{{Type: "output_text", Text: "hello"}},
		}},
		Usage: responses.ResponseUsage{
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
		},
	})

	require.NoError(t, err)
	require.Equal(t, 10, resp.PromptTokens)
	require.Equal(t, 5, resp.CompletionTokens)
	require.Equal(t, 15, resp.TotalTokens)
}

func TestOpenAIClient_ChatReturnsToolCallsFromChatCompletions(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(_ context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			raw, err := json.Marshal(params)
			require.NoError(t, err)
			require.Contains(t, string(raw), `"name":"time"`)

			return &openai.ChatCompletion{
				ID:    "resp_123",
				Model: "returned-model",
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{ToolCalls: []openai.ChatCompletionMessageToolCallUnion{{
						ID:   "call-1",
						Type: "function",
						Function: openai.ChatCompletionMessageFunctionToolCallFunction{
							Name:      "time",
							Arguments: "{}",
						},
					}}},
				}},
			}, nil
		},
	}

	resp, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "what time is it?"}},
		Tools:    []ToolDefinition{{Name: "time", Description: "Returns the current time.", InputSchema: map[string]any{"type": "object"}}},
	})
	require.NoError(t, err)
	require.True(t, resp.RequiresToolCalls)
	require.Equal(t, []ToolCall{{ID: "call-1", Name: "time", Input: "{}"}}, resp.ToolCalls)
}

func TestOpenAIClient_ChatRejectsChatCompletionToolCallWithoutID(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{ToolCalls: []openai.ChatCompletionMessageToolCallUnion{{
						Function: openai.ChatCompletionMessageFunctionToolCallFunction{Name: "time", Arguments: "{}"},
					}}},
				}},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "tool call id is required")
}

func TestOpenAIClient_ChatRejectsChatCompletionToolCallWithoutName(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{ToolCalls: []openai.ChatCompletionMessageToolCallUnion{{
						ID:       "call-1",
						Function: openai.ChatCompletionMessageFunctionToolCallFunction{Arguments: "{}"},
					}}},
				}},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "tool call name is required")
}

func TestOpenAIClient_ChatRejectsChatCompletionResponseWithoutChoices(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{ID: "resp_123"}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, `chat completion response "resp_123" contained no choices`)
}

func TestOpenAIClient_ChatRejectsEmptyChatCompletionResponse(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				ID:    "resp_empty",
				Model: "returned-model",
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{},
				}},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "model returned empty response")
}

func TestOpenAIClient_ChatUsesRefusalAsOutputText(t *testing.T) {
	client := &OpenAIClient{
		createChatCompletion: func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				ID:    "resp_refusal",
				Model: "returned-model",
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Refusal: "I can't do that."},
				}},
			}, nil
		},
	}

	resp, err := client.Chat(context.Background(), Request{
		Model:    "test-model",
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, &Response{ID: "resp_refusal", Model: "returned-model", OutputText: "I can't do that."}, resp)
}

func TestOpenAIClient_ChatReturnsResponseAndBuildsResponsesRequest(t *testing.T) {
	var captured responses.ResponseNewParams
	client := &OpenAIClient{
		createResponse: func(_ context.Context, params responses.ResponseNewParams) (*responses.Response, error) {
			captured = params
			return &responses.Response{
				ID:    "resp_456",
				Model: "gpt-5.1",
				Output: []responses.ResponseOutputItemUnion{{
					Type: "message",
					Content: []responses.ResponseOutputMessageContentUnion{{
						Type: "output_text",
						Text: "hello from responses",
					}},
				}},
			}, nil
		},
	}

	resp, err := client.Chat(context.Background(), Request{
		Model:        "gpt-5.1",
		APIMode:      APIModeResponses,
		Instructions: "  be concise  ",
		Messages: []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "  hello  "},
			{Role: handmsg.RoleAssistant, Content: "calling tool", ToolCalls: []handmsg.ToolCall{{ID: "call-1", Name: "time", Input: " {} "}}},
			{Role: handmsg.RoleTool, Content: `{"output":"2026-03-24T00:00:00Z"}`, ToolCallID: "call-1"},
		},
		Tools:           []ToolDefinition{{Name: "time", Description: "Returns the current time.", InputSchema: map[string]any{"type": "object"}}},
		MaxOutputTokens: 111,
		Temperature:     0.5,
	})
	require.NoError(t, err)
	require.Equal(t, &Response{ID: "resp_456", Model: "gpt-5.1", OutputText: "hello from responses"}, resp)

	logutils.PrettyPrintJSON(captured, "<<")
	raw, err := json.Marshal(captured)
	require.NoError(t, err)
	rawText := string(raw)
	require.Contains(t, rawText, `"model":"gpt-5.1"`)
	require.Contains(t, rawText, `"instructions":"be concise"`)
	require.Contains(t, rawText, `"max_output_tokens":111`)
	require.Contains(t, rawText, `"temperature":0.5`)
	require.Contains(t, rawText, `"type":"message"`)
	require.Contains(t, rawText, `"type":"input_text"`)
	require.Contains(t, rawText, `"type":"function_call"`)
	require.Contains(t, rawText, `"type":"function_call_output"`)
	require.Contains(t, rawText, `"call_id":"call-1"`)
	require.Contains(t, rawText, `"name":"time"`)
}

func TestOpenAIClient_ChatBuildsResponsesStructuredOutputRequest(t *testing.T) {
	var captured responses.ResponseNewParams
	client := &OpenAIClient{
		createResponse: func(_ context.Context, params responses.ResponseNewParams) (*responses.Response, error) {
			captured = params
			return &responses.Response{
				ID:    "resp_456",
				Model: "gpt-5.1",
				Output: []responses.ResponseOutputItemUnion{{
					Type: "message",
					Content: []responses.ResponseOutputMessageContentUnion{{
						Type: "output_text",
						Text: `{"session_summary":"ok","current_task":"","discoveries":[],"open_questions":[],"next_actions":[]}`,
					}},
				}},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
		StructuredOutput: &StructuredOutput{
			Name:        "session_summary",
			Description: "summary payload",
			Strict:      true,
			Schema:      map[string]any{"type": "object"},
		},
	})
	require.NoError(t, err)

	raw, err := json.Marshal(captured)
	require.NoError(t, err)
	rawText := string(raw)
	require.Contains(t, rawText, `"text":{"format":{`)
	require.Contains(t, rawText, `"type":"json_schema"`)
	require.Contains(t, rawText, `"name":"session_summary"`)
	require.Contains(t, rawText, `"description":"summary payload"`)
	require.Contains(t, rawText, `"strict":true`)
}

func TestOpenAIClient_ChatReturnsToolCallsFromResponses(t *testing.T) {
	client := &OpenAIClient{
		createResponse: func(_ context.Context, params responses.ResponseNewParams) (*responses.Response, error) {
			raw, err := json.Marshal(params)
			require.NoError(t, err)
			require.Contains(t, string(raw), `"type":"function"`)

			return &responses.Response{
				ID:     "resp_789",
				Model:  "gpt-5.1",
				Output: []responses.ResponseOutputItemUnion{responseOutputItemFromJSON(t, `{"type":"function_call","call_id":"call-1","name":"time","arguments":"{}"}`)},
			}, nil
		},
	}

	resp, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "what time is it?"}},
		Tools:    []ToolDefinition{{Name: "time", Description: "Returns the current time.", InputSchema: map[string]any{"type": "object"}}},
	})
	require.NoError(t, err)
	require.True(t, resp.RequiresToolCalls)
	require.Equal(t, []ToolCall{{ID: "call-1", Name: "time", Input: "{}"}}, resp.ToolCalls)
}

func TestOpenAIClient_ChatReturnsResponseErrorResponses(t *testing.T) {
	expectedErr := errors.New("upstream failed")
	client := &OpenAIClient{
		createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return nil, expectedErr
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.ErrorIs(t, err, expectedErr)
}

func TestOpenAIClient_ChatRequiresResponsesResponse(t *testing.T) {
	client := &OpenAIClient{
		createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return nil, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "model response is required")
}

func TestOpenAIClient_ChatRejectsResponseToolCallWithoutID(t *testing.T) {
	client := &OpenAIClient{
		createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return &responses.Response{Output: []responses.ResponseOutputItemUnion{
				responseOutputItemFromJSON(t, `{"type":"function_call","name":"time","arguments":"{}"}`),
			}}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "tool call id is required")
}

func TestOpenAIClient_ChatRejectsResponseToolCallWithoutName(t *testing.T) {
	client := &OpenAIClient{
		createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return &responses.Response{Output: []responses.ResponseOutputItemUnion{
				responseOutputItemFromJSON(t, `{"type":"function_call","call_id":"call-1","arguments":"{}"}`),
			}}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "tool call name is required")
}

func TestOpenAIClient_ChatReturnsResponsesFailureError(t *testing.T) {
	client := &OpenAIClient{
		createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return &responses.Response{
				Status: responses.ResponseStatusFailed,
				Error:  responses.ResponseError{Message: "provider failed"},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "provider failed")
}

func TestOpenAIClient_ChatReturnsResponsesIncompleteErrorWithoutUsableOutput(t *testing.T) {
	client := &OpenAIClient{
		createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return &responses.Response{
				Status:            responses.ResponseStatusIncomplete,
				IncompleteDetails: responses.ResponseIncompleteDetails{Reason: "max_output_tokens"},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "response incomplete: max_output_tokens")
}

func TestOpenAIClient_ChatReturnsResponsesIncompleteSuccessWithText(t *testing.T) {
	client := &OpenAIClient{
		createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return &responses.Response{
				ID:     "resp_123",
				Model:  "gpt-5.1",
				Status: responses.ResponseStatusIncomplete,
				Output: []responses.ResponseOutputItemUnion{{
					Type: "message",
					Content: []responses.ResponseOutputMessageContentUnion{{
						Type: "output_text",
						Text: "partial answer",
					}},
				}},
			}, nil
		},
	}

	resp, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, "partial answer", resp.OutputText)
}

func TestOpenAIClient_ChatRejectsUnexpectedResponsesStatus(t *testing.T) {
	client := &OpenAIClient{
		createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return &responses.Response{Status: responses.ResponseStatusInProgress}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "response status is in_progress")
}

func TestOpenAIClient_ChatReturnsResponsesFailureErrorWithoutProviderMessage(t *testing.T) {
	client := &OpenAIClient{
		createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return &responses.Response{Status: responses.ResponseStatusFailed}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "response failed")
}

func TestOpenAIClient_ChatReturnsResponsesIncompleteErrorWithUnknownReason(t *testing.T) {
	client := &OpenAIClient{
		createResponse: func(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
			return &responses.Response{Status: responses.ResponseStatusIncomplete}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:    "gpt-5.1",
		APIMode:  APIModeResponses,
		Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
	})
	require.EqualError(t, err, "response incomplete: unknown")
}

func TestOpenAIClient_ChatLogsRequestDebugDumpForChatCompletions(t *testing.T) {
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
				Model: "test-model",
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: "hello back"},
				}},
			}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{
		Model:         "test-model",
		Messages:      []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
		DebugRequests: true,
	})
	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, `"mode":"chat-completions"`)
	require.Contains(t, output, `"content":"hello"`)
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
		createResponse: func(_ context.Context, _ responses.ResponseNewParams) (*responses.Response, error) {
			return &responses.Response{ID: "resp_123", Model: "gpt-5.1", Output: []responses.ResponseOutputItemUnion{{Type: "message", Content: []responses.ResponseOutputMessageContentUnion{{Type: "output_text", Text: "hello back"}}}}}, nil
		},
	}

	_, err := client.Chat(context.Background(), Request{Model: "gpt-5.1", APIMode: APIModeResponses, Instructions: "be concise", Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}, DebugRequests: true})
	require.NoError(t, err)
	output := buf.String()
	require.Contains(t, output, "model request debug dump")
	require.Contains(t, output, `"provider":"openai-compatible"`)
	require.Contains(t, output, `"mode":"responses"`)
	require.Contains(t, output, `"model":"gpt-5.1"`)
	require.Contains(t, output, `"text":"hello"`)
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

	logRequestDebugDump(APIModeResponses, responses.ResponseNewParams{})
	require.Contains(t, buf.String(), "Failed to marshal model request debug dump")
}

func TestNormalizeGenerateRequestDefaultsAPIMode(t *testing.T) {
	normalized, err := normalizeGenerateRequest(Request{Model: "test-model", Messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}})
	require.NoError(t, err)
	require.Equal(t, APIModeChatCompletions, normalized.APIMode)
}

func TestBuildChatCompletionsRequestIncludesPlainAssistantMessage(t *testing.T) {
	params := buildChatCompletionsRequest(normalizedGenerateRequest{
		Model: "test-model",
		Messages: []handmsg.Message{{
			Role:    handmsg.RoleAssistant,
			Content: "hello back",
		}},
	})

	raw, err := json.Marshal(params)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"role":"assistant"`)
	require.Contains(t, string(raw), `"content":"hello back"`)
}

func TestBuildChatCompletionsRequestIncludesAssistantToolCallContent(t *testing.T) {
	params := buildChatCompletionsRequest(normalizedGenerateRequest{
		Model: "test-model",
		Messages: []handmsg.Message{{
			Role:      handmsg.RoleAssistant,
			Content:   "calling tool",
			ToolCalls: []handmsg.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
		}},
	})

	raw, err := json.Marshal(params)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"content":"calling tool"`)
	require.Contains(t, string(raw), `"tool_calls"`)
}

func TestBuildChatCompletionsRequestIncludesToolMessages(t *testing.T) {
	params := buildChatCompletionsRequest(normalizedGenerateRequest{
		Model:        "test-model",
		Instructions: "be concise",
		Messages: []handmsg.Message{
			{Role: handmsg.RoleAssistant, ToolCalls: []handmsg.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}}},
			{Role: handmsg.RoleTool, Content: `{"output":"2026-03-24T00:00:00Z"}`, ToolCallID: "call-1"},
		},
		Tools: []ToolDefinition{{Name: "time", Description: "Returns time", InputSchema: map[string]any{"type": "object"}}},
	})
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	rawText := string(raw)
	require.Contains(t, rawText, `"role":"developer"`)
	require.Contains(t, rawText, `"tool_calls"`)
	require.Contains(t, rawText, `"tool_call_id":"call-1"`)
	require.Contains(t, rawText, `"tools"`)
}

func TestNormalizeMessagesAcceptsAssistantToolCallWithoutContent(t *testing.T) {
	messages, err := normalizeMessages([]handmsg.Message{{
		Role:      handmsg.RoleAssistant,
		ToolCalls: []handmsg.ToolCall{{ID: "call-1", Name: "time", Input: " {} "}},
	}})
	require.NoError(t, err)
	require.Equal(t, []handmsg.Message{{
		Role:      handmsg.RoleAssistant,
		ToolCalls: []handmsg.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
	}}, messages)
}

func TestNormalizeMessagesPropagatesToolCallNormalizationError(t *testing.T) {
	_, err := normalizeMessages([]handmsg.Message{{
		Role:      handmsg.RoleAssistant,
		ToolCalls: []handmsg.ToolCall{{Name: "time", Input: "{}"}},
	}})
	require.EqualError(t, err, "tool call id is required")
}

func TestNormalizeMessagesTrimsName(t *testing.T) {
	messages, err := normalizeMessages([]handmsg.Message{{
		Role:       handmsg.RoleTool,
		Content:    `{"output":"ok"}`,
		Name:       " time ",
		ToolCallID: "call-1",
	}})
	require.NoError(t, err)
	require.Equal(t, "time", messages[0].Name)
}

func TestNormalizeMessagesRequiresToolCallIDForToolMessages(t *testing.T) {
	_, err := normalizeMessages([]handmsg.Message{{
		Role:    handmsg.RoleTool,
		Content: `{"output":"ok"}`,
	}})
	require.EqualError(t, err, "tool call id is required")
}

func TestNormalizeToolCallsRejectsMissingID(t *testing.T) {
	_, err := normalizeToolCalls([]handmsg.ToolCall{{Name: "time", Input: "{}"}})
	require.EqualError(t, err, "tool call id is required")
}

func TestNormalizeToolCallsRejectsMissingName(t *testing.T) {
	_, err := normalizeToolCalls([]handmsg.ToolCall{{ID: "call-1", Input: "{}"}})
	require.EqualError(t, err, "tool call name is required")
}

func TestNormalizeToolCallsReturnsNilWhenEmpty(t *testing.T) {
	toolCalls, err := normalizeToolCalls(nil)
	require.NoError(t, err)
	require.Nil(t, toolCalls)
}

func TestExtractChatCompletionsToolCallsReturnsNilWhenEmpty(t *testing.T) {
	toolCalls, err := extractChatCompletionsToolCalls(nil)
	require.NoError(t, err)
	require.Nil(t, toolCalls)
}

func TestExtractChatCompletionsToolCallsRejectsMissingID(t *testing.T) {
	_, err := extractChatCompletionsToolCalls([]openai.ChatCompletionMessageToolCallUnion{{
		Function: openai.ChatCompletionMessageFunctionToolCallFunction{Name: "time", Arguments: "{}"},
	}})
	require.EqualError(t, err, "tool call id is required")
}

func TestExtractChatCompletionsToolCallsRejectsMissingName(t *testing.T) {
	_, err := extractChatCompletionsToolCalls([]openai.ChatCompletionMessageToolCallUnion{{
		ID:       "call-1",
		Function: openai.ChatCompletionMessageFunctionToolCallFunction{Arguments: "{}"},
	}})
	require.EqualError(t, err, "tool call name is required")
}

func responseOutputItemFromJSON(t *testing.T, raw string) responses.ResponseOutputItemUnion {
	t.Helper()
	var item responses.ResponseOutputItemUnion
	require.NoError(t, json.Unmarshal([]byte(raw), &item))
	return item
}

func TestLogRequestDebugDumpRedactsSensitiveFields(t *testing.T) {
	originalLogger := log.Logger
	originalLevel := zerolog.GlobalLevel()
	t.Cleanup(func() {
		log.Logger = originalLogger
		zerolog.SetGlobalLevel(originalLevel)
	})

	buf := &bytes.Buffer{}
	log.Logger = zerolog.New(buf)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	logRequestDebugDump(APIModeResponses, map[string]any{
		"authorization": "Bearer secret-token",
		"nested":        map[string]any{"api_key": "sk-secret-key"},
	})

	output := buf.String()
	require.Contains(t, output, `"authorization":"[REDACTED]"`)
	require.Contains(t, output, `"api_key":"[REDACTED]"`)
	require.NotContains(t, output, "secret-token")
}
