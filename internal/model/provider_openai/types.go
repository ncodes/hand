package provider_openai

import models "github.com/wandxy/hand/internal/model"

const (
	// APIOpenAICompletions identifies the OpenAI-compatible chat completions API.
	APIOpenAICompletions = models.APIOpenAICompletions
	// APIOpenAIResponses identifies the OpenAI responses API.
	APIOpenAIResponses = models.APIOpenAIResponses
	// APIAnthropicMessages identifies the Anthropic messages API.
	APIAnthropicMessages = models.APIAnthropicMessages

	// StreamChannelAssistant identifies assistant text stream deltas.
	StreamChannelAssistant = models.StreamChannelAssistant
	// StreamChannelReasoning identifies reasoning stream deltas.
	StreamChannelReasoning = models.StreamChannelReasoning
)

// Client is the provider-neutral model client contract implemented by OpenAIClient.
type Client = models.Client

// Request is the provider-neutral model request accepted by OpenAIClient.
type Request = models.Request

// Response is the provider-neutral model response returned by OpenAIClient.
type Response = models.Response

// StreamChannel identifies the kind of streaming text delta.
type StreamChannel = models.StreamChannel

// StreamDelta is a single streaming text or reasoning update.
type StreamDelta = models.StreamDelta

// StructuredOutput describes a JSON schema response format request.
type StructuredOutput = models.StructuredOutput

// ToolCall is a normalized model tool call.
type ToolCall = models.ToolCall

// ToolDefinition is a normalized model-visible tool schema.
type ToolDefinition = models.ToolDefinition
