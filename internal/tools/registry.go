package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/wandxy/hand/internal/instructions"
)

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

type Call struct {
	Name   string
	Input  string
	Source string
}

type Result struct {
	Output string
	Error  string
}

type Handler interface {
	Invoke(context.Context, Call) (Result, error)
}

type HandlerFunc func(context.Context, Call) (Result, error)

func (f HandlerFunc) Invoke(ctx context.Context, call Call) (Result, error) {
	return f(ctx, call)
}

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

type Group struct {
	Name     string
	Tools    []string
	Includes []string
}

type Policy struct {
	GroupNames   []string
	Capabilities Capabilities
	Platform     string
}

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

type Definition struct {
	Name             string
	Description      string
	UsageInstruction instructions.Instruction
	InputSchema      map[string]any
	Groups           []string
	Requires         Capabilities
	Platforms        []string
	Handler          Handler
}

type Definitions []Definition

func (d Definitions) Has(name string) bool {
	_, ok := d.Get(name)
	return ok
}

func (d Definitions) Get(name string) (Definition, bool) {
	name = strings.TrimSpace(name)
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
		if strings.TrimSpace(def.Name) == "" {
			continue
		}

		names = append(names, def.Name)
	}

	if len(names) == 0 {
		return nil
	}

	return names
}
