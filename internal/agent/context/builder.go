package context

import (
	"github.com/wandxy/hand/internal/agent/memory"
	"github.com/wandxy/hand/internal/messages"
)

// Input contains the sources used to assemble model-visible context for a turn.
type Input struct {
	SessionHistory  []messages.Message
	EmittedMessages []messages.Message
	Summary         *memory.SummaryState
	Pinned          *memory.PinnedState
}

// Builder assembles the model-visible context for a turn.
type Builder struct{}

// New returns a new Builder.
func New() *Builder {
	return &Builder{}
}

// Build assembles the current turn context.
func (b *Builder) Build(input Input) []messages.Message {
	size := len(input.SessionHistory) + len(input.EmittedMessages)
	if size == 0 {
		return nil
	}

	built := make([]messages.Message, 0, size)
	built = append(built, messages.CloneMessages(input.SessionHistory)...)
	built = append(built, messages.CloneMessages(input.EmittedMessages)...)
	return built
}
