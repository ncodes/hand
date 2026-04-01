package tools

import (
	"context"
	"encoding/json"
)

type Registry interface {
	Register(Definition) error
	RegisterGroup(Group) error
	Get(string) (Definition, bool)
	GetGroup(string) (Group, bool)
	List() []Definition
	ListGroups() []Group
	Resolve(Policy) ([]Definition, error)
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
	Name        string
	Description string
	InputSchema map[string]any
	Groups      []string
	Requires    Capabilities
	Platforms   []string
	Handler     Handler
}
