package prompt

import "context"

type Provider interface {
	LoadBaseInstructions(context.Context, RunContext) (Instructions, error)
	BuildEnvironmentInstruction(context.Context, EnvironmentInput) (Instruction, error)
}

type RunContext struct {
	SessionID          string
	PublicSessionID    string
	EffectiveSessionID string
	ProfileName        string
}

type EnvironmentInput struct {
	SessionID       string
	ActiveTools     []string
	ActiveGroups    []string
	WorkingDir      string
	Model           string
	SummaryModel    string
	ModelProvider   string
	SummaryProvider string
	APIMode         string
	Platform        string
	WebProvider     string
	Capabilities    Capabilities
}

type Capabilities struct {
	Filesystem bool
	Network    bool
	Exec       bool
	Browser    bool
	Memory     bool
}

type Instruction struct {
	Name  string
	Value string
}

type Instructions []Instruction
