package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wandxy/hand/internal/agent/compaction"
	ctxbuilder "github.com/wandxy/hand/internal/agent/context"
	agentmemory "github.com/wandxy/hand/internal/agent/memory"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/storage"
	"github.com/wandxy/hand/internal/trace"
)

// Turn executes a single response turn against a resolved session.
type Turn struct {
	// ctx is the request context used for session writes during the turn.
	ctx context.Context
	// cfg provides model and execution settings for the turn.
	cfg *config.Config
	// modelClient executes model requests for the turn.
	modelClient models.Client
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
}

// NewTurn constructs a Turn with the dependencies needed for one response turn.
func NewTurn(
	cfg *config.Config,
	modelClient models.Client,
	sessionManager *sessionstore.Manager,
	invokeToolFn func(context.Context, environment.Environment, models.ToolCall) handmsg.Message,
	runtimeEnv environment.Environment,
) *Turn {
	return &Turn{
		cfg:            cfg,
		modelClient:    modelClient,
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
		t.memoryService = agentmemory.NewService(t.cfg, t.modelClient, t.sessionManager)
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
		t.instructions = setInstruction(t.instructions, instruct.Instruction{Name: requestInstructionName, Value: requestInstruct})
		defer func() {
			t.instructions = t.instructions.WithoutName(requestInstructionName)
		}()
	}

	traceSession := t.env.NewTraceSession(t.sessionID)
	defer traceSession.Close()

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
			Instructions:  t.buildRequestInstructions(),
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
		request.Instructions = t.buildRequestInstructions()
		request.Messages = t.Context()
		t.memory.RecordSummaryApplied(traceSession)
		recordPreflightCompactionTrace(traceSession, t.cfg, request, t.lastPromptTokens)
		traceSession.Record(trace.EvtModelRequest, request)

		agentLog.Debug().
			Str("model", t.cfg.Model).
			Int("context_messages", len(request.Messages)).
			Int("tools", len(request.Tools)).
			Msg("sending model request")

		resp, err := t.modelClient.Complete(ctx, request)
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
			toolMessage := t.invokeTool(ctx, toolCall)
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
		Instructions:  t.buildRequestInstructions(instruct.BuildSummary(budget.Remaining())),
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

// buildRequestInstructions builds the system prompt sent to the model: optional persisted session
// summary, then the turn's base instructions, then any extra instruction blocks.
func (t *Turn) buildRequestInstructions(extra ...instruct.Instructions) string {
	if t == nil {
		return ""
	}

	instructions := t.instructions
	if t.memory != nil {
		if summaryInstructions, ok := t.memory.RenderSummaryInstructions(); ok {
			instructions = instruct.New(summaryInstructions).Append(instructions...)
		}
	}
	for _, block := range extra {
		instructions = instructions.Append(block...)
	}

	return instructions.String()
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
