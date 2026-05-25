package e2e

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	models "github.com/wandxy/hand/internal/model"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

// RequestAssert validates a scripted model request in e2e tests.
type RequestAssert func(models.Request) error

// Step describes one step.
type Step struct {
	Response *models.Response
	Stream   []models.StreamDelta
	Err      error
	Check    RequestAssert
}

// Client wraps a gRPC connection to the Hand service.
type Client struct {
	mu       sync.Mutex
	steps    []Step
	requests []models.Request
	call     int
}

// NewClient returns a client configured with the supplied dependencies.
func NewClient(steps ...Step) *Client {
	clonedSteps := make([]Step, len(steps))
	copy(clonedSteps, steps)

	return &Client{steps: clonedSteps}
}

// NewTextClient returns a scripted model client that emits text responses.
func NewTextClient(text string) *Client {
	return NewClient(OutputTextStep(text))
}

// NewToolClient returns a scripted model client that emits tool calls.
func NewToolClient(toolCall models.ToolCall, finalText string) *Client {
	return NewClient(
		ToolStep(toolCall),
		Step{
			Response: &models.Response{OutputText: strings.TrimSpace(finalText)},
			Check:    AssertToolRoundTrip(toolCall),
		},
	)
}

// OutputTextStep returns a scripted text response step.
func OutputTextStep(text string) Step {
	return Step{
		Response: &models.Response{OutputText: strings.TrimSpace(text)},
	}
}

// StreamStep returns a scripted streaming response step.
func StreamStep(text string, deltas ...models.StreamDelta) Step {
	step := OutputTextStep(text)
	step.Stream = append([]models.StreamDelta(nil), deltas...)
	return step
}

// ToolStep returns a scripted tool-call response step.
func ToolStep(toolCall models.ToolCall) Step {
	trimmed := models.ToolCall{
		ID:    strings.TrimSpace(toolCall.ID),
		Name:  strings.TrimSpace(toolCall.Name),
		Input: strings.TrimSpace(toolCall.Input),
	}

	return Step{
		Response: &models.Response{
			ToolCalls:         []models.ToolCall{trimmed},
			RequiresToolCalls: true,
		},
	}
}

// AssertToolRoundTrip returns an assertion for a complete tool-call round trip.
func AssertToolRoundTrip(toolCall models.ToolCall) RequestAssert {
	expectedID := strings.TrimSpace(toolCall.ID)
	expectedName := strings.TrimSpace(toolCall.Name)

	return func(req models.Request) error {
		if len(req.Messages) < 3 {
			return errors.New("expected tool round-trip request messages")
		}

		var assistantMessage *handmsg.Message
		var toolMessage *handmsg.Message
		for i := range req.Messages {
			message := req.Messages[i]
			switch {
			case message.Role == handmsg.RoleAssistant && len(message.ToolCalls) > 0:
				assistantMessage = &message
			case message.Role == handmsg.RoleTool && strings.TrimSpace(message.ToolCallID) == expectedID:
				toolMessage = &message
			}
		}

		if assistantMessage == nil {
			return errors.New("expected assistant tool-call message before follow-up completion")
		}
		if toolMessage == nil {
			return fmt.Errorf("expected tool message for tool call %q", expectedID)
		}
		if len(assistantMessage.ToolCalls) != 1 {
			return errors.New("expected exactly one assistant tool call")
		}

		call := assistantMessage.ToolCalls[0]
		if strings.TrimSpace(call.ID) != expectedID {
			return fmt.Errorf("expected assistant tool call id %q", expectedID)
		}
		if strings.TrimSpace(call.Name) != expectedName {
			return fmt.Errorf("expected assistant tool call name %q", expectedName)
		}
		if strings.TrimSpace(toolMessage.Name) != expectedName {
			return fmt.Errorf("expected tool message name %q", expectedName)
		}

		return nil
	}
}

func (d *Client) Requests() []models.Request {
	if d == nil {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.requests)
}

func (d *Client) Complete(_ context.Context, req models.Request) (*models.Response, error) {
	return d.complete(req, nil)
}

func (d *Client) CompleteStream(
	_ context.Context,
	req models.Request,
	onTextDelta func(models.StreamDelta),
) (*models.Response, error) {
	return d.complete(req, onTextDelta)
}

func (d *Client) complete(
	req models.Request,
	onTextDelta func(models.StreamDelta),
) (*models.Response, error) {
	if d == nil {
		return nil, errors.New("e2e model client is required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.call >= len(d.steps) {
		return nil, errors.New("missing model client step")
	}

	step := d.steps[d.call]
	d.requests = append(d.requests, cloneRequest(req))
	d.call++

	if step.Check != nil {
		if err := step.Check(req); err != nil {
			return nil, err
		}
	}
	if step.Err != nil {
		return nil, step.Err
	}
	if onTextDelta != nil {
		for _, delta := range step.Stream {
			onTextDelta(delta)
		}
	}
	if step.Response == nil {
		return nil, errors.New("model client step response is required")
	}

	cloned := *step.Response
	if len(step.Response.ToolCalls) > 0 {
		cloned.ToolCalls = slices.Clone(step.Response.ToolCalls)
	}

	return &cloned, nil
}

func cloneRequest(req models.Request) models.Request {
	cloned := req
	if len(req.Messages) > 0 {
		cloned.Messages = handmsg.CloneMessages(req.Messages)
	}
	if len(req.Tools) > 0 {
		cloned.Tools = append([]models.ToolDefinition(nil), req.Tools...)
	}
	return cloned
}
