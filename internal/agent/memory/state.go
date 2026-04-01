package memory

import handmsg "github.com/wandxy/hand/internal/messages"

// SummaryState contains summary messages that may later participate in context assembly.
type SummaryState struct {
	Messages []handmsg.Message
}

// PinnedState contains durable messages that may later be injected into context assembly.
type PinnedState struct {
	Messages []handmsg.Message
}
