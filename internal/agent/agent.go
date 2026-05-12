package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentsummary "github.com/wandxy/hand/internal/agent/context/summary"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/environment"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	"github.com/wandxy/hand/internal/state/search"
	"github.com/wandxy/hand/internal/tools"
	webextract "github.com/wandxy/hand/internal/tools/webextract"
	"github.com/wandxy/hand/internal/trace"
	pkgcache "github.com/wandxy/hand/pkg/cache"
	"github.com/wandxy/hand/pkg/logutils"
)

var jsonMarshal = json.Marshal

const defaultRecallSummaryCacheTTL = constants.DefaultRecallSummaryCacheTTL

var agentLog = logutils.InitLogger("agent")

type ServiceAPI interface {
	Respond(context.Context, string, RespondOptions) (string, error)
	CreateSession(context.Context, string) (storage.Session, error)
	ListSessions(context.Context) ([]storage.Session, error)
	UseSession(context.Context, string) error
	CurrentSession(context.Context) (string, error)
	RecallSessionSummary(context.Context, string) (storage.SessionSummary, error)
	CompactSession(context.Context, string) (CompactSessionResult, error)
	RepairSession(context.Context, RepairSessionOptions) (RepairSessionResult, error)
	ContextStatus(context.Context, string) (ContextStatus, error)
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

type RepairSessionOptions = search.VectorRepairOptions

type RepairSessionResult = search.VectorRepairResult

type ContextStatus struct {
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

var runRecallSessionSummary = func(
	service *agentsummary.Service,
	ctx context.Context,
	session storage.Session,
	traceSession trace.Session,
) (*agentsummary.SummaryState, error) {
	return service.RecallSessionSummary(ctx, session, traceSession)
}

var newRecallSummaryCache = func() *pkgcache.Cache[string, storage.SessionSummary] {
	return pkgcache.New(pkgcache.Options[string, storage.SessionSummary]{
		TTL: defaultRecallSummaryCacheTTL,
		Clone: func(summary storage.SessionSummary) storage.SessionSummary {
			return storage.CloneSessionSummary(summary)
		},
	})
}

var openStore = statemanager.OpenStoreWithRerankerClient

var newStateManager = statemanager.NewManager

// Agent coordinates agent lifecycle, sessions, and turn execution.
type Agent struct {
	ctx                context.Context
	cfg                *config.Config
	modelClient        models.Client
	summaryClient      models.Client
	env                environment.Environment
	stateMgr           *statemanager.Manager
	recallSummaryCache *pkgcache.Cache[string, storage.SessionSummary]
	turnMessages       []handmsg.Message
	initialized        bool
}

// NewAgent constructs an Agent with its runtime dependencies.
// When optionalSummary is empty or its first element is nil, summary/compaction calls use modelClient.
func NewAgent(ctx context.Context, cfg *config.Config, modelClient models.Client, optionalSummary ...models.Client) *Agent {
	var summaryClient models.Client
	if len(optionalSummary) > 0 {
		summaryClient = optionalSummary[0]
	}
	if summaryClient == nil {
		summaryClient = modelClient
	}
	return &Agent{
		ctx:                ctx,
		cfg:                cfg,
		modelClient:        modelClient,
		summaryClient:      summaryClient,
		recallSummaryCache: newRecallSummaryCache(),
	}
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

	if err := a.ensureStateManager(); err != nil {
		return err
	}

	if err := a.stateMgr.Start(ctx); err != nil {
		return err
	}

	a.env = newEnvironment(ctx, a.cfg)
	a.env.SetStateManager(a.stateMgr)
	a.env.SetModelClient(a.summaryClient)
	if err := a.env.Prepare(); err != nil {
		return err
	}

	a.turnMessages = nil
	a.initialized = true

	agentLog.Info().Msg("agent started")

	return nil
}

func (a *Agent) Close() error {
	if a == nil || a.stateMgr == nil {
		return nil
	}

	if a.shouldFlushMemoryBeforeContextLoss() {
		ctx := normalizeContext(a.ctx)
		if sessionID, err := a.stateMgr.CurrentSession(ctx); err == nil && strings.TrimSpace(sessionID) != "" {
			traceSession := trace.NoopSession()
			if a.env != nil {
				traceSession = a.env.NewTraceSession(sessionID)
			}
			a.maybeFlushMemoryBeforeContextLoss(ctx, sessionID, memoryFlushTriggerControlledExit, traceSession)
			traceSession.Close()
		}
	}

	return a.stateMgr.Close()
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

	if !a.initialized || a.stateMgr == nil {
		return "", errors.New("environment has not been initialized")
	}

	env := a.env
	if env == nil {
		env = newEnvironment(ctx, a.cfg)
		env.SetStateManager(a.stateMgr)
		env.SetModelClient(a.summaryClient)
		if err := env.Prepare(); err != nil {
			return "", err
		}
	}

	if env.Tools() == nil {
		return "", errors.New("tool registry is required")
	}

	a.env = env

	agentLog.Info().Str("session_id", opts.SessionID).Str("model", a.cfg.Models.Main.Name).Msg("responding to user message")

	turn := NewTurn(
		a.cfg,
		a.modelClient,
		a.summaryClient,
		a.stateMgr,
		a.invokeToolWithEnvironment,
		env,
	)
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
		toolsList = append(toolsList, modelToolDefinitionFromToolDefinition(definition))
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
	return invokeToolWithEnvironment(ctx, env, toolCall, a.summaryClient, a.cfg)
}

func invokeToolWithEnvironment(
	ctx context.Context,
	env environment.Environment,
	toolCall models.ToolCall,
	summaryClient models.Client,
	cfg *config.Config,
) handmsg.Message {
	result := map[string]any{"name": toolCall.Name}

	if env == nil || env.Tools() == nil {
		result["error"] = "tool registry is required"
		return toolResultMessage(toolCall, result)
	}

	ctx = webextract.WithSummarizer(ctx, webextract.NewExtractSummarizer(summaryClient, cfg))

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

	return toolResultMessage(toolCall, result)
}

func toolResultMessage(toolCall models.ToolCall, result map[string]any) handmsg.Message {
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

	if !a.initialized || a.stateMgr == nil {
		return storage.Session{}, errors.New("environment has not been initialized")
	}

	return a.stateMgr.CreateSession(normalizeContext(ctx), id)
}

func (a *Agent) ListSessions(ctx context.Context) ([]storage.Session, error) {
	if a == nil {
		return nil, errors.New("agent is required")
	}

	if !a.initialized || a.stateMgr == nil {
		return nil, errors.New("environment has not been initialized")
	}

	return a.stateMgr.ListSessions(normalizeContext(ctx))
}

func (a *Agent) UseSession(ctx context.Context, id string) error {
	if a == nil {
		return errors.New("agent is required")
	}

	if !a.initialized || a.stateMgr == nil {
		return errors.New("environment has not been initialized")
	}

	ctx = normalizeContext(ctx)
	targetSession, err := a.stateMgr.Resolve(ctx, id)
	if err != nil {
		return err
	}

	currentSessionID, currentErr := a.stateMgr.CurrentSession(ctx)
	if currentErr == nil &&
		strings.TrimSpace(currentSessionID) != "" &&
		strings.TrimSpace(currentSessionID) != targetSession.ID {
		traceSession := trace.NoopSession()
		if a.env != nil {
			traceSession = a.env.NewTraceSession(currentSessionID)
		}
		a.maybeFlushMemoryBeforeContextLoss(ctx, currentSessionID, memoryFlushTriggerSessionReset, traceSession)
		traceSession.Close()
	}

	return a.stateMgr.UseSession(ctx, targetSession.ID)
}

func (a *Agent) CurrentSession(ctx context.Context) (string, error) {
	if a == nil {
		return "", errors.New("agent is required")
	}

	if !a.initialized || a.stateMgr == nil {
		return "", errors.New("environment has not been initialized")
	}

	return a.stateMgr.CurrentSession(normalizeContext(ctx))
}

func (a *Agent) CompactSession(ctx context.Context, id string) (CompactSessionResult, error) {
	summary, session, err := a.summarizeSession(ctx, id, agentsummary.SummarizeSessionOptions{})
	if err != nil {
		return CompactSessionResult{}, err
	}

	return CompactSessionResult{
		SessionID:            summary.SessionID,
		SourceEndOffset:      summary.SourceEndOffset,
		SourceMessageCount:   summary.SourceMessageCount,
		UpdatedAt:            summary.UpdatedAt,
		CurrentContextLength: session.LastPromptTokens,
		TotalContextLength:   a.cfg.Models.Main.ContextLength,
	}, nil
}

func (a *Agent) RepairSession(
	ctx context.Context,
	opts RepairSessionOptions,
) (RepairSessionResult, error) {
	if a == nil {
		return RepairSessionResult{}, errors.New("agent is required")
	}
	if !a.initialized || a.stateMgr == nil {
		return RepairSessionResult{}, errors.New("environment has not been initialized")
	}

	return a.stateMgr.RepairVectorStore(normalizeContext(ctx), opts)
}

// RecallSessionSummary returns a recall-specific session summary without persisting it.
func (a *Agent) RecallSessionSummary(ctx context.Context, id string) (storage.SessionSummary, error) {
	if a == nil {
		return storage.SessionSummary{}, errors.New("agent is required")
	}
	if a.cfg == nil {
		return storage.SessionSummary{}, errors.New("config is required")
	}
	if !a.initialized || a.stateMgr == nil {
		return storage.SessionSummary{}, errors.New("environment has not been initialized")
	}
	if a.modelClient == nil {
		return storage.SessionSummary{}, errors.New("model client is required")
	}

	ctx = normalizeContext(ctx)

	session, err := a.stateMgr.Resolve(ctx, id)
	if err != nil {
		return storage.SessionSummary{}, err
	}

	messageCount, err := a.stateMgr.CountMessages(ctx, session.ID, storage.MessageQueryOptions{})
	if err != nil {
		return storage.SessionSummary{}, err
	}

	if summary, ok := a.cachedRecallSummary(session.ID, messageCount); ok {
		return summary, nil
	}

	agentLog.Info().Str("session_id", session.ID).Msg("manual recall session summary requested")

	traceSession := trace.NoopSession()
	if a.env != nil {
		traceSession = a.env.NewTraceSession(session.ID)
	}
	defer traceSession.Close()

	summaryService := agentsummary.NewService(a.cfg, a.modelClient, a.summaryClient, a.stateMgr)
	summary, err := runRecallSessionSummary(summaryService, ctx, session, traceSession)
	if err != nil {
		return storage.SessionSummary{}, err
	}
	if summary == nil {
		return storage.SessionSummary{}, errors.New("summary is required")
	}

	result := storage.SessionSummary{
		SessionID:          summary.SessionID,
		SourceEndOffset:    summary.SourceEndOffset,
		SourceMessageCount: summary.SourceMessageCount,
		UpdatedAt:          summary.UpdatedAt,
		SessionSummary:     summary.SessionSummary,
		CurrentTask:        summary.CurrentTask,
		Discoveries:        summary.Discoveries,
		OpenQuestions:      summary.OpenQuestions,
		NextActions:        summary.NextActions,
	}

	a.storeRecallSummary(result)

	return result, nil
}

func (a *Agent) summarizeSession(
	ctx context.Context,
	id string,
	opts agentsummary.SummarizeSessionOptions,
) (storage.SessionSummary, storage.Session, error) {
	if a == nil {
		return storage.SessionSummary{}, storage.Session{}, errors.New("agent is required")
	}
	if a.cfg == nil {
		return storage.SessionSummary{}, storage.Session{}, errors.New("config is required")
	}
	if !a.initialized || a.stateMgr == nil {
		return storage.SessionSummary{}, storage.Session{}, errors.New("environment has not been initialized")
	}
	if a.modelClient == nil {
		return storage.SessionSummary{}, storage.Session{}, errors.New("model client is required")
	}

	session, err := a.stateMgr.Resolve(normalizeContext(ctx), id)
	if err != nil {
		return storage.SessionSummary{}, storage.Session{}, err
	}

	agentLog.Info().Str("session_id", session.ID).Msg("manual session summary requested")

	traceSession := trace.NoopSession()
	if a.env != nil {
		traceSession = a.env.NewTraceSession(session.ID)
	}
	defer traceSession.Close()

	a.maybeFlushMemoryBeforeContextLoss(ctx, session.ID, memoryFlushTriggerCompression, traceSession)

	summaryService := agentsummary.NewService(a.cfg, a.modelClient, a.summaryClient, a.stateMgr)
	summary, err := summaryService.SummarizeSession(
		normalizeContext(ctx),
		session,
		agentsummary.SummarizeSessionOptions(opts),
		traceSession,
	)
	if err != nil {
		agentLog.Error().Str("session_id", session.ID).Err(err).Msg("session summary failed")
		return storage.SessionSummary{}, storage.Session{}, err
	}

	return storage.SessionSummary{
		SessionID:          summary.SessionID,
		SourceEndOffset:    summary.SourceEndOffset,
		SourceMessageCount: summary.SourceMessageCount,
		UpdatedAt:          summary.UpdatedAt,
		SessionSummary:     summary.SessionSummary,
		CurrentTask:        summary.CurrentTask,
		Discoveries:        append([]string(nil), summary.Discoveries...),
		OpenQuestions:      append([]string(nil), summary.OpenQuestions...),
		NextActions:        append([]string(nil), summary.NextActions...),
	}, session, nil
}

func (a *Agent) ContextStatus(ctx context.Context, id string) (ContextStatus, error) {
	if a == nil {
		return ContextStatus{}, errors.New("agent is required")
	}
	if a.cfg == nil {
		return ContextStatus{}, errors.New("config is required")
	}
	if !a.initialized || a.stateMgr == nil {
		return ContextStatus{}, errors.New("environment has not been initialized")
	}

	session, err := a.stateMgr.Resolve(normalizeContext(ctx), id)
	if err != nil {
		return ContextStatus{}, err
	}

	summary, _, err := a.stateMgr.GetSummary(normalizeContext(ctx), session.ID)
	if err != nil {
		return ContextStatus{}, err
	}

	total := max(a.cfg.Models.Main.ContextLength, 0)
	used := max(session.LastPromptTokens, 0)
	remaining := max(total-used, 0)

	status := ContextStatus{
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

func (a *Agent) GetSession(ctx context.Context, id string) (ContextStatus, error) {
	return a.ContextStatus(ctx, id)
}

func (a *Agent) ensureStateManager() error {
	if a == nil {
		return errors.New("agent is required")
	}
	if a.cfg == nil {
		return errors.New("config is required")
	}
	if a.stateMgr != nil {
		return nil
	}

	store, err := openStore(a.cfg, a.summaryClient)
	if err != nil {
		return err
	}

	manager, err := newStateManager(
		store,
		getDurationOrDefault(a.cfg.Session.DefaultIdleExpiry, 24*time.Hour),
		getDurationOrDefault(a.cfg.Session.ArchiveRetention, 30*24*time.Hour),
	)
	if err != nil {
		return err
	}

	a.stateMgr = manager
	return nil
}

func (a *Agent) cachedRecallSummary(sessionID string, messageCount int) (storage.SessionSummary, bool) {
	if a == nil || a.recallSummaryCache == nil {
		return storage.SessionSummary{}, false
	}

	summary, ok := a.recallSummaryCache.Get(sessionID)
	if !ok {
		return storage.SessionSummary{}, false
	}

	if !isFullRecallSummary(summary, messageCount) {
		a.recallSummaryCache.Delete(sessionID)
		return storage.SessionSummary{}, false
	}

	return summary, true
}

func (a *Agent) storeRecallSummary(summary storage.SessionSummary) {
	if a == nil || a.recallSummaryCache == nil || strings.TrimSpace(summary.SessionID) == "" {
		return
	}

	a.recallSummaryCache.Set(summary.SessionID, summary)
}

func isFullRecallSummary(summary storage.SessionSummary, messageCount int) bool {
	return summary.SourceMessageCount == messageCount && summary.SourceEndOffset == messageCount
}

func getDurationOrDefault(value, fallback time.Duration) time.Duration {
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

func modelToolDefinitionFromToolDefinition(definition tools.Definition) models.ToolDefinition {
	return models.ToolDefinition{
		Name:        definition.Name,
		Description: definition.Description,
		InputSchema: definition.InputSchema,
	}
}

func assistantToolCallMessageFromResponse(resp *models.Response) (handmsg.Message, error) {
	return normalizeTurnMessage(handmsg.Message{
		Role:      handmsg.RoleAssistant,
		Content:   strings.TrimSpace(resp.OutputText),
		ToolCalls: modelToolCallsToContextToolCalls(resp.ToolCalls),
	})
}

func recordModelRequest(traceSession trace.Session, request models.Request) {
	traceSession.Record(trace.EvtModelRequest, request)
}

func recordModelResponse(traceSession trace.Session, resp *models.Response) {
	traceSession.Record(trace.EvtModelResponse, resp)
}

func modelToolCallsToContextToolCalls(toolCalls []models.ToolCall) []handmsg.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}

	normalized := make([]handmsg.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		normalized = append(normalized, handmsg.ToolCall{ID: toolCall.ID, Name: toolCall.Name, Input: toolCall.Input})
	}

	return normalized
}
