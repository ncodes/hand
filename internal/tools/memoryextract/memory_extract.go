package memoryextract

import (
	"context"
	"errors"
	"fmt"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/memory/episodic"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
)

type input struct {
	SessionID       string `json:"session_id,omitempty"`
	OffsetStart     *int   `json:"offset_start,omitempty"`
	OffsetEnd       *int   `json:"offset_end,omitempty"`
	WindowSize      int    `json:"window_size,omitempty"`
	MaxWindows      int    `json:"max_windows,omitempty"`
	MaxWindowChars  int    `json:"max_window_chars,omitempty"`
	MaxWindowTokens int    `json:"max_window_tokens,omitempty"`
}

func Definition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:             "memory_extract",
		Description:      "Extract source-linked episodic memories from a completed session or bounded message window.",
		Groups:           []string{"core"},
		Requires:         tools.Capabilities{Memory: true},
		UsageInstruction: instructions.BuildMemoryExtractGuidance(),
		InputSchema: common.ObjectSchema(map[string]any{
			"session_id":   common.StringSchema("Optional session id. When omitted, use the current session."),
			"offset_start": common.IntegerSchema("Optional inclusive message offset start. Defaults to the start of the session."),
			"offset_end":   common.IntegerSchema("Optional exclusive message offset end. Defaults to the end of the session."),
			"window_size": common.IntegerSchema(fmt.Sprintf(
				"Optional messages per extraction window. Defaults to %d and is capped at %d.",
				episodic.DefaultWindowSize,
				episodic.MaxWindowSize,
			)),
			"max_windows": common.IntegerSchema(fmt.Sprintf(
				"Optional maximum windows to process. Defaults to %d and is capped at %d.",
				episodic.DefaultMaxWindows,
				episodic.MaxWindows,
			)),
			"max_window_chars": common.IntegerSchema(fmt.Sprintf(
				"Optional maximum characters retained per extracted episode. Defaults to %d and is capped at %d.",
				episodic.DefaultMaxWindowChars,
				episodic.MaxWindowChars,
			)),
			"max_window_tokens": common.IntegerSchema(fmt.Sprintf(
				"Optional rough token estimate budget per extracted episode. Defaults to %d and is capped at %d.",
				episodic.DefaultMaxWindowTokens,
				episodic.MaxWindowTokens,
			)),
		}),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}
			if runtime == nil {
				return common.ToolError("tool_error", "memory extraction is not configured"), nil
			}

			extractReq, err := normalizeRequest(ctx, req)
			if err != nil {
				return common.ToolError("invalid_input", err.Error()), nil
			}

			result, err := runtime.ExtractEpisodes(ctx, extractReq)
			if err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			return common.EncodeOutput(result)
		}),
	}
}

func normalizeRequest(ctx context.Context, req input) (episodic.Request, error) {
	if req.OffsetStart != nil && *req.OffsetStart < 0 {
		return episodic.Request{}, errors.New("offset_start must be greater than or equal to zero")
	}
	if req.OffsetEnd != nil && *req.OffsetEnd < 0 {
		return episodic.Request{}, errors.New("offset_end must be greater than or equal to zero")
	}
	if req.OffsetStart != nil && req.OffsetEnd != nil && *req.OffsetEnd < *req.OffsetStart {
		return episodic.Request{}, errors.New("offset_end must be greater than or equal to offset_start")
	}

	windowSize, err := bounded(req.WindowSize, episodic.DefaultWindowSize, episodic.MaxWindowSize, "window_size")
	if err != nil {
		return episodic.Request{}, err
	}
	maxWindows, err := bounded(req.MaxWindows, episodic.DefaultMaxWindows, episodic.MaxWindows, "max_windows")
	if err != nil {
		return episodic.Request{}, err
	}
	maxChars, err := bounded(req.MaxWindowChars, episodic.DefaultMaxWindowChars, episodic.MaxWindowChars, "max_window_chars")
	if err != nil {
		return episodic.Request{}, err
	}
	maxTokens, err := bounded(req.MaxWindowTokens, episodic.DefaultMaxWindowTokens, episodic.MaxWindowTokens, "max_window_tokens")
	if err != nil {
		return episodic.Request{}, err
	}

	return episodic.Request{
		SessionID:       req.SessionID,
		OffsetStart:     req.OffsetStart,
		OffsetEnd:       req.OffsetEnd,
		WindowSize:      windowSize,
		MaxWindows:      maxWindows,
		MaxWindowChars:  maxChars,
		MaxWindowTokens: maxTokens,
		Trigger:         "tool",
		Trace:           tools.TraceRecorderFromContext(ctx),
	}, nil
}

func bounded(value int, fallback int, max int, name string) (int, error) {
	if value < 0 {
		return 0, errors.New(name + " must be greater than or equal to 0")
	}
	if value == 0 {
		return fallback, nil
	}
	if value > max {
		return max, nil
	}
	return value, nil
}
