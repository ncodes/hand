package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

var jsonMarshal = json.Marshal

const requestInstructionName = "request.instruct"

// RespondOptions configures a single response turn.
type RespondOptions struct {
	Instruct  string
	SessionID string
}

type executionEnvironment interface {
	Prepare() error
	Instructions() instruct.Instructions
	Tools() environment.ToolRegistry
	ToolPolicy() tools.Policy
	NewIterationBudget() environment.IterationBudget
	NewTraceSession() trace.Session
}

var newRuntimeEnvironment = func(ctx context.Context, cfg *config.Config) executionEnvironment {
	return environment.NewEnvironment(ctx, cfg)
}

var openSessionStore = sessionstore.OpenStore

var newSessionManager = sessionstore.NewManager

// Agent coordinates agent lifecycle, sessions, and turn execution.
type Agent struct {
	ctx          context.Context
	cfg          *config.Config
	modelClient  models.Client
	env          executionEnvironment
	manager      *sessionstore.Manager
	turnMessages []handmsg.Message
	initialized  bool
}

// NewAgent constructs an Agent with its runtime dependencies.
func NewAgent(ctx context.Context, cfg *config.Config, modelClient models.Client) *Agent {
	return &Agent{ctx: ctx, cfg: cfg, modelClient: modelClient}
}

// Respond executes a single user turn and returns the assistant reply.
func (c *Agent) Respond(ctx context.Context, msg string, opts RespondOptions) (string, error) {
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

	if !c.initialized || c.manager == nil {
		return "", errors.New("environment has not been initialized")
	}

	runtimeEnv := c.env
	if c.initialized || runtimeEnv == nil {
		runtimeEnv = newRuntimeEnvironment(ctx, c.cfg)
		if err := runtimeEnv.Prepare(); err != nil {
			return "", err
		}
	}

	if runtimeEnv.Tools() == nil {
		return "", errors.New("tool registry is required")
	}

	c.env = runtimeEnv

	turn := NewTurn(c.cfg, c.modelClient, c.manager, c.invokeToolWithEnvironment, runtimeEnv)
	reply, err := turn.Run(ctx, msg, opts)
	c.turnMessages = turn.TurnMessages()

	return reply, err
}

// Start initializes the agent runtime and session manager.
func (c *Agent) Start(ctx context.Context) error {
	if c == nil {
		return errors.New("agent is required")
	}
	if c.cfg == nil {
		return errors.New("config is required")
	}

	ctx = normalizeContext(ctx)
	c.ctx = ctx

	if err := c.ensureSessionManager(); err != nil {
		return err
	}

	if err := c.manager.Start(ctx); err != nil {
		return err
	}

	c.env = newRuntimeEnvironment(ctx, c.cfg)
	if err := c.env.Prepare(); err != nil {
		return err
	}

	c.turnMessages = nil
	c.initialized = true

	return nil
}

// TurnMessages returns the messages emitted during the most recent turn.
func (c *Agent) TurnMessages() []handmsg.Message {
	if c == nil || len(c.turnMessages) == 0 {
		return nil
	}

	messages := make([]handmsg.Message, len(c.turnMessages))
	copy(messages, c.turnMessages)
	return messages
}

func (c *Agent) availableToolDefinitions() ([]models.ToolDefinition, error) {
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

func (c *Agent) invokeTool(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
	return c.invokeToolWithEnvironment(ctx, c.env, toolCall)
}

func (c *Agent) invokeToolWithEnvironment(ctx context.Context, runtimeEnv executionEnvironment, toolCall models.ToolCall) handmsg.Message {
	result := map[string]any{"name": toolCall.Name}

	if runtimeEnv == nil || runtimeEnv.Tools() == nil {
		result["error"] = "tool registry is required"
		raw, _ := jsonMarshal(result)
		return handmsg.Message{Role: handmsg.RoleTool, Name: toolCall.Name, ToolCallID: toolCall.ID, Content: string(raw)}
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

	return handmsg.Message{Role: handmsg.RoleTool, Name: toolCall.Name, ToolCallID: toolCall.ID, Content: content}
}

// CreateSession creates a new session or returns the existing one for the id.
func (c *Agent) CreateSession(ctx context.Context, id string) (sessionstore.Session, error) {
	if c == nil {
		return sessionstore.Session{}, errors.New("agent is required")
	}

	if !c.initialized || c.manager == nil {
		return sessionstore.Session{}, errors.New("environment has not been initialized")
	}

	return c.manager.CreateSession(normalizeContext(ctx), id)
}

// ListSessions returns the known sessions.
func (c *Agent) ListSessions(ctx context.Context) ([]sessionstore.Session, error) {
	if c == nil {
		return nil, errors.New("agent is required")
	}

	if !c.initialized || c.manager == nil {
		return nil, errors.New("environment has not been initialized")
	}

	return c.manager.ListSessions(normalizeContext(ctx))
}

// UseSession marks the named session as the current session.
func (c *Agent) UseSession(ctx context.Context, id string) error {
	if c == nil {
		return errors.New("agent is required")
	}

	if !c.initialized || c.manager == nil {
		return errors.New("environment has not been initialized")
	}

	return c.manager.UseSession(normalizeContext(ctx), id)
}

// CurrentSession returns the id of the current session.
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

	store, err := openSessionStore(c.cfg)
	if err != nil {
		return err
	}

	manager, err := newSessionManager(
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

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func toContextToolCalls(toolCalls []models.ToolCall) []handmsg.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}

	normalized := make([]handmsg.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		normalized = append(normalized, handmsg.ToolCall{ID: toolCall.ID, Name: toolCall.Name, Input: toolCall.Input})
	}

	return normalized
}
