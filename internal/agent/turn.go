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
	// invokeToolFn performs tool execution for requested tool calls.
	invokeToolFn func(context.Context, executionEnvironment, models.ToolCall) handmsg.Message
	// runtimeEnv supplies tools, instructions, tracing, and iteration budget.
	runtimeEnv executionEnvironment
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
	invokeToolFn func(context.Context, executionEnvironment, models.ToolCall) handmsg.Message,
	runtimeEnv executionEnvironment,
) *Turn {
	return &Turn{
		cfg:            cfg,
		modelClient:    modelClient,
		sessionManager: sessionManager,
		invokeToolFn:   invokeToolFn,
		runtimeEnv:     runtimeEnv,
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

	if t.runtimeEnv == nil {
		return errors.New("runtime environment is required")
	}

	if t.sessionManager == nil {
		return errors.New("session manager is required")
	}

	session, err := t.sessionManager.Resolve(ctx, opts.SessionID)
	if err != nil {
		return err
	}

	messages, err := t.sessionManager.GetMessages(ctx, session.ID, storage.MessageQueryOptions{})
	if err != nil {
		return err
	}

	summary, _, err := t.sessionManager.GetSummary(ctx, session.ID)
	if err != nil {
		return err
	}

	t.ctx = ctx
	t.instructions = t.runtimeEnv.Instructions()
	t.sessionHistory = messages
	t.emittedMessages = nil
	t.memory = &agentmemory.Memory{Summary: agentmemory.SummaryFromStorage(summary)}
	t.sessionID = session.ID
	t.lastPromptTokens = session.LastPromptTokens
	t.summaryRefreshAttempted = false

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

	traceSession := t.runtimeEnv.NewTraceSession()
	defer traceSession.Close()

	userMessage, err := handmsg.NewMessage(handmsg.RoleUser, msg)
	if err != nil {
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	t.emittedMessages = append(t.emittedMessages, userMessage)
	if err := t.appendSessionMessages([]handmsg.Message{userMessage}); err != nil {
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	traceSession.Record("user.message.accepted", map[string]any{"message": msg})

	budget := t.runtimeEnv.NewIterationBudget()
	for budget.Consume() {
		if err := ctx.Err(); err != nil {
			traceSession.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		availableToolDefinitions, err := t.availableToolDefinitions()
		if err != nil {
			traceSession.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		request := models.Request{
			Model:         t.cfg.Model,
			APIMode:       t.cfg.ModelAPIMode,
			Instructions:  t.instructions.String(),
			Messages:      t.Context(),
			Tools:         availableToolDefinitions,
			DebugRequests: t.cfg.DebugRequests,
		}

		if !t.summaryRefreshAttempted {
			t.summaryRefreshAttempted = true
			_ = t.memory.MaybeRefreshSummary(ctx, agentmemory.SummaryRefreshInput{
				Config:           t.cfg,
				LastPromptTokens: t.lastPromptTokens,
				ModelClient:      t.modelClient,
				Request:          request,
				SessionHistory:   t.sessionHistory,
				SessionID:        t.sessionID,
				SummaryStore:     t.sessionManager,
				TraceSession:     traceSession,
			})
		}

		request.Messages = t.Context()
		t.memory.RecordSummaryApplied(traceSession)
		recordPreflightCompactionTrace(traceSession, t.cfg, request, t.lastPromptTokens)
		traceSession.Record("model.request", request)

		resp, err := t.modelClient.Chat(ctx, request)
		if err != nil {
			traceSession.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		if resp == nil {
			err = errors.New("model response is required")
			traceSession.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		traceSession.Record("model.response", resp)
		if err := t.recordPostflightUsage(traceSession, resp); err != nil {
			return "", err
		}

		if !resp.RequiresToolCalls {
			assistantMessage, err := handmsg.NewMessage(handmsg.RoleAssistant, resp.OutputText)
			if err != nil {
				traceSession.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}

			t.emittedMessages = append(t.emittedMessages, assistantMessage)
			if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
				traceSession.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}

			traceSession.Record("final.assistant.response", map[string]any{"message": resp.OutputText})
			return resp.OutputText, nil
		}

		if len(resp.ToolCalls) == 0 {
			err = errors.New("model requested tool execution without tool calls")
			traceSession.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		assistantMessage := handmsg.Message{
			Role:      handmsg.RoleAssistant,
			Content:   strings.TrimSpace(resp.OutputText),
			ToolCalls: toContextToolCalls(resp.ToolCalls),
		}

		assistantMessage, err = normalizeTurnMessage(assistantMessage)
		if err != nil {
			traceSession.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		t.emittedMessages = append(t.emittedMessages, assistantMessage)

		if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
			traceSession.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		for _, toolCall := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				traceSession.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}

			traceSession.Record("tool.invocation.started", toolCall)
			toolMessage := t.invokeTool(ctx, toolCall)
			traceSession.Record("tool.invocation.completed", toolMessage)

			toolMessage, err = normalizeTurnMessage(toolMessage)
			if err != nil {
				traceSession.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}

			t.emittedMessages = append(t.emittedMessages, toolMessage)

			if err := t.appendSessionMessages([]handmsg.Message{toolMessage}); err != nil {
				traceSession.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}
		}
	}

	reply, err := t.summaryFallback(ctx, budget, traceSession)
	if err != nil {
		return "", err
	}

	return reply, nil
}

func (t *Turn) appendSessionMessages(messages []handmsg.Message) error {
	return t.sessionManager.AppendMessages(t.ctx, t.sessionID, messages)
}

func (t *Turn) availableToolDefinitions() ([]models.ToolDefinition, error) {
	if t == nil || t.runtimeEnv == nil || t.runtimeEnv.Tools() == nil {
		return nil, nil
	}

	definitions, err := t.runtimeEnv.Tools().Resolve(t.runtimeEnv.ToolPolicy())
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

	return t.invokeToolFn(ctx, t.runtimeEnv, toolCall)
}

func (t *Turn) summaryFallback(ctx context.Context, budget environment.IterationBudget, traceSession trace.Session) (string, error) {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	instructions := t.instructions.Chain(instruct.BuildSummary(budget.Remaining())...)
	request := models.Request{
		Model:         t.cfg.Model,
		APIMode:       t.cfg.ModelAPIMode,
		Instructions:  instructions.String(),
		Messages:      t.Context(),
		Tools:         nil,
		DebugRequests: t.cfg.DebugRequests,
	}

	traceSession.Record("summary.fallback.started", map[string]any{"remaining_iterations": budget.Remaining()})
	t.memory.RecordSummaryApplied(traceSession)
	recordPreflightCompactionTrace(traceSession, t.cfg, request, t.lastPromptTokens)
	traceSession.Record("model.request", request)

	resp, err := t.modelClient.Chat(ctx, request)
	if err != nil {
		wrapped := fmt.Errorf("iteration limit reached and summary failed: %w", err)
		traceSession.Record("session.failed", map[string]any{"error": wrapped.Error()})
		return "", wrapped
	}

	if resp == nil {
		err = errors.New("model response is required")
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	traceSession.Record("model.response", resp)
	if err := t.recordPostflightUsage(traceSession, resp); err != nil {
		return "", err
	}

	if resp.RequiresToolCalls {
		err = fmt.Errorf("iteration limit reached and summary requested more tools")
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	assistantMessage, err := handmsg.NewMessage(handmsg.RoleAssistant, resp.OutputText)
	if err != nil {
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	t.emittedMessages = append(t.emittedMessages, assistantMessage)
	if err := t.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	traceSession.Record("final.assistant.response", map[string]any{"message": resp.OutputText})
	return resp.OutputText, nil
}

func (t *Turn) Context() []handmsg.Message {
	builder := t.contextBuilder
	if builder == nil {
		builder = ctxbuilder.New()
	}

	return builder.Build(ctxbuilder.Input{
		SessionHistory:  t.sessionHistory,
		EmittedMessages: t.emittedMessages,
		Memory:          t.memory,
	})
}

func (t *Turn) recordPostflightUsage(traceSession trace.Session, resp *models.Response) error {
	if t == nil || resp == nil || resp.PromptTokens <= 0 {
		return nil
	}

	t.lastPromptTokens = resp.PromptTokens
	if err := t.sessionManager.UpdateLastPromptTokens(t.ctx, t.sessionID, resp.PromptTokens); err != nil {
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return err
	}

	traceSession.Record("context.postflight.usage_recorded", map[string]any{
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
