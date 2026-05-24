package host

import (
	"context"
	"errors"

	agentsession "github.com/wandxy/hand/pkg/agent/session"

	handmsg "github.com/wandxy/hand/internal/messages"
	storage "github.com/wandxy/hand/internal/state/core"
)

var errSessionManagerRequired = errors.New("session manager is required")

type SessionManager interface {
	Resolve(context.Context, string) (storage.Session, error)
	GetMessages(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error)
	AppendMessages(context.Context, string, []handmsg.Message) error
	UpdateLastPromptTokens(context.Context, string, int) error
	AppendTraceEvent(context.Context, storage.TraceEvent) (storage.TraceEvent, error)
}

type SessionStore struct {
	manager SessionManager
}

func NewSessionStore(manager SessionManager) *SessionStore {
	return &SessionStore{manager: manager}
}

func (s *SessionStore) Resolve(ctx context.Context, id string) (agentsession.Session, error) {
	if s == nil || s.manager == nil {
		return agentsession.Session{}, errSessionManagerRequired
	}

	resolved, err := s.manager.Resolve(ctx, id)
	if err != nil {
		return agentsession.Session{}, err
	}

	return agentSessionFromStorageSession(resolved), nil
}

func (s *SessionStore) GetMessages(
	ctx context.Context,
	id string,
	query agentsession.MessageQuery,
) ([]handmsg.Message, error) {
	if s == nil || s.manager == nil {
		return nil, errSessionManagerRequired
	}

	return s.manager.GetMessages(ctx, id, messageQueryToStorageMessageQuery(query))
}

func (s *SessionStore) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	if s == nil || s.manager == nil {
		return errSessionManagerRequired
	}

	return s.manager.AppendMessages(ctx, id, messages)
}

func (s *SessionStore) UpdateLastPromptTokens(ctx context.Context, id string, tokens int) error {
	if s == nil || s.manager == nil {
		return errSessionManagerRequired
	}

	return s.manager.UpdateLastPromptTokens(ctx, id, tokens)
}

func (s *SessionStore) AppendTraceEvent(
	ctx context.Context,
	event agentsession.TraceEvent,
) (agentsession.TraceEvent, error) {
	if s == nil || s.manager == nil {
		return agentsession.TraceEvent{}, errSessionManagerRequired
	}

	stored, err := s.manager.AppendTraceEvent(ctx, storageTraceEventFromAgentTraceEvent(event))
	if err != nil {
		return agentsession.TraceEvent{}, err
	}

	return agentTraceEventFromStorageTraceEvent(stored), nil
}

func agentSessionFromStorageSession(value storage.Session) agentsession.Session {
	return agentsession.Session{
		CreatedAt:                  value.CreatedAt,
		Compaction:                 agentCompactionFromStorageCompaction(value.Compaction),
		ID:                         value.ID,
		EpisodicCheckpointOffset:   value.EpisodicCheckpointOffset,
		LastPromptTokens:           value.LastPromptTokens,
		ReflectionCheckpointOffset: value.ReflectionCheckpointOffset,
		Title:                      value.Title,
		TitleSource:                value.TitleSource,
		UpdatedAt:                  value.UpdatedAt,
	}
}

func agentCompactionFromStorageCompaction(value storage.SessionCompaction) agentsession.Compaction {
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

func messageQueryToStorageMessageQuery(value agentsession.MessageQuery) storage.MessageQueryOptions {
	return storage.MessageQueryOptions{
		Archived: value.Archived,
		Limit:    value.Limit,
		Name:     value.Name,
		Order:    value.Order,
		Offset:   value.Offset,
		Role:     handmsg.Role(value.Role),
	}
}

func storageTraceEventFromAgentTraceEvent(value agentsession.TraceEvent) storage.TraceEvent {
	return storage.TraceEvent{
		ID:        value.ID,
		SessionID: value.SessionID,
		Sequence:  value.Sequence,
		Type:      value.Type,
		Timestamp: value.Timestamp,
		Payload:   value.Payload,
	}
}

func agentTraceEventFromStorageTraceEvent(value storage.TraceEvent) agentsession.TraceEvent {
	return agentsession.TraceEvent{
		ID:        value.ID,
		SessionID: value.SessionID,
		Sequence:  value.Sequence,
		Type:      value.Type,
		Timestamp: value.Timestamp,
		Payload:   value.Payload,
	}
}
