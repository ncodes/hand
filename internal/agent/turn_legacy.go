package agent

import (
	"context"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/manager"
	agentprompt "github.com/wandxy/hand/pkg/agent/prompt"
	agentsession "github.com/wandxy/hand/pkg/agent/session"
	agenttool "github.com/wandxy/hand/pkg/agent/tool"
)

// NewTurn keeps the existing Hand constructor while the turn loop moves behind reusable boundaries.
func NewTurn(
	cfg *config.Config,
	modelClient models.Client,
	summaryClient models.Client,
	stateMgr *manager.Manager,
	invokeToolFn func(context.Context, environment.Environment, models.ToolCall) handmsg.Message,
	runtimeEnv environment.Environment,
) *Turn {
	if summaryClient == nil {
		summaryClient = modelClient
	}
	if invokeToolFn == nil && runtimeEnv != nil {
		invokeToolFn = func(ctx context.Context, env environment.Environment, toolCall models.ToolCall) handmsg.Message {
			return invokeToolWithEnvironment(ctx, env, toolCall, summaryClient, cfg)
		}
	}

	var sessionStore agentsession.Store
	var traceRecorder agentsession.TraceRecorder
	if stateMgr != nil {
		store := legacySessionStore{manager: stateMgr}
		sessionStore = store
		traceRecorder = store
	}
	var promptProvider agentprompt.Provider
	var traceSessions traceSessionFactory
	var safetyEvents safetyTraceEventSource
	var memoryProviders memoryProviderSource
	var iterationBudgets iterationBudgetFactory
	var plans planStateStore
	var legacyRuntime any
	var toolInvoker any
	if runtimeEnv != nil {
		promptProvider = legacyPromptProvider{env: runtimeEnv}
		traceSessions = runtimeEnv
		safetyEvents = runtimeEnv
		memoryProviders = runtimeEnv
		iterationBudgets = runtimeEnv
		plans = runtimeEnv
		legacyRuntime = runtimeEnv
	}
	if invokeToolFn != nil {
		toolInvoker = func(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
			return invokeToolFn(ctx, runtimeEnv, toolCall)
		}
	}

	return NewTurnWithSessionStore(
		cfg,
		modelClient,
		summaryClient,
		stateMgr,
		sessionStore,
		traceRecorder,
		nil,
		agenttool.Policy{},
		promptProvider,
		traceSessions,
		safetyEvents,
		memoryProviders,
		iterationBudgets,
		plans,
		legacyRuntime,
		toolInvoker,
	)
}

type legacySessionStore struct {
	manager *manager.Manager
}

func (s legacySessionStore) Resolve(ctx context.Context, id string) (agentsession.Session, error) {
	resolved, err := s.manager.Resolve(ctx, id)
	if err != nil {
		return agentsession.Session{}, err
	}

	return legacySessionFromStorageSession(resolved), nil
}

func (s legacySessionStore) GetMessages(
	ctx context.Context,
	id string,
	query agentsession.MessageQuery,
) ([]handmsg.Message, error) {
	return s.manager.GetMessages(ctx, id, storage.MessageQueryOptions{
		Archived: query.Archived,
		Limit:    query.Limit,
		Name:     query.Name,
		Order:    query.Order,
		Offset:   query.Offset,
		Role:     handmsg.Role(query.Role),
	})
}

func (s legacySessionStore) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	return s.manager.AppendMessages(ctx, id, messages)
}

func (s legacySessionStore) UpdateLastPromptTokens(ctx context.Context, id string, tokens int) error {
	return s.manager.UpdateLastPromptTokens(ctx, id, tokens)
}

func (s legacySessionStore) AppendTraceEvent(
	ctx context.Context,
	event agentsession.TraceEvent,
) (agentsession.TraceEvent, error) {
	stored, err := s.manager.AppendTraceEvent(ctx, storage.TraceEvent{
		ID:        event.ID,
		SessionID: event.SessionID,
		Sequence:  event.Sequence,
		Type:      event.Type,
		Timestamp: event.Timestamp,
		Payload:   event.Payload,
	})
	if err != nil {
		return agentsession.TraceEvent{}, err
	}

	return agentsession.TraceEvent{
		ID:        stored.ID,
		SessionID: stored.SessionID,
		Sequence:  stored.Sequence,
		Type:      stored.Type,
		Timestamp: stored.Timestamp,
		Payload:   stored.Payload,
	}, nil
}

func legacySessionFromStorageSession(value storage.Session) agentsession.Session {
	return agentsession.Session{
		CreatedAt:                  value.CreatedAt,
		Compaction:                 legacyCompactionFromStorageCompaction(value.Compaction),
		ID:                         value.ID,
		EpisodicCheckpointOffset:   value.EpisodicCheckpointOffset,
		LastPromptTokens:           value.LastPromptTokens,
		ReflectionCheckpointOffset: value.ReflectionCheckpointOffset,
		Title:                      value.Title,
		TitleSource:                value.TitleSource,
		UpdatedAt:                  value.UpdatedAt,
	}
}

func legacyCompactionFromStorageCompaction(value storage.SessionCompaction) agentsession.Compaction {
	return agentsession.Compaction{
		CompletedAt:        value.CompletedAt,
		FailedAt:           value.FailedAt,
		LastError:          value.LastError,
		RequestedAt:        value.RequestedAt,
		StartedAt:          value.StartedAt,
		Status:             agentsession.CompactionStatus(value.Status),
		TargetMessageCount: value.TargetMessageCount,
		TargetOffset:       value.TargetOffset,
	}
}

type legacyPromptProvider struct {
	env environment.Environment
}

func (p legacyPromptProvider) LoadBaseInstructions(
	context.Context,
	agentprompt.RunContext,
) (agentprompt.Instructions, error) {
	if p.env == nil {
		return nil, nil
	}

	instructions := p.env.Instructions()
	if len(instructions) == 0 {
		return nil, nil
	}

	result := make(agentprompt.Instructions, 0, len(instructions))
	for _, instruction := range instructions {
		result = append(result, agentprompt.Instruction{
			Name:  instruction.Name,
			Value: instruction.Value,
		})
	}

	return result, nil
}

func (p legacyPromptProvider) BuildEnvironmentInstruction(
	context.Context,
	agentprompt.EnvironmentInput,
) (agentprompt.Instruction, error) {
	return agentprompt.Instruction{}, nil
}
