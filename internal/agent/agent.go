package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/config"
	handctx "github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/instruction"
	"github.com/wandxy/hand/internal/models"
	sessionstore "github.com/wandxy/hand/internal/storage/session"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

var jsonMarshal = json.Marshal

const requestInstructInstructionName = "request.instruct"

type ChatOptions struct {
	Instruct  string
	SessionID string
}

type runtimeEnvironment interface {
	Prepare() error
	Context() environment.Context
	Tools() environment.ToolRegistry
	ToolPolicy() tools.Policy
	NewIterationBudget() environment.IterationBudget
	NewTraceSession() trace.Session
}

var newRuntimeEnvironment = func(ctx context.Context, cfg *config.Config) runtimeEnvironment {
	return environment.NewEnvironment(ctx, cfg)
}

type Agent struct {
	ctx         context.Context
	cfg         *config.Config
	modelClient models.Client
	env         runtimeEnvironment
	manager     *sessionstore.Manager
	initialized bool
}

func NewAgent(ctx context.Context, cfg *config.Config, modelClient models.Client) *Agent {
	return &Agent{ctx: ctx, cfg: cfg, modelClient: modelClient}
}

func (c *Agent) Chat(ctx context.Context, msg string, opts ChatOptions) (string, error) {
	if c == nil {
		return "", errors.New("agent is required")
	}
	if c.cfg == nil {
		return "", errors.New("config is required")
	}
	if !c.initialized && c.env == nil {
		return "", errors.New("environment has not been initialized")
	}
	if c.modelClient == nil {
		return "", errors.New("model client is required")
	}
	if strings.TrimSpace(msg) == "" {
		return "", errors.New("message is required")
	}

	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := c.ensureSessionManager(); err != nil {
		return "", err
	}

	runtimeEnv := c.env
	if c.initialized || runtimeEnv == nil {
		runtimeEnv = newRuntimeEnvironment(c.ctx, c.cfg)
		if err := runtimeEnv.Prepare(); err != nil {
			return "", err
		}
	}
	if runtimeEnv.Tools() == nil {
		return "", errors.New("tool registry is required")
	}

	session, err := c.manager.ResolveChatSession(ctx, opts.SessionID)
	if err != nil {
		return "", err
	}

	handCtx := runtimeEnv.Context()
	if handCtx == nil {
		return "", errors.New("runtime context is required")
	}
	messages, err := c.manager.GetMessages(ctx, session.ID, sessionstore.MessageQueryOptions{})
	if err != nil {
		return "", err
	}
	for _, message := range messages {
		if err := handCtx.AddMessage(message); err != nil {
			return "", err
		}
	}
	c.env = runtimeEnv

	instruct := strings.TrimSpace(opts.Instruct)
	if instruct != "" {
		handCtx.SetInstruction(handctx.Instruction{Name: requestInstructInstructionName, Value: instruct})
		defer handCtx.RemoveInstruction(requestInstructInstructionName)
	}

	appendSessionMessages := func(messages []handctx.Message) error {
		return c.manager.AppendMessages(ctx, session.ID, messages)
	}

	trace := runtimeEnv.NewTraceSession()
	defer trace.Close()

	if err := handCtx.AddUserMessage(msg); err != nil {
		trace.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}
	currentMessages := handCtx.GetMessages()
	if err := appendSessionMessages(currentMessages[len(currentMessages)-1:]); err != nil {
		trace.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}
	trace.Record("user.message.accepted", map[string]any{"message": msg})

	budget := runtimeEnv.NewIterationBudget()
	for budget.Consume() {
		if err := ctx.Err(); err != nil {
			trace.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		toolDefinitions, err := c.toolDefinitions()
		if err != nil {
			trace.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		request := models.GenerateRequest{
			Model:         c.cfg.Model,
			APIMode:       c.cfg.ModelAPIMode,
			Instructions:  handCtx.GetInstructions().String(),
			Messages:      handCtx.GetMessages(),
			Tools:         toolDefinitions,
			DebugRequests: c.cfg.DebugRequests,
		}
		trace.Record("model.request", request)

		resp, err := c.modelClient.Chat(ctx, request)
		if err != nil {
			trace.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}
		if resp == nil {
			err = errors.New("model response is required")
			trace.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}
		trace.Record("model.response", resp)

		if !resp.RequiresToolCalls {
			if err := handCtx.AddAssistantMessage(resp.OutputText); err != nil {
				trace.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}
			currentMessages := handCtx.GetMessages()
			if err := appendSessionMessages(currentMessages[len(currentMessages)-1:]); err != nil {
				trace.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}
			trace.Record("final.assistant.response", map[string]any{"message": resp.OutputText})
			return resp.OutputText, nil
		}

		if len(resp.ToolCalls) == 0 {
			err = errors.New("model requested tool execution without tool calls")
			trace.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		assistantMessage := handctx.Message{Role: handctx.RoleAssistant, Content: strings.TrimSpace(resp.OutputText), ToolCalls: toContextToolCalls(resp.ToolCalls)}
		if err := handCtx.AddMessage(assistantMessage); err != nil {
			trace.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}
		if err := appendSessionMessages([]handctx.Message{assistantMessage}); err != nil {
			trace.Record("session.failed", map[string]any{"error": err.Error()})
			return "", err
		}

		for _, toolCall := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				trace.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}

			trace.Record("tool.invocation.started", toolCall)
			toolMessage := c.invokeToolWithEnvironment(ctx, runtimeEnv, toolCall)
			trace.Record("tool.invocation.completed", toolMessage)

			if err := handCtx.AddMessage(toolMessage); err != nil {
				trace.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}
			if err := appendSessionMessages([]handctx.Message{toolMessage}); err != nil {
				trace.Record("session.failed", map[string]any{"error": err.Error()})
				return "", err
			}
		}
	}

	reply, err := c.summaryFallback(ctx, budget, handCtx, trace)
	if err != nil {
		return "", err
	}
	currentMessages = handCtx.GetMessages()
	if err := appendSessionMessages(currentMessages[len(currentMessages)-1:]); err != nil {
		trace.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}
	return reply, nil
}

func (c *Agent) Run(context.Context) error {
	if c == nil {
		return errors.New("agent is required")
	}
	if c.cfg == nil {
		return errors.New("config is required")
	}

	if err := c.ensureSessionManager(); err != nil {
		return err
	}
	if err := c.manager.StartMaintenanceWorker(c.ctx, time.Minute); err != nil {
		return err
	}

	c.env = newRuntimeEnvironment(c.ctx, c.cfg)
	if err := c.env.Prepare(); err != nil {
		return err
	}
	c.initialized = true

	return nil
}

func (c *Agent) Conversation() handctx.Conversation {
	if c == nil || c.env == nil {
		return handctx.NewConversation()
	}
	return c.env.Context().GetConversation()
}

func (c *Agent) toolDefinitions() ([]models.ToolDefinition, error) {
	if c == nil || c.env == nil || c.env.Tools() == nil {
		return nil, nil
	}

	definitions, err := c.env.Tools().Resolve(c.env.ToolPolicy())
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

func (c *Agent) invokeTool(ctx context.Context, toolCall models.ToolCall) handctx.Message {
	return c.invokeToolWithEnvironment(ctx, c.env, toolCall)
}

func (c *Agent) invokeToolWithEnvironment(ctx context.Context, runtimeEnv runtimeEnvironment, toolCall models.ToolCall) handctx.Message {
	result := map[string]any{"name": toolCall.Name}
	if runtimeEnv == nil || runtimeEnv.Tools() == nil {
		result["error"] = "tool registry is required"
		raw, _ := jsonMarshal(result)
		return handctx.Message{Role: handctx.RoleTool, Name: toolCall.Name, ToolCallID: toolCall.ID, Content: string(raw)}
	}

	toolResult, err := runtimeEnv.Tools().Invoke(ctx, tools.Call{
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
		result["output"] = strings.TrimSpace(toolResult.Output)
	}

	raw, marshalErr := jsonMarshal(result)
	content := ""
	if marshalErr != nil {
		content = fmt.Sprintf(`{"name":%q,"error":%q}`, toolCall.Name, marshalErr.Error())
	} else {
		content = string(raw)
	}

	return handctx.Message{Role: handctx.RoleTool, Name: toolCall.Name, ToolCallID: toolCall.ID, Content: content}
}

func (c *Agent) CreateSession(ctx context.Context, id string) (sessionstore.Session, error) {
	if c == nil {
		return sessionstore.Session{}, errors.New("agent is required")
	}
	if !c.initialized || c.manager == nil {
		return sessionstore.Session{}, errors.New("environment has not been initialized")
	}
	return c.manager.CreateSession(normalizeContext(ctx), id)
}

func (c *Agent) ListSessions(ctx context.Context) ([]sessionstore.Session, error) {
	if c == nil {
		return nil, errors.New("agent is required")
	}
	if !c.initialized || c.manager == nil {
		return nil, errors.New("environment has not been initialized")
	}
	return c.manager.ListSessions(normalizeContext(ctx))
}

func (c *Agent) UseSession(ctx context.Context, id string) error {
	if c == nil {
		return errors.New("agent is required")
	}
	if !c.initialized || c.manager == nil {
		return errors.New("environment has not been initialized")
	}
	return c.manager.UseSession(normalizeContext(ctx), id)
}

func (c *Agent) CurrentSession(ctx context.Context) (string, error) {
	if c == nil {
		return "", errors.New("agent is required")
	}
	if !c.initialized || c.manager == nil {
		return "", errors.New("environment has not been initialized")
	}
	return c.manager.CurrentSession(normalizeContext(ctx))
}

func (c *Agent) ensureSessionManager() error {
	if c == nil {
		return errors.New("agent is required")
	}
	if c.cfg == nil {
		return errors.New("config is required")
	}
	if c.manager != nil {
		return nil
	}

	store, err := sessionstore.OpenStore(c.cfg)
	if err != nil {
		return err
	}
	manager, err := sessionstore.NewManager(
		store,
		durationOrDefault(c.cfg.SessionDefaultIdleExpiry, 24*time.Hour),
		durationOrDefault(c.cfg.SessionArchiveRetention, 30*24*time.Hour),
	)
	if err != nil {
		return err
	}
	c.manager = manager
	return nil
}

func durationOrDefault(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func normalizeToolError(raw string) any {
	var toolErr tools.Error
	if err := json.Unmarshal([]byte(raw), &toolErr); err == nil &&
		strings.TrimSpace(toolErr.Code) != "" &&
		strings.TrimSpace(toolErr.Message) != "" {
		return toolErr
	}
	return raw
}

func (c *Agent) summaryFallback(ctx context.Context, budget environment.IterationBudget, handCtx environment.Context, trace trace.Session) (string, error) {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		trace.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	instructions := handCtx.GetInstructions().Chain(instruction.BuildSummary(budget.Remaining())...)
	request := models.GenerateRequest{
		Model:         c.cfg.Model,
		APIMode:       c.cfg.ModelAPIMode,
		Instructions:  instructions.String(),
		Messages:      handCtx.GetMessages(),
		Tools:         nil,
		DebugRequests: c.cfg.DebugRequests,
	}

	trace.Record("summary.fallback.started", map[string]any{"remaining_iterations": budget.Remaining()})
	trace.Record("model.request", request)

	resp, err := c.modelClient.Chat(ctx, request)
	if err != nil {
		wrapped := fmt.Errorf("iteration limit reached and summary failed: %w", err)
		trace.Record("session.failed", map[string]any{"error": wrapped.Error()})
		return "", wrapped
	}
	if resp == nil {
		err = errors.New("model response is required")
		trace.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	trace.Record("model.response", resp)

	if resp.RequiresToolCalls {
		err = fmt.Errorf("iteration limit reached and summary requested more tools")
		trace.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	if err := handCtx.AddAssistantMessage(resp.OutputText); err != nil {
		trace.Record("session.failed", map[string]any{"error": err.Error()})
		return "", err
	}

	trace.Record("final.assistant.response", map[string]any{"message": resp.OutputText})
	return resp.OutputText, nil
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func toContextToolCalls(toolCalls []models.ToolCall) []handctx.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}

	normalized := make([]handctx.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		normalized = append(normalized, handctx.ToolCall{ID: toolCall.ID, Name: toolCall.Name, Input: toolCall.Input})
	}
	return normalized
}
