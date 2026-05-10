package constants

const (
	// MemoryProviderDefault identifies the built-in memory provider.
	MemoryProviderDefault = "default-memory"
)

const (
	// MemoryPinnedFileName is the automatic pinned memory file name.
	MemoryPinnedFileName = "memory.md"
	// DefaultMemoryPinnedMaxChars is the fallback total character budget for pinned memory.
	DefaultMemoryPinnedMaxChars = 2200
	// DefaultMemoryPinnedItemChars is the fallback per-item character budget for pinned memory.
	DefaultMemoryPinnedItemChars = 2200
)

const (
	// AgentPinnedMemoryRetrievalLimit is the maximum pinned memory items injected per turn.
	AgentPinnedMemoryRetrievalLimit = 1
	// AgentPinnedMemoryRetrievalItemChars is the per-item character budget for pinned memory injection.
	AgentPinnedMemoryRetrievalItemChars = 2200
	// AgentSearchMemoryRetrievalLimit is the maximum searched memory items injected per turn.
	AgentSearchMemoryRetrievalLimit = 3
	// AgentSearchMemoryRetrievalItemChars is the per-item character budget for searched memory injection.
	AgentSearchMemoryRetrievalItemChars = 700
	// AgentSearchMemoryRetrievalMinScore is the minimum score for searched memories injected per turn.
	AgentSearchMemoryRetrievalMinScore = 0.75
	// AgentMemoryContextInstructionChars is the total character budget for memory context instructions.
	AgentMemoryContextInstructionChars = 4500
)

const (
	// MemorySearchToolDefaultLimit is the fallback memory search tool result limit.
	MemorySearchToolDefaultLimit = 10
	// MemorySearchToolMaxLimit is the hard maximum memory search tool result limit.
	MemorySearchToolMaxLimit = 50
	// MemorySearchToolDefaultMaxChars is the fallback per-result character budget for memory search.
	MemorySearchToolDefaultMaxChars = 800
	// MemorySearchToolMaxChars is the hard maximum per-result character budget for memory search.
	MemorySearchToolMaxChars = 4000
)

const (
	// SessionMessagesToolMaxMessageIDs is the maximum explicit message IDs accepted by the session messages tool.
	SessionMessagesToolMaxMessageIDs = 50
	// SessionMessagesToolMaxAnchorWindowSize is the maximum before/after size for anchored message windows.
	SessionMessagesToolMaxAnchorWindowSize = 20
	// SessionMessagesToolMaxOffsetRangeSize is the maximum offset range size returned by the session messages tool.
	SessionMessagesToolMaxOffsetRangeSize = 50
	// SessionMessagesToolDefaultMaxChars is the fallback character budget for session messages output.
	SessionMessagesToolDefaultMaxChars = 4000
	// SessionMessagesToolMaxChars is the hard maximum character budget for session messages output.
	SessionMessagesToolMaxChars = 16000
)
