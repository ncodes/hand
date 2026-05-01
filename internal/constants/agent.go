package constants

import "time"

const (
	// DefaultRecallSummaryCacheTTL is the default TTL for cached recall summaries.
	DefaultRecallSummaryCacheTTL = 15 * time.Minute
	// PlanHydrationPageSize is the number of plan entries loaded per hydration page.
	PlanHydrationPageSize = 10
	// RecentSessionTail is the number of recent session messages kept near the active context.
	RecentSessionTail = 8
	// RecallWindowMessages is the maximum message count considered for recall windows.
	RecallWindowMessages = 64
	// RecallWindowTokens is the maximum token budget considered for recall windows.
	RecallWindowTokens = 12000
	// RecallMergeSummaries is the maximum summary count considered during recall merging.
	RecallMergeSummaries = 8
	// RecallMergeTokens is the maximum token budget considered during recall merging.
	RecallMergeTokens = 8000
)

const (
	// DefaultContextLength is the fallback model context length.
	DefaultContextLength = 128000
	// DefaultCompactionTrigger is the default context utilization ratio that triggers compaction.
	DefaultCompactionTrigger = 0.85
	// DefaultCompactionWarn is the default context utilization ratio that emits compaction warnings.
	DefaultCompactionWarn = 0.95
	// RoughTokenCharRatio estimates one token per this many characters.
	RoughTokenCharRatio = 4
)
