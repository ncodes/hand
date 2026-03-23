package tools

import (
	"context"
)

// Call is the normalized tool invocation request.
type Call struct {
	Name   string
	Input  string
	Source string
}

// Result is the normalized output returned by a tool invocation.
type Result struct {
	Output string
	Error  string
}

// Handler executes a tool call.
type Handler interface {
	Invoke(context.Context, Call) (Result, error)
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc func(context.Context, Call) (Result, error)

// Invoke executes the tool call.
func (f HandlerFunc) Invoke(ctx context.Context, call Call) (Result, error) {
	return f(ctx, call)
}

// Definition describes a tool and its execution contract.
type Definition struct {
	Name        string         // Stable identifier used to reference the tool
	Description string         // Explains what the tool does for model and operator use
	InputSchema map[string]any // Describes the accepted tool input shape
	Handler     Handler        // Executes the tool call
}

// Registry stores tool definitions and dispatches tool calls.
type Registry interface {
	Register(Definition) error
	Get(string) (Definition, bool)
	List() []Definition
	Invoke(context.Context, Call) (Result, error)
}
