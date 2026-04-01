package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/trace"
)

type Turn struct {
	ctx             context.Context
	cfg             *config.Config
	modelClient     models.Client
	sessionManager  *sessionstore.Manager
	invokeToolFn    func(context.Context, executionEnvironment, models.ToolCall) handmsg.Message
	runtimeEnv      executionEnvironment
	instructions    instruct.Instructions
	sessionHistory  []handmsg.Message
	emittedMessages []handmsg.Message
	sessionID       string
}

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
	}
}

func (r *Turn) loadTurnContext(ctx context.Context, opts RespondOptions) error {
	if r == nil {
		return errors.New("agent is required")
	}

	if r.cfg == nil {
		return errors.New("config is required")
	}

	if r.modelClient == nil {
		return errors.New("model client is required")
	}

	if r.runtimeEnv == nil {
		return errors.New("runtime environment is required")
	}

	if r.sessionManager == nil {
		return errors.New("session manager is required")
	}

	session, err := r.sessionManager.ResolveSession(ctx, opts.SessionID)
	if err != nil {
		return err
	}

	messages, err := r.sessionManager.GetMessages(ctx, session.ID, sessionstore.MessageQueryOptions{})
	if err != nil {
		return err
	}

	r.ctx = ctx
	r.instructions = r.runtimeEnv.Instructions()
	r.sessionHistory = messages
	r.emittedMessages = nil
	r.sessionID = session.ID

	return nil
}

func (r *Turn) Run(ctx context.Context, msg string, opts RespondOptions) (string, error) {
	if err := r.loadTurnContext(ctx, opts); err != nil {
		return "", err
	}

	requestInstruct := strings.TrimSpace(opts.Instruct)
	if requestInstruct != "" {
		r.instructions = setInstruction(r.instructions, instruct.Instruction{Name: requestInstructionName, Value: requestInstruct})
		defer func() {
			r.instructions = r.instructions.WithoutName(requestInstructionName)
		}()
	}

	traceSession := r.runtimeEnv.NewTraceSession()
	defer traceSession.Close()

	userMessage, err := handmsg.NewMessage(handmsg.RoleUser, msg)
	if err != nil {
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	r.emittedMessages = append(r.emittedMessages, userMessage)
	if err := r.appendSessionMessages([]handmsg.Message{userMessage}); err != nil {
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	traceSession.Record("user.message.accepted", map[string]any{"message": msg})

	budget := r.runtimeEnv.NewIterationBudget()
	for budget.Consume() {
		if err := ctx.Err(); err != nil {
			traceSession.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		availableToolDefinitions, err := r.availableToolDefinitions()
		if err != nil {
			traceSession.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		request := models.Request{
			Model:         r.cfg.Model,
			APIMode:       r.cfg.ModelAPIMode,
			Instructions:  r.instructions.String(),
			Messages:      r.requestMessages(),
			Tools:         availableToolDefinitions,
			DebugRequests: r.cfg.DebugRequests,
		}
		traceSession.Record("model.request", request)

		resp, err := r.modelClient.Chat(ctx, request)
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

		if !resp.RequiresToolCalls {
			assistantMessage, err := handmsg.NewMessage(handmsg.RoleAssistant, resp.OutputText)
			if err != nil {
				traceSession.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}

			r.emittedMessages = append(r.emittedMessages, assistantMessage)
			if err := r.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
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

		r.emittedMessages = append(r.emittedMessages, assistantMessage)

		if err := r.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
			traceSession.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		for _, toolCall := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				traceSession.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}

			traceSession.Record("tool.invocation.started", toolCall)
			toolMessage := r.invokeTool(ctx, toolCall)
			traceSession.Record("tool.invocation.completed", toolMessage)

			toolMessage, err = normalizeTurnMessage(toolMessage)
			if err != nil {
				traceSession.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}

			r.emittedMessages = append(r.emittedMessages, toolMessage)

			if err := r.appendSessionMessages([]handmsg.Message{toolMessage}); err != nil {
				traceSession.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}
		}
	}

	reply, err := r.summaryFallback(ctx, budget, traceSession)
	if err != nil {
		return "", err
	}

	return reply, nil
}

func (r *Turn) appendSessionMessages(messages []handmsg.Message) error {
	return r.sessionManager.AppendMessages(r.ctx, r.sessionID, messages)
}

func (r *Turn) availableToolDefinitions() ([]models.ToolDefinition, error) {
	if r == nil || r.runtimeEnv == nil || r.runtimeEnv.Tools() == nil {
		return nil, nil
	}

	definitions, err := r.runtimeEnv.Tools().Resolve(r.runtimeEnv.ToolPolicy())
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

func (r *Turn) invokeTool(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
	if r.invokeToolFn == nil {
		return handmsg.Message{
			Role:       handmsg.RoleTool,
			Name:       toolCall.Name,
			ToolCallID: toolCall.ID,
			Content:    `{"error":"tool invocation is required"}`,
		}
	}

	return r.invokeToolFn(ctx, r.runtimeEnv, toolCall)
}

func (r *Turn) summaryFallback(ctx context.Context, budget environment.IterationBudget, traceSession trace.Session) (string, error) {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	instructions := r.instructions.Chain(instruct.BuildSummary(budget.Remaining())...)
	request := models.Request{
		Model:         r.cfg.Model,
		APIMode:       r.cfg.ModelAPIMode,
		Instructions:  instructions.String(),
		Messages:      r.requestMessages(),
		Tools:         nil,
		DebugRequests: r.cfg.DebugRequests,
	}

	traceSession.Record("summary.fallback.started", map[string]any{"remaining_iterations": budget.Remaining()})
	traceSession.Record("model.request", request)

	resp, err := r.modelClient.Chat(ctx, request)
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

	r.emittedMessages = append(r.emittedMessages, assistantMessage)
	if err := r.appendSessionMessages([]handmsg.Message{assistantMessage}); err != nil {
		traceSession.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	traceSession.Record("final.assistant.response", map[string]any{"message": resp.OutputText})
	return resp.OutputText, nil
}

func (r *Turn) requestMessages() []handmsg.Message {
	messages := make([]handmsg.Message, 0, len(r.sessionHistory)+len(r.emittedMessages))
	messages = append(messages, r.sessionHistory...)
	messages = append(messages, r.emittedMessages...)
	return messages
}

func (r *Turn) TurnMessages() []handmsg.Message {
	if len(r.emittedMessages) == 0 {
		return nil
	}

	messages := make([]handmsg.Message, len(r.emittedMessages))
	copy(messages, r.emittedMessages)
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
