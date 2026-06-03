package agentstub

import (
	"context"
	"strings"

	agentapi "github.com/wandxy/hand/internal/agent"
	models "github.com/wandxy/hand/internal/model"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
	agent "github.com/wandxy/hand/pkg/agent"
)

// AgentServiceStub is a test stub for agent service.
type AgentServiceStub struct {
	ChatInput            string
	RespondOptions       rpcclient.RespondOptions
	Reply                string
	Deltas               []string
	Events               []agent.Event
	RespondErr           error
	Err                  error
	CloseErr             error
	Closed               bool
	CreatedSession       storage.Session
	CreatedSessionID     string
	CreateSessionOptions rpcclient.CreateSessionOptions
	Sessions             []storage.Session
	ArchivedSessions     []storage.Session
	ListOptions          storage.SessionListOptions
	UsedSessionID        string
	UseSessionErr        error
	ArchivedSessionID    string
	ArchiveSessionErr    error
	UnarchivedSessionID  string
	UnarchivedSession    storage.Session
	UnarchiveSessionErr  error
	RenamedSessionID     string
	RenamedSessionTitle  string
	RenamedSession       storage.Session
	RenameSessionErr     error
	CurrentSessionResult storage.Session
	CompactResult        rpcclient.CompactSessionResult
	RepairOptions        search.VectorRepairOptions
	RepairResult         search.VectorRepairResult
	SummaryResult        storage.SessionSummary
	StatusResult         rpcclient.ContextStatus
	TimelineOptions      agentapi.SessionTimelineOptions
	TimelineResult       agentapi.SessionTimeline
	ProviderList         agentapi.ProviderList
	ModelList            agentapi.ModelList
	ModelListOptions     agentapi.ModelListOptions
	SelectedModel        models.Option
	SelectedModelID      string
	SelectedModelOptions agentapi.ModelSelectOptions
	SelectModelErr       error
	ProviderAPIKey       string
	ProviderAPIKeyID     string
	SetProviderAPIKeyErr error
}

func (s *AgentServiceStub) Respond(_ context.Context, msg string, opts rpcclient.RespondOptions) (string, error) {
	s.ChatInput = msg
	s.RespondOptions = opts
	if opts.OnEvent != nil {
		events := s.Events
		if len(events) == 0 {
			deltas := s.Deltas
			if len(deltas) == 0 && s.Reply != "" {
				deltas = []string{s.Reply}
			}
			events = make([]agent.Event, 0, len(deltas))
			for _, delta := range deltas {
				events = append(events, agent.Event{
					Kind:    agent.EventKindTextDelta,
					Channel: "assistant",
					Text:    delta,
				})
			}
		}
		for _, event := range events {
			opts.OnEvent(event)
		}
	}
	if s.RespondErr != nil {
		return s.Reply, s.RespondErr
	}
	return s.Reply, s.Err
}

func (s *AgentServiceStub) SessionAPI() rpcclient.SessionAPI {
	return s
}

func (s *AgentServiceStub) ModelAPI() rpcclient.ModelAPI {
	return s
}

func (s *AgentServiceStub) ListProviders(context.Context) (agentapi.ProviderList, error) {
	return s.ProviderList, s.Err
}

func (s *AgentServiceStub) ListModels(_ context.Context, opts ...agentapi.ModelListOptions) (agentapi.ModelList, error) {
	if len(opts) > 0 {
		s.ModelListOptions = opts[0]
	}

	return s.ModelList, s.Err
}

func (s *AgentServiceStub) SelectModel(_ context.Context, id string, opts ...agentapi.ModelSelectOptions) (models.Option, error) {
	s.SelectedModelID = id
	if len(opts) > 0 {
		s.SelectedModelOptions = opts[0]
	}
	if s.SelectModelErr != nil {
		return models.Option{}, s.SelectModelErr
	}
	if strings.TrimSpace(s.SelectedModel.ID) != "" {
		return s.SelectedModel, s.Err
	}

	return models.Option{ID: id, Current: true}, s.Err
}

func (s *AgentServiceStub) SetProviderAPIKey(_ context.Context, provider string, apiKey string) error {
	s.ProviderAPIKeyID = provider
	s.ProviderAPIKey = apiKey

	return s.SetProviderAPIKeyErr
}

func (s *AgentServiceStub) Create(ctx context.Context, id string) (storage.Session, error) {
	return s.CreateSession(ctx, id)
}

func (s *AgentServiceStub) CreateSession(_ context.Context, id string) (storage.Session, error) {
	s.CreatedSessionID = id
	return s.CreatedSession, s.Err
}

func (s *AgentServiceStub) CreateWithOptions(
	ctx context.Context,
	opts rpcclient.CreateSessionOptions,
) (storage.Session, error) {
	return s.CreateSessionWithOptions(ctx, opts)
}

func (s *AgentServiceStub) CreateSessionWithOptions(
	_ context.Context,
	opts rpcclient.CreateSessionOptions,
) (storage.Session, error) {
	s.CreateSessionOptions = opts
	s.CreatedSessionID = opts.ID
	return s.CreatedSession, s.Err
}

func (s *AgentServiceStub) List(ctx context.Context, opts ...rpcclient.SessionListOptions) ([]storage.Session, error) {
	return s.ListSessions(ctx, opts...)
}

func (s *AgentServiceStub) ListSessions(_ context.Context, opts ...storage.SessionListOptions) ([]storage.Session, error) {
	if len(opts) > 0 {
		s.ListOptions = opts[0]
		if opts[0].Archived != nil && *opts[0].Archived {
			return s.ArchivedSessions, s.Err
		}
	}
	return s.Sessions, s.Err
}

func (s *AgentServiceStub) Use(ctx context.Context, id string) error {
	return s.UseSession(ctx, id)
}

func (s *AgentServiceStub) UseSession(_ context.Context, id string) error {
	s.UsedSessionID = id
	if s.UseSessionErr != nil {
		return s.UseSessionErr
	}
	return s.Err
}

func (s *AgentServiceStub) Archive(ctx context.Context, id string) error {
	return s.ArchiveSession(ctx, id)
}

func (s *AgentServiceStub) ArchiveSession(_ context.Context, id string) error {
	s.ArchivedSessionID = id
	if s.ArchiveSessionErr != nil {
		return s.ArchiveSessionErr
	}
	return s.Err
}

func (s *AgentServiceStub) Unarchive(ctx context.Context, id string) (storage.Session, error) {
	return s.UnarchiveSession(ctx, id)
}

func (s *AgentServiceStub) UnarchiveSession(_ context.Context, id string) (storage.Session, error) {
	s.UnarchivedSessionID = id
	if s.UnarchiveSessionErr != nil {
		return storage.Session{}, s.UnarchiveSessionErr
	}
	if strings.TrimSpace(s.UnarchivedSession.ID) != "" {
		return s.UnarchivedSession, s.Err
	}
	return storage.Session{ID: id}, s.Err
}

func (s *AgentServiceStub) Rename(ctx context.Context, id string, title string) (storage.Session, error) {
	return s.RenameSession(ctx, id, title)
}

func (s *AgentServiceStub) RenameSession(_ context.Context, id string, title string) (storage.Session, error) {
	s.RenamedSessionID = id
	s.RenamedSessionTitle = title
	if s.RenameSessionErr != nil {
		return storage.Session{}, s.RenameSessionErr
	}
	if strings.TrimSpace(s.RenamedSession.ID) != "" {
		return s.RenamedSession, s.Err
	}
	return storage.Session{ID: id, Title: title, TitleSource: storage.SessionTitleSourceManual}, s.Err
}

func (s *AgentServiceStub) Current(ctx context.Context) (storage.Session, error) {
	return s.CurrentSession(ctx)
}

func (s *AgentServiceStub) CurrentSession(context.Context) (storage.Session, error) {
	if strings.TrimSpace(s.CurrentSessionResult.ID) != "" || strings.TrimSpace(s.CurrentSessionResult.Title) != "" {
		return s.CurrentSessionResult, s.Err
	}

	return storage.Session{}, s.Err
}

func (s *AgentServiceStub) RecallSessionSummary(context.Context, string) (storage.SessionSummary, error) {
	return s.SummaryResult, s.Err
}

func (s *AgentServiceStub) Compact(ctx context.Context, id string) (rpcclient.CompactSessionResult, error) {
	return s.CompactSession(ctx, id)
}

func (s *AgentServiceStub) CompactSession(context.Context, string) (rpcclient.CompactSessionResult, error) {
	return s.CompactResult, s.Err
}

func (s *AgentServiceStub) Repair(
	ctx context.Context,
	opts search.VectorRepairOptions,
) (search.VectorRepairResult, error) {
	return s.RepairSession(ctx, opts)
}

func (s *AgentServiceStub) RepairSession(
	_ context.Context,
	opts search.VectorRepairOptions,
) (search.VectorRepairResult, error) {
	s.RepairOptions = opts
	return s.RepairResult, s.Err
}

func (s *AgentServiceStub) Status(ctx context.Context, id string) (rpcclient.ContextStatus, error) {
	return s.GetSessionStatus(ctx, id)
}

func (s *AgentServiceStub) GetSessionStatus(context.Context, string) (rpcclient.ContextStatus, error) {
	return s.StatusResult, s.Err
}

func (s *AgentServiceStub) ContextStatus(context.Context, string) (agent.ContextStatus, error) {
	return agent.ContextStatus(s.StatusResult), s.Err
}

func (s *AgentServiceStub) Timeline(
	ctx context.Context,
	opts agentapi.SessionTimelineOptions,
) (agentapi.SessionTimeline, error) {
	return s.GetSessionTimeline(ctx, opts)
}

func (s *AgentServiceStub) GetSessionTimeline(
	_ context.Context,
	opts agentapi.SessionTimelineOptions,
) (agentapi.SessionTimeline, error) {
	s.TimelineOptions = opts
	return s.TimelineResult, s.Err
}

func (s *AgentServiceStub) Close() error {
	s.Closed = true
	return s.CloseErr
}

// AgentRunnerStub is a test stub for agent runner.
type AgentRunnerStub struct {
	AgentServiceStub
	StartFunc func(context.Context) error
}

func (s *AgentRunnerStub) Start(ctx context.Context) error {
	if s.StartFunc != nil {
		return s.StartFunc(ctx)
	}

	return nil
}
