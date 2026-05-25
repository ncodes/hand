package provider_anthropic

import (
	"time"

	"github.com/rs/zerolog/log"

	models "github.com/wandxy/hand/internal/model"
)

func logModelClientRequestStarted(req normalizedGenerateRequest, stream bool) {
	log.Debug().
		Str("provider", "anthropic").
		Str("api", models.APIAnthropicMessages).
		Str("model", req.Model).
		Bool("stream", stream).
		Int("messages", len(req.Messages)).
		Int("tools", len(req.Tools)).
		Int64("max_output_tokens", req.MaxOutputTokens).
		Msg("model request started")
}

func logModelClientRequestCompleted(req normalizedGenerateRequest, stream bool, resp *Response) {
	event := log.Debug().
		Str("provider", "anthropic").
		Str("api", models.APIAnthropicMessages).
		Str("model", req.Model).
		Bool("stream", stream)
	if resp != nil {
		event = event.
			Str("response_id", resp.ID).
			Int("prompt_tokens", resp.PromptTokens).
			Int("completion_tokens", resp.CompletionTokens).
			Int("total_tokens", resp.TotalTokens).
			Bool("requires_tool_calls", resp.RequiresToolCalls)
	}
	event.Msg("model request completed")
}

func logModelClientRequestFailed(req normalizedGenerateRequest, stream bool, err error) {
	log.Debug().
		Err(err).
		Str("provider", "anthropic").
		Str("api", models.APIAnthropicMessages).
		Str("model", req.Model).
		Bool("stream", stream).
		Msg("model request failed")
}

func logRequestDebugMetadata(req normalizedGenerateRequest) {
	log.Debug().
		Str("provider", "anthropic").
		Str("api", models.APIAnthropicMessages).
		Str("model", req.Model).
		Int("messages", len(req.Messages)).
		Int("tools", len(req.Tools)).
		Time("logged_at", time.Now().UTC()).
		Msg("model request debug metadata")
}
