package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	ctxbuilder "github.com/wandxy/hand/internal/agent/context"
	"github.com/wandxy/hand/internal/agent/context/compaction"
	summarizer "github.com/wandxy/hand/internal/agent/context/summary"
	"github.com/wandxy/hand/internal/agent/runcontext"
	"github.com/wandxy/hand/internal/config"
	envbudget "github.com/wandxy/hand/internal/environment/budget"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	instruct "github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/memory"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/profile"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
	agentcore "github.com/wandxy/hand/pkg/agent"
	agentprompt "github.com/wandxy/hand/pkg/agent/prompt"
	agentsession "github.com/wandxy/hand/pkg/agent/session"
	agenttool "github.com/wandxy/hand/pkg/agent/tool"
)

const requestInstructionName = "request.instruct"

type traceSessionFactory interface {
	NewTraceSessionForRun(runcontext.Context) trace.Session
}

type safetyTraceEventSource interface {
	SafetyTraceEvents() []guardrails.SafetyTracePayloadOptions
}

type memoryProviderSource interface {
	MemoryProvider() memory.Provider
}

type iterationBudgetFactory interface {
	NewIterationBudget() envbudget.IterationBudget
}

type planStateStore interface {
	CurrentPlan(string) envtypes.Plan
	HydratePlan(string, envtypes.Plan)
}

type legacyToolRegistry interface {
	ListGroups() []tools.Group
	Resolve(tools.Policy) (tools.Definitions, error)
	Invoke(context.Context, tools.Call) (tools.Result, error)
}

// Turn executes a single response turn against a resolved session.
type Turn struct {
	// Request context for session writes during the turn.
	ctx context.Context

	// Model and execution settings for the turn.
	cfg *config.Config

	// Model client responsible for model requests for the turn.
	modelClient models.Client

	// Summary client for compaction/summary model requests. Falls back to modelClient if nil.
	summaryClient models.Client

	// Session store used for turn-scoped session reads and writes.
	sessionStore agentsession.Store

	// Trace recorder used for turn-scoped persisted trace writes.
	traceRecorder agentsession.TraceRecorder

	// Loads/persists summary state for the turn.
	summaryService *summarizer.Service

	// Summary store used to lazily build the summary service with current turn config.
	summaryStore summarizer.SummaryStore

	// Tool registry used to resolve and invoke model-visible tools.
	toolRegistry agenttool.Registry

	// Tool policy used to filter tool definitions for this runtime.
	toolPolicy agenttool.Policy

	// Prompt provider supplies reusable prompt inputs from the host runtime.
	promptProvider agentprompt.Provider

	// Trace sessions are opened by the host runtime.
	traceSessions traceSessionFactory

	// Safety events loaded by the host runtime are replayed into the turn trace.
	safetyEvents safetyTraceEventSource

	// Memory provider source supplies durable memory for prompt retrieval.
	memoryProviders memoryProviderSource

	// Iteration budget factory controls model/tool loop limits.
	iterationBudgets iterationBudgetFactory

	// Plan state store keeps active plan context for the turn.
	plans planStateStore

	// env is a legacy runtime shim for transitional tests and wrappers.
	env any

	// Tool invocation function for executing tool calls during legacy tests and wrappers.
	invokeToolFn any

	// Builds model-visible message context for the turn.
	contextBuilder *ctxbuilder.Builder

	// Base instruction set sent to model.
	instructions instruct.Instructions

	// Per-call guidance from RespondOptions.Instruct.
	requestInstruction instruct.Instruction

	// Durable memory retrieved for the current turn.
	memoryInstruction instruct.Instruction

	// Persisted messages loaded before the turn starts.
	sessionHistory []handmsg.Message

	// Messages emitted during the current turn.
	emittedMessages []handmsg.Message

	// Summary state used for active context assembly.
	summary *summarizer.State

	// Offset represented by sessionHistory[0].
	sessionHistoryOffset int

	// Session ID being read/written to.
	sessionID string

	// State identity, profile, and lineage for this run.
	runCtx runcontext.Context

	// Most recent actual prompt token count.
	lastPromptTokens int

	// Tracks context size last evaluated for summary refresh.
	summaryRefreshAttemptedMessageCount int

	// Indicates if plan state was restored from session history.
	planHydrated bool
}

// NewTurnWithSessionStore constructs a Turn object with reusable session dependencies.
func NewTurnWithSessionStore(
	cfg *config.Config,
	modelClient models.Client,
	summaryClient models.Client,
	summaryStore summarizer.SummaryStore,
	sessionStore agentsession.Store,
	traceRecorder agentsession.TraceRecorder,
	toolRegistry agenttool.Registry,
	toolPolicy agenttool.Policy,
	promptProvider agentprompt.Provider,
	traceSessions traceSessionFactory,
	safetyEvents safetyTraceEventSource,
	memoryProviders memoryProviderSource,
	iterationBudgets iterationBudgetFactory,
	plans planStateStore,
	legacyRuntime any,
	invokeToolFn any,
) *Turn {
	if summaryClient == nil {
		summaryClient = modelClient
	}

	return &Turn{
		cfg:              cfg,
		modelClient:      modelClient,
		summaryClient:    summaryClient,
		summaryStore:     summaryStore,
		sessionStore:     sessionStore,
		traceRecorder:    traceRecorder,
		toolRegistry:     toolRegistry,
		toolPolicy:       toolPolicy,
		promptProvider:   promptProvider,
		traceSessions:    traceSessions,
		safetyEvents:     safetyEvents,
		memoryProviders:  memoryProviders,
		iterationBudgets: iterationBudgets,
		plans:            plans,
		env:              legacyRuntime,
		invokeToolFn:     invokeToolFn,
		contextBuilder:   ctxbuilder.New(),
	}
}

// load initializes all the dependencies, session, summary, instructions, message history,
// and plan state for a new Turn execution. Returns error if required initializations fail.
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

	if !t.hasRuntimeCapabilities() {
		return errors.New("runtime environment is required")
	}

	if t.sessionStore == nil {
		return errors.New("session store is required")
	}

	// Resolve and load session for the turn; fail if not found.
	session, err := t.sessionStore.Resolve(ctx, opts.SessionID)
	if err != nil {
		return err
	}

	if t.summaryService == nil {
		if t.summaryStore == nil {
			return errors.New("summary store is required")
		}
		t.summaryService = summarizer.NewService(t.cfg, t.modelClient, t.summaryClient, t.summaryStore)
	}

	// Load active summary state for context assembly.
	summary, err := t.summaryService.Load(ctx, session.ID)
	if err != nil {
		return err
	}

	// Offset for loading messages, using summary end if available.
	tailOffset := 0
	if summary != nil && summary.Current != nil {
		tailOffset = max(summary.Current.SourceEndOffset, 0)
	}

	// Load messages after the (possibly summarized) offset.
	messages, err := t.sessionStore.GetMessages(ctx, session.ID, agentsession.MessageQuery{Offset: tailOffset})
	if err != nil {
		return err
	}

	// New identity context for session run.
	t.runCtx, err = newRootRunContext(session.ID)
	if err != nil {
		return err
	}

	instructions, err := t.loadBaseInstructions(ctx, session.ID)
	if err != nil {
		return err
	}

	// Assign all loaded state to this Turn instance.
	t.ctx = ctx
	t.instructions = instructions
	t.requestInstruction = instruct.Instruction{}
	t.memoryInstruction = instruct.Instruction{}
	t.sessionHistory = messages
	t.emittedMessages = nil
	t.summary = summary
	t.sessionHistoryOffset = tailOffset
	t.sessionID = session.ID

	t.lastPromptTokens = session.LastPromptTokens
	t.summaryRefreshAttemptedMessageCount = 0

	// Optionally hydrate restored plan from session history.
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

func (t *Turn) loadBaseInstructions(ctx context.Context, sessionID string) (instruct.Instructions, error) {
	if t == nil {
		return nil, nil
	}

	if t.promptProvider != nil {
		instructions, err := t.promptProvider.LoadBaseInstructions(ctx, agentprompt.RunContext{
			SessionID:          sessionID,
			PublicSessionID:    t.runCtx.Session.PublicID,
			EffectiveSessionID: t.runCtx.Session.EffectiveID,
			ProfileName:        t.runCtx.ProfileName,
		})
		if err != nil {
			return nil, err
		}

		return instructionsFromPromptInstructions(instructions), nil
	}

	return nil, nil
}

func (t *Turn) hasRuntimeCapabilities() bool {
	return t != nil &&
		(t.promptProvider != nil ||
			t.traceSessions != nil ||
			t.safetyEvents != nil ||
			t.memoryProviders != nil ||
			t.iterationBudgets != nil ||
			t.plans != nil ||
			t.toolRegistry != nil ||
			t.invokeToolFn != nil ||
			t.env != nil)
}

func instructionsFromPromptInstructions(instructions agentprompt.Instructions) instruct.Instructions {
	if len(instructions) == 0 {
		return nil
	}

	result := make(instruct.Instructions, 0, len(instructions))
	for _, instruction := range instructions {
		result = append(result, instruct.Instruction{
			Name:  instruction.Name,
			Value: instruction.Value,
		})
	}

	return result
}

func (t *Turn) newTraceSessionForRun() trace.Session {
	if t == nil {
		return trace.NoopSession()
	}
	if source, ok := t.env.(traceSessionFactory); ok {
		return source.NewTraceSessionForRun(t.runCtx)
	}
	if t.traceSessions == nil {
		return trace.NoopSession()
	}

	return t.traceSessions.NewTraceSessionForRun(t.runCtx)
}

func (t *Turn) newIterationBudget() envbudget.IterationBudget {
	if t == nil {
		return envbudget.New(0)
	}
	if source, ok := t.env.(iterationBudgetFactory); ok {
		return source.NewIterationBudget()
	}
	if t.iterationBudgets == nil {
		return envbudget.New(0)
	}

	return t.iterationBudgets.NewIterationBudget()
}

func (t *Turn) currentPlan(sessionID string) envtypes.Plan {
	if t == nil {
		return envtypes.Plan{}
	}
	if source, ok := t.env.(planStateStore); ok {
		return source.CurrentPlan(sessionID)
	}
	if t.plans == nil {
		return envtypes.Plan{}
	}

	return t.plans.CurrentPlan(sessionID)
}

func (t *Turn) hydratePlan(sessionID string, plan envtypes.Plan) {
	if t == nil {
		return
	}
	if store, ok := t.env.(planStateStore); ok {
		store.HydratePlan(sessionID, plan)
		return
	}
	if t.plans == nil {
		return
	}

	t.plans.HydratePlan(sessionID, plan)
}

func (t *Turn) legacyToolRegistryAndPolicy() (legacyToolRegistry, tools.Policy, bool) {
	registry, ok := t.legacyToolRegistry()
	if !ok || registry == nil {
		return nil, tools.Policy{}, false
	}

	policy, _ := t.legacyToolPolicy()
	return registry, policy, true
}

func (t *Turn) legacyToolRegistry() (legacyToolRegistry, bool) {
	if t == nil || t.env == nil {
		return nil, false
	}

	method := reflect.ValueOf(t.env).MethodByName("Tools")
	if !method.IsValid() || method.Type().NumIn() != 0 || method.Type().NumOut() != 1 {
		return nil, false
	}

	result := method.Call(nil)[0]
	if !result.IsValid() || result.IsNil() {
		return nil, false
	}

	registry, ok := result.Interface().(legacyToolRegistry)
	return registry, ok
}

func (t *Turn) legacyToolPolicy() (tools.Policy, bool) {
	if t == nil || t.env == nil {
		return tools.Policy{}, false
	}

	method := reflect.ValueOf(t.env).MethodByName("ToolPolicy")
	if !method.IsValid() || method.Type().NumIn() != 0 || method.Type().NumOut() != 1 {
		return tools.Policy{}, false
	}

	policy, ok := method.Call(nil)[0].Interface().(tools.Policy)
	return policy, ok
}

// Run executes the turn's logic, handling instructions, tool actions, tracing,
// safety enforcement, and returns the final assistant reply for this turn.
func (t *Turn) Run(ctx context.Context, msg string, opts RespondOptions) (string, error) {
	// Initialize all turn state and dependencies.
	if err := t.load(ctx, opts); err != nil {
		return "", err
	}

	// Inject per-request instruction if present.
	if requestInstruct := strings.TrimSpace(opts.Instruct); requestInstruct != "" {
		t.requestInstruction = instruct.Instruction{
			Name:  requestInstructionName,
			Value: requestInstruct,
		}
	}

	// Set up trace session for visibility/diagnostics. Include fanout if tracing callback specified.
	traceSession := t.newTraceSessionForRun()
	if opts.OnTraceEvent != nil {
		traceSession = newFanoutTraceSession(traceSession, t.getStateSessionID(), opts.OnTraceEvent)
	}
	defer traceSession.Close()

	// Log content safety events from environment, from history/context.
	t.recordLoadedContentSafety(traceSession)

	// Trace hydrated plan if restored from session/history.
	if t.planHydrated {
		plan := t.currentPlan(t.getStateSessionID())
		traceSession.Record(
			trace.EvtPlanHydrated,
			trace.PlanEventPayload{
				SessionID:    t.getStateSessionID(),
				Steps:        hydratedPlanStepsToTracePayload(plan.Steps),
				Summary:      hydratedPlanSummaryToTracePayload(summarizeHydratedPlan(plan)),
				ActiveStepID: getActiveHydratedPlanStepID(plan),
				Explanation:  strings.TrimSpace(plan.Explanation),
				Source:       "history",
			},
		)
	}

	// Content safety: block & log if user input is not permitted.
	if t.cfg.InputSafetyEnabled() {
		inputSafety := guardrails.CheckInputSafety(msg, "user")
		if inputSafety.Blocked {
			traceSession.Record(trace.EvtInputSafetyBlocked, getInputSafetyTracePayload(t.sessionID, msg, inputSafety))
			return inputSafety.RefusalMessage, nil
		}
	}

	// Compose and emit user's input message to session.
	userMessage, err := handmsg.NewMessage(handmsg.RoleUser, msg)
	if err != nil {
		traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
		return "", err
	}
	t.emittedMessages = append(t.emittedMessages, userMessage)

	// Append user message to session history.
	if err := t.appendSessionMessages([]handmsg.Message{userMessage}); err != nil {
		traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
		return "", err
	}

	traceSession.Record(trace.EvtUserMessageAccepted, trace.UserMessageAcceptedPayload{Message: msg})

	// Retrieve memory instruction for this input.
	t.memoryInstruction = t.retrieveMemoryInstruction(ctx, msg, traceSession)

	// Set up multi-step iteration budget for LLM/tool call loop.
	budget := t.newIterationBudget()

	// Check if streaming is enabled on config/override.
	streamingEnabled := t.cfg.StreamEnabled()
	if opts.Stream != nil {
		streamingEnabled = *opts.Stream
	}

	// Main iteration loop: tries to get a valid response or perform tool actions until budget exhausted.
	runStep := func(ctx context.Context) (agentcore.LoopDecision, error) {
		// Check for context cancellation.
		if err := ctx.Err(); err != nil {
			traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
			return agentcore.LoopDecision{}, err
		}

		// Query available tool definitions for this turn; may vary per session/tool policy.
		availableToolDefinitions, err := t.availableToolDefinitions()
		if err != nil {
			traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
			return agentcore.LoopDecision{}, err
		}

		// Build model request and assemble all prompt-side context for completion.
		request := models.Request{
			Model:         t.cfg.Models.Main.Name,
			APIMode:       t.cfg.Models.Main.APIMode,
			Instructions:  t.buildRequestInstructions(availableToolDefinitions),
			Messages:      t.Context(),
			Tools:         availableToolDefinitions,
			DebugRequests: t.cfg.Debug.Requests,
		}

		// Refresh summary and possibly adjust session context after model request construction.
		t.maybeRefreshSummary(ctx, request, traceSession)

		// Rebuild prompt/context after summary possibly changed.
		request.Instructions = t.buildRequestInstructions(availableToolDefinitions)
		request.Messages = t.Context()

		// Trace summary application and preflight compaction/model events.
		t.summary.RecordSummaryApplied(traceSession)
		recordPreflightCompactionTrace(traceSession, t.cfg, request, t.lastPromptTokens, t.canCompactPersistedHistory())
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

		// --- Make model inference call (streaming or blocking) ---
		var (
			resp               *models.Response
			reasoningStartedAt time.Time
			reasoningEndedAt   time.Time
		)

		if streamingEnabled {
			resp, err = t.modelClient.CompleteStream(ctx, request, func(delta models.StreamDelta) {
				if delta.Text == "" {
					return
				}
				if delta.Channel == models.StreamChannelReasoning {
					now := time.Now().UTC()
					if reasoningStartedAt.IsZero() {
						reasoningStartedAt = now
					}
					reasoningEndedAt = now
				}
				event := Event{Kind: EventKindTextDelta, Channel: string(delta.Channel), Text: delta.Text}
				if opts.OnEvent != nil {
					opts.OnEvent(event)
				}
			})
		} else {
			// Blocking, non-stream model completion.
			resp, err = t.modelClient.Complete(ctx, request)
		}

		// Model request failed or provided no response.
		if err != nil {
			agentLog.Warn().
				Str("event", "model request dispatch failed").
				Str("provider", t.cfg.Models.Main.Provider).
				Str("mode", t.cfg.Models.Main.APIMode).
				Str("model", t.cfg.Models.Main.Name).
				Bool("stream", streamingEnabled).
				Str("error_kind", getAgentModelErrorKind(err)).
				Msg("model request dispatch failed")
			traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
			return agentcore.LoopDecision{}, err
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
			traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
			return agentcore.LoopDecision{}, err
		}

		// Mark model inference timing for diagnostics/storage.
		t.recordModelReasoningCompleted(reasoningStartedAt, reasoningEndedAt)
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

		// Record postflight token usage for usage/analytics.
		if err := t.recordPostflightUsage(traceSession, resp); err != nil {
			return agentcore.LoopDecision{}, err
		}

		// -- Assistant textual reply path (no tool calls required) --
		if !resp.RequiresToolCalls {
			reply := t.applyAssistantOutputSafety(traceSession, resp.OutputText, streamingEnabled)

			assistantMessage, err := handmsg.NewMessage(handmsg.RoleAssistant, reply)
			if err != nil {
				traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
				return agentcore.LoopDecision{}, err
			}

			// Append assistant message to emitted messages.
			t.emittedMessages = append(t.emittedMessages, assistantMessage)

			// Append assistant message to session history.
			if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
				traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
				return agentcore.LoopDecision{}, err
			}

			traceSession.Record(trace.EvtFinalAssistantResponse, trace.FinalAssistantResponsePayload{Message: reply})
			agentLog.Info().
				Str("session_id", t.sessionID).
				Msg("turn completed")

			return agentcore.LoopDecision{Done: true, Reply: reply}, nil
		}

		// -- Tool call required path --

		// If model asks for tool calls, ensure at least one is present.
		if len(resp.ToolCalls) == 0 {
			err = errors.New("model requested tool execution without tool calls")
			traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
			return agentcore.LoopDecision{}, err
		}

		// Represent tool call(s) as assistant message in session.
		assistantMessage, err := assistantToolCallMessageFromResponse(resp)
		if err != nil {
			traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
			return agentcore.LoopDecision{}, err
		}

		// Append assistant message to emitted messages.
		t.emittedMessages = append(t.emittedMessages, assistantMessage)
		if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
			traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
			return agentcore.LoopDecision{}, err
		}

		// For each tool call: execute, trace, and record as tool message.
		for _, toolCall := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
				return agentcore.LoopDecision{}, err
			}

			// Start tool invocation logging.
			agentLog.Info().
				Str("event", "tool invocation started").
				Str("relationship", "tool_call_from_current_model_response").
				Str("tool", toolCall.Name).
				Str("tool_call_id", toolCall.ID).
				Msg("tool invocation started")

			traceSession.Record(trace.EvtToolInvocationStarted, trace.ToolInvocationStartedPayload{
				ID:           toolCall.ID,
				Name:         toolCall.Name,
				Input:        toolCall.Input,
				PlanState:    getPlanToolInputState(toolCall.Name, toolCall.Input),
				ProcessState: getProcessToolInputState(toolCall.Name, toolCall.Input),
			})

			// Invoke tool and get tool message.
			toolCtx := tools.WithTraceRecorder(t.getToolContext(ctx), traceSession)
			toolMessage := t.invokeTool(toolCtx, toolCall)

			traceSession.Record(trace.EvtToolInvocationCompleted, trace.ToolInvocationCompletedPayload{
				ToolCallID:   toolMessage.ToolCallID,
				Name:         toolMessage.Name,
				Content:      toolMessage.Content,
				PlanState:    getPlanToolOutputState(toolMessage.Name, toolMessage.Content),
				ProcessState: getProcessToolOutputState(toolMessage.Name, toolMessage.Content),
			})

			agentLog.Info().
				Str("event", "tool invocation completed").
				Str("relationship", "tool_result_for_current_model_response").
				Str("tool", toolCall.Name).
				Str("tool_call_id", toolCall.ID).
				Int("output_chars", len([]rune(toolMessage.Content))).
				Int("output_bytes", len(toolMessage.Content)).
				Msg("tool invocation completed")

			// Normalize tool message and check for serialization/safety invariants.
			toolMessage, err = normalizeTurnMessage(toolMessage)
			if err != nil {
				traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
				return agentcore.LoopDecision{}, err
			}

			t.emittedMessages = append(t.emittedMessages, toolMessage)
			if err := t.appendSessionMessages([]handmsg.Message{toolMessage}); err != nil {
				traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
				return agentcore.LoopDecision{}, err
			}
		}
		return agentcore.LoopDecision{}, nil
	}

	return agentcore.RunModelToolLoop(ctx, agentcore.ModelToolLoopOptions{
		Consume: budget.Consume,
		RunStep: runStep,
		OnExhausted: func(ctx context.Context) (string, error) {
			// If iteration budget is exhausted, fallback to summary-based result and finish.
			agentLog.Warn().
				Str("session_id", t.sessionID).
				Msg("iteration budget exhausted, falling back to summary")

			return t.summaryFallback(ctx, budget, traceSession)
		},
	})
}

// trimSessionHistoryToSummary trims session history up to summary end offset.
// Used after summary compaction and context refresh events.
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

// maybeRefreshSummary refreshes summary context if the message count has grown since last summary attempt.
// It may update summary state and trims session history if needed.
func (t *Turn) maybeRefreshSummary(ctx context.Context, request models.Request, traceSession trace.Session) {
	if t == nil || t.summaryService == nil {
		return
	}

	messageCount := len(request.Messages)
	if messageCount <= 0 || messageCount <= t.summaryRefreshAttemptedMessageCount {
		return
	}

	// Flush memory before compaction.
	t.maybeFlushMemoryBeforeCompaction(ctx, request, traceSession)

	// Maybe refresh summary.
	previousHistoryOffset := t.sessionHistoryOffset
	t.summaryRefreshAttemptedMessageCount = messageCount
	_ = t.summaryService.MaybeRefreshSummary(ctx, t.summary, summarizer.RefreshInput{
		LastPromptTokens: t.lastPromptTokens,
		Request:          request,
		SessionID:        t.sessionID,
		TraceSession:     traceSession,
	})

	t.trimSessionHistoryToSummary()

	// Reset prompt tokens if session history offset changed.
	if t.sessionHistoryOffset != previousHistoryOffset {
		t.lastPromptTokens = 0
	}
}

// canCompactPersistedHistory returns true if the session history is large enough to be compacted.
func (t *Turn) canCompactPersistedHistory() bool {
	if t == nil {
		return false
	}
	return len(t.sessionHistory) > t.cfg.CompactionRecentSessionTailEffective()
}

// appendSessionMessages persists new emitted messages to the session state.
func (t *Turn) appendSessionMessages(messages []handmsg.Message) error {
	return t.sessionStore.AppendMessages(t.ctx, t.sessionID, messages)
}

// applyAssistantOutputSafety performs output safety checks on assistant responses if enabled.
// For streaming, checks are deferred to client side.
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

// recordModelReasoningCompleted logs and saves model inference duration for a turn.
func (t *Turn) recordModelReasoningCompleted(startedAt time.Time, endedAt time.Time) {
	if t == nil || t.traceRecorder == nil || t.sessionID == "" || startedAt.IsZero() {
		return
	}
	if endedAt.IsZero() || endedAt.Before(startedAt) {
		endedAt = startedAt
	}

	duration := max(endedAt.Sub(startedAt).Round(time.Millisecond), time.Second)

	event := agentsession.TraceEvent{
		SessionID: t.sessionID,
		Type:      trace.EvtModelReasoningCompleted,
		Timestamp: endedAt,
		Payload:   trace.ModelReasoningCompletedPayload{DurationMS: duration.Milliseconds()},
	}
	if _, err := t.traceRecorder.AppendTraceEvent(t.ctx, event); err != nil {
		if !errors.Is(err, storage.ErrTraceStoreUnsupported) {
			agentLog.Warn().
				Err(err).
				Str("event", trace.EvtModelReasoningCompleted).
				Str("session_id", t.sessionID).
				Msg("failed to persist reasoning completion trace")
		}
		return
	}

	agentLog.Debug().
		Str("event", trace.EvtModelReasoningCompleted).
		Str("session_id", t.sessionID).
		Int64("duration_ms", duration.Milliseconds()).
		Msg("model reasoning completed")
}

// getOutputRedactor returns a redactor instance configured for the session and output PII settings.
func (t *Turn) getOutputRedactor() guardrails.Redactor {
	if t == nil || t.cfg == nil {
		return guardrails.NewRedactorWithOptions(guardrails.RedactorOptions{DisablePII: true})
	}
	return guardrails.NewRedactorWithOptions(guardrails.RedactorOptions{
		DisablePII: !t.cfg.OutputPIIRedactionEnabled(),
	})
}

// recordLoadedContentSafety emits trace events for any content safety violations loaded for this session.
func (t *Turn) recordLoadedContentSafety(traceSession trace.Session) {
	if t == nil || traceSession == nil {
		return
	}

	source, _ := t.env.(safetyTraceEventSource)
	if source == nil {
		source = t.safetyEvents
	}
	if source == nil {
		return
	}

	for _, event := range source.SafetyTraceEvents() {
		traceSession.Record(trace.EvtLoadedContentSafetyBlocked, safetyEventPayloadFromOptions(event))
	}
}

// getInputSafetyTracePayload builds a standardized payload for a blocked input safety event.
func getInputSafetyTracePayload(sessionID string, content string, result guardrails.InputSafetyResult) trace.SafetyEventPayload {
	return safetyEventPayloadFromOptions(guardrails.SafetyTracePayloadOptions{
		SessionID:     sessionID,
		Source:        "user",
		Action:        "blocked",
		ContentLength: len([]rune(content)),
		Blocked:       result.Blocked,
		Findings:      result.Findings,
		Refusal:       result.RefusalMessage,
	})
}

// getOutputSafetyTracePayload builds a standardized payload for assistant output safety check results.
func getOutputSafetyTracePayload(sessionID string, content string, result guardrails.OutputSafetyResult) trace.SafetyEventPayload {
	action := "redacted"
	if result.Blocked {
		action = "blocked"
	}
	return safetyEventPayloadFromOptions(guardrails.SafetyTracePayloadOptions{
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

// safetyEventPayloadFromOptions normalizes guardrail trace payloads for logging/tracing.
func safetyEventPayloadFromOptions(opts guardrails.SafetyTracePayloadOptions) trace.SafetyEventPayload {
	return trace.SafetyEventPayload{
		SessionID:     strings.TrimSpace(opts.SessionID),
		Source:        strings.TrimSpace(opts.Source),
		Action:        strings.TrimSpace(opts.Action),
		ContentLength: opts.ContentLength,
		Blocked:       opts.Blocked,
		Redacted:      opts.Redacted,
		Refusal:       strings.TrimSpace(opts.Refusal),
		Findings:      guardrails.SafetyFindingLogFields(opts.Findings),
	}
}

// getPlanToolInputState returns plan tool state representation for tracing if name/type matches.
func getPlanToolInputState(name string, input string) *trace.PlanToolState {
	if strings.TrimSpace(strings.ToLower(name)) != "plan_tool" {
		return nil
	}
	return trace.PlanToolInputState(input)
}

// getPlanToolOutputState returns plan tool output state for tracing where matched.
func getPlanToolOutputState(name string, output string) *trace.PlanToolState {
	if strings.TrimSpace(strings.ToLower(name)) != "plan_tool" {
		return nil
	}
	return trace.PlanToolOutputState(output)
}

// getProcessToolInputState returns process tool input state for tracing where matched.
func getProcessToolInputState(name string, input string) *trace.ProcessToolState {
	if strings.TrimSpace(strings.ToLower(name)) != "process" {
		return nil
	}
	return trace.ProcessToolInputState(input)
}

// getProcessToolOutputState returns process tool output state for tracing where matched.
func getProcessToolOutputState(name string, output string) *trace.ProcessToolState {
	if strings.TrimSpace(strings.ToLower(name)) != "process" {
		return nil
	}
	return trace.ProcessToolOutputState(output)
}

// getStateSessionID reports the canonical session ID for trace/state operations for this turn.
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

// getToolContext produces a context carrying the relevant session/run metadata for tools.
func (t *Turn) getToolContext(ctx context.Context) context.Context {
	if t == nil {
		return tools.WithSessionID(ctx, "")
	}
	if strings.TrimSpace(t.runCtx.Session.PublicID) != "" {
		return tools.WithRunContext(ctx, t.runCtx)
	}
	return tools.WithSessionID(ctx, t.sessionID)
}

// availableToolDefinitions resolves tool definitions available for this turn, using environment/tool policy.
func (t *Turn) availableToolDefinitions() ([]models.ToolDefinition, error) {
	if t == nil {
		return nil, nil
	}

	if registry, policy, ok := t.legacyToolRegistryAndPolicy(); ok {
		definitions, err := registry.Resolve(policy)
		if err != nil {
			return nil, err
		}

		toolsList := make([]models.ToolDefinition, 0, len(definitions))
		for _, definition := range definitions {
			toolsList = append(toolsList, modelToolDefinitionFromToolDefinition(definition))
		}
		return toolsList, nil
	}

	if t.toolRegistry != nil {
		definitions, err := t.toolRegistry.Resolve(t.toolPolicy)
		if err != nil {
			return nil, err
		}

		return agenttool.DefinitionsToModel(definitions), nil
	}

	return nil, nil
}

// invokeTool executes a tool call, optionally using turn's tool invocation handler.
func (t *Turn) invokeTool(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
	if t == nil {
		return handmsg.Message{
			Role:       handmsg.RoleTool,
			Name:       toolCall.Name,
			ToolCallID: toolCall.ID,
			Content:    `{"error":"tool invocation is required"}`,
		}
	}

	if t.invokeToolFn != nil {
		if message, ok := t.invokeToolWithLegacyHook(ctx, toolCall); ok {
			return message
		}
	}

	if t.toolRegistry != nil {
		return t.toolRegistry.Invoke(ctx, agenttool.CallFromModel(toolCall))
	}

	if registry, _, ok := t.legacyToolRegistryAndPolicy(); ok {
		return t.invokeToolWithLegacyRuntime(ctx, registry, toolCall)
	}

	return handmsg.Message{
		Role:       handmsg.RoleTool,
		Name:       toolCall.Name,
		ToolCallID: toolCall.ID,
		Content:    `{"error":"tool invocation is required"}`,
	}
}

func (t *Turn) invokeToolWithLegacyRuntime(
	ctx context.Context,
	registry legacyToolRegistry,
	toolCall models.ToolCall,
) handmsg.Message {
	result := map[string]any{"name": toolCall.Name}
	if registry == nil {
		result["error"] = "tool registry is required"
		return toolResultMessage(toolCall, result)
	}

	toolResult, err := registry.Invoke(ctx, tools.Call{
		Name:   toolCall.Name,
		Input:  toolCall.Input,
		Source: "model",
	})
	if err != nil {
		result["error"] = err.Error()
	}
	if strings.TrimSpace(toolResult.Error) != "" {
		result["error"] = normalizeToolError(strings.TrimSpace(toolResult.Error))
	}
	if strings.TrimSpace(toolResult.Output) != "" {
		result["output"] = sanitizeToolOutputForModel(ctx, toolCall.Name, toolResult.Output, t.cfg)
	}

	return toolResultMessage(toolCall, result)
}

func (t *Turn) invokeToolWithLegacyHook(ctx context.Context, toolCall models.ToolCall) (handmsg.Message, bool) {
	switch invoke := t.invokeToolFn.(type) {
	case func(context.Context, models.ToolCall) handmsg.Message:
		return invoke(ctx, toolCall), true
	}

	value := reflect.ValueOf(t.invokeToolFn)
	if !value.IsValid() || value.Kind() != reflect.Func || value.Type().NumIn() != 3 || value.Type().NumOut() != 1 {
		return handmsg.Message{}, false
	}
	if !value.Type().Out(0).AssignableTo(reflect.TypeOf(handmsg.Message{})) {
		return handmsg.Message{}, false
	}

	args := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.Zero(value.Type().In(1)),
		reflect.ValueOf(toolCall),
	}
	if t.env != nil {
		envValue := reflect.ValueOf(t.env)
		if envValue.Type().AssignableTo(value.Type().In(1)) {
			args[1] = envValue
		}
	}

	return value.Call(args)[0].Interface().(handmsg.Message), true
}

// summaryFallback runs the fallback summary request and returns the assistant reply
// when iteration budget was exhausted and a summary response is needed for completion.
func (t *Turn) summaryFallback(ctx context.Context, budget envbudget.IterationBudget, traceSession trace.Session) (string, error) {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
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

	traceSession.Record(
		trace.EvtSummaryFallbackStarted,
		trace.SummaryFallbackStartedPayload{RemainingIterations: budget.Remaining()},
	)
	t.summary.RecordSummaryApplied(traceSession)
	recordPreflightCompactionTrace(traceSession, t.cfg, request, t.lastPromptTokens, t.canCompactPersistedHistory())
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
		traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: wrapped.Error()})
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
		traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
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
		traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
		return "", err
	}

	reply := t.applyAssistantOutputSafety(traceSession, resp.OutputText, false)

	assistantMessage, err := handmsg.NewMessage(handmsg.RoleAssistant, reply)
	if err != nil {
		traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
		return "", err
	}

	t.emittedMessages = append(t.emittedMessages, assistantMessage)
	if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
		traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
		return "", err
	}

	traceSession.Record(trace.EvtFinalAssistantResponse, trace.FinalAssistantResponsePayload{Message: reply})
	return reply, nil
}

// getAgentModelErrorKind standardizes error kind reporting for model errors.
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

// buildRequestInstructions assembles the system prompt/instructions for the model in recommended order.
func (t *Turn) buildRequestInstructions(
	activeToolDefinitions []models.ToolDefinition,
	extra ...instruct.Instructions,
) string {
	if t == nil {
		return ""
	}

	instructions := t.instructions

	// Plan context: prepend plan instructions and policy if present.
	if planInstructions := t.renderPlanInstructions(); planInstructions != "" {
		if policy, ok := instructions.GetByName(instruct.PlanningPolicyInstructionName); ok {
			instructions = instruct.New(policy.Value).
				Append(instruct.Instruction{Value: planInstructions}).
				Append(instructions.WithoutName(instruct.PlanningPolicyInstructionName)...)
		} else {
			instructions = instruct.New(planInstructions).Append(instructions...)
		}
	}

	// Add any summary instructions from summary state.
	if t.summary != nil {
		if summaryInstructions, ok := t.summary.RenderSummaryInstructions(); ok {
			instructions = instructions.Append(instruct.Instruction{Value: summaryInstructions})
		}
	}

	// Add memory retrieved for this turn.
	instructions = instructions.Append(t.memoryInstruction)

	// Add environment context (tools, capabilities).
	environmentContext := t.buildEnvironmentContextInstruction(activeToolDefinitions)
	instructions = instructions.Append(environmentContext)

	// Add single-turn request instruction if provided.
	instructions = instructions.Append(t.requestInstruction)

	// Add any final extra instruction blocks (e.g. fallback summary).
	for _, block := range extra {
		instructions = instructions.Append(block...)
	}

	return instructions.String()
}

// newRootRunContext creates a normalized run context from the session ID and active profile (if any).
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

// Context rebuilds prompt-visible message context for a model turn, combining summary/prefix/history/emitted.
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

// recordPostflightUsage persists post-model token usage for analytics and session tracking.
func (t *Turn) recordPostflightUsage(traceSession trace.Session, resp *models.Response) error {
	if t == nil || resp == nil || resp.PromptTokens <= 0 {
		return nil
	}

	t.lastPromptTokens = resp.PromptTokens
	if err := t.sessionStore.UpdateLastPromptTokens(t.ctx, t.sessionID, resp.PromptTokens); err != nil {
		traceSession.Record(trace.EvtSessionFailed, trace.SessionFailedPayload{Error: err.Error()})
		return err
	}

	traceSession.Record(trace.EvtContextPostflightUsage, trace.ContextEventPayload{
		Source:           compaction.ActualSource,
		PromptTokens:     resp.PromptTokens,
		CompletionTokens: resp.CompletionTokens,
		TotalTokens:      resp.TotalTokens,
	})

	return nil
}

// Messages returns copies of all assistant and tool messages emitted during this turn.
// Used in testing and downstream consumers.
func (t *Turn) Messages() []handmsg.Message {
	if len(t.emittedMessages) == 0 {
		return nil
	}

	messages := make([]handmsg.Message, len(t.emittedMessages))
	copy(messages, t.emittedMessages)

	return messages
}

// normalizeTurnMessage calls handmsg.NormalizeMessage to check/standardize a turn message for correctness.
func normalizeTurnMessage(message handmsg.Message) (handmsg.Message, error) {
	return handmsg.NormalizeMessage(message)
}
