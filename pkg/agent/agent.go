package agent

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/wandxy/hand/pkg/agent/message"
	"github.com/wandxy/hand/pkg/agent/model"
	"github.com/wandxy/hand/pkg/agent/prompt"
	"github.com/wandxy/hand/pkg/agent/session"
	"github.com/wandxy/hand/pkg/agent/tool"
)

const defaultMaxIterations = 8

type Responder interface {
	Respond(context.Context, string, RespondOptions) (string, error)
}

type Options struct {
	Model          string
	APIMode        string
	ModelClient    model.Client
	SessionStore   session.Store
	ToolRegistry   tool.Registry
	ToolPolicy     tool.Policy
	PromptProvider prompt.Provider
	RunContext     prompt.RunContext
	MaxIterations  int
	DebugRequests  bool
}

type RespondOptions struct {
	Instruct    string
	SessionID   string
	Stream      *bool
	TraceEvents bool
	OnEvent     func(Event)
}

type Agent struct {
	opts Options
}

func New(opts Options) (*Agent, error) {
	return NewAgent(opts)
}

func NewAgent(opts Options) (*Agent, error) {
	if opts.ModelClient == nil {
		return nil, errors.New("model client is required")
	}
	if opts.SessionStore == nil {
		return nil, errors.New("session store is required")
	}
	if opts.MaxIterations <= 0 {
		opts.MaxIterations = defaultMaxIterations
	}

	return &Agent{opts: opts}, nil
}

func (a *Agent) Respond(ctx context.Context, input string, opts RespondOptions) (string, error) {
	if a == nil {
		return "", errors.New("agent is required")
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("message is required")
	}

	resolved, err := a.opts.SessionStore.Resolve(ctx, opts.SessionID)
	if err != nil {
		return "", err
	}

	history, err := a.opts.SessionStore.GetMessages(ctx, resolved.ID, session.MessageQuery{})
	if err != nil {
		return "", err
	}

	userMessage, err := message.New(message.RoleUser, input)
	if err != nil {
		return "", err
	}
	if err := a.opts.SessionStore.AppendMessages(ctx, resolved.ID, []message.Message{userMessage}); err != nil {
		return "", err
	}

	emitted := []message.Message{userMessage}
	remainingIterations := a.opts.MaxIterations
	runStep := func(ctx context.Context) (LoopDecision, error) {
		request, err := a.buildRequest(ctx, resolved.ID, history, emitted, opts)
		if err != nil {
			return LoopDecision{}, err
		}

		resp, err := a.complete(ctx, request, opts)
		if err != nil {
			return LoopDecision{}, err
		}
		if resp == nil {
			return LoopDecision{}, errors.New("model response is required")
		}

		if resp.PromptTokens > 0 {
			if err := a.opts.SessionStore.UpdateLastPromptTokens(ctx, resolved.ID, resp.PromptTokens); err != nil {
				return LoopDecision{}, err
			}
		}

		if !resp.RequiresToolCalls {
			assistantMessage, err := message.New(message.RoleAssistant, resp.OutputText)
			if err != nil {
				return LoopDecision{}, err
			}
			if err := a.opts.SessionStore.AppendMessages(ctx, resolved.ID, []message.Message{assistantMessage}); err != nil {
				return LoopDecision{}, err
			}

			return LoopDecision{Done: true, Reply: assistantMessage.Content}, nil
		}

		if len(resp.ToolCalls) == 0 {
			return LoopDecision{}, errors.New("model requested tool execution without tool calls")
		}
		if a.opts.ToolRegistry == nil {
			return LoopDecision{}, errors.New("tool registry is required")
		}

		assistantMessage, err := message.Normalize(message.Message{
			Role:      message.RoleAssistant,
			ToolCalls: model.ToolCallsToMessageToolCalls(resp.ToolCalls),
		})
		if err != nil {
			return LoopDecision{}, err
		}
		if err := a.opts.SessionStore.AppendMessages(ctx, resolved.ID, []message.Message{assistantMessage}); err != nil {
			return LoopDecision{}, err
		}
		emitted = append(emitted, assistantMessage)

		for _, modelToolCall := range resp.ToolCalls {
			toolMessage, err := message.Normalize(a.opts.ToolRegistry.Invoke(ctx, tool.CallFromModel(modelToolCall)))
			if err != nil {
				return LoopDecision{}, err
			}
			if err := a.opts.SessionStore.AppendMessages(ctx, resolved.ID, []message.Message{toolMessage}); err != nil {
				return LoopDecision{}, err
			}
			emitted = append(emitted, toolMessage)
		}

		return LoopDecision{}, nil
	}

	return RunModelToolLoop(ctx, ModelToolLoopOptions{
		Consume: func() bool {
			if remainingIterations <= 0 {
				return false
			}
			remainingIterations--
			return true
		},
		RunStep: runStep,
		OnExhausted: func(context.Context) (string, error) {
			return "", errors.New("iteration budget exhausted after " + strconv.Itoa(a.opts.MaxIterations) + " steps")
		},
	})
}

func (a *Agent) buildRequest(
	ctx context.Context,
	sessionID string,
	history []message.Message,
	emitted []message.Message,
	opts RespondOptions,
) (model.Request, error) {
	definitions, err := a.resolveToolDefinitions()
	if err != nil {
		return model.Request{}, err
	}

	instructions, err := a.buildInstructions(ctx, sessionID, definitions, opts)
	if err != nil {
		return model.Request{}, err
	}

	return model.Request{
		Model:         a.opts.Model,
		APIMode:       a.opts.APIMode,
		Instructions:  instructions,
		Messages:      append(message.CloneMessages(history), emitted...),
		Tools:         definitions,
		DebugRequests: a.opts.DebugRequests,
	}, nil
}

func (a *Agent) complete(ctx context.Context, request model.Request, opts RespondOptions) (*model.Response, error) {
	if opts.Stream != nil && *opts.Stream {
		return a.opts.ModelClient.CompleteStream(ctx, request, func(delta model.StreamDelta) {
			if opts.OnEvent == nil || delta.Text == "" {
				return
			}
			opts.OnEvent(Event{
				Kind:    EventKindTextDelta,
				Channel: string(delta.Channel),
				Text:    delta.Text,
			})
		})
	}

	return a.opts.ModelClient.Complete(ctx, request)
}

func (a *Agent) resolveToolDefinitions() ([]model.ToolDefinition, error) {
	if a.opts.ToolRegistry == nil {
		return nil, nil
	}

	definitions, err := a.opts.ToolRegistry.Resolve(a.opts.ToolPolicy)
	if err != nil {
		return nil, err
	}

	return tool.DefinitionsToModel(definitions), nil
}

func (a *Agent) buildInstructions(
	ctx context.Context,
	sessionID string,
	definitions []model.ToolDefinition,
	opts RespondOptions,
) (string, error) {
	blocks := make([]string, 0, 3)
	if a.opts.PromptProvider != nil {
		runContext := a.opts.RunContext
		if strings.TrimSpace(runContext.SessionID) == "" {
			runContext.SessionID = sessionID
		}

		baseInstructions, err := a.opts.PromptProvider.LoadBaseInstructions(ctx, runContext)
		if err != nil {
			return "", err
		}
		blocks = appendInstructionValues(blocks, baseInstructions)

		environmentInstruction, err := a.opts.PromptProvider.BuildEnvironmentInstruction(ctx, prompt.EnvironmentInput{
			SessionID:   sessionID,
			ActiveTools: modelToolNames(definitions),
			Model:       a.opts.Model,
			APIMode:     a.opts.APIMode,
		})
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(environmentInstruction.Value) != "" {
			blocks = append(blocks, strings.TrimSpace(environmentInstruction.Value))
		}
	}

	if instruct := strings.TrimSpace(opts.Instruct); instruct != "" {
		blocks = append(blocks, instruct)
	}

	return strings.Join(blocks, "\n\n"), nil
}

func appendInstructionValues(blocks []string, instructions prompt.Instructions) []string {
	for _, instruction := range instructions {
		if value := strings.TrimSpace(instruction.Value); value != "" {
			blocks = append(blocks, value)
		}
	}

	return blocks
}

func modelToolNames(definitions []model.ToolDefinition) []string {
	if len(definitions) == 0 {
		return nil
	}

	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		if name := strings.TrimSpace(definition.Name); name != "" {
			names = append(names, name)
		}
	}

	return names
}
