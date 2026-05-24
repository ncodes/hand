package memory

import (
	"context"

	"github.com/wandxy/hand/pkg/agent/message"
	"github.com/wandxy/hand/pkg/agent/prompt"
)

type Provider interface {
	LoadPromptInstruction(context.Context, Query) (prompt.Instruction, error)
	FlushBeforeCompaction(context.Context, FlushInput) error
}

type Query struct {
	SessionID string
	UserText  string
	Messages  []message.Message
}

type FlushInput struct {
	SessionID string
	Trigger   string
	Messages  []message.Message
}
