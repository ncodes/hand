package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentsummary "github.com/wandxy/hand/internal/agent/context/summary"
	"github.com/wandxy/hand/internal/agent/runcontext"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/guardrails"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	"github.com/wandxy/hand/internal/state/search"
	"github.com/wandxy/hand/internal/tools"
	webextract "github.com/wandxy/hand/internal/tools/webextract"
	"github.com/wandxy/hand/internal/trace"
	agentcore "github.com/wandxy/hand/pkg/agent"
	pkgcache "github.com/wandxy/hand/pkg/cache"
	"github.com/wandxy/hand/pkg/logutils"
)

var jsonMarshal = json.Marshal

const defaultRecallSummaryCacheTTL = constants.DefaultRecallSummaryCacheTTL

const (
	EventKindTextDelta = agentcore.EventKindTextDelta
	EventKindTrace     = agentcore.EventKindTrace
)

var agentLog = logutils.InitLogger("agent")

type RespondOptions = agentcore.RespondOptions

type Event = agentcore.Event

type CompactSessionResult = agentcore.CompactSessionResult

type RepairSessionOptions = search.VectorRepairOptions

type RepairSessionResult = search.VectorRepairResult

type ContextStatus = agentcore.ContextStatus

type SessionTimelineOptions = agentcore.SessionTimelineOptions

type SessionTimeline = agentcore.SessionTimeline

type SessionTimelineMessage = agentcore.SessionTimelineMessage

type SessionTimelineTraceEvent = agentcore.SessionTimelineTraceEvent

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

// Start opens state, prepares the runtime environment, and marks the agent ready for requests.
func (a *Agent) Start(ctx context.Context) error {
	if a == nil {
		return errors.New("agent is required")
	}
	if a.cfg == nil {
		return errors.New("config is required")
	}

	ctx = normalizeContext(ctx)
	a.ctx = ctx

	// State is started before environment preparation because tools may need
	// session, trace, memory, or vector-store access during Prepare.
	if err := a.ensureStateManager(); err != nil {
		return err
	}

	if err := a.stateMgr.Start(ctx); err != nil {
		return err
	}

	// Environment setup wires the durable state and summary model client into
	// tools so they can execute with the same runtime identity as the agent.
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

// Close flushes memory candidates when configured and then closes state resources.
func (a *Agent) Close() error {
	if a == nil || a.stateMgr == nil {
		return nil
	}

	if !a.shouldFlushMemoryBeforeContextLoss() {
		return a.stateMgr.Close()
	}

	ctx := normalizeContext(a.ctx)
	sessionID, err := a.stateMgr.CurrentSession(ctx)
	if err == nil && strings.TrimSpace(sessionID) != "" {
		// Controlled shutdown can lose recent context, so give memory extraction
		// one last chance to preserve useful facts before closing storage.
		traceSession := trace.NoopSession()
		if a.env != nil {
			traceSession = a.openTraceSessionForSession(sessionID)
		}
		a.maybeFlushMemoryBeforeContextLoss(ctx, sessionID, memoryFlushTriggerControlledExit, traceSession)
		traceSession.Close()
	}

	return a.stateMgr.Close()
}

// Respond executes one user turn in the active or requested session.
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

	// Tests may construct an initialized agent with a nil environment. In that
	// case we lazily prepare one here, but normal app startup goes through Start.
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

	// Turn owns per-response state such as loaded history, retrieved memory,
	// request instruction overrides, streaming callbacks, and emitted messages.
	turn := a.newTurn(env, a.invokeToolWithEnvironment)
	reply, err := turn.Run(ctx, msg, opts)
	a.turnMessages = turn.Messages()
	if err == nil {
		a.maybeGenerateSessionTitle(ctx, turn.sessionID)
	}

	return reply, err
}

// TurnMessages returns a defensive copy of messages emitted by the most recent turn.
func (a *Agent) TurnMessages() []handmsg.Message {
	if a == nil || len(a.turnMessages) == 0 {
		return nil
	}

	messages := make([]handmsg.Message, len(a.turnMessages))
	copy(messages, a.turnMessages)
	return messages
}

// availableToolDefinitions resolves tools available under the current environment policy.
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

// invokeTool executes a model-requested tool using the agent's current environment.
func (a *Agent) invokeTool(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
	return a.invokeToolWithEnvironment(ctx, a.env, toolCall)
}

// getRootRunContext creates the root run identity for a public session ID.
func (a *Agent) getRootRunContext(sessionID string) (runcontext.Context, error) {
	return newRootRunContext(sessionID)
}

// openTraceSessionForSession opens a trace stream scoped to a session's root run.
func (a *Agent) openTraceSessionForSession(sessionID string) trace.Session {
	if a == nil || a.env == nil {
		return trace.NoopSession()
	}

	runCtx, err := a.getRootRunContext(sessionID)
	if err != nil {
		return trace.NoopSession()
	}

	return a.env.NewTraceSessionForRun(runCtx)
}

// invokeToolWithEnvironment executes a tool using a supplied environment.
func (a *Agent) invokeToolWithEnvironment(
	ctx context.Context,
	env environment.Environment,
	toolCall models.ToolCall,
) handmsg.Message {
	return invokeToolWithEnvironment(ctx, env, toolCall, a.summaryClient, a.cfg)
}

// invokeToolWithEnvironment adapts the tool registry response into a model-visible tool message.
func invokeToolWithEnvironment(
	ctx context.Context,
	env environment.Environment,
	toolCall models.ToolCall,
	summaryClient models.Client,
	cfg *config.Config,
) handmsg.Message {
	result := map[string]any{"name": toolCall.Name}

	// A missing registry is represented as a tool result instead of panicking so
	// the model sees a normal tool failure and the turn can complete cleanly.
	if env == nil || env.Tools() == nil {
		result["error"] = "tool registry is required"
		return toolResultMessage(toolCall, result)
	}

	// Web extraction can perform secondary summarization; thread the summary
	// client through context so the tool does not need to know about Agent.
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
		result["output"] = sanitizeToolOutputForModel(ctx, toolCall.Name, toolResult.Output, cfg)
	}

	return toolResultMessage(toolCall, result)
}

// sanitizeToolOutputForModel applies output guardrails before tool output is returned to the model.
func sanitizeToolOutputForModel(ctx context.Context, toolName string, output string, cfg *config.Config) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	if cfg == nil || !cfg.OutputSafetyEnabled() {
		return output
	}

	result := guardrails.CheckUntrustedContentSafety(
		output,
		"tool."+strings.TrimSpace(toolName),
		guardrails.NewRedactorWithOptions(guardrails.RedactorOptions{
			DisablePII: !cfg.OutputPIIRedactionEnabled(),
		}),
	)
	if result.Blocked || result.Redacted {
		recordToolOutputSafety(ctx, toolName, output, result)
	}
	return strings.TrimSpace(result.Content)
}

// recordToolOutputSafety records redaction/blocking decisions for tool output.
func recordToolOutputSafety(
	ctx context.Context,
	toolName string,
	output string,
	result guardrails.UntrustedContentSafetyResult,
) {
	recorder := tools.TraceRecorderFromContext(ctx)
	if recorder == nil {
		return
	}

	action := "redacted"
	if result.Blocked {
		action = "blocked"
	}
	recorder.Record(trace.EvtToolOutputSafetyApplied, trace.SafetyEventPayload{
		Source:        "tool." + strings.TrimSpace(toolName),
		Action:        action,
		ContentLength: len([]rune(output)),
		Blocked:       result.Blocked,
		Redacted:      result.Redacted,
		Findings:      guardrails.SafetyFindingLogFields(result.Findings),
	})
}

// toolResultMessage serializes a tool result map into the assistant conversation format.
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

// CreateSession creates or returns a named session through the state manager.
func (a *Agent) CreateSession(ctx context.Context, id string) (storage.Session, error) {
	if a == nil {
		return storage.Session{}, errors.New("agent is required")
	}

	if !a.initialized || a.stateMgr == nil {
		return storage.Session{}, errors.New("environment has not been initialized")
	}

	return a.stateMgr.CreateSession(normalizeContext(ctx), id)
}

// ListSessions returns all known sessions.
func (a *Agent) ListSessions(ctx context.Context) ([]storage.Session, error) {
	if a == nil {
		return nil, errors.New("agent is required")
	}

	if !a.initialized || a.stateMgr == nil {
		return nil, errors.New("environment has not been initialized")
	}

	return a.stateMgr.ListSessions(normalizeContext(ctx))
}

// UseSession switches the current session and flushes memory for the previous one when needed.
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
		// Switching sessions can leave useful recent context behind, so run the
		// same memory preservation path used for shutdown/compaction.
		traceSession := trace.NoopSession()
		if a.env != nil {
			traceSession = a.openTraceSessionForSession(currentSessionID)
		}
		a.maybeFlushMemoryBeforeContextLoss(ctx, currentSessionID, memoryFlushTriggerSessionReset, traceSession)
		traceSession.Close()
	}

	return a.stateMgr.UseSession(ctx, targetSession.ID)
}

// CurrentSession returns the full current session record.
func (a *Agent) CurrentSession(ctx context.Context) (storage.Session, error) {
	if a == nil {
		return storage.Session{}, errors.New("agent is required")
	}

	if !a.initialized || a.stateMgr == nil {
		return storage.Session{}, errors.New("environment has not been initialized")
	}

	ctx = normalizeContext(ctx)
	id, err := a.stateMgr.CurrentSession(ctx)
	if err != nil {
		return storage.Session{}, err
	}

	session, ok, err := a.stateMgr.Get(ctx, id)
	if err != nil {
		return storage.Session{}, err
	}
	if !ok {
		return storage.Session{}, fmt.Errorf("session %q not found", id)
	}

	return session, nil
}

// CompactSession forces persisted compaction for a session and returns compacted context metrics.
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

// RepairSession rebuilds or checks session vector indexes.
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

	// Recall summaries are temporary: they are useful for inspection and
	// retrieval, but they should not advance the session compaction offset.
	traceSession := trace.NoopSession()
	if a.env != nil {
		traceSession = a.openTraceSessionForSession(session.ID)
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

// summarizeSession forces a persisted summary/compaction pass for a session.
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

	// Manual compaction is another context-loss boundary, so memory extraction
	// runs before the summary replaces old messages in active context.
	traceSession := trace.NoopSession()
	if a.env != nil {
		traceSession = a.openTraceSessionForSession(session.ID)
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

// ContextStatus reports prompt-token and compaction status for a session.
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

// GetSession is the compatibility alias for ContextStatus.
func (a *Agent) GetSession(ctx context.Context, id string) (ContextStatus, error) {
	return a.ContextStatus(ctx, id)
}

// ensureStateManager lazily opens storage and creates the state manager.
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

// cachedRecallSummary returns a cached recall summary only when it still covers the full session.
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

// storeRecallSummary stores a defensive copy through the recall summary cache.
func (a *Agent) storeRecallSummary(summary storage.SessionSummary) {
	if a == nil || a.recallSummaryCache == nil || strings.TrimSpace(summary.SessionID) == "" {
		return
	}

	a.recallSummaryCache.Set(summary.SessionID, summary)
}

// isFullRecallSummary reports whether a cached summary covers every message in the session.
func isFullRecallSummary(summary storage.SessionSummary, messageCount int) bool {
	return summary.SourceMessageCount == messageCount && summary.SourceEndOffset == messageCount
}

// getDurationOrDefault chooses fallback when value is unset or invalid.
func getDurationOrDefault(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

// normalizeToolError preserves structured tool errors when possible.
func normalizeToolError(raw string) any {
	var toolErr tools.Error
	if err := json.Unmarshal([]byte(raw), &toolErr); err == nil &&
		strings.TrimSpace(toolErr.Code) != "" &&
		strings.TrimSpace(toolErr.Message) != "" {
		return toolErr
	}

	return raw
}

// normalizeContext replaces a nil context with context.Background.
func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// modelToolDefinitionFromToolDefinition converts registry tool definitions into model tool definitions.
func modelToolDefinitionFromToolDefinition(definition tools.Definition) models.ToolDefinition {
	return models.ToolDefinition{
		Name:        definition.Name,
		Description: definition.Description,
		InputSchema: definition.InputSchema,
	}
}

// assistantToolCallMessageFromResponse converts model tool calls into a persisted assistant message.
func assistantToolCallMessageFromResponse(resp *models.Response) (handmsg.Message, error) {
	return normalizeTurnMessage(handmsg.Message{
		Role:      handmsg.RoleAssistant,
		Content:   strings.TrimSpace(resp.OutputText),
		ToolCalls: modelToolCallsToContextToolCalls(resp.ToolCalls),
	})
}

// recordModelRequest records the full model request shape for trace inspection.
func recordModelRequest(traceSession trace.Session, request models.Request) {
	traceSession.Record(trace.EvtModelRequest, request)
}

// recordModelResponse records model response metadata while dropping assistant text from traces.
func recordModelResponse(traceSession trace.Session, resp *models.Response) {
	if resp == nil {
		traceSession.Record(trace.EvtModelResponse, resp)
		return
	}

	safeResponse := *resp
	safeResponse.OutputText = ""
	traceSession.Record(trace.EvtModelResponse, safeResponse)
}

// modelToolCallsToContextToolCalls converts model tool calls into session message tool calls.
func modelToolCallsToContextToolCalls(toolCalls []models.ToolCall) []handmsg.ToolCall {
	return models.ToolCallsToMessageToolCalls(toolCalls)
}
