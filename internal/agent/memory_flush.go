package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/config"
	instruct "github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
	agentcore "github.com/wandxy/hand/pkg/agent"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
	models "github.com/wandxy/hand/pkg/agent/model"
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

// maybeFlushMemoryBeforeCompaction preserves useful facts before automatic compaction drops context.
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

// maybeFlushMemoryBeforeContextLoss preserves memory before session switches or shutdown.
func (a *Agent) maybeFlushMemoryBeforeContextLoss(
	ctx context.Context,
	sessionID string,
	trigger string,
	traceSession trace.Session,
) {
	if !a.shouldFlushMemoryBeforeContextLoss() {
		return
	}

	// Reuse a Turn so memory flush sees the same context-building, tool context,
	// and session state paths as a normal response.
	turn := a.newTurn(a.env, nil)
	if err := turn.load(ctx, agentcore.RespondOptions{SessionID: sessionID}); err != nil {
		recordMemoryFlushFailure(traceSession, trigger, err)
		return
	}
	if err := turn.flushMemoryBeforeContextLoss(ctx, trigger, traceSession); err != nil {
		recordMemoryFlushFailure(traceSession, trigger, err)
	}
}

// shouldFlushMemoryBeforeContextLoss checks agent-level prerequisites for memory flush.
func (a *Agent) shouldFlushMemoryBeforeContextLoss() bool {
	if a == nil || a.cfg == nil || !a.initialized || a.stateMgr == nil || a.env == nil {
		return false
	}
	if a.modelClient == nil || !a.cfg.MemoryEnabled() || !a.cfg.MemoryFlushEnabled() {
		return false
	}

	return true
}

// shouldFlushMemoryBeforeCompaction checks whether this request is likely to compact context.
func (t *Turn) shouldFlushMemoryBeforeCompaction(request models.Request) bool {
	if t == nil ||
		t.cfg == nil ||
		!t.cfg.MemoryEnabled() ||
		!t.cfg.MemoryFlushEnabled() ||
		!isCompactionEnabled(t.cfg) {
		return false
	}

	if _, _, hasLegacyTools := t.environmentToolRegistryAndPolicy(); t.modelClient == nil ||
		(t.toolRegistry == nil && t.invokeToolFn == nil && !hasLegacyTools) {
		return false
	}

	return getCompactionEvaluator(t.cfg).
		Evaluate(request, t.lastPromptTokens).
		Triggered()
}

// flushMemoryBeforeContextLoss asks the model to write durable memories before context is lost.
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

	flushClient := t.summaryClient
	if flushClient == nil {
		flushClient = t.modelClient
	}
	if flushClient == nil {
		return errors.New("memory flush model client is required")
	}

	// Respect a separate flush timeout so preservation cannot hang compaction or
	// shutdown indefinitely.
	flushCtx := ctx
	var cancel context.CancelFunc
	if cfg.Memory.Flush.Timeout > 0 {
		flushCtx, cancel = context.WithTimeout(ctx, cfg.Memory.Flush.Timeout)
		defer cancel()
	}

	// Seed the flush model with the same context the next model request would
	// have seen, plus an explicit request to extract durable memory.
	messages := append(handmsg.CloneMessages(t.Context()), handmsg.Message{
		Role:    handmsg.RoleUser,
		Content: instruct.BuildMemoryFlushRequest(trigger),
	})
	agentLog.Debug().
		Str("event", trace.EvtMemoryFlushStarted).
		Str("trigger", trigger).
		Str("session_id", t.sessionID).
		Int("max_calls", cfg.Memory.Flush.MaxCalls).
		Int("tool_count", len(flushTools)).
		Msg("memory flush started before context loss")
	traceSession.Record(trace.EvtMemoryFlushStarted, trace.MemoryEventPayload{
		Trigger:   trigger,
		MaxCalls:  cfg.Memory.Flush.MaxCalls,
		ToolCount: len(flushTools),
	})

	callCount := 0
	for callCount < cfg.Memory.Flush.MaxCalls {
		if err := flushCtx.Err(); err != nil {
			return err
		}

		request := models.Request{
			Model:           cfg.SummaryModelEffective(),
			APIMode:         cfg.SummaryModelAPIModeEffective(),
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
			Str("model", request.Model).
			Str("mode", request.APIMode).
			Int("message_count", len(request.Messages)).
			Int("tool_count", len(request.Tools)).
			Msg("memory flush model request prepared")
		traceSession.Record(trace.EvtMemoryFlushModelRequested, trace.MemoryEventPayload{
			Trigger:      trigger,
			MessageCount: len(request.Messages),
			ToolCount:    len(request.Tools),
		})
		recordModelRequest(traceSession, request)

		// The flush loop allows the model to request one or more memory write
		// tools, bounded by MaxCalls so it cannot consume the turn.
		resp, err := flushClient.Complete(flushCtx, request)
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
			traceSession.Record(trace.EvtMemoryFlushCompleted, trace.MemoryEventPayload{
				Trigger:   trigger,
				Status:    "no_op",
				ToolCalls: callCount,
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
			// memory_extract needs the active session id; normalize only that tool
			// so other memory tools retain the model-provided input shape.
			toolCall = t.normalizeMemoryFlushToolCall(toolCall)

			agentLog.Debug().
				Str("event", trace.EvtMemoryFlushWriteRequested).
				Str("trigger", trigger).
				Str("session_id", t.sessionID).
				Str("tool", toolCall.Name).
				Str("tool_call_id", toolCall.ID).
				Msg("memory flush write tool requested")
			traceSession.Record(trace.EvtMemoryFlushWriteRequested, trace.MemoryEventPayload{
				Trigger:    trigger,
				Tool:       toolCall.Name,
				ToolCallID: toolCall.ID,
			})

			toolCtx := tools.WithTraceRecorder(t.getToolContext(flushCtx), traceSession)
			if _, err := normalizeTurnMessage(t.invokeFlushTool(toolCtx, toolCall)); err != nil {
				return err
			}

			recordMemoryFlushCompleted(traceSession, trigger, t.sessionID, "tool_executed", callCount)
			return nil
		}
	}

	recordMemoryFlushCompleted(traceSession, trigger, t.sessionID, "bounded", callCount)

	return nil
}

// normalizeMemoryFlushToolCall injects session_id into memory_extract calls.
func (t *Turn) normalizeMemoryFlushToolCall(toolCall models.ToolCall) models.ToolCall {
	if t == nil || strings.TrimSpace(toolCall.Name) != "memory_extract" {
		return toolCall
	}

	sessionID := strings.TrimSpace(t.sessionID)
	if sessionID == "" {
		return toolCall
	}

	input := map[string]any{}
	if strings.TrimSpace(toolCall.Input) != "" {
		if err := json.Unmarshal([]byte(toolCall.Input), &input); err != nil {
			return toolCall
		}
	}
	input["session_id"] = sessionID

	raw, err := jsonMarshal(input)
	if err != nil {
		return toolCall
	}

	toolCall.Input = string(raw)
	return toolCall
}

// invokeFlushTool uses the turn's tool boundary so memory flush follows normal tool behavior.
func (t *Turn) invokeFlushTool(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
	if t == nil {
		return invokeToolWithEnvironment(ctx, nil, toolCall, nil, nil)
	}
	if _, _, hasLegacyTools := t.environmentToolRegistryAndPolicy(); t.invokeToolFn == nil && t.toolRegistry == nil && !hasLegacyTools {
		return invokeToolWithEnvironment(ctx, nil, toolCall, nil, nil)
	}

	return t.invokeTool(ctx, toolCall)
}

// availableMemoryFlushToolDefinitions returns only memory tools that are safe during flush.
func (t *Turn) availableMemoryFlushToolDefinitions() ([]models.ToolDefinition, error) {
	definitions, err := t.availableToolDefinitions()
	if err != nil {
		return nil, err
	}
	if len(definitions) == 0 {
		return nil, nil
	}

	flushTools := make([]models.ToolDefinition, 0, len(memoryFlushToolNames))
	for _, definition := range definitions {
		if _, ok := memoryFlushToolNames[definition.Name]; !ok {
			continue
		}
		flushTools = append(flushTools, definition)
	}

	return flushTools, nil
}

// recordMemoryFlushFailure records a failed or timed-out memory flush.
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
	traceSession.Record(event, trace.MemoryEventPayload{
		Trigger: trigger,
		Error:   err.Error(),
	})
}

// recordMemoryFlushCompleted records successful memory flush termination.
func recordMemoryFlushCompleted(
	traceSession trace.Session,
	trigger string,
	sessionID string,
	status string,
	toolCalls int,
) {
	agentLog.Debug().
		Str("event", trace.EvtMemoryFlushCompleted).
		Str("trigger", trigger).
		Str("session_id", sessionID).
		Str("status", status).
		Int("tool_calls", toolCalls).
		Msg("memory flush completed before context loss")
	traceSession.Record(trace.EvtMemoryFlushCompleted, trace.MemoryEventPayload{
		Trigger:   trigger,
		Status:    status,
		ToolCalls: toolCalls,
	})
}

// recordMemoryFlushSkipped records a non-fatal reason memory flush did no work.
func recordMemoryFlushSkipped(traceSession trace.Session, trigger string, reason string) {
	reason = strings.TrimSpace(reason)
	agentLog.Debug().
		Str("event", trace.EvtMemoryFlushSkipped).
		Str("trigger", trigger).
		Str("reason", reason).
		Msg("memory flush skipped before context loss")
	traceSession.Record(trace.EvtMemoryFlushSkipped, trace.MemoryEventPayload{
		Trigger: trigger,
		Reason:  reason,
	})
}
