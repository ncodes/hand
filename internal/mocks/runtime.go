package mocks

import (
	"context"
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/config"
	handcontext "github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

type ModelClientStub struct {
	Requests  []models.GenerateRequest
	Responses []*models.GenerateResponse
	Errors    []error
	Err       error
	CallCount int
}

func (s *ModelClientStub) Chat(_ context.Context, req models.GenerateRequest) (*models.GenerateResponse, error) {
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
	PrepareErr      error
	RuntimeContext  environment.Context
	ToolRegistry    environment.ToolRegistry
	Policy          tools.Policy
	IterationBudget environment.IterationBudget
	TraceSession    trace.Session
}

func (s *EnvironmentStub) Prepare() error {
	return s.PrepareErr
}

func (s *EnvironmentStub) Context() environment.Context {
	return s.RuntimeContext
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

type ContextStub struct {
	Instructions        handcontext.Instructions
	Messages            []handcontext.Message
	Conversation        handcontext.Conversation
	AddUserMessageErr   error
	AddAssistantMsgErr  error
	AddMessageErr       error
	AddMessageErrOnCall int
	AddMessageCallCount int
}

func (s *ContextStub) GetInstructions() handcontext.Instructions {
	return s.Instructions
}

func (s *ContextStub) SetInstruction(instruction handcontext.Instruction) {
	instruction.Name = strings.TrimSpace(instruction.Name)
	instruction.Value = strings.TrimSpace(instruction.Value)
	if instruction.Name == "" {
		s.Instructions = append(s.Instructions, instruction)
		return
	}
	for idx, existing := range s.Instructions {
		if existing.Name == instruction.Name {
			if instruction.Value == "" {
				s.Instructions = append(s.Instructions[:idx], s.Instructions[idx+1:]...)
				return
			}
			s.Instructions[idx] = instruction
			return
		}
	}
	if instruction.Value != "" {
		s.Instructions = append(s.Instructions, instruction)
	}
}

func (s *ContextStub) RemoveInstruction(name string) {
	s.Instructions = s.Instructions.WithoutName(name)
}

func (s *ContextStub) AddMessage(message handcontext.Message) error {
	s.AddMessageCallCount++
	if s.AddMessageErrOnCall > 0 && s.AddMessageCallCount == s.AddMessageErrOnCall {
		return s.AddMessageErr
	}
	if s.AddMessageErr != nil && s.AddMessageErrOnCall == 0 {
		return s.AddMessageErr
	}
	s.Messages = append(s.Messages, message)
	_ = s.Conversation.Append(message)
	return nil
}

func (s *ContextStub) AddUserMessage(content string) error {
	if s.AddUserMessageErr != nil {
		return s.AddUserMessageErr
	}
	message, err := handcontext.NewMessage(handcontext.RoleUser, content)
	if err != nil {
		return err
	}
	s.Messages = append(s.Messages, message)
	_ = s.Conversation.Append(message)
	return nil
}

func (s *ContextStub) AddAssistantMessage(content string) error {
	if s.AddAssistantMsgErr != nil {
		return s.AddAssistantMsgErr
	}
	message, err := handcontext.NewMessage(handcontext.RoleAssistant, content)
	if err != nil {
		return err
	}
	s.Messages = append(s.Messages, message)
	_ = s.Conversation.Append(message)
	return nil
}

func (s *ContextStub) GetMessages() []handcontext.Message {
	out := make([]handcontext.Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}

func (s *ContextStub) GetConversation() handcontext.Conversation {
	return s.Conversation
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
