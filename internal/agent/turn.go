package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wandxy/hand/internal/agent/compaction"
	ctxbuilder "github.com/wandxy/hand/internal/agent/context"
	agentmemory "github.com/wandxy/hand/internal/agent/memory"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/storage"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

const planHydrationPageSize = 10

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
	// sessionManager resolves sessions and persists turn messages.
	sessionManager *sessionstore.Manager
	// memoryService loads and refreshes persisted memory state for the turn.
	memoryService *agentmemory.Service
	// invokeToolFn performs tool execution for requested tool calls.
	invokeToolFn func(context.Context, environment.Environment, models.ToolCall) handmsg.Message
	// env supplies tools, instructions, tracing, and iteration budget.
	env environment.Environment
	// contextBuilder assembles the model-visible message context for the turn.
	contextBuilder *ctxbuilder.Builder
	// instructions is the request-scoped instruction set sent to the model.
	instructions instruct.Instructions
	// sessionHistory contains persisted messages loaded before the turn starts.
	sessionHistory []handmsg.Message
	// emittedMessages contains messages produced during the current turn.
	emittedMessages []handmsg.Message
	// memory contains persisted memory state used in active context assembly.
	memory *agentmemory.Memory
	// sessionHistoryOffset is the absolute persisted offset represented by sessionHistory[0].
	sessionHistoryOffset int
	// sessionID identifies the session being read from and written to.
	sessionID string
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
	sessionManager *sessionstore.Manager,
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
		sessionManager: sessionManager,
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

	if t.sessionManager == nil {
		return errors.New("session manager is required")
	}

	if t.memoryService == nil {
		t.memoryService = agentmemory.NewService(t.cfg, t.modelClient, t.summaryClient, t.sessionManager)
	}

	session, err := t.sessionManager.Resolve(ctx, opts.SessionID)
	if err != nil {
		return err
	}

	memory, err := t.memoryService.Load(ctx, session.ID)
	if err != nil {
		return err
	}

	tailOffset := 0
	if memory != nil && memory.Summary != nil {
		tailOffset = max(memory.Summary.SourceEndOffset, 0)
	}

	messages, err := t.sessionManager.GetMessages(ctx, session.ID, storage.MessageQueryOptions{Offset: tailOffset})
	if err != nil {
		return err
	}
	t.ctx = ctx
	t.instructions = t.env.Instructions()
	t.sessionHistory = messages
	t.emittedMessages = nil
	t.memory = memory
	t.sessionHistoryOffset = tailOffset
	t.sessionID = session.ID
	t.lastPromptTokens = session.LastPromptTokens
	t.summaryRefreshAttempted = false
	t.planHydrated, err = t.hydratePlanFromHistory(ctx, session.ID)
	if err != nil {
		return err
	}

	agentLog.Debug().
		Str("session_id", session.ID).
		Int("history_offset", tailOffset).
		Int("history_messages", len(messages)).
		Msg("turn loaded")

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

	traceSession := t.env.NewTraceSession(t.sessionID)
	defer traceSession.Close()
	if t.planHydrated {
		plan := t.env.CurrentPlan(t.sessionID)
		traceSession.Record(trace.EvtPlanHydrated, map[string]any{
			"session_id":     t.sessionID,
			"steps":          plan.Steps,
			"summary":        summarizeHydratedPlan(plan),
			"active_step_id": activeHydratedPlanStepID(plan),
			"explanation":    strings.TrimSpace(plan.Explanation),
			"source":         "history",
		})
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
			Model:         t.cfg.Model,
			APIMode:       t.cfg.ModelAPIMode,
			Instructions:  t.buildRequestInstructions(availableToolDefinitions),
			Messages:      t.Context(),
			Tools:         availableToolDefinitions,
			DebugRequests: t.cfg.DebugRequests,
		}

		if !t.summaryRefreshAttempted {
			t.summaryRefreshAttempted = true
			_ = t.memoryService.MaybeRefreshMemory(ctx, t.memory, agentmemory.RefreshInput{
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
		t.memory.RecordSummaryApplied(traceSession)
		recordPreflightCompactionTrace(traceSession, t.cfg, request, t.lastPromptTokens)
		traceSession.Record(trace.EvtModelRequest, request)

		agentLog.Debug().
			Str("model", t.cfg.Model).
			Int("context_messages", len(request.Messages)).
			Int("tools", len(request.Tools)).
			Msg("sending model request")

		var resp *models.Response
		if streamingEnabled {
			deltas := make([]Event, 0, 16)
			allowLiveDelivery := opts.OnEvent != nil
			resp, err = t.modelClient.CompleteStream(ctx, request, func(delta models.StreamDelta) {
				if delta.Text == "" {
					return
				}
				event := Event{Channel: string(delta.Channel), Text: delta.Text}
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
			traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
			return "", err
		}

		if resp == nil {
			err = errors.New("model response is required")
			traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
			return "", err
		}

		traceSession.Record(trace.EvtModelResponse, resp)

		agentLog.Debug().
			Int("prompt_tokens", resp.PromptTokens).
			Int("completion_tokens", resp.CompletionTokens).
			Bool("requires_tool_calls", resp.RequiresToolCalls).
			Msg("model response received")

		if err := t.recordPostflightUsage(traceSession, resp); err != nil {
			return "", err
		}

		if !resp.RequiresToolCalls {
			assistantMessage, err := handmsg.NewMessage(handmsg.RoleAssistant, resp.OutputText)
			if err != nil {
				traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
				return "", err
			}

			t.emittedMessages = append(t.emittedMessages, assistantMessage)
			if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
				traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
				return "", err
			}

			traceSession.Record(trace.EvtFinalAssistantResponse, map[string]any{"message": resp.OutputText})

			agentLog.Info().Str("session_id", t.sessionID).Msg("turn completed")

			return resp.OutputText, nil
		}

		if len(resp.ToolCalls) == 0 {
			err = errors.New("model requested tool execution without tool calls")
			traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
			return "", err
		}

		assistantMessage := handmsg.Message{
			Role:      handmsg.RoleAssistant,
			Content:   strings.TrimSpace(resp.OutputText),
			ToolCalls: toContextToolCalls(resp.ToolCalls),
		}

		assistantMessage, err = normalizeTurnMessage(assistantMessage)
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

			agentLog.Debug().Str("tool", toolCall.Name).Str("tool_call_id", toolCall.ID).Msg("invoking tool")

			traceSession.Record(trace.EvtToolInvocationStarted, toolCall)
			toolCtx := tools.WithTraceRecorder(tools.WithSessionID(ctx, t.sessionID), traceSession)
			toolMessage := t.invokeTool(toolCtx, toolCall)
			traceSession.Record(trace.EvtToolInvocationCompleted, toolMessage)

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
	if t == nil || t.memory == nil || t.memory.Summary == nil {
		return
	}

	targetOffset := max(t.memory.Summary.SourceEndOffset, 0)
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
	return t.sessionManager.AppendMessages(t.ctx, t.sessionID, messages)
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
		toolsList = append(toolsList, models.ToolDefinition{
			Name:        definition.Name,
			Description: definition.Description,
			InputSchema: definition.InputSchema,
		})
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

func (t *Turn) summaryFallback(ctx context.Context, budget environment.IterationBudget, traceSession trace.Session) (string, error) {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	request := models.Request{
		Model:         t.cfg.Model,
		APIMode:       t.cfg.ModelAPIMode,
		Instructions:  t.buildRequestInstructions(nil, instruct.BuildSummary(budget.Remaining())),
		Messages:      t.Context(),
		Tools:         nil,
		DebugRequests: t.cfg.DebugRequests,
	}

	traceSession.Record(trace.EvtSummaryFallbackStarted, map[string]any{"remaining_iterations": budget.Remaining()})
	t.memory.RecordSummaryApplied(traceSession)
	recordPreflightCompactionTrace(traceSession, t.cfg, request, t.lastPromptTokens)
	traceSession.Record(trace.EvtModelRequest, request)

	resp, err := t.modelClient.Complete(ctx, request)
	if err != nil {
		agentLog.Error().Err(err).Str("session_id", t.sessionID).Msg("summary fallback model call failed")
		wrapped := fmt.Errorf("iteration limit reached and summary failed: %w", err)
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": wrapped.Error()})
		return "", wrapped
	}

	if resp == nil {
		err = errors.New("model response is required")
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	traceSession.Record(trace.EvtModelResponse, resp)
	if err := t.recordPostflightUsage(traceSession, resp); err != nil {
		return "", err
	}

	if resp.RequiresToolCalls {
		err = fmt.Errorf("iteration limit reached and summary requested more tools")
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	assistantMessage, err := handmsg.NewMessage(handmsg.RoleAssistant, resp.OutputText)
	if err != nil {
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	t.emittedMessages = append(t.emittedMessages, assistantMessage)
	if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
		traceSession.Record(trace.EvtSessionFailed, map[string]any{"error": err.Error()})
		return "", err
	}

	traceSession.Record(trace.EvtFinalAssistantResponse, map[string]any{"message": resp.OutputText})
	return resp.OutputText, nil
}

// buildRequestInstructions assembles the system prompt sent to the model in this order:
// planning policy, hydrated active plan context, remaining base instructions, optional
// persisted memory summary, optional environment context, optional request-scoped
// instruction, then any extra blocks.
func (t *Turn) buildRequestInstructions(
	activeToolDefinitions []models.ToolDefinition,
	extra ...instruct.Instructions,
) string {
	if t == nil {
		return ""
	}

	instructions := t.instructions

	// Hold request-scoped guidance aside so it can be appended after memory summary.
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

	// Add persisted memory summary after base instructions and plan context.
	if t.memory != nil {
		if summaryInstructions, ok := t.memory.RenderSummaryInstructions(); ok {
			instructions = instructions.Append(instruct.Instruction{Value: summaryInstructions})
		}
	}

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

		t.env.HydratePlan(t.sessionID, plan)
		return true
	}

	t.env.HydratePlan(t.sessionID, empty)
	return false
}

func (t *Turn) renderPlanInstructions() string {
	if t == nil || t.env == nil {
		return ""
	}

	plan := t.env.CurrentPlan(t.sessionID)
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
		messages, err := t.sessionManager.GetMessages(ctx, sessionID, storage.MessageQueryOptions{
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

func activeHydratedPlanStepID(plan envtypes.Plan) string {
	for _, step := range plan.Steps {
		if step.Status == envtypes.PlanStatusInProgress {
			return step.ID
		}
	}
	return ""
}

func (t *Turn) Context() []handmsg.Message {
	builder := t.contextBuilder
	if builder == nil {
		builder = ctxbuilder.New()
	}

	recall := t.memory.Recall(t.sessionHistory)

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
	if err := t.sessionManager.UpdateLastPromptTokens(t.ctx, t.sessionID, resp.PromptTokens); err != nil {
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
