package context

import (
	"github.com/wandxy/hand/internal/agent/memory"
	"github.com/wandxy/hand/internal/messages"
)

// Input contains the sources used to assemble model-visible context for a turn.
type Input struct {
	SessionHistory  []messages.Message
	EmittedMessages []messages.Message
	Memory          *memory.Memory
}

// Builder assembles the model-visible context for a turn.
type Builder struct{}

// New returns a new Builder.
func New() *Builder {
	return &Builder{}
}

// Build assembles the current turn context.
func (b *Builder) Build(input Input) []messages.Message {
	sessionHistory := input.SessionHistory
	if input.Memory != nil && input.Memory.Summary != nil {
		start := min(max(input.Memory.Summary.SourceEndOffset, 0), len(sessionHistory))
		sessionHistory = sessionHistory[start:]
	}

	size := len(sessionHistory) + len(input.EmittedMessages)
	summaryMessage, ok := input.Memory.RenderSummaryMessage()
	if ok {
		size++
	}

	if size == 0 {
		return nil
	}

	built := make([]messages.Message, 0, size)
	if ok {
		built = append(built, summaryMessage)
	}
	built = append(built, messages.CloneMessages(sessionHistory)...)
	built = append(built, messages.CloneMessages(input.EmittedMessages)...)
	return built
}
