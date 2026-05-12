package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/config"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

const memoryFlushTriggerCompression = "compression"
const memoryFlushTriggerSessionReset = "session reset"
const memoryFlushTriggerControlledExit = "controlled exit"

var memoryFlushToolNames = map[string]struct{}{
	"memory_extract": {},
	"memory_add":     {},
	"memory_update":  {},
	"memory_delete":  {},
}

func (t *Turn) maybeFlushMemoryBeforeCompaction(
	ctx context.Context,
	request models.Request,
	traceSession trace.Session,
) {
	if !t.shouldFlushMemoryBeforeCompaction(request) {
		return
	}

	if err := t.flushMemoryBeforeContextLoss(ctx, memoryFlushTriggerCompression, traceSession); err != nil {
		recordMemoryFlushFailure(traceSession, memoryFlushTriggerCompression, err)
	}
}

func (a *Agent) maybeFlushMemoryBeforeContextLoss(
	ctx context.Context,
	sessionID string,
	trigger string,
	traceSession trace.Session,
) {
	if !a.shouldFlushMemoryBeforeContextLoss() {
		return
	}

	turn := NewTurn(a.cfg, a.modelClient, a.summaryClient, a.stateMgr, nil, a.env)
	if err := turn.load(ctx, RespondOptions{SessionID: sessionID}); err != nil {
		recordMemoryFlushFailure(traceSession, trigger, err)
		return
	}
	if err := turn.flushMemoryBeforeContextLoss(ctx, trigger, traceSession); err != nil {
		recordMemoryFlushFailure(traceSession, trigger, err)
	}
}

func (a *Agent) shouldFlushMemoryBeforeContextLoss() bool {
	if a == nil || a.cfg == nil || !a.initialized || a.stateMgr == nil || a.env == nil {
		return false
	}
	if a.modelClient == nil || !a.cfg.MemoryEnabled() || !a.cfg.MemoryFlushEnabled() {
		return false
	}

	return true
}

func (t *Turn) shouldFlushMemoryBeforeCompaction(request models.Request) bool {
	if t == nil ||
		t.cfg == nil ||
		!t.cfg.MemoryEnabled() ||
		!t.cfg.MemoryFlushEnabled() ||
		!isCompactionEnabled(t.cfg) {
		return false
	}

	if t.modelClient == nil || t.env == nil || t.env.Tools() == nil {
		return false
	}

	return getCompactionEvaluator(t.cfg).
		Evaluate(request, t.lastPromptTokens).
		Triggered()
}

func (t *Turn) flushMemoryBeforeContextLoss(
	ctx context.Context,
	trigger string,
	traceSession trace.Session,
) error {
	if t == nil {
		return errors.New("turn is required")
	}

	flushTools, err := t.availableMemoryFlushToolDefinitions()
	if err != nil {
		return err
	}
	if len(flushTools) == 0 {
		recordMemoryFlushSkipped(traceSession, trigger, "no_supported_tools")
		return nil
	}

	cfg := t.cfg
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.Normalize()

	flushCtx := ctx
	var cancel context.CancelFunc
	if cfg.Memory.Flush.Timeout > 0 {
		flushCtx, cancel = context.WithTimeout(ctx, cfg.Memory.Flush.Timeout)
		defer cancel()
	}

	messages := handmsg.CloneMessages(t.Context())
	agentLog.Debug().
		Str("event", trace.EvtMemoryFlushStarted).
		Str("trigger", trigger).
		Str("session_id", t.sessionID).
		Int("max_calls", cfg.Memory.Flush.MaxCalls).
		Int("tool_count", len(flushTools)).
		Msg("memory flush started before context loss")
	traceSession.Record(trace.EvtMemoryFlushStarted, map[string]any{
		"trigger":    trigger,
		"max_calls":  cfg.Memory.Flush.MaxCalls,
		"tool_count": len(flushTools),
	})

	callCount := 0
	for callCount < cfg.Memory.Flush.MaxCalls {
		if err := flushCtx.Err(); err != nil {
			return err
		}

		request := models.Request{
			Model:           cfg.Models.Main.Name,
			APIMode:         cfg.Models.Main.APIMode,
			Instructions:    instruct.BuildMemoryFlushGuidance(trigger).Value,
			Messages:        messages,
			Tools:           flushTools,
			MaxOutputTokens: cfg.Memory.Flush.MaxOutputTokens,
			DebugRequests:   cfg.Debug.Requests,
		}

		agentLog.Debug().
			Str("event", trace.EvtMemoryFlushModelRequested).
			Str("trigger", trigger).
			Str("session_id", t.sessionID).
			Int("message_count", len(request.Messages)).
			Int("tool_count", len(request.Tools)).
			Msg("memory flush model request prepared")
		traceSession.Record(trace.EvtMemoryFlushModelRequested, map[string]any{
			"trigger":       trigger,
			"message_count": len(request.Messages),
			"tool_count":    len(request.Tools),
		})
		recordModelRequest(traceSession, request)

		resp, err := t.modelClient.Complete(flushCtx, request)
		if err != nil {
			return err
		}
		recordModelResponse(traceSession, resp)
		if resp == nil {
			return errors.New("model response is required")
		}
		if !resp.RequiresToolCalls {
			agentLog.Debug().
				Str("event", trace.EvtMemoryFlushCompleted).
				Str("trigger", trigger).
				Str("session_id", t.sessionID).
				Str("status", "no_op").
				Int("tool_calls", callCount).
				Msg("memory flush completed before context loss")
			traceSession.Record(trace.EvtMemoryFlushCompleted, map[string]any{
				"trigger":    trigger,
				"status":     "no_op",
				"tool_calls": callCount,
			})
			return nil
		}
		if len(resp.ToolCalls) == 0 {
			return errors.New("memory flush requested tool execution without tool calls")
		}

		assistantMessage, err := assistantToolCallMessageFromResponse(resp)
		if err != nil {
			return err
		}
		messages = append(messages, assistantMessage)

		for _, toolCall := range resp.ToolCalls {
			if callCount >= cfg.Memory.Flush.MaxCalls {
				break
			}
			callCount++
			if _, ok := memoryFlushToolNames[strings.TrimSpace(toolCall.Name)]; !ok {
				recordMemoryFlushSkipped(traceSession, trigger, "unsupported_tool:"+strings.TrimSpace(toolCall.Name))
				continue
			}

			agentLog.Debug().
				Str("event", trace.EvtMemoryFlushWriteRequested).
				Str("trigger", trigger).
				Str("session_id", t.sessionID).
				Str("tool", toolCall.Name).
				Str("tool_call_id", toolCall.ID).
				Msg("memory flush write tool requested")
			traceSession.Record(trace.EvtMemoryFlushWriteRequested, map[string]any{
				"trigger":      trigger,
				"tool":         toolCall.Name,
				"tool_call_id": toolCall.ID,
			})

			toolCtx := tools.WithTraceRecorder(tools.WithSessionID(flushCtx, t.sessionID), traceSession)
			toolMessage := t.invokeFlushTool(toolCtx, toolCall)
			toolMessage, err = normalizeTurnMessage(toolMessage)
			if err != nil {
				return err
			}
			messages = append(messages, toolMessage)
		}
	}

	agentLog.Debug().
		Str("event", trace.EvtMemoryFlushCompleted).
		Str("trigger", trigger).
		Str("session_id", t.sessionID).
		Str("status", "bounded").
		Int("tool_calls", callCount).
		Msg("memory flush completed before context loss")
	traceSession.Record(trace.EvtMemoryFlushCompleted, map[string]any{
		"trigger":    trigger,
		"status":     "bounded",
		"tool_calls": callCount,
	})

	return nil
}

func (t *Turn) invokeFlushTool(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
	if t.invokeToolFn != nil {
		return t.invokeTool(ctx, toolCall)
	}

	return invokeToolWithEnvironment(ctx, t.env, toolCall, t.summaryClient, t.cfg)
}

func (t *Turn) availableMemoryFlushToolDefinitions() ([]models.ToolDefinition, error) {
	if t == nil || t.env == nil || t.env.Tools() == nil {
		return nil, nil
	}

	definitions, err := t.env.Tools().Resolve(t.env.ToolPolicy())
	if err != nil {
		return nil, err
	}

	flushTools := make([]models.ToolDefinition, 0, len(memoryFlushToolNames))
	for _, definition := range definitions {
		if _, ok := memoryFlushToolNames[definition.Name]; !ok {
			continue
		}
		flushTools = append(flushTools, modelToolDefinitionFromToolDefinition(definition))
	}

	return flushTools, nil
}

func recordMemoryFlushFailure(traceSession trace.Session, trigger string, err error) {
	if err == nil {
		return
	}

	event := trace.EvtMemoryFlushFailed
	if errors.Is(err, context.DeadlineExceeded) {
		event = trace.EvtMemoryFlushTimeout
	}

	agentLog.Warn().
		Err(err).
		Str("event", event).
		Str("trigger", trigger).
		Msg("memory flush failed before context loss")
	traceSession.Record(event, map[string]any{
		"trigger": trigger,
		"error":   err.Error(),
	})
}

func recordMemoryFlushSkipped(traceSession trace.Session, trigger string, reason string) {
	reason = strings.TrimSpace(reason)
	agentLog.Debug().
		Str("event", trace.EvtMemoryFlushSkipped).
		Str("trigger", trigger).
		Str("reason", reason).
		Msg("memory flush skipped before context loss")
	traceSession.Record(trace.EvtMemoryFlushSkipped, map[string]any{
		"trigger": trigger,
		"reason":  reason,
	})
}
