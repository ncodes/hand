package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	memory "github.com/wandxy/hand/internal/agent/memory"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/storage"
	storagefactory "github.com/wandxy/hand/internal/storage/factory"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/pkg/logutils"
)

var jsonMarshal = json.Marshal

const requestInstructionName = "request.instruct"

var agentLog = logutils.InitLogger("agent")

type ServiceAPI interface {
	Respond(context.Context, string, RespondOptions) (string, error)
	CreateSession(context.Context, string) (storage.Session, error)
	ListSessions(context.Context) ([]storage.Session, error)
	UseSession(context.Context, string) error
	CurrentSession(context.Context) (string, error)
	CompactSession(context.Context, string) (CompactSessionResult, error)
	SessionContextStatus(context.Context, string) (SessionContextStatus, error)
}

type RespondOptions struct {
	Instruct  string
	SessionID string
	Stream    *bool
	OnEvent   func(Event)
}

type Event struct {
	Channel string
	Text    string
}

type CompactSessionResult struct {
	SessionID            string
	SourceEndOffset      int
	SourceMessageCount   int
	UpdatedAt            time.Time
	CurrentContextLength int
	TotalContextLength   int
}

type SessionContextStatus struct {
	SessionID        string
	Offset           int
	Size             int
	Length           int
	Used             int
	Remaining        int
	UsedPct          float64
	RemainingPct     float64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompactionStatus string
}

var newEnvironment = func(ctx context.Context, cfg *config.Config) environment.Environment {
	return environment.NewEnvironment(ctx, cfg)
}

var openSessionStore = storagefactory.OpenSessionStore

var newSessionManager = sessionstore.NewManager

// Agent coordinates agent lifecycle, sessions, and turn execution.
type Agent struct {
	ctx          context.Context
	cfg          *config.Config
	modelClient  models.Client
	env          environment.Environment
	sessionMgr   *sessionstore.Manager
	turnMessages []handmsg.Message
	initialized  bool
}

// NewAgent constructs an Agent with its runtime dependencies.
func NewAgent(ctx context.Context, cfg *config.Config, modelClient models.Client) *Agent {
	return &Agent{ctx: ctx, cfg: cfg, modelClient: modelClient}
}

func (a *Agent) Start(ctx context.Context) error {
	if a == nil {
		return errors.New("agent is required")
	}
	if a.cfg == nil {
		return errors.New("config is required")
	}

	ctx = normalizeContext(ctx)
	a.ctx = ctx

	if err := a.ensureSessionManager(); err != nil {
		return err
	}

	if err := a.sessionMgr.Start(ctx); err != nil {
		return err
	}

	a.env = newEnvironment(ctx, a.cfg)
	if err := a.env.Prepare(); err != nil {
		return err
	}

	a.turnMessages = nil
	a.initialized = true

	agentLog.Info().Msg("agent started")

	return nil
}

func (a *Agent) Respond(ctx context.Context, msg string, opts RespondOptions) (string, error) {
	if a == nil {
		return "", errors.New("agent is required")
	}
	if a.cfg == nil {
		return "", errors.New("config is required")
	}
	if !a.initialized && a.env == nil {
		return "", errors.New("environment has not been initialized")
	}
	if a.modelClient == nil {
		return "", errors.New("model client is required")
	}

	if strings.TrimSpace(msg) == "" {
		return "", errors.New("message is required")
	}

	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if !a.initialized || a.sessionMgr == nil {
		return "", errors.New("environment has not been initialized")
	}

	env := a.env
	if a.initialized || env == nil {
		env = newEnvironment(ctx, a.cfg)
		if err := env.Prepare(); err != nil {
			return "", err
		}
	}

	if env.Tools() == nil {
		return "", errors.New("tool registry is required")
	}

	a.env = env

	agentLog.Info().Str("session_id", opts.SessionID).Str("model", a.cfg.Model).Msg("responding to user message")

	turn := NewTurn(a.cfg, a.modelClient, a.sessionMgr, a.invokeToolWithEnvironment, env)
	reply, err := turn.Run(ctx, msg, opts)
	a.turnMessages = turn.Messages()

	return reply, err
}

func (a *Agent) TurnMessages() []handmsg.Message {
	if a == nil || len(a.turnMessages) == 0 {
		return nil
	}

	messages := make([]handmsg.Message, len(a.turnMessages))
	copy(messages, a.turnMessages)
	return messages
}

func (a *Agent) availableToolDefinitions() ([]models.ToolDefinition, error) {
	if a == nil || a.env == nil || a.env.Tools() == nil {
		return nil, nil
	}

	definitions, err := a.env.Tools().Resolve(a.env.ToolPolicy())
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

func (a *Agent) invokeTool(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
	return a.invokeToolWithEnvironment(ctx, a.env, toolCall)
}

func (a *Agent) invokeToolWithEnvironment(
	ctx context.Context,
	env environment.Environment,
	toolCall models.ToolCall,
) handmsg.Message {
	result := map[string]any{"name": toolCall.Name}

	if env == nil || env.Tools() == nil {
		result["error"] = "tool registry is required"
		raw, _ := jsonMarshal(result)
		return handmsg.Message{Role: handmsg.RoleTool, Name: toolCall.Name, ToolCallID: toolCall.ID, Content: string(raw)}
	}

	toolResult, err := env.Tools().Invoke(ctx, tools.Call{
		Name:   toolCall.Name,
		Input:  toolCall.Input,
		Source: "model",
	})

	if err != nil {
		agentLog.Warn().Str("tool", toolCall.Name).Err(err).Msg("tool invocation failed")
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

func (a *Agent) CreateSession(ctx context.Context, id string) (storage.Session, error) {
	if a == nil {
		return storage.Session{}, errors.New("agent is required")
	}

	if !a.initialized || a.sessionMgr == nil {
		return storage.Session{}, errors.New("environment has not been initialized")
	}

	return a.sessionMgr.CreateSession(normalizeContext(ctx), id)
}

func (a *Agent) ListSessions(ctx context.Context) ([]storage.Session, error) {
	if a == nil {
		return nil, errors.New("agent is required")
	}

	if !a.initialized || a.sessionMgr == nil {
		return nil, errors.New("environment has not been initialized")
	}

	return a.sessionMgr.ListSessions(normalizeContext(ctx))
}

func (a *Agent) UseSession(ctx context.Context, id string) error {
	if a == nil {
		return errors.New("agent is required")
	}

	if !a.initialized || a.sessionMgr == nil {
		return errors.New("environment has not been initialized")
	}

	return a.sessionMgr.UseSession(normalizeContext(ctx), id)
}

func (a *Agent) CurrentSession(ctx context.Context) (string, error) {
	if a == nil {
		return "", errors.New("agent is required")
	}

	if !a.initialized || a.sessionMgr == nil {
		return "", errors.New("environment has not been initialized")
	}

	return a.sessionMgr.CurrentSession(normalizeContext(ctx))
}

func (a *Agent) CompactSession(ctx context.Context, id string) (CompactSessionResult, error) {
	if a == nil {
		return CompactSessionResult{}, errors.New("agent is required")
	}
	if a.cfg == nil {
		return CompactSessionResult{}, errors.New("config is required")
	}
	if !a.initialized || a.sessionMgr == nil {
		return CompactSessionResult{}, errors.New("environment has not been initialized")
	}
	if a.modelClient == nil {
		return CompactSessionResult{}, errors.New("model client is required")
	}

	session, err := a.sessionMgr.Resolve(normalizeContext(ctx), id)
	if err != nil {
		return CompactSessionResult{}, err
	}

	agentLog.Info().Str("session_id", session.ID).Msg("manual session compaction requested")

	traceSession := trace.NoopSession()
	if a.env != nil {
		traceSession = a.env.NewTraceSession(session.ID)
	}
	defer traceSession.Close()

	memoryService := memory.NewService(a.cfg, a.modelClient, a.sessionMgr)
	summary, err := memoryService.CompactSession(normalizeContext(ctx), session, traceSession)
	if err != nil {
		agentLog.Error().Str("session_id", session.ID).Err(err).Msg("session compaction failed")
		return CompactSessionResult{}, err
	}

	return CompactSessionResult{
		SessionID:            summary.SessionID,
		SourceEndOffset:      summary.SourceEndOffset,
		SourceMessageCount:   summary.SourceMessageCount,
		UpdatedAt:            summary.UpdatedAt,
		CurrentContextLength: session.LastPromptTokens,
		TotalContextLength:   a.cfg.ContextLength,
	}, nil
}

func (a *Agent) SessionContextStatus(ctx context.Context, id string) (SessionContextStatus, error) {
	if a == nil {
		return SessionContextStatus{}, errors.New("agent is required")
	}
	if a.cfg == nil {
		return SessionContextStatus{}, errors.New("config is required")
	}
	if !a.initialized || a.sessionMgr == nil {
		return SessionContextStatus{}, errors.New("environment has not been initialized")
	}

	session, err := a.sessionMgr.Resolve(normalizeContext(ctx), id)
	if err != nil {
		return SessionContextStatus{}, err
	}

	summary, _, err := a.sessionMgr.GetSummary(normalizeContext(ctx), session.ID)
	if err != nil {
		return SessionContextStatus{}, err
	}

	total := max(a.cfg.ContextLength, 0)
	used := max(session.LastPromptTokens, 0)
	remaining := max(total-used, 0)

	status := SessionContextStatus{
		SessionID:        session.ID,
		Offset:           max(summary.SourceEndOffset, 0),
		Size:             max(summary.SourceMessageCount, 0),
		Length:           total,
		Used:             used,
		Remaining:        remaining,
		CreatedAt:        session.CreatedAt,
		UpdatedAt:        session.UpdatedAt,
		CompactionStatus: string(session.Compaction.Status),
	}
	if total > 0 {
		status.UsedPct = float64(used) / float64(total)
		status.RemainingPct = float64(remaining) / float64(total)
	}

	return status, nil
}

func (a *Agent) GetSession(ctx context.Context, id string) (SessionContextStatus, error) {
	return a.SessionContextStatus(ctx, id)
}

func (a *Agent) ensureSessionManager() error {
	if a == nil {
		return errors.New("agent is required")
	}
	if a.cfg == nil {
		return errors.New("config is required")
	}
	if a.sessionMgr != nil {
		return nil
	}

	store, err := openSessionStore(a.cfg)
	if err != nil {
		return err
	}

	manager, err := newSessionManager(
		store,
		durationOrDefault(a.cfg.SessionDefaultIdleExpiry, 24*time.Hour),
		durationOrDefault(a.cfg.SessionArchiveRetention, 30*24*time.Hour),
	)
	if err != nil {
		return err
	}

	a.sessionMgr = manager
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
