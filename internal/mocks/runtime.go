package mocks

import (
	"context"
	"errors"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	instruct "github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

type ModelClientStub struct {
	Requests  []models.Request
	Responses []*models.Response
	Errors    []error
	Err       error
	CallCount int
}

func (s *ModelClientStub) Chat(_ context.Context, req models.Request) (*models.Response, error) {
	s.Requests = append(s.Requests, req)
	if s.CallCount < len(s.Errors) && s.Errors[s.CallCount] != nil {
		s.CallCount++
		return nil, s.Errors[s.CallCount-1]
	}

	if s.Err != nil {
		return nil, s.Err
	}

	if s.CallCount >= len(s.Responses) {
		return nil, errors.New("missing stubbed response")
	}
	response := s.Responses[s.CallCount]
	s.CallCount++
	return response, nil
}

type EnvironmentStub struct {
	PrepareErr       error
	InstructionsList instruct.Instructions
	ToolRegistry     environment.ToolRegistry
	Policy           tools.Policy
	IterationBudget  environment.IterationBudget
	TraceSession     trace.Session
}

func (s *EnvironmentStub) Prepare() error {
	return s.PrepareErr
}

func (s *EnvironmentStub) Instructions() instruct.Instructions {
	return s.InstructionsList
}

func (s *EnvironmentStub) Tools() environment.ToolRegistry {
	return s.ToolRegistry
}

func (s *EnvironmentStub) ToolPolicy() tools.Policy {
	return s.Policy
}

func (s *EnvironmentStub) NewIterationBudget() environment.IterationBudget {
	if s.IterationBudget.Remaining() <= 0 {
		return environment.NewIterationBudget(config.DefaultMaxIterations)
	}

	return s.IterationBudget
}

func (s *EnvironmentStub) NewTraceSession() trace.Session {
	if s.TraceSession == nil {
		return trace.NoopSession()
	}

	return s.TraceSession
}

type ToolRegistryStub struct {
	Definitions    []tools.Definition
	Groups         []tools.Group
	LastToolPolicy tools.Policy
	Result         tools.Result
	Err            error
	ResolveErr     error
}

func (s *ToolRegistryStub) List() []tools.Definition {
	return s.Definitions
}

func (s *ToolRegistryStub) GetGroup(name string) (tools.Group, bool) {
	for _, group := range s.Groups {
		if group.Name == name {
			return group, true
		}
	}

	return tools.Group{}, false
}

func (s *ToolRegistryStub) ListGroups() []tools.Group {
	return s.Groups
}

func (s *ToolRegistryStub) Resolve(opts tools.Policy) ([]tools.Definition, error) {
	s.LastToolPolicy = opts
	return s.Definitions, s.ResolveErr
}

func (s *ToolRegistryStub) Invoke(context.Context, tools.Call) (tools.Result, error) {
	return s.Result, s.Err
}

type TraceSessionStub struct {
	SessionID string
	Events    []trace.Event
	Closed    bool
}

func (s *TraceSessionStub) ID() string {
	return s.SessionID
}

func (s *TraceSessionStub) Record(eventType string, payload any) {
	s.Events = append(s.Events, trace.Event{Type: eventType, Payload: payload})
}

func (s *TraceSessionStub) Close() {
	s.Closed = true
}
