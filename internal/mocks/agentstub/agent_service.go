package agentstub

import (
	"context"
	"strings"

	agentapi "github.com/wandxy/hand/internal/agent"
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
	Sessions             []storage.Session
	UsedSessionID        string
	UseSessionErr        error
	CurrentSessionResult storage.Session
	CompactResult        rpcclient.CompactSessionResult
	RepairOptions        search.VectorRepairOptions
	RepairResult         search.VectorRepairResult
	SummaryResult        storage.SessionSummary
	StatusResult         rpcclient.ContextStatus
	TimelineOptions      agentapi.SessionTimelineOptions
	TimelineResult       agentapi.SessionTimeline
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

func (s *AgentServiceStub) CreateSession(_ context.Context, id string) (storage.Session, error) {
	s.CreatedSessionID = id
	return s.CreatedSession, s.Err
}

func (s *AgentServiceStub) ListSessions(context.Context) ([]storage.Session, error) {
	return s.Sessions, s.Err
}

func (s *AgentServiceStub) UseSession(_ context.Context, id string) error {
	s.UsedSessionID = id
	if s.UseSessionErr != nil {
		return s.UseSessionErr
	}
	return s.Err
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

func (s *AgentServiceStub) CompactSession(context.Context, string) (rpcclient.CompactSessionResult, error) {
	return s.CompactResult, s.Err
}

func (s *AgentServiceStub) RepairSession(
	_ context.Context,
	opts search.VectorRepairOptions,
) (search.VectorRepairResult, error) {
	s.RepairOptions = opts
	return s.RepairResult, s.Err
}

func (s *AgentServiceStub) GetSession(context.Context, string) (rpcclient.ContextStatus, error) {
	return s.StatusResult, s.Err
}

func (s *AgentServiceStub) ContextStatus(context.Context, string) (agent.ContextStatus, error) {
	return agent.ContextStatus(s.StatusResult), s.Err
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
