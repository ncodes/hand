package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	ctxbuilder "github.com/wandxy/hand/internal/agent/context"
	"github.com/wandxy/hand/internal/agent/context/compaction"
	agentsummary "github.com/wandxy/hand/internal/agent/context/summary"
	"github.com/wandxy/hand/internal/agent/runcontext"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/environment"
	envbudget "github.com/wandxy/hand/internal/environment/budget"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/profile"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

const requestInstructionName = "request.instruct"
const planHydrationPageSize = constants.PlanHydrationPageSize

// Turn executes a single response turn against a resolved session.
type Turn struct {
	// ctx is the request context used for session writes during the turn.
	ctx context.Context
	// cfg provides model and execution settings for the turn.
	cfg *config.Config
	// modelClient executes model requests for the turn.
	modelClient models.Client
	// summaryClient executes compaction/summary model requests; when nil, modelClient is used.
	summaryClient models.Client
	// stateMgr resolves sessions and persists turn messages.
	stateMgr *statemanager.Manager
	// summaryService loads and refreshes persisted summary state for the turn.
	summaryService *agentsummary.Service
	// invokeToolFn performs tool execution for requested tool calls.
	invokeToolFn func(context.Context, environment.Environment, models.ToolCall) handmsg.Message
	// env supplies tools, instructions, tracing, and iteration budget.
	env environment.Environment
	// contextBuilder assembles the model-visible message context for the turn.
	contextBuilder *ctxbuilder.Builder
	// instructions is the request-scoped instruction set sent to the model.
	instructions instruct.Instructions
	// memoryInstruction contains retrieved durable memory for the current turn.
	memoryInstruction instruct.Instruction
	// sessionHistory contains persisted messages loaded before the turn starts.
	sessionHistory []handmsg.Message
	// emittedMessages contains messages produced during the current turn.
	emittedMessages []handmsg.Message
	// summary contains persisted summary state used in active context assembly.
	summary *agentsummary.State
	// sessionHistoryOffset is the absolute persisted offset represented by sessionHistory[0].
	sessionHistoryOffset int
	// sessionID identifies the session being read from and written to.
	sessionID string
	// runCtx carries effective state identity, personality, profile, and lineage for this run.
	runCtx runcontext.Context
	// lastPromptTokens stores the most recent actual prompt token count for the session.
	lastPromptTokens int
	// summaryRefreshAttempted prevents multiple summary refresh attempts in one turn.
	summaryRefreshAttempted bool
	// planHydrated indicates whether plan state was restored from session history.
	planHydrated bool
}

// NewTurn constructs a Turn with the dependencies needed for one response turn.
func NewTurn(
	cfg *config.Config,
	modelClient models.Client,
	summaryClient models.Client,
	stateMgr *statemanager.Manager,
	invokeToolFn func(context.Context, environment.Environment, models.ToolCall) handmsg.Message,
	runtimeEnv environment.Environment,
) *Turn {
	if summaryClient == nil {
		summaryClient = modelClient
	}
	return &Turn{
		cfg:            cfg,
		modelClient:    modelClient,
		summaryClient:  summaryClient,
		stateMgr:       stateMgr,
		invokeToolFn:   invokeToolFn,
		env:            runtimeEnv,
		contextBuilder: ctxbuilder.New(),
	}
}

func (t *Turn) load(ctx context.Context, opts RespondOptions) error {
	if t == nil {
		return errors.New("agent is required")
	}

	if t.cfg == nil {
		return errors.New("config is required")
	}

	if t.modelClient == nil {
		return errors.New("model client is required")
	}

	if t.env == nil {
		return errors.New("runtime environment is required")
	}

	if t.stateMgr == nil {
		return errors.New("state manager is required")
	}

	if t.summaryService == nil {
		t.summaryService = agentsummary.NewService(t.cfg, t.modelClient, t.summaryClient, t.stateMgr)
	}

	session, err := t.stateMgr.Resolve(ctx, opts.SessionID)
	if err != nil {
		return err
	}

	summary, err := t.summaryService.Load(ctx, session.ID)
	if err != nil {
		return err
	}

	tailOffset := 0
	if summary != nil && summary.Current != nil {
		tailOffset = max(summary.Current.SourceEndOffset, 0)
	}

	messages, err := t.stateMgr.GetMessages(ctx, session.ID, storage.MessageQueryOptions{Offset: tailOffset})
	if err != nil {
		return err
	}
	t.ctx = ctx
	t.instructions = t.env.Instructions()
	t.memoryInstruction = instruct.Instruction{}
	t.sessionHistory = messages
	t.emittedMessages = nil
	t.summary = summary
	t.sessionHistoryOffset = tailOffset
	t.sessionID = session.ID
	t.runCtx, err = newRootRunContext(session.ID)
	if err != nil {
		return err
	}
	t.lastPromptTokens = session.LastPromptTokens
	t.summaryRefreshAttempted = false
	t.planHydrated, err = t.hydratePlanFromHistory(ctx, t.getStateSessionID())
	if err != nil {
		return err
	}

	agentLog.Debug().
		Str("session_id", session.ID).
		Str("plan", "load_summary_load_history_hydrate_plan_prepare_instructions").
		Int("history_offset", tailOffset).
		Int("history_messages", len(messages)).
		Msg("turn context loaded for response generation")

	return nil
}

// Run executes the turn and returns the assistant reply.
func (t *Turn) Run(ctx context.Context, msg string, opts RespondOptions) (string, error) {
	if err := t.load(ctx, opts); err != nil {
		return "", err
	}

	requestInstruct := strings.TrimSpace(opts.Instruct)
	if requestInstruct != "" {
		t.instructions = setInstruction(t.instructions, instruct.Instruction{
			Name:  requestInstructionName,
			Value: requestInstruct,
		})
		defer func() {
			t.instructions = t.instructions.WithoutName(requestInstructionName)
		}()
	}

	traceSession := t.env.NewTraceSessionForRun(t.runCtx)

	if opts.OnTraceEvent != nil {
		traceSession = newFanoutTraceSession(traceSession, t.getStateSessionID(), opts.OnTraceEvent)
	}

	defer traceSession.Close()

	t.recordLoadedContentSafety(traceSession)

	if t.planHydrated {
		plan := t.env.CurrentPlan(t.getStateSessionID())
		traceSession.Record(
			trace.EvtPlanHydrated,
			map[string]any{
				"session_id":     t.getStateSessionID(),
				"steps":          plan.Steps,
				"summary":        summarizeHydratedPlan(plan),
				"active_step_id": getActiveHydratedPlanStepID(plan),
				"explanation":    strings.TrimSpace(plan.Explanation),
				"source":         "history",
			},
		)
	}

	if t.cfg.InputSafetyEnabled() {
		inputSafety := guardrails.CheckInputSafety(msg, "user")
		if inputSafety.Blocked {
			traceSession.Record(trace.EvtInputSafetyBlocked, getInputSafetyTracePayload(t.sessionID, msg, inputSafety))
			return inputSafety.RefusalMessage, nil
		}
	}

	userMessage, err := handmsg.NewMessage(handmsg.RoleUser, msg)
	if err != nil {
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	t.emittedMessages = append(t.emittedMessages, userMessage)
	if err := t.appendSessionMessages([]handmsg.Message{userMessage}); err != nil {
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	traceSession.Record(trace.EvtUserMessageAccepted, map[string]any{"message": msg})

	t.memoryInstruction = t.retrieveMemoryInstruction(ctx, msg, traceSession)

	budget := t.env.NewIterationBudget()
	streamingEnabled := t.cfg.StreamEnabled()
	if opts.Stream != nil {
		streamingEnabled = *opts.Stream
	}

	for budget.Consume() {
		if err := ctx.Err(); err != nil {
			traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
			return "", err
		}

		availableToolDefinitions, err := t.availableToolDefinitions()
		if err != nil {
			traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
			return "", err
		}

		request := models.Request{
			Model:         t.cfg.Models.Main.Name,
			APIMode:       t.cfg.Models.Main.APIMode,
			Instructions:  t.buildRequestInstructions(availableToolDefinitions),
			Messages:      t.Context(),
			Tools:         availableToolDefinitions,
			DebugRequests: t.cfg.Debug.Requests,
		}

		if !t.summaryRefreshAttempted {
			t.summaryRefreshAttempted = true
			t.maybeFlushMemoryBeforeCompaction(ctx, request, traceSession)
			_ = t.summaryService.MaybeRefreshSummary(ctx, t.summary, agentsummary.RefreshInput{
				LastPromptTokens: t.lastPromptTokens,
				Request:          request,
				SessionID:        t.sessionID,
				TraceSession:     traceSession,
			})
			t.trimSessionHistoryToSummary()
		}

		// Refresh may persist a new session summary; rebuild instructions after it.
		request.Instructions = t.buildRequestInstructions(availableToolDefinitions)

		// Attach the current context to the request
		request.Messages = t.Context()

		// Record trace events
		t.summary.RecordSummaryApplied(traceSession)
		recordPreflightCompactionTrace(traceSession, t.cfg, request, t.lastPromptTokens)
		recordModelRequest(traceSession, request)

		agentLog.Info().
			Str("event", "model request dispatch started").
			Str("plan", "assemble_context_send_model_stream_or_complete_handle_response").
			Str("provider", t.cfg.Models.Main.Provider).
			Str("mode", t.cfg.Models.Main.APIMode).
			Str("model", t.cfg.Models.Main.Name).
			Bool("stream", streamingEnabled).
			Int("context_messages", len(request.Messages)).
			Int("tools", len(request.Tools)).
			Bool("debug_requests", t.cfg.Debug.Requests).
			Msg("model request dispatch started")

		var resp *models.Response
		if streamingEnabled {
			deltas := make([]Event, 0, 16)
			allowLiveDelivery := opts.OnEvent != nil
			resp, err = t.modelClient.CompleteStream(ctx, request, func(delta models.StreamDelta) {
				if delta.Text == "" {
					return
				}
				event := Event{Kind: EventKindTextDelta, Channel: string(delta.Channel), Text: delta.Text}
				if allowLiveDelivery {
					opts.OnEvent(event)
					return
				}
				deltas = append(deltas, event)
			})
		} else {
			resp, err = t.modelClient.Complete(ctx, request)
		}
		if err != nil {
			agentLog.Warn().
				Str("event", "model request dispatch failed").
				Str("provider", t.cfg.Models.Main.Provider).
				Str("mode", t.cfg.Models.Main.APIMode).
				Str("model", t.cfg.Models.Main.Name).
				Bool("stream", streamingEnabled).
				Str("error_kind", getAgentModelErrorKind(err)).
				Msg("model request dispatch failed")
			traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
			return "", err
		}

		if resp == nil {
			err = errors.New("model response is required")
			agentLog.Warn().
				Str("event", "model request dispatch failed").
				Str("provider", t.cfg.Models.Main.Provider).
				Str("mode", t.cfg.Models.Main.APIMode).
				Str("model", t.cfg.Models.Main.Name).
				Bool("stream", streamingEnabled).
				Str("error_kind", "missing_response").
				Msg("model request dispatch failed")
			traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
			return "", err
		}

		recordModelResponse(traceSession, resp)

		agentLog.Info().
			Str("event", "model response received").
			Str("relationship", "response_to_current_turn_model_request").
			Str("provider", t.cfg.Models.Main.Provider).
			Str("mode", t.cfg.Models.Main.APIMode).
			Str("model", t.cfg.Models.Main.Name).
			Str("response_model", resp.Model).
			Bool("stream", streamingEnabled).
			Int("prompt_tokens", resp.PromptTokens).
			Int("completion_tokens", resp.CompletionTokens).
			Int("total_tokens", resp.TotalTokens).
			Int("tool_call_count", len(resp.ToolCalls)).
			Bool("requires_tool_calls", resp.RequiresToolCalls).
			Msg("model response received")

		if err := t.recordPostflightUsage(traceSession, resp); err != nil {
			return "", err
		}

		if !resp.RequiresToolCalls {
			reply := t.applyAssistantOutputSafety(traceSession, resp.OutputText, streamingEnabled)

			assistantMessage, err := handmsg.NewMessage(handmsg.RoleAssistant, reply)
			if err != nil {
				traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
				return "", err
			}

			t.emittedMessages = append(t.emittedMessages, assistantMessage)
			if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
				traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
				return "", err
			}

			traceSession.Record(trace.EvtFinalAssistantResponse, map[string]any{"message": reply})

			agentLog.Info().
				Str("session_id", t.sessionID).
				Msg("turn completed")

			return reply, nil
		}

		if len(resp.ToolCalls) == 0 {
			err = errors.New("model requested tool execution without tool calls")
			traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
			return "", err
		}

		assistantMessage, err := assistantToolCallMessageFromResponse(resp)
		if err != nil {
			traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
			return "", err
		}

		t.emittedMessages = append(t.emittedMessages, assistantMessage)

		if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
			traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
			return "", err
		}

		for _, toolCall := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
				return "", err
			}

			agentLog.Info().
				Str("event", "tool invocation started").
				Str("relationship", "tool_call_from_current_model_response").
				Str("tool", toolCall.Name).
				Str("tool_call_id", toolCall.ID).
				Msg("tool invocation started")

			traceSession.Record(trace.EvtToolInvocationStarted, toolCall)
			toolCtx := tools.WithTraceRecorder(t.getToolContext(ctx), traceSession)
			toolMessage := t.invokeTool(toolCtx, toolCall)
			traceSession.Record(trace.EvtToolInvocationCompleted, toolMessage)

			agentLog.Info().
				Str("event", "tool invocation completed").
				Str("relationship", "tool_result_for_current_model_response").
				Str("tool", toolCall.Name).
				Str("tool_call_id", toolCall.ID).
				Int("output_chars", len([]rune(toolMessage.Content))).
				Int("output_bytes", len(toolMessage.Content)).
				Msg("tool invocation completed")

			toolMessage, err = normalizeTurnMessage(toolMessage)
			if err != nil {
				traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
				return "", err
			}

			t.emittedMessages = append(t.emittedMessages, toolMessage)

			if err := t.appendSessionMessages([]handmsg.Message{toolMessage}); err != nil {
				traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
				return "", err
			}
		}
	}

	agentLog.Warn().
		Str("session_id", t.sessionID).
		Msg("iteration budget exhausted, falling back to summary")

	reply, err := t.summaryFallback(ctx, budget, traceSession)
	if err != nil {
		return "", err
	}

	return reply, nil
}

func (t *Turn) trimSessionHistoryToSummary() {
	if t == nil || t.summary == nil || t.summary.Current == nil {
		return
	}

	targetOffset := max(t.summary.Current.SourceEndOffset, 0)
	if targetOffset <= t.sessionHistoryOffset {
		return
	}

	delta := targetOffset - t.sessionHistoryOffset
	if delta >= len(t.sessionHistory) {
		t.sessionHistory = nil
		t.sessionHistoryOffset = targetOffset
		return
	}

	t.sessionHistory = handmsg.CloneMessages(t.sessionHistory[delta:])
	t.sessionHistoryOffset = targetOffset
}

func (t *Turn) appendSessionMessages(messages []handmsg.Message) error {
	return t.stateMgr.AppendMessages(t.ctx, t.sessionID, messages)
}

func (t *Turn) applyAssistantOutputSafety(traceSession trace.Session, output string, streamingEnabled bool) string {
	if streamingEnabled {
		return output
	}
	if t == nil || t.cfg == nil || !t.cfg.OutputSafetyEnabled() {
		return output
	}

	result := guardrails.CheckOutputSafety(output, "assistant", t.getOutputRedactor())
	if traceSession != nil && (result.Blocked || result.Redacted) {
		traceSession.Record(trace.EvtOutputSafetyApplied, getOutputSafetyTracePayload(t.sessionID, output, result))
	}

	return result.Content
}

func (t *Turn) getOutputRedactor() guardrails.Redactor {
	if t == nil || t.cfg == nil {
		return guardrails.NewRedactorWithOptions(guardrails.RedactorOptions{DisablePII: true})
	}

	return guardrails.NewRedactorWithOptions(guardrails.RedactorOptions{
		DisablePII: !t.cfg.OutputPIIRedactionEnabled(),
	})
}

func (t *Turn) recordLoadedContentSafety(traceSession trace.Session) {
	if t == nil || t.env == nil || traceSession == nil {
		return
	}

	for _, event := range t.env.SafetyTraceEvents() {
		traceSession.Record(trace.EvtLoadedContentSafetyBlocked, guardrails.SafetyTracePayload(event))
	}
}

func getInputSafetyTracePayload(sessionID string, content string, result guardrails.InputSafetyResult) map[string]any {
	return guardrails.SafetyTracePayload(guardrails.SafetyTracePayloadOptions{
		SessionID:     sessionID,
		Source:        "user",
		Action:        "blocked",
		ContentLength: len([]rune(content)),
		Blocked:       result.Blocked,
		Findings:      result.Findings,
		Refusal:       result.RefusalMessage,
	})
}

func getOutputSafetyTracePayload(sessionID string, content string, result guardrails.OutputSafetyResult) map[string]any {
	action := "redacted"
	if result.Blocked {
		action = "blocked"
	}
	return guardrails.SafetyTracePayload(guardrails.SafetyTracePayloadOptions{
		SessionID:     sessionID,
		Source:        "assistant",
		Action:        action,
		ContentLength: len([]rune(content)),
		Blocked:       result.Blocked,
		Redacted:      result.Redacted,
		Findings:      result.Findings,
		Refusal:       result.RefusalMessage,
	})
}

func (t *Turn) getStateSessionID() string {
	if t == nil {
		return storage.DefaultSessionID
	}
	if strings.TrimSpace(t.runCtx.Session.EffectiveID) != "" ||
		strings.TrimSpace(t.runCtx.Session.PublicID) != "" {
		return t.runCtx.StateSessionID()
	}
	if value := strings.TrimSpace(t.sessionID); value != "" {
		return value
	}

	return t.runCtx.StateSessionID()
}

func (t *Turn) getToolContext(ctx context.Context) context.Context {
	if t == nil {
		return tools.WithSessionID(ctx, "")
	}

	if strings.TrimSpace(t.runCtx.Session.PublicID) != "" {
		return tools.WithRunContext(ctx, t.runCtx)
	}

	return tools.WithSessionID(ctx, t.sessionID)
}

func (t *Turn) availableToolDefinitions() ([]models.ToolDefinition, error) {
	if t == nil || t.env == nil || t.env.Tools() == nil {
		return nil, nil
	}

	definitions, err := t.env.Tools().Resolve(t.env.ToolPolicy())
	if err != nil {
		return nil, err
	}

	toolsList := make([]models.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		toolsList = append(toolsList, modelToolDefinitionFromToolDefinition(definition))
	}

	return toolsList, nil
}

func (t *Turn) invokeTool(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
	if t.invokeToolFn == nil {
		return handmsg.Message{
			Role:       handmsg.RoleTool,
			Name:       toolCall.Name,
			ToolCallID: toolCall.ID,
			Content:    `{"error":"tool invocation is required"}`,
		}
	}

	return t.invokeToolFn(ctx, t.env, toolCall)
}

func (t *Turn) summaryFallback(ctx context.Context, budget envbudget.IterationBudget, traceSession trace.Session) (string, error) {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	request := models.Request{
		Model:         t.cfg.Models.Main.Name,
		APIMode:       t.cfg.Models.Main.APIMode,
		Instructions:  t.buildRequestInstructions(nil, instruct.BuildSummary(budget.Remaining())),
		Messages:      t.Context(),
		Tools:         nil,
		DebugRequests: t.cfg.Debug.Requests,
	}

	traceSession.Record(trace.EvtSummaryFallbackStarted, map[string]any{"remaining_iterations": budget.Remaining()})
	t.summary.RecordSummaryApplied(traceSession)
	recordPreflightCompactionTrace(traceSession, t.cfg, request, t.lastPromptTokens)
	recordModelRequest(traceSession, request)

	agentLog.Info().
		Str("event", "summary fallback model request started").
		Str("plan", "send_summary_fallback_prompt_without_tools").
		Str("provider", t.cfg.Models.Main.Provider).
		Str("mode", t.cfg.Models.Main.APIMode).
		Str("model", t.cfg.Models.Main.Name).
		Int("context_messages", len(request.Messages)).
		Bool("debug_requests", t.cfg.Debug.Requests).
		Msg("summary fallback model request started")

	resp, err := t.modelClient.Complete(ctx, request)
	if err != nil {
		agentLog.Error().
			Err(err).
			Str("event", "summary fallback model request failed").
			Str("session_id", t.sessionID).
			Str("provider", t.cfg.Models.Main.Provider).
			Str("mode", t.cfg.Models.Main.APIMode).
			Str("model", t.cfg.Models.Main.Name).
			Str("error_kind", getAgentModelErrorKind(err)).
			Msg("summary fallback model request failed")
		wrapped := fmt.Errorf("iteration limit reached and summary failed: %w", err)
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": wrapped.Error()})
		return "", wrapped
	}

	if resp == nil {
		err = errors.New("model response is required")
		agentLog.Error().
			Str("event", "summary fallback model request failed").
			Str("session_id", t.sessionID).
			Str("provider", t.cfg.Models.Main.Provider).
			Str("mode", t.cfg.Models.Main.APIMode).
			Str("model", t.cfg.Models.Main.Name).
			Str("error_kind", "missing_response").
			Msg("summary fallback model request failed")
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	recordModelResponse(traceSession, resp)
	agentLog.Info().
		Str("event", "summary fallback model response received").
		Str("provider", t.cfg.Models.Main.Provider).
		Str("mode", t.cfg.Models.Main.APIMode).
		Str("model", t.cfg.Models.Main.Name).
		Str("response_model", resp.Model).
		Int("prompt_tokens", resp.PromptTokens).
		Int("completion_tokens", resp.CompletionTokens).
		Int("total_tokens", resp.TotalTokens).
		Msg("summary fallback model response received")

	if err := t.recordPostflightUsage(traceSession, resp); err != nil {
		return "", err
	}

	if resp.RequiresToolCalls {
		err = fmt.Errorf("iteration limit reached and summary requested more tools")
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	reply := t.applyAssistantOutputSafety(traceSession, resp.OutputText, false)

	assistantMessage, err := handmsg.NewMessage(handmsg.RoleAssistant, reply)
	if err != nil {
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	t.emittedMessages = append(t.emittedMessages, assistantMessage)
	if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	traceSession.Record(trace.EvtFinalAssistantResponse, map[string]any{"message": reply})
	return reply, nil
}

func getAgentModelErrorKind(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	value := strings.ToLower(err.Error())
	switch {
	case strings.Contains(value, "response is required"):
		return "missing_response"
	case strings.Contains(value, "timeout"):
		return "timeout"
	default:
		return "operation_failed"
	}
}

// buildRequestInstructions assembles the system prompt sent to the model in this order:
// planning policy, hydrated active plan context, remaining base instructions, optional
// persisted summary instructions, optional environment context, optional request-scoped
// instruction, then any extra blocks.
func (t *Turn) buildRequestInstructions(
	activeToolDefinitions []models.ToolDefinition,
	extra ...instruct.Instructions,
) string {
	if t == nil {
		return ""
	}

	instructions := t.instructions

	// Hold request-scoped guidance aside so it can be appended after the summary.
	requestInstruction, hasRequestInstruction := instructions.GetByName(requestInstructionName)
	if hasRequestInstruction {
		instructions = instructions.WithoutName(requestInstructionName)
	}

	// Prepend hydrated plan context and keep planning policy ahead of it when present.
	if planInstructions := t.renderPlanInstructions(); planInstructions != "" {
		if policy, ok := instructions.GetByName(instruct.PlanningPolicyInstructionName); ok {
			instructions = instruct.New(policy.Value).
				Append(instruct.Instruction{Value: planInstructions}).
				Append(instructions.WithoutName(instruct.PlanningPolicyInstructionName)...)
		} else {
			instructions = instruct.New(planInstructions).Append(instructions...)
		}
	}

	// Add persisted summary instructions after base instructions and plan context.
	if t.summary != nil {
		if summaryInstructions, ok := t.summary.RenderSummaryInstructions(); ok {
			instructions = instructions.Append(instruct.Instruction{Value: summaryInstructions})
		}
	}

	instructions = instructions.Append(t.memoryInstruction)

	environmentContext := t.buildEnvironmentContextInstruction(activeToolDefinitions)
	instructions = instructions.Append(environmentContext)

	// Append the per-request instruction after summary and environment context.
	if hasRequestInstruction {
		instructions = instructions.Append(requestInstruction)
	}

	// Append any caller-provided extras last, such as summary-fallback guidance.
	for _, block := range extra {
		instructions = instructions.Append(block...)
	}

	return instructions.String()
}
func (t *Turn) hydratePlanFromMessages(messages []handmsg.Message) bool {
	if t == nil || t.env == nil {
		return false
	}

	empty := envtypes.Plan{}
	for _, message := range messages {
		if message.Role != handmsg.RoleTool || message.Name != "plan_tool" {
			continue
		}

		plan, ok := decodeHydratedPlan(message.Content)
		if !ok {
			continue
		}

		t.env.HydratePlan(t.getStateSessionID(), plan)
		return true
	}

	t.env.HydratePlan(t.getStateSessionID(), empty)
	return false
}

func (t *Turn) renderPlanInstructions() string {
	if t == nil || t.env == nil {
		return ""
	}

	plan := t.env.CurrentPlan(t.getStateSessionID())
	activeSteps := make([]envtypes.PlanStep, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		if step.Status == envtypes.PlanStatusCompleted || step.Status == envtypes.PlanStatusCancelled {
			continue
		}
		activeSteps = append(activeSteps, step)
	}
	if len(activeSteps) == 0 {
		return ""
	}

	lines := []string{
		"# Plan Context",
		"",
		"## Active Plan",
	}
	for _, step := range activeSteps {
		lines = append(lines, "- ["+step.Status+"] "+step.Content)
	}
	if explanation := strings.TrimSpace(plan.Explanation); explanation != "" {
		lines = append(lines, "", "## Plan Update Reason", "", explanation)
	}

	return strings.Join(lines, "\n")
}

func decodeHydratedPlan(content string) (envtypes.Plan, bool) {
	type toolMessageEnvelope struct {
		Output string `json:"output"`
	}

	var envelope toolMessageEnvelope
	if err := json.Unmarshal([]byte(content), &envelope); err == nil && strings.TrimSpace(envelope.Output) != "" {
		if plan, ok := decodeHydratedPlanPayload(envelope.Output); ok {
			return plan, true
		}
	}

	return decodeHydratedPlanPayload(content)
}

func decodeHydratedPlanPayload(content string) (envtypes.Plan, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return envtypes.Plan{}, false
	}

	stepsRaw, ok := raw["steps"]
	if !ok {
		return envtypes.Plan{}, false
	}

	var steps []envtypes.PlanStep
	if err := json.Unmarshal(stepsRaw, &steps); err != nil {
		return envtypes.Plan{}, false
	}

	explanation := ""
	if explanationRaw, ok := raw["explanation"]; ok {
		_ = json.Unmarshal(explanationRaw, &explanation)
	}

	plan := envtypes.Plan{Steps: steps, Explanation: strings.TrimSpace(explanation)}
	if err := envtypes.ValidatePlan(plan); err != nil {
		return envtypes.Plan{}, false
	}

	return plan, true
}

func (t *Turn) hydratePlanFromHistory(ctx context.Context, sessionID string) (bool, error) {
	offset := 0
	for {
		messages, err := t.stateMgr.GetMessages(ctx, sessionID, storage.MessageQueryOptions{
			Role:   handmsg.RoleTool,
			Name:   "plan_tool",
			Order:  storage.MessageOrderDesc,
			Limit:  planHydrationPageSize,
			Offset: offset,
		})
		if err != nil {
			return false, err
		}
		if len(messages) == 0 {
			t.env.HydratePlan(sessionID, envtypes.Plan{})
			return false, nil
		}
		if t.hydratePlanFromMessages(messages) {
			return true, nil
		}
		offset += len(messages)
	}
}

func summarizeHydratedPlan(plan envtypes.Plan) envtypes.PlanSummary {
	summary := envtypes.PlanSummary{Total: len(plan.Steps)}
	for _, step := range plan.Steps {
		switch step.Status {
		case envtypes.PlanStatusPending:
			summary.Pending++
		case envtypes.PlanStatusInProgress:
			summary.InProgress++
		case envtypes.PlanStatusCompleted:
			summary.Completed++
		case envtypes.PlanStatusCancelled:
			summary.Cancelled++
		}
	}
	return summary
}

func getActiveHydratedPlanStepID(plan envtypes.Plan) string {
	for _, step := range plan.Steps {
		if step.Status == envtypes.PlanStatusInProgress {
			return step.ID
		}
	}
	return ""
}

func newRootRunContext(sessionID string) (runcontext.Context, error) {
	runCtx, err := runcontext.NewParent(sessionID)
	if err != nil {
		return runcontext.Context{}, err
	}

	if active := profile.Active(); strings.TrimSpace(active.Name) != "" {
		runCtx.ProfileName = active.Name
	}

	return runCtx.Normalize()
}

func (t *Turn) Context() []handmsg.Message {
	builder := t.contextBuilder
	if builder == nil {
		builder = ctxbuilder.New()
	}

	recall := t.summary.Recall(t.sessionHistory)

	return builder.Build(ctxbuilder.Input{
		PrefixMessages:  recall.PrefixMessages,
		SessionHistory:  recall.SessionHistory,
		EmittedMessages: t.emittedMessages,
	})
}

func (t *Turn) recordPostflightUsage(traceSession trace.Session, resp *models.Response) error {
	if t == nil || resp == nil || resp.PromptTokens <= 0 {
		return nil
	}

	t.lastPromptTokens = resp.PromptTokens
	if err := t.stateMgr.UpdateLastPromptTokens(t.ctx, t.sessionID, resp.PromptTokens); err != nil {
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return err
	}

	traceSession.Record(trace.EvtContextPostflightUsage, map[string]any{
		"source":            compaction.ActualSource,
		"prompt_tokens":     resp.PromptTokens,
		"completion_tokens": resp.CompletionTokens,
		"total_tokens":      resp.TotalTokens,
	})

	return nil
}

// Messages returns the messages emitted during the turn.
func (t *Turn) Messages() []handmsg.Message {
	if len(t.emittedMessages) == 0 {
		return nil
	}

	messages := make([]handmsg.Message, len(t.emittedMessages))
	copy(messages, t.emittedMessages)

	return messages
}

func normalizeTurnMessage(message handmsg.Message) (handmsg.Message, error) {
	return handmsg.NormalizeMessage(message)
}

func setInstruction(instructions instruct.Instructions, instruction instruct.Instruction) instruct.Instructions {
	instruction.Name = strings.TrimSpace(instruction.Name)
	instruction.Value = strings.TrimSpace(instruction.Value)

	if instruction.Name == "" {
		if instruction.Value == "" {
			return instructions
		}

		return append(instructions, instruction)
	}

	for idx, existing := range instructions {
		if existing.Name != instruction.Name {
			continue
		}

		if instruction.Value == "" {
			return append(instructions[:idx], instructions[idx+1:]...)
		}

		updated := make(instruct.Instructions, len(instructions))
		copy(updated, instructions)
		updated[idx] = instruction
		return updated
	}

	if instruction.Value == "" {
		return instructions
	}

	return append(instructions, instruction)
}
