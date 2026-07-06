package tools

import (
	"context"
	"encoding/json"

	"github.com/wandxy/morph/internal/instructions"
	"github.com/wandxy/morph/pkg/str"
)

/*
Package tools defines the runtime-facing tool contract.

Definitions describe what the model may call, policies decide which definitions
are available in a run, and handlers execute calls against the prepared
environment. The package intentionally keeps these shapes independent from the
model package so host adapters can translate between model-facing and
runtime-facing representations.
*/

// Registry describes tool definitions, groups, policies, and invocation handlers.
type Registry interface {
	Register(Definition) error
	RegisterGroup(Group) error
	Get(string) (Definition, bool)
	GetGroup(string) (Group, bool)
	List() Definitions
	ListGroups() []Group
	Resolve(Policy) (Definitions, error)
	Invoke(context.Context, Call) (Result, error)
}

// Call is a runtime tool invocation request.
type Call struct {
	Name   string
	Input  string
	Source string
}

// Result is the structured output of a runtime tool invocation.
type Result struct {
	Output string
	Error  string
}

// Handler executes a tool call.
type Handler interface {
	Invoke(context.Context, Call) (Result, error)
}

// HandlerFunc adapts a function into a tool handler.
type HandlerFunc func(context.Context, Call) (Result, error)

func (f HandlerFunc) Invoke(ctx context.Context, call Call) (Result, error) {
	return f(ctx, call)
}

// Capabilities lists runtime permissions required or provided by a tool.
type Capabilities struct {
	Filesystem bool
	Network    bool
	Exec       bool
	Browser    bool
	Memory     bool
}

func (c Capabilities) Supports(required Capabilities) bool {
	return (!required.Filesystem || c.Filesystem) &&
		(!required.Network || c.Network) &&
		(!required.Exec || c.Exec) &&
		(!required.Browser || c.Browser) &&
		(!required.Memory || c.Memory)
}

// Group names a reusable set of tools and included groups.
type Group struct {
	Name     string
	Tools    []string
	Includes []string
}

// Policy selects tool groups and capabilities for a run.
type Policy struct {
	GroupNames   []string
	Capabilities Capabilities
	Platform     string
}

// Error is the JSON-encoded error shape returned by tools.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

func (e Error) String() string {
	raw, err := json.Marshal(e)
	if err != nil {
		return `{"code":"tool_error","message":"failed to encode tool error"}`
	}

	return string(raw)
}

// Definition describes one tool exposed to the model.
type Definition struct {
	Name             string
	Description      string
	UsageInstruction instructions.Instruction
	InputSchema      map[string]any
	ParallelSafe     bool
	Groups           []string
	Requires         Capabilities
	Platforms        []string
	Handler          Handler
}

// Definitions is a list of tool definitions with lookup helpers.
type Definitions []Definition

func (d Definitions) Has(name string) bool {
	_, ok := d.Get(name)
	return ok
}

func (d Definitions) Get(name string) (Definition, bool) {
	stringValue1 := str.String(name)
	name = stringValue1.Trim()
	if name == "" {
		return Definition{}, false
	}

	for _, def := range d {
		if def.Name == name {
			return def, true
		}
	}

	return Definition{}, false
}

func (d Definitions) Names() []string {
	if len(d) == 0 {
		return nil
	}

	names := make([]string, 0, len(d))
	for _, def := range d {
		stringValue2 := str.String(def.Name)
		if stringValue2.Trim() == "" {
			continue
		}

		names = append(names, def.Name)
	}

	if len(names) == 0 {
		return nil
	}

	return names
}
