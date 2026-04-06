package agentstub

import (
	"context"

	"github.com/wandxy/hand/internal/agent"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/internal/storage"
)

type AgentServiceStub struct {
	ChatInput        string
	RespondOptions   rpcclient.RespondOptions
	Reply            string
	Deltas           []string
	Events           []agent.Event
	RespondErr       error
	Err              error
	CloseErr         error
	Closed           bool
	CreatedSession   storage.Session
	Sessions         []storage.Session
	UsedSessionID    string
	CurrentSessionID string
	CompactResult    rpcclient.CompactSessionResult
	StatusResult     rpcclient.SessionContextStatus
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
				events = append(events, agent.Event{Channel: "assistant", Text: delta})
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

func (s *AgentServiceStub) CreateSession(context.Context, string) (storage.Session, error) {
	return s.CreatedSession, s.Err
}

func (s *AgentServiceStub) ListSessions(context.Context) ([]storage.Session, error) {
	return s.Sessions, s.Err
}

func (s *AgentServiceStub) UseSession(_ context.Context, id string) error {
	s.UsedSessionID = id
	return s.Err
}

func (s *AgentServiceStub) CurrentSession(context.Context) (string, error) {
	return s.CurrentSessionID, s.Err
}

func (s *AgentServiceStub) CompactSession(context.Context, string) (rpcclient.CompactSessionResult, error) {
	return s.CompactResult, s.Err
}

func (s *AgentServiceStub) GetSession(context.Context, string) (rpcclient.SessionContextStatus, error) {
	return s.StatusResult, s.Err
}

func (s *AgentServiceStub) SessionContextStatus(context.Context, string) (agent.SessionContextStatus, error) {
	return agent.SessionContextStatus(s.StatusResult), s.Err
}

func (s *AgentServiceStub) Close() error {
	s.Closed = true
	return s.CloseErr
}

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
